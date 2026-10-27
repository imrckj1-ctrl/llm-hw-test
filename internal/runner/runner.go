// Package runner 任务执行引擎(预热/阶梯/并发/缓存/稳定性/质量,§5.6)
package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"llmhwtest/internal/llmclient"
	"llmhwtest/internal/exporter"
	"llmhwtest/internal/store"
)

// Event SSE 事件(进度/日志/状态)
type Event struct {
	Type    string `json:"type"` // progress/log/status/item
	Message string `json:"message,omitempty"`
	Level   string `json:"level,omitempty"`
	Progress int   `json:"progress,omitempty"`
	Status  string `json:"status,omitempty"`
	Item    string `json:"item,omitempty"`
	TS      int64  `json:"ts"`
}

// EventBus 按任务分发事件
type EventBus struct {
	mu   sync.Mutex
	subs map[int64][]chan Event
}

func NewEventBus() *EventBus { return &EventBus{subs: map[int64][]chan Event{}} }

func (b *EventBus) Subscribe(taskID int64) chan Event {
	ch := make(chan Event, 256)
	b.mu.Lock()
	b.subs[taskID] = append(b.subs[taskID], ch)
	b.mu.Unlock()
	return ch
}

func (b *EventBus) Unsubscribe(taskID int64, ch chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	list := b.subs[taskID]
	for i, c := range list {
		if c == ch {
			b.subs[taskID] = append(list[:i], list[i+1:]...)
			break
		}
	}
}

func (b *EventBus) Publish(taskID int64, ev Event) {
	ev.TS = store.NowMs()
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ch := range b.subs[taskID] {
		select {
		case ch <- ev:
		default: // 慢消费者丢弃,不阻塞执行
		}
	}
}

type Runner struct {
	st      *store.Store
	client  *llmclient.Client
	Bus     *EventBus
	dataDir string
	// V2.19 执行控制:任务暂停 / 跳过当前测试项
	ctlMu     sync.Mutex
	pausing   map[int64]bool               // 暂停请求(任务级,取消路径落 paused 而非 cancelled)
	skipItem  map[int64]bool               // 跳过当前测试项请求
	itemAbort map[int64]context.CancelFunc // 当前执行项的项级 cancel(跳过用)
}

// parseExtra 自定义请求参数(任务高级参数 JSON)
func parseExtra(p store.TaskParams) map[string]any {
	if p.Advanced.ExtraParams == "" {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(p.Advanced.ExtraParams), &m); err != nil || len(m) == 0 {
		return nil
	}
	return m
}

func New(st *store.Store, dataDir string) *Runner {
	return &Runner{st: st, client: llmclient.New(), Bus: NewEventBus(), dataDir: dataDir,
		pausing: map[int64]bool{}, skipItem: map[int64]bool{}, itemAbort: map[int64]context.CancelFunc{}}
}

// RequestPause 请求暂停任务:中止当前请求,任务落「已暂停」,点「继续跑」从断点续跑(V2.19)
func (r *Runner) RequestPause(taskID int64) {
	r.ctlMu.Lock()
	r.pausing[taskID] = true
	r.ctlMu.Unlock()
}

// popPausing 取出并清除暂停标记
func (r *Runner) popPausing(taskID int64) bool {
	r.ctlMu.Lock()
	defer r.ctlMu.Unlock()
	v := r.pausing[taskID]
	delete(r.pausing, taskID)
	return v
}

// RequestSkipItem 请求跳过当前测试项:中止当前项,该项标「已跳过」,任务继续下一项(V2.19)
// 返回是否有正在执行的项被取消
func (r *Runner) RequestSkipItem(taskID int64) bool {
	r.ctlMu.Lock()
	abort, ok := r.itemAbort[taskID]
	if ok {
		r.skipItem[taskID] = true
	}
	r.ctlMu.Unlock()
	if ok {
		abort()
	}
	return ok
}

