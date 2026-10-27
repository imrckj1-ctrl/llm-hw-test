// 五项测试项执行规则(§5.6 / §6.3-§6.8)
package runner

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"llmhwtest/internal/artifact"
	"llmhwtest/internal/judge"
	"llmhwtest/internal/llmclient"
	"llmhwtest/internal/store"
)

// doRequest 通用请求:带任务级超时,失败记样本,不阻塞(F-5.3 / §6.1)

// errEngineDead 引擎死亡哨兵(V2.14):已判定服务崩溃后,后续请求不再发出
var errEngineDead = fmt.Errorf("服务已崩溃(引擎无响应),请求不再发出")

// isServerDead 引擎死亡类错误:连接被拒(进程已死)/EngineCore 内部崩溃/连接被重置
func isServerDead(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	for _, sig := range []string{"connection refused", "EngineCore encountered", "connection reset", "no route to host"} {
		if strings.Contains(s, sig) {
			return true
		}
	}
	return false
}

// maxTokens > 0 时覆盖任务生成参数(能力题/产物题需要更大输出预算,V2.4)
// meta 为模块自定义数据(JSON 字符串,如 NIAH 的针与命中,V2.5 规范扩展点)
func (r *Runner) doRequest(ec *execContext, text string, phase, tier string, concurrency, seq int, collectRaw bool, maxTokens int, meta ...string) (*llmclient.Result, error) {
	p := ec.params
	effMaxTokens := p.Gen.MaxTokens
	if maxTokens > 0 {
		effMaxTokens = maxTokens
	} else if maxTokens < 0 {
		// V2.20:明确不携带 max_tokens(质量题:模型自行输出到截断为止)
		effMaxTokens = -1
	}
	req := &llmclient.ChatRequest{
		Model: ec.task.ModelName, Prompt: text,
		Temperature: p.Gen.Temperature, TopP: p.Gen.TopP, TopK: p.Gen.TopK,
		MaxTokens: effMaxTokens, IgnoreEOS: p.Advanced.IgnoreEOS, APIKey: ec.apiKey,
	}
	// V2.20:多轮对话(缓存测试对话式叠加):ec.pendingMsgs 非空时优先
	if len(ec.pendingMsgs) > 0 {
		req.Messages = ec.pendingMsgs
		ec.pendingMsgs = nil
	}
	// V2.2 思考模式与自定义参数
	// V2.23(口径 A):思考等级仅质量验证(能力测试)携带,性能项一律不带(横向对比口径统一)
	if phase == "quality" {
		switch p.Advanced.ThinkingMode {
		case "off":
			off := false
			req.EnableThinking = &off // Qwen/vLLM 系
		case "minimal", "low", "medium", "high", "xhigh", "max":
			req.ReasoningEffort = p.Advanced.ThinkingMode // OpenAI 系
		}
	}
	if p.Advanced.ExtraParams != "" {
		if ec.extraParams != nil {
			req.Extra = ec.extraParams
		}
	}
	to := time.Duration(p.Advanced.RequestTimeoutSec) * time.Second
	// V2.20:同上,0/负数回退默认 300s(API 直建未传超时时不能立刻死)
	if to <= 0 {
		to = 300 * time.Second
	}
	// V2.14:引擎已死 → 不再发请求,直接返回(样本都不落,避免垃圾数据)
	ec.mu.Lock()
	dead := ec.engineDead
	ec.mu.Unlock()
	if dead {
		return nil, errEngineDead
	}
	sendStart := time.Now()
	// 限流/过载(429/5xx)/偶发超时 退避重试,最多 3 次;只记录最终结果样本(V2.6:超时也重试,网关抖动不再直接判失败)
	var res *llmclient.Result
	var err error
	backoffs := []time.Duration{2 * time.Second, 5 * time.Second}
	for attempt := 0; ; attempt++ {
		ctx, cancel := context.WithTimeout(ec.ctx, to)
		res, err = ec.cali.client.ChatStream(ctx, ec.task.APIBaseURL, req)
		cancel()
		// V2.14:引擎死亡类错误(拒连/EngineCore 崩溃)不重试,一次失败直接落定
		retryable := (llmclient.Retryable(err) || isTimeout(err)) && !isServerDead(err)
		if err == nil || !retryable || attempt >= len(backoffs) || ec.ctx.Err() != nil {
			break
		}
		r.log(ec.task.ID, "warn", fmt.Sprintf("服务限流/过载(%s),%v 后重试(第 %d 次)", err, backoffs[attempt], attempt+1))
		select {
		case <-ec.ctx.Done():
			return res, ec.ctx.Err()
		case <-time.After(backoffs[attempt]):
		}
	}
	sm := &store.Sample{
		TaskID: ec.task.ID, ItemID: ec.itemID, RunNo: ec.runNo,
		Phase: phase, Tier: tier, Concurrency: concurrency, Seq: seq, Status: "ok",
	}
	if len(meta) > 0 {
		sm.Meta = meta[0]
	}
	if res != nil {
		sm.SendTS, sm.FirstChunkTS, sm.EndTS = res.SendTS, res.FirstChunkTS, res.EndTS
		sm.TtftMs, sm.TotalMs = res.TtftMs(), res.TotalMs()
		sm.PromptTokens, sm.CompletionTokens = res.PromptTokens, res.CompletionTokens
		sm.TokensEstimated = res.TokensEstimated
		sm.Speed = res.Speed()
		sm.ReasoningOnly = res.ReasoningOnly()
		sm.CachedTokens = res.CachedTokens
		sm.RequestID = r.st.SaveRequestBody(res.RequestBody) // 请求体去重入库(全量日志)
		// usage 合法性护栏:completion 超 max_tokens 15% 且无任何思维链痕迹 → 数据不可信,不参与中位
		// (思维链模型的 reasoning token 计入 completion 属正常,超上限不是坏数据)
		hasReasoning := res.Reasoning != "" || res.ReasoningTokens > 0
		if effMaxTokens > 0 && !hasReasoning && res.CompletionTokens > effMaxTokens*115/100 {
			sm.UsageSuspect = true
			r.log(ec.task.ID, "warn", fmt.Sprintf("usage 异常:completion_tokens=%d 超过 max_tokens=%d,该样本标记不参与中位数",
				res.CompletionTokens, effMaxTokens))
		}
		// 输出未满:实际输出 < 90% max_tokens 且非 length 结束(§6.6)
		if res.CompletionTokens > 0 && res.CompletionTokens < int(float64(effMaxTokens)*0.9) && res.FinishReason != "length" {
			sm.OutputIncomplete = true
		}
		// 全量日志:所有样本保存返回全文;思维链模型正文为空时存思维链
		sm.RawResponse = res.Content
		if strings.TrimSpace(sm.RawResponse) == "" && res.Reasoning != "" {
			sm.RawResponse = "[思维链]\n" + res.Reasoning
		}
	}
	if err != nil {
		sm.Status = "error"
		if isTimeout(err) {
			sm.Status = "timeout"
		}
		sm.Error = err.Error()
		// V2.14:失败样本也记录时间戳(崩溃时间线可还原)
		sm.SendTS = sendStart.UnixMilli()
		sm.EndTS = time.Now().UnixMilli()
		// V2.14:连续硬失败 3 次 → 判定引擎死亡,中止任务
		if isServerDead(err) {
			ec.mu.Lock()
			ec.itemHadDeadErr = true
			ec.deadStreak++
			streak := ec.deadStreak
			trip := ec.deadStreak >= 3 && !ec.engineDead
			if trip {
				ec.engineDead = true
			}
			ec.mu.Unlock()
			if trip {
				r.log(ec.task.ID, "error", fmt.Sprintf("引擎连续 %d 次硬失败(拒连/崩溃),判定服务已死,中止剩余请求", streak))
				if ec.abort != nil {
					ec.abort()
				}
			}
		}
	} else {
		ec.mu.Lock()
		ec.deadStreak = 0
		ec.mu.Unlock()
	}
	// 校准冻结文本的达标标记(§6.2)
	if ct, cerr := ec.cali.Calibrate(ec.ctx, tier); cerr == nil && ct.mismatch && (phase == "formal" || phase == "cold" || phase == "stability") {
		sm.LengthMismatch = true
	}
	smID, ierr := r.st.InsertSample(sm)
	if ierr != nil {
		r.log(ec.task.ID, "error", "样本入库失败: "+ierr.Error())
	}
	ec.mu.Lock()
	ec.lastSampleID = smID
	ec.mu.Unlock()
	return res, err
}

