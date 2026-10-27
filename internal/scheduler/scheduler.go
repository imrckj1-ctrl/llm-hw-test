// Package scheduler 设备级 FIFO 队列 + 全局并行上限 + 取消/中断恢复(§5.5)
package scheduler

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"llmhwtest/internal/runner"
	"llmhwtest/internal/store"
)

type Scheduler struct {
	st      *store.Store
	runner  *runner.Runner
	mu      sync.Mutex
	running map[int64]context.CancelFunc // taskID → cancel
	wake    chan struct{}
	stop    chan struct{}
	stopped bool
}

func New(st *store.Store, dataDir string) *Scheduler {
	s := &Scheduler{
		st:      st,
		running: map[int64]context.CancelFunc{},
		wake:    make(chan struct{}, 1),
		stop:    make(chan struct{}),
	}
	s.runner = runner.New(st, dataDir)
	return s
}

func (s *Scheduler) Events() *runner.EventBus { return s.runner.Bus }

func (s *Scheduler) parallelLimit() int {
	v, ok := s.st.GetSetting("parallel_limit")
	if !ok {
		return 4
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 4
	}
	return n
}

// EnqueueTask 单任务入队(状态 → 排队中)
func (s *Scheduler) EnqueueTask(taskID int64) error {
	t, err := s.st.GetTask(taskID)
	if err != nil {
		return err
	}
	if t.Status == "done" {
		return fmt.Errorf("已完成任务不可重跑,请使用「复制为新任务」") // 修改清单-4:完成任务永久只读
	}
	if t.Status == "queued" || t.Status == "running" {
		return fmt.Errorf("任务已在队列或执行中")
	}
	// 重跑语义:终态任务再启动 → run_no+1,历史 run 保留(F-5.4)
	if t.Status != "pending" {
		s.st.Exec(`UPDATE tasks SET run_no=run_no+1, progress=0, current_item='', error='' WHERE id=?`, taskID)
	} else {
		s.st.Exec(`UPDATE tasks SET progress=0, current_item='', error='' WHERE id=?`, taskID)
	}
	s.st.SetTaskStatus(taskID, "queued", "")
	s.st.AddTaskLog(taskID, "info", "任务入队")
	s.notify()
	return nil
}

// EnqueueProject 项目一键启动:未执行任务按序入队(F-4.4)
func (s *Scheduler) EnqueueProject(projectID int64) (int, error) {
	tasks, err := s.st.ListTasks(projectID, 0, "")
	if err != nil {
		return 0, err
	}
	n := 0
	for _, t := range tasks {
		if t.Status == "pending" || t.Status == "interrupted" || t.Status == "cancelled" || t.Status == "failed" {
			if err := s.EnqueueTask(t.ID); err == nil {
				n++
			}
		}
	}
	if n == 0 {
		return 0, fmt.Errorf("项目内没有可启动的任务")
	}
	return n, nil
}