// popSkip 取出并清除跳过标记
func (r *Runner) popSkip(taskID int64) bool {
	r.ctlMu.Lock()
	defer r.ctlMu.Unlock()
	v := r.skipItem[taskID]
	delete(r.skipItem, taskID)
	return v
}

func (r *Runner) setItemAbort(taskID int64, abort context.CancelFunc) {
	r.ctlMu.Lock()
	r.itemAbort[taskID] = abort
	r.ctlMu.Unlock()
}

func (r *Runner) clearItemAbort(taskID int64) {
	r.ctlMu.Lock()
	delete(r.itemAbort, taskID)
	r.ctlMu.Unlock()
}

// SkipNextItem 暂停态跳过(V2.19):把下一个待执行测试项标「已跳过」,
// 继续跑(Resume)时该项不再执行,之后可在任务结束后单项补测。
// 返回被跳过的测试项类型;没有可跳过的项时报错。
func (r *Runner) SkipNextItem(taskID int64) (string, error) {
	task, err := r.st.GetTask(taskID)
	if err != nil {
		return "", err
	}
	params := store.ParseTaskParams(task.ParamsJSON)
	items, _ := r.st.ListTestItems(taskID, 0)
	doneTypes := map[string]bool{}
	// 已有但未完成的行(中断/取消/待执行)优先直接改标,不重复建行
	pendingRow := map[string]int64{}
	for _, it := range items {
		if it.Status == "done" || it.Status == "skipped" {
			doneTypes[it.Type] = true
		} else if _, ok := pendingRow[it.Type]; !ok {
			pendingRow[it.Type] = it.ID
		}
	}
	for _, m := range r.modules() {
		if !m.Enabled(params) || doneTypes[m.Name] {
			continue
		}
		if id, ok := pendingRow[m.Name]; ok {
			r.st.ItemFinished(id, "skipped", "手动跳过,可补测")
		} else {
			id, err := r.st.CreateTestItem(taskID, m.Name, task.RunNo)
			if err != nil {
				return "", err
			}
			r.st.ItemFinished(id, "skipped", "手动跳过,可补测")
		}
		r.Bus.Publish(taskID, Event{Type: "item", Item: m.Name, Status: "skipped"})
		r.log(taskID, "warn", "测试项「"+r.itemDisplayName(m.Name)+"」已手动跳过(暂停态),继续跑时不再执行")
		return m.Name, nil
	}
	return "", fmt.Errorf("没有可跳过的待执行测试项")
}

// execContext 单次运行的上下文
type execContext struct {
	ctx       context.Context
	task      *store.Task
	params    store.TaskParams
	apiKey    string
	extraParams map[string]any
	itemID    int64
	runNo     int
	cali      *calibrator
	progBase  int
	progSpan  int
	progCnt   int
	progTotal int
	// lastSampleID doRequest 最近一次落库样本 ID(模块补写 meta 用,V2.5)
	lastSampleID int64
	// V2.14 引擎死亡检测:拒连/EngineCore 连续硬失败 3 次 → 判定服务崩溃,中止任务
	engineDead bool
	deadStreak int
	abort      context.CancelFunc
	// itemHadDeadErr 当前测试项内出现过引擎死亡类错误(该项数据已污染,不可标 done)
	itemHadDeadErr bool
	// resumedItem 当前项为中断续跑(V2.20:模块按已有样本数接着跑)
	resumedItem bool
	// pendingMsgs 多轮对话请求(V2.20 缓存对话式叠加,串行项使用,用完即清)
	pendingMsgs []map[string]string
	// mu 保护并发写:并发/稳定性用 goroutine 齐发,doRequest 内对下列字段的写必须串行
	mu sync.Mutex
}

func (r *Runner) log(taskID int64, level, msg string) {
	r.st.AddTaskLog(taskID, level, msg)
	r.Bus.Publish(taskID, Event{Type: "log", Level: level, Message: msg})
}