// repeatsFor 分项重复次数(0 → 用全局 Repeats)
func repeatsFor(ec *execContext, item string) int {
	e := ec.params.Exec
	n := 0
	switch item {
	case "ladder":
		n = e.LadderRepeats
	case "concurrency":
		n = e.ConcurrencyRepeats
	case "cache":
		n = e.CacheRepeats
	}
	if n <= 0 {
		n = e.Repeats
	}
	if n <= 0 {
		n = 3
	}
	return n
}

// warmup 预热 N 次,丢弃(§5.6);仅并发/稳定性使用——阶梯测试 V2.4 起不预热
func (r *Runner) warmup(ec *execContext, text, tier string) error {
	n := ec.params.Exec.Warmup
	for i := 1; i <= n; i++ {
		if ec.ctx.Err() != nil {
			return ec.ctx.Err()
		}
		r.log(ec.task.ID, "info", fmt.Sprintf("[%s] 预热 %d/%d", tier, i, n))
		if _, err := r.doRequest(ec, text, "warmup", tier, 1, i, false, 0); err != nil {
			r.log(ec.task.ID, "warn", fmt.Sprintf("预热请求失败(继续): %v", err))
		}
	}
	return nil
}

// ---------- 阶梯测试(§5.6):单并发,逐档,超限跳档;V2.4 不预热,每次随机前缀规避缓存 ----------