// CancelTask 取消:排队中→已取消;执行中→立即中止当前请求(F-5.1)
// 注意:先查运行表再按 DB 状态分支——dispatch 认领后到 TaskStarted 落库之间
// 存在窗口,此时 DB 仍是 queued 但任务已在运行表,直接改库会被 TaskStarted 覆盖回 running
func (s *Scheduler) CancelTask(taskID int64) error {
	t, err := s.st.GetTask(taskID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	cancel, running := s.running[taskID]
	s.mu.Unlock()
	if running {
		s.st.AddTaskLog(taskID, "warn", "收到取消请求,正在中止当前请求...")
		cancel()
		return nil
	}
	switch t.Status {
	case "queued":
		s.st.SetTaskStatus(taskID, "cancelled", "排队中取消")
		s.st.AddTaskLog(taskID, "warn", "排队中被取消")
		return nil
	case "running":
		return fmt.Errorf("任务不在执行列表")
	default:
		return fmt.Errorf("任务状态 %s 不可取消", t.Status)
	}
}

// PauseTask 暂停任务:执行中→中止当前请求落「已暂停」,释放并行名额,点「继续跑」断点续跑;
// 排队中→直接落「已暂停」(出队)(V2.19)
func (s *Scheduler) PauseTask(taskID int64) error {
	t, err := s.st.GetTask(taskID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	cancel, running := s.running[taskID]
	s.mu.Unlock()
	if running {
		s.runner.RequestPause(taskID)
		s.st.AddTaskLog(taskID, "warn", "收到暂停请求,正在中止当前请求...")
		cancel()
		return nil
	}
	switch t.Status {
	case "queued":
		s.st.SetTaskStatus(taskID, "paused", "排队中暂停")
		s.st.AddTaskLog(taskID, "warn", "排队中被暂停(出队)")
		return nil
	case "running":
		return fmt.Errorf("任务不在执行列表")
	default:
		return fmt.Errorf("任务状态 %s 不可暂停", t.Status)
	}
}

// SkipItem 跳过测试项(V2.19):
//   执行中 → 中止当前项,该项标「已跳过」,任务继续下一项
//   已暂停 → 把下一个待执行项标「已跳过」,继续跑时不再执行
func (s *Scheduler) SkipItem(taskID int64) error {
	t, err := s.st.GetTask(taskID)
	if err != nil {
		return err
	}
	if t.Status == "paused" {
		name, err := s.runner.SkipNextItem(taskID)
		if err != nil {
			return err
		}
		s.st.AddTaskLog(taskID, "warn", fmt.Sprintf("手动跳过下一测试项: %s(继续跑时不再执行)", name))
		return nil
	}
	s.mu.Lock()
	_, running := s.running[taskID]
	s.mu.Unlock()
	if !running || t.Status != "running" {
		return fmt.Errorf("任务不在执行中,无法跳过测试项")
	}
	if !s.runner.RequestSkipItem(taskID) {
		return fmt.Errorf("当前没有可跳过的测试项")
	}
	s.st.AddTaskLog(taskID, "warn", "收到跳过请求,正在跳过当前测试项...")
	return nil
}

// launch 统一启动执行 goroutine:带 panic 兜底(任何模块 panic 不再打挂整个进程)
func (s *Scheduler) launch(taskID int64, ctx context.Context, fn func(context.Context, int64)) {
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				s.st.AddTaskLog(taskID, "error", fmt.Sprintf("执行引擎内部错误(已拦截,进程保持存活): %v", rec))
				s.st.TaskFinished(taskID, "failed", fmt.Sprintf("内部错误: %v", rec))
			}
			s.mu.Lock()
			delete(s.running, taskID)
			s.mu.Unlock()
			s.notify()
		}()
		fn(ctx, taskID)
	}()
}

// serialKey 串行组 key:有设备按设备(F-5.2),纯模型按项目(V2.2)
func serialKey(t *store.Task) string {
	if t.DeviceID > 0 {
		return fmt.Sprintf("d%d", t.DeviceID)
	}
	return fmt.Sprintf("p%d", t.ProjectID)
}

// checkLaunch 手动启动类操作(继续跑/单项重跑)的准入检查(V2.22):
// 与 dispatch 同口径——并行名额 + 同串行组不冲突;不满足返回可读错误
func (s *Scheduler) checkLaunch(t *store.Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.running) >= s.parallelLimit() {
		return fmt.Errorf("当前并行名额已满(上限 %d 条),等运行中的任务结束后再试", s.parallelLimit())
	}
	key := serialKey(t)
	for tid := range s.running {
		rt, err := s.st.GetTask(tid)
		if err == nil && serialKey(rt) == key {
			return fmt.Errorf("同一设备/项目已有任务在执行,等它结束后再试")
		}
	}
	return nil
}

// RerunItem 单测试项重跑(F-5.3)
func (s *Scheduler) RerunItem(taskID, itemID int64) error {
	t, err := s.st.GetTask(taskID)
	if err != nil {
		return err
	}
	// V2.19:done 任务允许补测(被跳过的项);RunSingleItem 结束后恢复原终态
	if t.Status == "running" || t.Status == "queued" {
		return fmt.Errorf("任务执行中,不可单项重跑")
	}
	s.mu.Lock()
	_, busy := s.running[taskID]
	s.mu.Unlock()
	if busy {
		return fmt.Errorf("任务执行中")
	}
	if err := s.checkLaunch(t); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.running[taskID] = cancel
	s.mu.Unlock()
	s.launch(taskID, ctx, func(c context.Context, tid int64) { s.runner.RunSingleItem(c, tid, itemID) })
	return nil
}