func (r *Runner) progress(ec *execContext, itemName string) {
	p := ec.progBase
	if ec.progTotal > 0 {
		p = ec.progBase + ec.progSpan*ec.progCnt/ec.progTotal
	}
	if p > 99 {
		p = 99
	}
	r.st.SetTaskProgress(ec.task.ID, p, itemName)
	r.Bus.Publish(ec.task.ID, Event{Type: "progress", Progress: p, Item: itemName})
}

// Run 执行一个任务(由 scheduler 调用)。ctx 取消 → 中断当前请求,已完成项数据保留(F-5.1)
func (r *Runner) Run(ctx context.Context, taskID int64) {
	r.runWithSkip(ctx, taskID, nil)
}

// Resume 继续跑:只跑未完成/未执行的测试项,已完成项保留(修改清单-1)
func (r *Runner) Resume(ctx context.Context, taskID int64) {
	task, err := r.st.GetTask(taskID)
	if err != nil {
		return
	}
	items, _ := r.st.ListTestItems(taskID, 0)
	skip := map[string]bool{}
	for _, it := range items {
		if it.Status == "done" {
			skip[it.Type] = true
			continue
		}
		if it.Status == "skipped" {
			// V2.19:手动跳过的项在「继续跑」时不重跑(之后可单独补测)
			skip[it.Type] = true
			continue
		}
		if it.Status == "interrupted" && it.Type != "cache" {
			// V2.20:暂停中断的项保留样本与项行,断点续跑(模块内按已有样本数续)
			// 缓存测试为对话式叠加(轮次链不可断),不支持断点续跑,走下方清样本重跑
			continue
		}
		// V2.14:重跑项(失败/取消/中断)先清掉旧样本与旧项行,
		// 避免与重跑数据叠加(与单项重跑同口径);quality 项同时清答案
		r.st.Exec(`DELETE FROM request_samples WHERE item_id=?`, it.ID)
		if it.Type == "quality" {
			r.st.Exec(`DELETE FROM quality_answers WHERE task_id=? AND run_no=?`, taskID, task.RunNo)
		}
		r.st.Exec(`DELETE FROM test_items WHERE id=?`, it.ID)
	}
	r.runWithSkip(ctx, taskID, skip)
}