func (r *Runner) runLadder(ec *execContext) error {
	tiers := ec.params.Exec.LadderTiers
	repeats := repeatsFor(ec, "ladder")
	ec.progTotal = len(tiers) * repeats
	r.log(ec.task.ID, "info", "阶梯测试:不预热;每次正式请求带随机前缀,规避缓存干扰(缓存效果由缓存测试单独度量)")
	// V2.20:断点续跑——按各档已有有效样本数接着跑
	doneCnt := map[string]int{}
	if ec.resumedItem {
		doneCnt = r.st.DoneSampleCounts(ec.itemID)
		r.log(ec.task.ID, "info", "阶梯测试断点续跑:按各档已有样本数继续")
	}
	for _, tier := range tiers {
		if ec.ctx.Err() != nil {
			return ec.ctx.Err()
		}
		target, err := TierTokens(tier)
		if err != nil {
			r.log(ec.task.ID, "warn", err.Error())
			continue
		}
		// 上下文超限自动跳档(§5.6)
		if ec.task.ContextLen > 0 && target > ec.task.ContextLen {
			r.log(ec.task.ID, "warn", fmt.Sprintf("[%s] 目标 %d tokens 超出上下文 %d,自动跳档并标注", tier, target, ec.task.ContextLen))
			r.st.InsertSample(&store.Sample{
				TaskID: ec.task.ID, ItemID: ec.itemID, RunNo: ec.runNo, Phase: "formal", Tier: tier,
				Concurrency: 1, Status: "skipped", Error: "超出上下文限制",
			})
			continue
		}
		ct, err := ec.cali.Calibrate(ec.ctx, tier)
		if err != nil {
			r.log(ec.task.ID, "error", fmt.Sprintf("[%s] 校准失败,跳过该档: %v", tier, err))
			continue
		}
		d := doneCnt["formal|"+tier]
		ec.progCnt += d
		for i := d + 1; i <= repeats; i++ {
			if ec.ctx.Err() != nil {
				return ec.ctx.Err()
			}
			text := randomPrefix(16) + "\n" + ct.text
			r.log(ec.task.ID, "info", fmt.Sprintf("[%s] 正式请求 %d/%d(随机前缀防缓存)", tier, i, repeats))
			if _, err := r.doRequest(ec, text, "formal", tier, 1, i, false, 0); err != nil {
				r.log(ec.task.ID, "warn", fmt.Sprintf("[%s] 请求失败: %v", tier, err))
			}
			ec.progCnt++
			r.progress(ec, "阶梯测试")
		}
	}
	return nil
}

// ---------- 并发测试(§6.4):批次齐发,熔断;V2.6 输入档可选,熔断加最小样本门槛 ----------

func (r *Runner) runConcurrency(ec *execContext) error {
	levels := ec.params.Exec.ConcurrencyLevels
	repeats := repeatsFor(ec, "concurrency")
	// V2.15:输入档多选(空=回退单档);熔断按输入档各自独立,引擎死亡仍全局中止
	ctiers := ec.params.Exec.ConcurrencyTiers
	if len(ctiers) == 0 {
		ct := ec.params.Exec.ConcurrencyTier
		if ct == "" {
			ct = "1K"
		}
		ctiers = []string{ct}
	}
	ec.progTotal = len(ctiers) * len(levels) * repeats
	// V2.20:断点续跑——按 (输入档,并发档) 已完成轮数接着跑
	doneCnt := map[string]int{}
	if ec.resumedItem {
		doneCnt = r.st.DoneSampleCounts(ec.itemID)
		r.log(ec.task.ID, "info", "并发测试断点续跑:按各档已完成轮数继续")
	}
	for _, ctier := range ctiers {
		if ec.ctx.Err() != nil {
			return ec.ctx.Err()
		}
		ct, err := ec.cali.Calibrate(ec.ctx, ctier)
		if err != nil {
			r.log(ec.task.ID, "error", fmt.Sprintf("[%s] 并发输入档校准失败,跳过该档: %v", ctier, err))
			continue
		}
		if len(ctiers) > 1 {
			r.log(ec.task.ID, "info", fmt.Sprintf("== 并发测试输入档 %s ==", ctier))
		}
		if err := r.warmup(ec, ct.text, ctier); err != nil {
			r.log(ec.task.ID, "warn", fmt.Sprintf("[%s] 预热失败,跳过该档: %v", ctier, err))
			continue
		}
		breakTripped := false
		for _, level := range levels {
			if ec.ctx.Err() != nil {
				return ec.ctx.Err()
			}
			if breakTripped {
				r.log(ec.task.ID, "warn", fmt.Sprintf("[%s 并发 %d] 因上级失败率过高熔断,跳过该档后续更高并发(§6.4)", ctier, level))
				r.st.InsertSample(&store.Sample{
					TaskID: ec.task.ID, ItemID: ec.itemID, RunNo: ec.runNo, Phase: "formal", Tier: ctier,
					Concurrency: level, Status: "skipped", Error: "失败率过高,熔断跳过",
				})
				continue
			}
			doneR := doneCnt[fmt.Sprintf("formal|%s|%d", ctier, level)] / level
			ec.progCnt += doneR
			for round := doneR + 1; round <= repeats; round++ {
				if ec.ctx.Err() != nil {
					return ec.ctx.Err()
				}
				r.log(ec.task.ID, "info", fmt.Sprintf("[%s 并发 %d] 第 %d/%d 轮,齐发 %d 个请求", ctier, level, round, repeats, level))
				var wg sync.WaitGroup
				results := make([]error, level)
				for k := 0; k < level; k++ {
					wg.Add(1)
					go func(k int) {
						defer wg.Done()
						_, err := r.doRequest(ec, ct.text, "formal", ctier, level, round*1000+k, false, 0)
						results[k] = err
					}(k)
				}
				wg.Wait()
				fail := 0
				for _, e := range results {
					if e != nil {
						fail++
					}
				}
				rate := float64(fail) / float64(level)
				r.log(ec.task.ID, "info", fmt.Sprintf("[%s 并发 %d] 第 %d 轮失败率 %.1f%%", ctier, level, round, rate*100))
				// V2.6 最小样本门槛:失败数 ≥2 且失败率 >20% 才熔断(单请求偶发超时不误触)
				if fail >= 2 && rate > 0.2 {
					breakTripped = true
					r.log(ec.task.ID, "warn", fmt.Sprintf("[%s 并发 %d] 失败率 %.1f%% > 20%%,触发熔断", ctier, level, rate*100))
				}
				ec.progCnt++
				r.progress(ec, "并发测试")
			}
		}
	}
	return nil
}