// SkipItemByID 按项跳过(V2.19 测试项级控制):
//   当前执行项(任务执行中) → 中止该项,任务继续下一项
//   任务已暂停/待执行等 → 直接把该项标「已跳过」(继续跑时不执行,可补测)
func (s *Scheduler) SkipItemByID(taskID, itemID int64) error {
	t, err := s.st.GetTask(taskID)
	if err != nil {
		return err
	}
	it, err := s.st.GetTestItem(itemID)
	if err != nil || it.TaskID != taskID {
		return fmt.Errorf("测试项不存在")
	}
	if it.Status == "done" {
		return fmt.Errorf("该测试项已完成,不可跳过(如需重测请用「单项重跑」)")
	}
	if it.Status == "skipped" {
		return fmt.Errorf("该测试项已是跳过状态")
	}
	if t.Status == "running" {
		if it.Status == "running" {
			// 当前执行项 → 中止本项,任务继续下一项
			if !s.runner.RequestSkipItem(taskID) {
				return fmt.Errorf("当前没有可中止的执行项")
			}
			s.st.AddTaskLog(taskID, "warn", "收到跳过请求,正在中止当前测试项...")
			return nil
		}
		return fmt.Errorf("任务执行中,跳过未开始的项请先暂停任务")
	}
	// 非执行中:直接标记跳过(继续跑/补测语义由 Resume/Rerun 承接)
	s.st.ItemFinished(itemID, "skipped", "手动跳过,可补测")
	s.Events().Publish(taskID, runner.Event{Type: "item", Item: it.Type, Status: "skipped"})
	s.st.AddTaskLog(taskID, "warn", fmt.Sprintf("测试项 %s 已手动跳过(可补测)", it.Type))
	return nil
}

func (s *Scheduler) notify() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// ResumeTask 继续跑:只执行未完成的测试项(修改清单-1)
func (s *Scheduler) ResumeTask(taskID int64) error {
	t, err := s.st.GetTask(taskID)
	if err != nil {
		return err
	}
	if t.Status == "done" {
		return fmt.Errorf("任务已完成,无需继续")
	}
	if t.Status == "running" || t.Status == "queued" {
		return fmt.Errorf("任务执行中/排队中")
	}
	s.mu.Lock()
	_, busy := s.running[taskID]
	s.mu.Unlock()
	if busy {
		return fmt.Errorf("任务执行中")
	}
	if err := s.checkLaunch(t); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.running[taskID] = cancel
	s.mu.Unlock()
	s.st.AddTaskLog(taskID, "info", "继续跑:从未完成项继续执行")
	s.launch(taskID, ctx, func(c context.Context, tid int64) { s.runner.Resume(c, tid) })
	return nil
}

// Start 调度循环:同设备串行(FIFO),不同设备并行,全局并行上限(F-5.2)
func (s *Scheduler) Start() {
	go func() {
		for {
			select {
			case <-s.stop:
				return
			case <-s.wake:
			}
			s.dispatch()
		}
	}()
	s.notify()
}

func (s *Scheduler) dispatch() {
	for {
		s.mu.Lock()
		runningCount := len(s.running)
		s.mu.Unlock()
		if runningCount >= s.parallelLimit() {
			return
		}
		// 取最老的排队任务,检查其串行组是否已有任务在跑
		// 串行组:有设备按设备(同设备串行 F-5.2);无设备(纯模型)按项目串行(V2.2)
		rows, err := s.st.DB().Query(`SELECT id, COALESCE(device_id,0), project_id FROM tasks WHERE status='queued' ORDER BY id`)
		if err != nil {
			return
		}
		type cand struct {
			id  int64
			key string
		}
		var cands []cand
		for rows.Next() {
			var c cand
			var dev, proj int64
			rows.Scan(&c.id, &dev, &proj)
			if dev > 0 {
				c.key = fmt.Sprintf("d%d", dev)
			} else {
				c.key = fmt.Sprintf("p%d", proj)
			}
			cands = append(cands, c)
		}
		rows.Close()
		if len(cands) == 0 {
			return
		}
		busy := map[string]bool{}
		s.mu.Lock()
		for tid := range s.running {
			if t, err := s.st.GetTask(tid); err == nil {
				if t.DeviceID > 0 {
					busy[fmt.Sprintf("d%d", t.DeviceID)] = true
				} else {
					busy[fmt.Sprintf("p%d", t.ProjectID)] = true
				}
			}
		}
		s.mu.Unlock()
		picked := int64(0)
		for _, c := range cands {
			if !busy[c.key] {
				picked = c.id
				break
			}
		}
		if picked == 0 {
			return
		}
		ctx, cancel := context.WithCancel(context.Background())
		s.mu.Lock()
		s.running[picked] = cancel
		s.mu.Unlock()
		// 原子认领:把 queued→running 落库,避免取消竞态(取消方先查运行表)
		if !s.st.ClaimTask(picked) {
			s.mu.Lock()
			delete(s.running, picked)
			s.mu.Unlock()
			cancel()
			continue // 已被并发取消/改动,挑下一个
		}
		s.launch(picked, ctx, func(c context.Context, tid int64) { s.runner.Run(c, tid) })
	}
}

// Shutdown 停止调度(运行中任务随进程退出,重启后标记中断)
func (s *Scheduler) Shutdown() {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	s.mu.Unlock()
	close(s.stop)
}