func (r *Runner) runWithSkip(ctx context.Context, taskID int64, skip map[string]bool) {
	task, err := r.st.GetTask(taskID)
	if err != nil {
		return
	}
	params := store.ParseTaskParams(task.ParamsJSON)
	apiKey, _ := r.st.DecryptSecret(task.APIKeyEnc)
	if ctx.Err() != nil {
		// 认领后立即被取消(调度窗口竞态):直接落终态,不做前置检查
		if r.popPausing(taskID) {
			r.st.SetTaskStatus(taskID, "paused", "手动暂停")
			r.Bus.Publish(taskID, Event{Type: "status", Status: "paused"})
		} else {
			r.st.TaskFinished(taskID, "cancelled", "用户取消")
			r.Bus.Publish(taskID, Event{Type: "status", Status: "cancelled"})
		}
		return
	}
	r.st.TaskStarted(taskID)
	r.st.TouchProjectStarted(task.ProjectID)
	r.Bus.Publish(taskID, Event{Type: "status", Status: "running"})
	r.log(taskID, "info", fmt.Sprintf("任务开始执行(run #%d):模型 %s @ %s", task.RunNo, task.ModelName, task.APIBaseURL))

	// V2.14:派生可中止 context——引擎死亡时由 runner 主动中止,与用户取消(父 ctx)区分
	rctx, rcancel := context.WithCancel(ctx)
	defer rcancel()

	finish := func(status, errMsg string) {
		// V2.12:任务终态(完成/失败)即冻结语料+题目快照,数据包内容自此固定
		if status == "done" || status == "failed" {
			r.st.FreezeTaskSnapshots(taskID, task.RunNo)
			// V2.23:终态即物化数据包目录(后台),数据包对话框秒开
			t := task
			go func() {
				if dir, err := exporter.MaterializeTaskPackage(r.st, r.dataDir, t); err == nil {
					r.log(taskID, "info", "数据包已物化: "+dir)
				}
			}()
		}
		r.st.TaskFinished(taskID, status, errMsg)
		r.st.TouchProjectFinished(task.ProjectID)
		r.Bus.Publish(taskID, Event{Type: "status", Status: status, Message: errMsg})
	}

	// 前置检查:API 可达性(失败 → 任务失败,§7.4)
	pingCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	if err := r.client.Ping(pingCtx, task.APIBaseURL, task.ModelName, apiKey); err != nil {
		cancel()
		r.log(taskID, "error", "前置检查失败,API 不可达: "+err.Error())
		finish("failed", "API 不可达: "+err.Error())
		return
	}
	cancel()
	r.log(taskID, "info", "前置检查通过,模型服务可达")

	ec := &execContext{
		ctx: rctx, task: task, params: params, apiKey: apiKey, runNo: task.RunNo,
		extraParams: parseExtra(params),
		cali: newCalibrator(r.client, r.st, task, params, apiKey, func(level, msg string) { r.log(taskID, level, msg) }),
		abort: rcancel,
	}

	// 测试项来自模块注册表(V2.5 规范)
	var enabled []TestModule
	for _, it := range r.modules() {
		if it.Enabled(params) {
			enabled = append(enabled, it)
		}
	}
	if len(enabled) == 0 {
		r.log(taskID, "warn", "未勾选任何测试项")
		finish("done", "")
		return
	}

	for i, it := range enabled {
		if ctx.Err() != nil {
			if r.popPausing(taskID) {
				r.log(taskID, "warn", "任务已暂停(手动),点「继续跑」从断点续跑")
				r.st.SetTaskStatus(taskID, "paused", "手动暂停")
				r.Bus.Publish(taskID, Event{Type: "status", Status: "paused"})
			} else {
				r.log(taskID, "warn", "任务已取消")
				finish("cancelled", "用户取消")
			}
			return
		}
		if skip[it.Name] {
			r.log(taskID, "info", "测试项「"+r.itemDisplayName(it.Name)+"」已完成,跳过(继续跑模式)")
			continue
		}
		// V2.20:中断项复用(断点续跑,样本不断链);NIAH 点位矩阵/缓存对话链不支持续跑,删旧行重建
		itemID := int64(0)
		resumed := false
		if it.Name != "niah" && it.Name != "cache" {
			if old := r.st.FindInterruptedItem(taskID, task.RunNo, it.Name); old > 0 {
				itemID = old
				resumed = true
			}
		}
		if itemID == 0 {
			var err error
			itemID, err = r.st.CreateTestItem(taskID, it.Name, task.RunNo)
			if err != nil {
				continue
			}
		}
		// V2.19:项级 context——「跳过当前项」只取消本项,不影响任务级 rctx
		itemCtx, itemCancel := context.WithCancel(rctx)
		ec.ctx = itemCtx
		r.setItemAbort(taskID, itemCancel)
		ec.itemID = itemID
		ec.resumedItem = resumed
		ec.progBase = i * 100 / len(enabled)
		ec.progSpan = 100 / len(enabled)
		ec.progCnt = 0
		ec.itemHadDeadErr = false
		r.st.ItemStarted(itemID)
		r.Bus.Publish(taskID, Event{Type: "item", Item: it.Name, Status: "running"})
		r.log(taskID, "info", "== 测试项开始: "+r.itemDisplayName(it.Name)+" ==")
		r.progress(ec, r.itemDisplayName(it.Name))

		runErr := it.Run(ec)
		itemCancel()
		r.clearItemAbort(taskID)
		if ec.engineDead {
			// V2.14:引擎死亡 → 当前项失败,剩余项全部标失败(不刷垃圾数据),任务快速收尾
			r.st.ItemFinished(itemID, "failed", "服务崩溃中断")
			r.Bus.Publish(taskID, Event{Type: "item", Item: it.Name, Status: "failed", Message: "服务崩溃中断"})
			r.log(taskID, "error", "引擎连续无响应,判定服务崩溃;任务中断,剩余测试项不再执行(修复服务后点「继续跑」可整项重跑)")
			for _, rest := range enabled[i+1:] {
				if restID, err := r.st.CreateTestItem(taskID, rest.Name, task.RunNo); err == nil {
					r.st.ItemFinished(restID, "failed", "服务崩溃,未执行")
					r.Bus.Publish(taskID, Event{Type: "item", Item: rest.Name, Status: "failed", Message: "服务崩溃,未执行"})
				}
			}
			finish("failed", "服务崩溃:引擎无响应,任务中断")
			return
		}
		switch {
		case r.popSkip(taskID) && ctx.Err() == nil:
			// V2.19:手动跳过当前项——已采集数据保留,项标「已跳过」,任务继续下一项
			r.st.ItemFinished(itemID, "skipped", "手动跳过,可在任务结束后补测")
			r.Bus.Publish(taskID, Event{Type: "item", Item: it.Name, Status: "skipped"})
			r.log(taskID, "warn", "测试项「"+r.itemDisplayName(it.Name)+"」已手动跳过,继续下一项(之后可补测)")
			continue
		case ctx.Err() != nil:
			if r.popPausing(taskID) {
				// V2.19:手动暂停——当前项标中断(继续跑时清样本重跑本项),任务落「已暂停」
				r.st.ItemFinished(itemID, "interrupted", "手动暂停")
				r.Bus.Publish(taskID, Event{Type: "item", Item: it.Name, Status: "interrupted"})
				r.st.SetTaskStatus(taskID, "paused", "手动暂停")
				r.Bus.Publish(taskID, Event{Type: "status", Status: "paused"})
				r.log(taskID, "warn", "任务已暂停(手动),测试项「"+r.itemDisplayName(it.Name)+"」中断,点「继续跑」从本项续跑")
			} else {
				r.st.ItemFinished(itemID, "cancelled", "用户取消")
				r.Bus.Publish(taskID, Event{Type: "item", Item: it.Name, Status: "cancelled"})
				r.log(taskID, "warn", "测试项「"+r.itemDisplayName(it.Name)+"」已取消,已完成项数据保留")
				finish("cancelled", "用户取消")
			}
			return
		case errors.Is(runErr, errStabilityFused):
			// V3.0.1:稳定性连续多波全部失败熔断——项标「已跳过」(可补测),任务继续下一项
			r.st.ItemFinished(itemID, "skipped", "连续多波全部失败,熔断跳过,可补测")
			r.Bus.Publish(taskID, Event{Type: "item", Item: it.Name, Status: "skipped"})
			r.log(taskID, "warn", "测试项「"+r.itemDisplayName(it.Name)+"」连续多波全部失败,熔断跳过,继续下一项(之后可补测)")
			continue
		case runErr != nil:
			// 单测试项失败不中断后续项(F-5.3)
			r.st.ItemFinished(itemID, "failed", runErr.Error())
			r.Bus.Publish(taskID, Event{Type: "item", Item: it.Name, Status: "failed", Message: runErr.Error()})
			r.log(taskID, "error", "测试项「"+r.itemDisplayName(it.Name)+"」失败: "+runErr.Error()+" ;继续后续项")
		default:
			if ec.itemHadDeadErr {
				// V2.14:项内出现过引擎死亡类错误 → 数据已污染,标 failed(继续跑时会整项重跑)
				r.st.ItemFinished(itemID, "failed", "服务崩溃,部分数据无效")
				r.Bus.Publish(taskID, Event{Type: "item", Item: it.Name, Status: "failed", Message: "服务崩溃,部分数据无效"})
				r.log(taskID, "warn", "测试项「"+r.itemDisplayName(it.Name)+"」完成但含引擎死亡错误,标记失败待重跑")
				break
			}
			r.st.ItemFinished(itemID, "done", "")
			r.Bus.Publish(taskID, Event{Type: "item", Item: it.Name, Status: "done"})
			r.log(taskID, "info", "测试项「"+r.itemDisplayName(it.Name)+"」完成")
		}
	}
	r.st.SetTaskProgress(taskID, 100, "")
	r.Bus.Publish(taskID, Event{Type: "progress", Progress: 100})
	finish("done", "")
	r.log(taskID, "info", "任务执行完成")
}