// ---------- 缓存测试(§6.3):冷轮随机前缀 / 热轮同输入重复 ----------

func randomPrefix(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	rand.Read(b)
	for i := range b {
		b[i] = letters[int(b[i])%len(letters)]
	}
	return string(b)
}

func (r *Runner) runCache(ec *execContext) error {
	// V2.6:缓存测试档位可显式多选,空=跟随阶梯档位
	tiers := ec.params.Exec.CacheTiers
	if len(tiers) == 0 {
		tiers = ec.params.Exec.LadderTiers
	}
	repeats := repeatsFor(ec, "cache")
	if repeats < 2 {
		repeats = 2 // 至少 2 轮才有「叠加」
	}
	// V2.20 重写:对话式叠加——每轮把上轮回答并入上下文,前缀逐轮累积;
	// 第 1 轮为冷(写缓存),后续轮前缀相同应命中;叠加轮次(cache_repeats)与档位(cache_tiers)用户可选
	ec.progTotal = len(tiers) * repeats
	r.log(ec.task.ID, "info", fmt.Sprintf("缓存测试(对话式叠加):每档 %d 轮,每轮把上轮回答并入上下文", repeats))
	for _, tier := range tiers {
		if ec.ctx.Err() != nil {
			return ec.ctx.Err()
		}
		target, err := TierTokens(tier)
		if err != nil {
			continue
		}
		if ec.task.ContextLen > 0 && target > ec.task.ContextLen {
			r.log(ec.task.ID, "warn", fmt.Sprintf("[%s] 缓存测试超出上下文,跳过", tier))
			continue
		}
		ct, err := ec.cali.Calibrate(ec.ctx, tier)
		if err != nil {
			r.log(ec.task.ID, "error", fmt.Sprintf("[%s] 校准失败,跳过缓存测试: %v", tier, err))
			continue
		}
		// 对话历史:逐轮累积 [user 文档][assistant 回答]...
		history := []map[string]string{}
		for i := 1; i <= repeats; i++ {
			if ec.ctx.Err() != nil {
				return ec.ctx.Err()
			}
			// 本轮再发同一文档,前缀 = 之前所有轮次的累积
			history = append(history, map[string]string{"role": "user", "content": ct.text})
			phase := "cold"
			if i > 1 {
				phase = "hot"
			}
			r.log(ec.task.ID, "info", fmt.Sprintf("[%s] 对话第 %d/%d 轮(上下文累计 %d 条消息)", tier, i, repeats, len(history)))
			ec.pendingMsgs = append([]map[string]string(nil), history...)
			res, err := r.doRequest(ec, "", phase, tier, 1, i, false, 0)
			if err != nil {
				r.log(ec.task.ID, "warn", fmt.Sprintf("[%s] 第 %d 轮请求失败: %v", tier, i, err))
				history = history[:len(history)-1] // 失败轮不入历史,保持前缀一致
				continue
			}
			if res != nil {
				history = append(history, map[string]string{"role": "assistant", "content": res.Content})
			}
			ec.progCnt++
			r.progress(ec, "缓存测试")
			// 轮间 0.5s 间隔,规避速率限制
			select {
			case <-ec.ctx.Done():
				return ec.ctx.Err()
			case <-time.After(500 * time.Millisecond):
			}
		}
		// V2.20:档位收尾做非流式缓存探测(SSE 尾帧在部分网关丢 prompt_tokens_details,
		// 非流式一定有),把 cached_tokens 写回该档最后一个 hot 样本
		if len(history) > 0 && ec.ctx.Err() == nil {
			pctx, pcancel := context.WithTimeout(ec.ctx, 120*time.Second)
			cached, prompt, perr := r.client.ProbeCacheHit(pctx, ec.task.APIBaseURL, ec.task.ModelName, history, ec.apiKey)
			pcancel()
			if perr != nil {
				r.log(ec.task.ID, "warn", fmt.Sprintf("[%s] 缓存探测失败(命中率取 SSE 口径): %v", tier, perr))
			} else {
				r.log(ec.task.ID, "info", fmt.Sprintf("[%s] 缓存探测:cached=%d / prompt=%d(命中率 %.1f%%)", tier, cached, prompt, 100*float64(cached)/float64(max(prompt, 1))))
				r.st.Exec(`UPDATE request_samples SET cached_tokens=? WHERE id=(
					SELECT id FROM request_samples WHERE item_id=? AND phase='hot' AND status='ok' ORDER BY id DESC LIMIT 1)`, cached, ec.itemID)
			}
		}
	}
	return nil
}

// ---------- 稳定性测试(§6.5):背靠背,窗口统计 ----------

// errStabilityFused 稳定性熔断哨兵(V3.0.1):连续多波全部失败即熔断,
// 项标「跳过」(可补测),任务直接进下一项,不再空跑满剩余时长
var errStabilityFused = errors.New("稳定性连续多波全部失败,触发熔断")

func (r *Runner) runStability(ec *execContext) error {
	minutes := ec.params.Exec.StabilityMinutes
	tier := ec.params.Exec.StabilityTier
	if tier == "" {
		tier = "10K"
	}
	conc := ec.params.Exec.StabilityConcurrency // V2.3:每波并发数(默认 1=背靠背)
	if conc <= 0 {
		conc = 1
	}
	ct, err := ec.cali.Calibrate(ec.ctx, tier)
	if err != nil {
		return fmt.Errorf("%s 档校准失败: %w", tier, err)
	}
	if err := r.warmup(ec, ct.text, tier); err != nil {
		return err
	}
	ec.progTotal = minutes // 进度按分钟推进(修复进度条在稳定性期间不动)
	// V2.20:断点续跑——按已采集时长折算剩余压测时间
	if ec.resumedItem {
		first, last := r.st.SampleTimeRange(ec.itemID)
		if first > 0 && last > first {
			elapsedMin := int((last - first) / 60000)
			if elapsedMin >= minutes {
				r.log(ec.task.ID, "info", "稳定性断点续跑:已采集时长已达目标,直接收尾")
				return nil
			}
			r.log(ec.task.ID, "info", fmt.Sprintf("稳定性断点续跑:已压测约 %d 分钟,续跑剩余 %d 分钟", elapsedMin, minutes-elapsedMin))
			minutes -= elapsedMin
			ec.progCnt += elapsedMin
		}
	}
	deadline := time.Now().Add(time.Duration(minutes) * time.Minute)
	r.log(ec.task.ID, "info", fmt.Sprintf("稳定性压测开始:%s 输入,每波并发 %d,时长 %d 分钟,波浪式背靠背", tier, conc, minutes))
	wave := 0
	failStreakWaves := 0 // V3.0.1:连续全失败波数(熔断计数)
	failStreakReqs := 0  // V3.0.1:连续失败请求累计(熔断计数)
	for time.Now().Before(deadline) {
		if ec.ctx.Err() != nil {
			return ec.ctx.Err()
		}
		wave++
		var wg sync.WaitGroup
		errs := make([]error, conc)
		for k := 0; k < conc; k++ {
			wg.Add(1)
			go func(k int) {
				defer wg.Done()
				_, err := r.doRequest(ec, ct.text, "stability", tier, conc, wave*1000+k, false, 0)
				errs[k] = err
			}(k)
		}
		wg.Wait()
		fails := 0
		for _, e := range errs {
			if e != nil {
				fails++
			}
		}
		if fails == conc && fails > 0 {
			failStreakWaves++
			failStreakReqs += fails
		} else {
			failStreakWaves = 0
			failStreakReqs = 0
		}
		// V3.0.1 熔断:连续 3 波全部失败且累计失败 ≥2 个请求 → 跳过本项剩余时长
		// (并发测试按档熔断的同口径;避免上游已挂仍空跑满 120 分钟)
		if failStreakWaves >= 3 && failStreakReqs >= 2 {
			r.log(ec.task.ID, "error", fmt.Sprintf("稳定性连续 %d 波全部失败(累计 %d 个请求),触发熔断,跳过后续压测,本项标记「跳过」可补测", failStreakWaves, failStreakReqs))
			return errStabilityFused
		}
		if fails > 0 {
			r.log(ec.task.ID, "warn", fmt.Sprintf("稳定性第 %d 波 %d/%d 失败(继续)", wave, fails, conc))
			select {
			case <-ec.ctx.Done():
				return ec.ctx.Err()
			case <-time.After(2 * time.Second):
			}
		}
		ec.progCnt = minutes - int(time.Until(deadline).Minutes())
		r.progress(ec, "稳定性测试")
	}
	r.log(ec.task.ID, "info", fmt.Sprintf("稳定性压测结束,共完成 %d 波 × %d 并发", wave, conc))
	return nil
}

// ---------- 质量抽查(§6.8):逐题发送,自动初筛 ----------