// RunSingleItem 单测试项重跑(覆盖该项结果,F-5.3)
func (r *Runner) RunSingleItem(ctx context.Context, taskID int64, oldItemID int64) {
	task, err := r.st.GetTask(taskID)
	if err != nil {
		return
	}
	old, err := r.st.GetTestItem(oldItemID)
	if err != nil {
		return
	}
	prevStatus := task.Status // 单项重跑前任务终态,结束后恢复(修复:不再无条件标 done)
	params := store.ParseTaskParams(task.ParamsJSON)
	apiKey, _ := r.st.DecryptSecret(task.APIKeyEnc)
	r.st.SetTaskStatus(taskID, "running", "")
	r.Bus.Publish(taskID, Event{Type: "status", Status: "running"})
	r.log(taskID, "info", "单项重跑: "+r.itemDisplayName(old.Type))

	// 覆盖该项旧数据
	r.st.Exec(`DELETE FROM request_samples WHERE item_id=?`, oldItemID)
	if old.Type == "quality" {
		r.st.Exec(`DELETE FROM quality_answers WHERE task_id=? AND run_no=?`, taskID, task.RunNo)
	}
	r.st.Exec(`DELETE FROM test_items WHERE id=?`, oldItemID)

	ec := &execContext{
		ctx: ctx, task: task, params: params, apiKey: apiKey, runNo: task.RunNo,
		extraParams: parseExtra(params),
		cali: newCalibrator(r.client, r.st, task, params, apiKey, func(level, msg string) { r.log(taskID, level, msg) }),
		progBase: 0, progSpan: 100,
	}
	mod := r.moduleByName(old.Type)
	if mod == nil {
		r.log(taskID, "error", "未知测试项类型: "+old.Type)
		return
	}
	itemID, _ := r.st.CreateTestItem(taskID, old.Type, task.RunNo)
	ec.itemID = itemID
	r.st.ItemStarted(itemID)
	r.Bus.Publish(taskID, Event{Type: "item", Item: old.Type, Status: "running"})

	runErr := mod.Run(ec)
	switch {
	case ctx.Err() != nil:
		r.st.ItemFinished(itemID, "cancelled", "用户取消")
		r.Bus.Publish(taskID, Event{Type: "item", Item: old.Type, Status: "cancelled"})
	case runErr != nil:
		r.st.ItemFinished(itemID, "failed", runErr.Error())
		r.Bus.Publish(taskID, Event{Type: "item", Item: old.Type, Status: "failed", Message: runErr.Error()})
		r.log(taskID, "error", "单项重跑失败: "+runErr.Error())
	default:
		r.st.ItemFinished(itemID, "done", "")
		r.Bus.Publish(taskID, Event{Type: "item", Item: old.Type, Status: "done"})
		r.log(taskID, "info", "单项重跑完成")
	}
	// 恢复任务到重跑前的终态(失败也恢复,单项结果以 test_items 为准)
	st := prevStatus
	if st == "running" || st == "queued" || st == "" {
		st = "done"
	}
	// V2.12:重跑覆盖结果后重新冻结快照
	r.st.FreezeTaskSnapshots(taskID, task.RunNo)
	r.st.TaskFinished(taskID, st, "")
	r.Bus.Publish(taskID, Event{Type: "status", Status: st})
}