func (r *Runner) runQuality(ec *execContext) error {
	questions, err := r.st.ListQuestions(true)
	if err != nil || len(questions) == 0 {
		return fmt.Errorf("题库为空或全部停用")
	}
	// V2.4 选题方式:all/cats/group/custom(空=按字段推断,兼容旧任务)
	mode := ec.params.Items.QualityMode
	if mode == "" {
		switch {
		case len(ec.params.Items.QualityQuestionIDs) > 0:
			mode = "custom"
		case ec.params.Items.QualityGroupID > 0:
			mode = "group"
		case len(ec.params.Items.CapabilityCats) > 0:
			mode = "cats"
		default:
			mode = "all"
		}
	}
	pickByIDs := func(ids []int64) {
		allow := map[int64]bool{}
		for _, id := range ids {
			allow[id] = true
		}
		var filtered []*store.Question
		for _, q := range questions {
			if allow[q.ID] {
				filtered = append(filtered, q)
			}
		}
		questions = filtered
	}
	switch mode {
	case "custom":
		pickByIDs(ec.params.Items.QualityQuestionIDs)
		r.log(ec.task.ID, "info", fmt.Sprintf("能力测试:自定义选题 %d 题", len(questions)))
	case "group":
		pickByIDs(r.st.GroupQuestionIDs(ec.params.Items.QualityGroupID))
		r.log(ec.task.ID, "info", fmt.Sprintf("能力测试:按题组 #%d 选题 %d 题", ec.params.Items.QualityGroupID, len(questions)))
	case "cats":
		allow := map[string]bool{}
		for _, c := range ec.params.Items.CapabilityCats {
			allow[c] = true
		}
		var filtered []*store.Question
		for _, q := range questions {
			if allow[q.Category] {
				filtered = append(filtered, q)
			}
		}
		questions = filtered
		r.log(ec.task.ID, "info", fmt.Sprintf("能力测试:按维度筛选 %d 题", len(questions)))
	}
	if len(questions) == 0 {
		return fmt.Errorf("所选范围无题目(题组/自定义选题中的题目可能已删除或停用)")
	}
	jd := judge.FromSettings(r.st)
	if !jd.Enabled() {
		r.log(ec.task.ID, "warn", "裁判模型未配置,LLM 评判项将标记「未评判」(系统设置可配)")
	}
	ec.progTotal = len(questions)
	// V2.20:断点续跑——已作答的题跳过(按 quality_answers 已有记录)
	if ec.resumedItem {
		doneQ := map[int64]bool{}
		if rows, err := r.st.DB().Query(`SELECT question_id FROM quality_answers WHERE task_id=? AND run_no=?`, ec.task.ID, ec.runNo); err == nil {
			for rows.Next() {
				var qid int64
				rows.Scan(&qid)
				doneQ[qid] = true
			}
			rows.Close()
		}
		if len(doneQ) > 0 {
			var rest []*store.Question
			for _, q := range questions {
				if doneQ[q.ID] {
					ec.progCnt++
				} else {
					rest = append(rest, q)
				}
			}
			r.log(ec.task.ID, "info", fmt.Sprintf("能力测试断点续跑:已完成 %d 题,续跑剩余 %d 题", len(doneQ), len(rest)))
			questions = rest
		}
	}
	for i, q := range questions {
		if ec.ctx.Err() != nil {
			return ec.ctx.Err()
		}
		r.runOneQuestion(ec, jd, q, i, len(questions))
		ec.progCnt++
		r.progress(ec, "能力测试")
	}
	return nil
}

// runOneQuestion 单题执行:发问 → 初筛 → 评判(规则/LLM/人工) → 产物落地
func (r *Runner) runOneQuestion(ec *execContext, jd *judge.Judge, q *store.Question, idx, total int) {
	tid := ec.task.ID
	prompt := q.Prompt
	expected := q.ExpectedAnswer
	// 长文找回:从激活语料动态埋针(运行时生成,答案每次都不同)
	if q.Category == "needle" {
		needle := "LWZ" + randomPrefix(5)
		corpus, err := ec.cali.corpusForTier("10K")
		if err != nil || corpus == "" {
			r.log(tid, "warn", "长文找回题缺少语料,跳过: "+q.Title)
			return
		}
		text := embedNeedle(corpus, needle)
		prompt = q.Prompt + "\n\n" + text
		expected = needle
		r.log(tid, "info", fmt.Sprintf("长文找回:已埋针 %s(语料 10K)", needle))
	} else if q.LongText != "" {
		prompt = q.Prompt + "\n\n" + q.LongText
	}
	r.log(tid, "info", fmt.Sprintf("能力测试 %d/%d: [%s]%s", idx+1, total, q.Category, q.Title))

	// V2.8:质量题默认走 pi 智能体(工具调用真实产出文件);失败回退直接问答
	usePi := piEnabled(r.st)
	var piFiles []string
	// V2.5:文档类产物题自动追加输出契约(JSON 规格,仅直接问答模式;pi 模式模型直接产出真文件)
	hint := ""
	if !usePi {
		hint = artifact.SchemaHint(q.ArtifactType)
	}
	if hint != "" {
		prompt += "\n\n" + hint
	}

	qa := &store.QualityAnswer{TaskID: tid, RunNo: ec.runNo, QuestionID: q.ID, Title: q.Title, ArtifactType: q.ArtifactType} // 类型快照,不随题库后续修改变化

	// 一致性维度:同题连发 3 次比对漂移
	repeats := 1
	if q.Category == "consistency" {
		repeats = 3
	}
	// V2.20:质量题请求明确不携带 max_tokens 字段(qMax=-1),模型自行输出到截断为止。
	// (修复 V2.17:qMax=0 在 doRequest 里被当「不覆盖」回退到任务参数——任务111 全题被 500 截断)
	qMax := -1
	var answers []string
	contentEmpty := false // 正文为空(仅思维链)
	for k := 0; k < repeats; k++ {
		if usePi {
			pres, perr := r.runPiQuestion(ec, q, prompt, qMax)
			if perr != nil {
				if ec.ctx.Err() != nil {
					return // 任务取消
				}
				var infra *piInfraError
				if errors.As(perr, &infra) {
					// 基础设施问题(执行器缺失/服务不支持工具):回退直接问答继续本题
					r.log(tid, "warn", "pi 执行器不可用,回退直接问答: "+perr.Error())
					usePi = false
					if h := artifact.SchemaHint(q.ArtifactType); h != "" && hint == "" {
						hint = h
						prompt += "\n\n" + h
					}
					k--
					continue
				}
				r.log(tid, "warn", fmt.Sprintf("题目「%s」pi 执行失败: %v", q.Title, perr))
				if pres != nil && pres.Answer != "" {
					qa.Answer = pres.Answer // goal_blocked 等场景保留已有产出
				}
				qa.AutoFlag, qa.AutoReason = "suspect", "pi 智能体执行失败: "+perr.Error()
				r.st.InsertQualityAnswer(qa)
				return
			}
			piFiles = pres.Files
			answers = append(answers, pres.Answer)
			meta, _ := json.Marshal(map[string]any{"engine": "pi", "turns": pres.Turns, "tool_calls": pres.ToolCalls,
				"goal_status": pres.GoalStatus, "iterations": pres.Iterations})
			r.st.InsertSample(&store.Sample{
				TaskID: tid, ItemID: ec.itemID, RunNo: ec.runNo, Phase: "quality",
				Concurrency: 1, Seq: idx*10 + k + 1, Status: "ok",
				TtftMs: pres.TtftMs, TotalMs: float64(pres.DurationMs),
				PromptTokens: pres.InputTokens, CompletionTokens: pres.OutputTokens,
				RawResponse: pres.Answer, Meta: string(meta),
			})
			r.log(tid, "info", fmt.Sprintf("题目「%s」pi 执行完成: %d 轮 %d 次工具调用,产出文件 %d 个",
				q.Title, pres.Turns, pres.ToolCalls, len(pres.Files)))
			continue
		}
		res, err := r.doRequest(ec, prompt, "quality", "", 1, idx*10+k+1, true, qMax)
		if err != nil {
			r.log(tid, "warn", fmt.Sprintf("题目「%s」请求失败: %v", q.Title, err))
			qa.AutoFlag, qa.AutoReason = "suspect", "请求失败: "+err.Error()
			r.st.InsertQualityAnswer(qa)
			return
		}
		ans := ""
		if res != nil {
			ans = res.Content
			if strings.TrimSpace(ans) == "" && res.Reasoning != "" {
				ans = "[思维链]\n" + res.Reasoning
				contentEmpty = true
			}
		}
		answers = append(answers, ans)
	}
	qa.Answer = strings.Join(answers, "\n\n———— 第 N 次回答分隔 ————\n\n")
	if repeats == 1 {
		qa.Answer = answers[0]
	}
	// 自动初筛(§6.8)
	qa.AutoFlag, qa.AutoReason = autoScreen(answers[0])
	if contentEmpty {
		qa.AutoFlag = "suspect"
		qa.AutoReason = "正文为空:max_tokens 被思维链耗尽或模型未输出正文(仅保留思维链记录)"
	}

	// 评判
	switch q.JudgeMode {
	case "rule":
		if contentEmpty {
			qa.JudgeReason = "未评判(正文为空)"
		} else {
			qa.JudgeScore, qa.JudgeReason = ruleCheck(q, answers, expected)
		}
	case "llm":
		if contentEmpty {
			qa.JudgeReason = "未评判(正文为空)"
		} else if jd.Enabled() {
			// 裁判题干必须带输出契约(产物由平台按 JSON 规格渲染),
			// 否则裁判会以「没生成文件」冤枉模型(任务48事故)
			judgeQ := q.Prompt
			if hint != "" {
				judgeQ += "\n\n" + hint
			}
			if score, reason, err := jd.Evaluate(ec.ctx, judgeQ, expected, answers[0]); err == nil {
				qa.JudgeScore, qa.JudgeReason = score, "裁判模型: "+reason
			} else {
				qa.JudgeReason = "评判失败: " + err.Error()
				r.log(tid, "warn", fmt.Sprintf("题目「%s」评判失败: %v", q.Title, err))
			}
		} else {
			qa.JudgeReason = "未评判(裁判模型未配置)"
		}
	default: // manual 仅人工
		qa.JudgeReason = "待人工标注"
	}

	// 产物落地(html/code/xlsx/pptx/docx/pdf);正文为空(仅思维链)时不提取——思维链内容不可信
	if q.ArtifactType != "" && q.ArtifactType != "none" {
		if len(piFiles) > 0 || (usePi && len(answers) > 0) {
			rel, ok := normalizePiArtifact(r.dataDir, tid, q.ID, piFiles, q.ArtifactType)
			qa.ArtifactPath = rel
			qa.ArtifactOK = ok
			note := "pi 智能体产出文件: " + strings.Join(piFiles, ", ")
			if len(piFiles) == 0 {
				note = "pi 智能体未产出文件"
			}
			qa.JudgeReason = joinReason(qa.JudgeReason, note)
			r.log(tid, "info", "产物登记: "+note)
		} else if contentEmpty {
			qa.ArtifactOK = false
			qa.JudgeReason = joinReason(qa.JudgeReason, "无产物(正文为空)")
			r.log(tid, "warn", fmt.Sprintf("题目「%s」正文为空,产物未生成", q.Title))
		} else {
			rel, ok, note := artifact.Save(r.dataDir, tid, q.ID, answers[0], q.ArtifactType)
			qa.ArtifactPath = rel
			qa.ArtifactOK = ok
			qa.JudgeReason = joinReason(qa.JudgeReason, note)
			r.log(tid, "info", fmt.Sprintf("产物落地:%s → %s", note, rel))
		}
	}

	r.st.InsertQualityAnswer(qa)
	if qa.JudgeScore != "" {
		r.log(tid, "info", fmt.Sprintf("题目「%s」评判: %s(%s)", q.Title, qa.JudgeScore, qa.JudgeReason))
	} else if qa.AutoFlag != "normal" {
		r.log(tid, "warn", fmt.Sprintf("题目「%s」自动初筛疑似异常: %s", q.Title, qa.AutoReason))
	}
}

// embedNeedle 在语料随机位置插入针句(避开首尾各 10%,真随机;修复除零与固定位置缺陷)
func embedNeedle(corpus, needle string) string {
	rs := []rune(corpus)
	sentence := "本文隐藏的密钥是:" + needle + "。"
	if len(rs) < 100 {
		return corpus + "\n" + sentence
	}
	lo, hi := len(rs)/10, len(rs)*9/10
	if hi <= lo {
		lo, hi = 0, len(rs)
	}
	pos := lo
	if span := hi - lo; span > 0 {
		var b [8]byte
		if _, err := rand.Read(b[:]); err == nil {
			pos = lo + int(binary.BigEndian.Uint64(b[:])%uint64(span))
		}
	}
	return string(rs[:pos]) + sentence + string(rs[pos:])
}

// joinReason 评判原因拼接(避免前导分号)
func joinReason(parts ...string) string {
	var out []string
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			out = append(out, strings.TrimSpace(p))
		}
	}
	return strings.Join(out, ";")
}

// ruleCheck 规则评判(指令遵循 JSON 校验 / 找回匹配 / 一致性相似度)
func ruleCheck(q *store.Question, answers []string, expected string) (string, string) {
	switch q.Category {
	case "needle":
		if strings.Contains(answers[0], expected) {
			return "pass", "成功找回密钥 " + expected
		}
		return "fail", "未找回密钥 " + expected
	case "instruction":
		// 回答须为合法 JSON 且包含 expected 中列出的必需字段(逗号分隔)
		ans := stripFence(answers[0])
		var obj map[string]any
		if json.Unmarshal([]byte(ans), &obj) != nil {
			return "fail", "输出不是合法 JSON"
		}
		var missing []string
		for _, k := range strings.Split(expected, ",") {
			k = strings.TrimSpace(k)
			if k == "" {
				continue
			}
			if _, ok := obj[k]; !ok {
				missing = append(missing, k)
			}
		}
		if len(missing) > 0 {
			return "fail", "JSON 缺少字段: " + strings.Join(missing, ",")
		}
		return "pass", "JSON 合法且字段齐全"
	case "consistency":
		sim := avgSimilarity(answers)
		if sim >= 0.7 {
			return "pass", fmt.Sprintf("一致性 %.0f%%(≥70%%)", sim*100)
		}
		return "fail", fmt.Sprintf("一致性 %.0f%%(<70%%)", sim*100)
	}
	return "", ""
}

func stripFence(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if i := strings.Index(s, "\n"); i >= 0 {
			s = s[i+1:]
		}
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	}
	return strings.TrimSpace(s)
}

// avgSimilarity 字符二元组 Jaccard 相似度(两两平均)
func avgSimilarity(answers []string) float64 {
	if len(answers) < 2 {
		return 1
	}
	bigrams := func(s string) map[string]bool {
		rs := []rune(strings.ReplaceAll(s, " ", ""))
		m := map[string]bool{}
		for i := 0; i+1 < len(rs); i++ {
			m[string(rs[i:i+2])] = true
		}
		return m
	}
	sets := make([]map[string]bool, len(answers))
	for i, a := range answers {
		sets[i] = bigrams(a)
	}
	var sum float64
	var n int
	for i := 0; i < len(sets); i++ {
		for j := i + 1; j < len(sets); j++ {
			inter := 0
			for k := range sets[i] {
				if sets[j][k] {
					inter++
				}
			}
			union := len(sets[i]) + len(sets[j]) - inter
			if union > 0 {
				sum += float64(inter) / float64(union)
			}
			n++
		}
	}
	if n == 0 {
		return 1
	}
	return sum / float64(n)
}

// autoScreen 自动初筛(§6.8):空回答/重复刷屏/乱码
func autoScreen(answer string) (flag string, reason string) {
	t := strings.TrimSpace(answer)
	if t == "" {
		return "suspect", "空回答(去除空白后长度 0)"
	}
	runes := []rune(t)
	// 乱码:替换符/控制字符占比 > 5%
	bad := 0
	for _, ch := range runes {
		if ch == 0xFFFD || (ch < 32 && ch != '\n' && ch != '\t' && ch != '\r') {
			bad++
		}
	}
	if float64(bad)/float64(len(runes)) > 0.05 {
		return "suspect", fmt.Sprintf("乱码占比 %.1f%% > 5%%", float64(bad)/float64(len(runes))*100)
	}
	// 重复刷屏:单一字符或 ≤8 字符短串重复占比 > 50%
	if rep := repetitionRatio(runes); rep > 0.5 {
		return "suspect", fmt.Sprintf("重复刷屏(重复占比 %.0f%% > 50%%)", rep*100)
	}
	return "normal", ""
}

// repetitionRatio 检测最短重复单元(≤8)覆盖比例 + 单字符最高占比
func repetitionRatio(runes []rune) float64 {
	n := len(runes)
	if n == 0 {
		return 0
	}
	for unit := 1; unit <= 8 && unit <= n/2; unit++ {
		match := 0
		for i := 0; i < n; i++ {
			if runes[i] == runes[i%unit] {
				match++
			}
		}
		if float64(match)/float64(n) > 0.9 {
			return float64(match) / float64(n)
		}
	}
	freq := map[rune]int{}
	for _, ch := range runes {
		freq[ch]++
	}
	maxF := 0
	for _, f := range freq {
		if f > maxF {
			maxF = f
		}
	}
	return float64(maxF) / float64(n)
}

var _ = context.Background
