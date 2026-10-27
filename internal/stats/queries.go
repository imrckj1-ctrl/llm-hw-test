// 结果聚合查询:全部即时聚合,不落冗余表(§7.3)。默认最新 run(F-5.4)。
package stats

import (
	"encoding/json"
	"math"
	"strconv"
	"sort"
	"strings"

	"llmhwtest/internal/store"
)

// DistStats 分布统计(V2.10):平均/P50/P95/P99/最短/最长
type DistStats struct {
	Avg float64 `json:"avg"`
	P50 float64 `json:"p50"`
	P95 float64 `json:"p95"`
	P99 float64 `json:"p99"`
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

// Dist 对一组样本求分布统计(空返回零值);P 值最近秩法
func Dist(vs []float64) DistStats {
	if len(vs) == 0 {
		return DistStats{}
	}
	s := append([]float64(nil), vs...)
	sort.Float64s(s)
	var sum float64
	for _, v := range s {
		sum += v
	}
	rank := func(p float64) float64 {
		i := int(math.Ceil(p*float64(len(s)))) - 1
		if i < 0 {
			i = 0
		}
		return s[i]
	}
	return DistStats{Avg: sum / float64(len(s)), P50: Median(s), P95: rank(0.95), P99: rank(0.99), Min: s[0], Max: s[len(s)-1]}
}

type LadderRow struct {
	Tier         string  `json:"tier"`
	PromptTokens int     `json:"prompt_tokens"`
	TtftMedian   float64 `json:"ttft_median"`
	SpeedMedian  float64 `json:"speed_median"`
	TotalMedian  float64 `json:"total_median"`
	Ttft         DistStats `json:"ttft"` // V2.10:TTFT 分布统计
	Speed        DistStats `json:"speed"` // V2.10:速度分布统计
	Samples      int     `json:"samples"`
	Planned      int     `json:"planned"`  // V2.14:计划次数(参数快照)
	OkCount      int     `json:"ok_count"` // V2.14:有效(成功)次数
	Skipped      bool    `json:"skipped"`
	Note         string  `json:"note"`
}

type ConcurrencyRow struct {
	Level       int     `json:"level"`
	Tier        string  `json:"tier"` // V2.14:并发输入上下文档位(如 2K)
	AvgTTFT     float64 `json:"avg_ttft"`
	P99TTFT     float64 `json:"p99_ttft"`
	AvgSpeed    float64 `json:"avg_speed"`
	Throughput  float64 `json:"throughput"`
	AvgPerReq   float64 `json:"avg_per_req"` // 平均每请求吞吐 = 总吞吐/并发数(V2.3)
	Ttft        DistStats `json:"ttft"` // V2.10:TTFT 分布统计(全部有效请求)
	Speed       DistStats `json:"speed"` // V2.10:单请求速度分布统计
	FailRate    float64 `json:"fail_rate"`
	Rounds      int     `json:"rounds"`
	PlannedRounds int   `json:"planned_rounds"` // V2.14:计划轮次
	OkCount     int     `json:"ok_count"`       // V2.14:成功请求数
	FailCount   int     `json:"fail_count"`     // V2.14:失败请求数
	CircuitTrip bool    `json:"circuit_trip"`
	Note        string  `json:"note"`
}

type CacheRow struct {
	Tier        string  `json:"tier"`
	ColdTTFT    float64 `json:"cold_ttft"`
	HotTTFT     float64 `json:"hot_ttft"`
	Improvement float64 `json:"improvement"`
	HitRate     float64 `json:"hit_rate"` // 热轮缓存命中率(cached/prompt,服务返回时)
	Ttft        DistStats `json:"ttft"` // V2.10:冷请求 TTFT 分布
	Speed       DistStats `json:"speed"` // V2.10:生成速度分布(全部有效样本)
	Note        string  `json:"note"`
}

type StabilityWindow struct {
	Index       int     `json:"index"`
	StartTS     int64   `json:"start_ts"`
	EndTS       int64   `json:"end_ts"`
	SpeedMedian float64 `json:"speed_median"`
	Ttft        DistStats `json:"ttft"` // V2.10:窗口 TTFT 分布
	Speed       DistStats `json:"speed"` // V2.10:窗口速度分布
	OkCount     int       `json:"ok_count"`   // V2.10:窗口成功请求数
	FailCount   int       `json:"fail_count"` // V2.10:窗口失败请求数
	Samples     int     `json:"samples"`
	PromptSum   int64   `json:"prompt_sum"`     // 窗口输入 token 总量(V2.3)
	CompleteSum int64   `json:"complete_sum"`   // 窗口输出 token 总量
	Trend       string  `json:"trend"`          // 较上窗 升/降/平
	Note        string  `json:"note"`
}

type TaskSummary struct {
	TaskID          int64   `json:"task_id"`
	TotalPrompt     int64   `json:"total_prompt"`     // 本次 run 累计输入 tokens
	TotalCompletion int64   `json:"total_completion"` // 累计输出 tokens(含思维链)
	Speed1K         float64 `json:"speed_1k"`
	Speed1KEst      bool    `json:"speed_1k_est"`
	TopTier         string  `json:"top_tier"`
	TopTierTTFT     float64 `json:"top_tier_ttft"`
	PeakThroughput  float64 `json:"peak_throughput"`
	PeakConcurrency int     `json:"peak_concurrency"`
	Degradation     float64 `json:"degradation"`
	DegradationNote string  `json:"degradation_note"`
	DecayTested     bool    `json:"decay_tested"` // 稳定性样本足以计算衰减(≥2 窗口);0% 与未测试需区分
	Quality         string  `json:"quality"`
	// V2.22:总调用次数(全部阶段,成功失败都计,实时随样本增长)
	TotalCalls      int     `json:"total_calls"`
	OkCalls         int     `json:"ok_calls"`
	FailCalls       int     `json:"fail_calls"`
}

type sampleRow struct {
	tier             string
	concurrency      int
	seq              int
	sendTS, firstTS  int64
	endTS            int64
	ttft, total      float64
	pt, ct           int
	est, incomplete  bool
	mismatch         bool
	usageSuspect     bool
	speed            float64
	status, errMsg   string
}

// plannedRepeats 计划次数(V2.14 进度计数):分项重复次数,0 回退全局 repeats
func plannedRepeats(st *store.Store, taskID int64, item string) int {
	t, err := st.GetTask(taskID)
	if err != nil {
		return 0
	}
	p := store.ParseTaskParams(t.ParamsJSON)
	r := p.Exec.Repeats
	switch item {
	case "ladder":
		if p.Exec.LadderRepeats > 0 {
			r = p.Exec.LadderRepeats
		}
	case "concurrency":
		if p.Exec.ConcurrencyRepeats > 0 {
			r = p.Exec.ConcurrencyRepeats
		}
	case "cache":
		if p.Exec.CacheRepeats > 0 {
			r = p.Exec.CacheRepeats
		}
	}
	return r
}

// tierOrder 档位排序键(V2.5:支持小数档,按目标 token 数排;V2.7 修复 M 后缀)
func tierOrder(tier string) int {
	t := strings.ToUpper(strings.TrimSpace(tier))
	mult := 1000
	if strings.HasSuffix(t, "M") {
		mult = 1000000
		t = strings.TrimSuffix(t, "M")
	} else {
		t = strings.TrimSuffix(t, "K")
	}
	f, err := strconv.ParseFloat(t, 64)
	if err != nil || f <= 0 {
		return 999999
	}
	return int(f * float64(mult))
}

// valid 参与中位数的样本(§6.1/§6.6:估算与输出未满不参与;usage 异常护栏同样剔除)
func (s sampleRow) valid() bool {
	return s.status == "ok" && !s.est && !s.incomplete && !s.usageSuspect
}

// Ladder 阶梯结果:每档一行
func Ladder(st *store.Store, taskID, runNo int64, itemID int64) []LadderRow {
	planned := plannedRepeats(st, taskID, "ladder")
	skipped := map[string]bool{}
	// 阶梯项样本:通过 item 类型过滤
	rows, _ := st.DB().Query(`SELECT tier,ttft_ms,total_ms,speed,prompt_tokens,tokens_estimated,output_incomplete,length_mismatch,COALESCE(usage_suspect,0),status,error
		FROM request_samples WHERE task_id=? AND run_no=? AND phase='formal' AND item_id IN (SELECT id FROM test_items WHERE task_id=? AND type='ladder') ORDER BY id`, taskID, runNo, taskID)
	byTier := map[string][]sampleRow{}
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var s sampleRow
			var est, inc, mm, us int
			rows.Scan(&s.tier, &s.ttft, &s.total, &s.speed, &s.pt, &est, &inc, &mm, &us, &s.status, &s.errMsg)
			s.est, s.incomplete, s.mismatch = est != 0, inc != 0, mm != 0
			s.usageSuspect = us != 0
			if s.status == "skipped" {
				skipped[s.tier] = true
				continue
			}
			byTier[s.tier] = append(byTier[s.tier], s)
		}
	}
	tiers := map[string]bool{}
	for t := range byTier {
		tiers[t] = true
	}
	for t := range skipped {
		tiers[t] = true
	}
	var sorted []string
	for t := range tiers {
		sorted = append(sorted, t)
	}
	sort.Slice(sorted, func(i, j int) bool { return tierOrder(sorted[i]) < tierOrder(sorted[j]) })
	var out []LadderRow
	for _, t := range sorted {
		row := LadderRow{Tier: t}
		if skipped[t] && len(byTier[t]) == 0 {
			row.Skipped = true
			row.Planned = planned
			row.Note = "超出上下文限制"
			out = append(out, row)
			continue
		}
		ss := byTier[t]
		row.Samples = len(ss)
		row.Planned = planned
		var ttfts, speeds, totals []float64
		var fbTtfts, fbSpeeds, fbTotals []float64 // 兜底:ok 但被标记(输出未满等)的样本
		var est, inc, mm bool
		okN := 0
		for _, s := range ss {
			if s.status == "ok" {
				okN++
			}
			if s.pt > 0 {
				row.PromptTokens = s.pt
			}
			if s.est {
				est = true
			}
			if s.incomplete {
				inc = true
			}
			if s.mismatch {
				mm = true
			}
			if s.valid() {
				ttfts = append(ttfts, s.ttft)
				speeds = append(speeds, s.speed)
				totals = append(totals, s.total)
			} else if s.status == "ok" {
				fbTtfts = append(fbTtfts, s.ttft)
				fbSpeeds = append(fbSpeeds, s.speed)
				fbTotals = append(fbTotals, s.total)
			}
		}
		row.OkCount = okN
		// V2.6:无有效样本但有被标记样本时,显示数值+标注(不再整行留空)
		if len(ttfts) == 0 && len(fbTtfts) > 0 {
			ttfts, speeds, totals = fbTtfts, fbSpeeds, fbTotals
		}
	row.TtftMedian = Median(ttfts)
		row.SpeedMedian = Median(speeds)
		row.TotalMedian = Median(totals)
		row.Ttft = Dist(ttfts)
		row.Speed = Dist(speeds)
		var notes []string
		if est {
			notes = append(notes, "估算")
		}
		if mm {
			notes = append(notes, "长度未达标")
		}
		if inc && len(ttfts) == 0 {
			notes = append(notes, "输出未满,仅供参考")
		} else if inc {
			notes = append(notes, "部分样本输出未满")
		}
		row.Note = strings.Join(notes, ";")
		out = append(out, row)
	}
	return out
}

// Concurrency 并发结果:每档一行,各轮指标取中位数(§6.4)
func Concurrency(st *store.Store, taskID, runNo int64) []ConcurrencyRow {
	planned := plannedRepeats(st, taskID, "concurrency")
	rows, err := st.DB().Query(`SELECT concurrency,seq,ttft_ms,speed,completion_tokens,send_ts,end_ts,status,tokens_estimated,output_incomplete,COALESCE(usage_suspect,0),error,tier
		FROM request_samples WHERE task_id=? AND run_no=? AND phase='formal' AND item_id IN (SELECT id FROM test_items WHERE task_id=? AND type='concurrency') ORDER BY id`, taskID, runNo, taskID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	type rec struct {
		level, seq      int
		ttft, speed     float64
		ct              int
		sendTS, endTS   int64
		status, errMsg  string
		tier            string
		est, incomplete bool
		usageSuspect  bool
	}
	var recs []rec
	skippedCells := map[string]bool{} // V2.15:熔断粒度为 输入档×并发档
	for rows.Next() {
		var r rec
		var est, inc, us int
		rows.Scan(&r.level, &r.seq, &r.ttft, &r.speed, &r.ct, &r.sendTS, &r.endTS, &r.status, &est, &inc, &us, &r.errMsg, &r.tier)
		r.est, r.incomplete = est != 0, inc != 0
		r.usageSuspect = us != 0
		if r.status == "skipped" {
			skippedCells[r.tier+"|"+strconv.Itoa(r.level)] = true
			continue
		}
		recs = append(recs, r)
	}
	type cellKey struct {
		tier  string
		level int
	}
	cellSet := map[cellKey]bool{}
	for _, r := range recs {
		cellSet[cellKey{r.tier, r.level}] = true
	}
	for k := range skippedCells {
		parts := strings.SplitN(k, "|", 2)
		lv, _ := strconv.Atoi(parts[1])
		cellSet[cellKey{parts[0], lv}] = true
	}
	var sortedCells []cellKey
	for k := range cellSet {
		sortedCells = append(sortedCells, k)
	}
	sort.Slice(sortedCells, func(i, j int) bool {
		if sortedCells[i].tier != sortedCells[j].tier {
			return tierOrder(sortedCells[i].tier) < tierOrder(sortedCells[j].tier)
		}
		return sortedCells[i].level < sortedCells[j].level
	})
	var out []ConcurrencyRow
	for _, ck := range sortedCells {
		level := ck.level
		row := ConcurrencyRow{Level: level, Tier: ck.tier}
		row.PlannedRounds = planned
		if skippedCells[ck.tier+"|"+strconv.Itoa(level)] {
			row.CircuitTrip = true
			row.Note = "失败率过高,熔断跳过"
			out = append(out, row)
			continue
		}
		// 按轮分组:seq/1000 = 轮次(runner 中 seq=round*1000+k)
		rounds := map[int][]rec{}
		for _, r := range recs {
			if r.level == level && r.tier == ck.tier {
				rounds[r.seq/1000] = append(rounds[r.seq/1000], r)
			}
		}
		var avgTTFTs, p99s, speeds, throughputs []float64
		var levelTtfts, levelSpeeds []float64 // 该并发档全部有效请求(V2.10 分布统计)
		total, fail := 0, 0
		for _, rs := range rounds {
			row.Rounds++
			var ttfts, spds []float64
			var sumCT int
			var minSend, maxEnd int64
			for _, r := range rs {
				total++
				if r.status != "ok" {
					fail++
					continue
				}
				if r.est || r.incomplete || r.usageSuspect {
					continue
				}
				ttfts = append(ttfts, r.ttft)
				spds = append(spds, r.speed)
				levelTtfts = append(levelTtfts, r.ttft)
				levelSpeeds = append(levelSpeeds, r.speed)
				sumCT += r.ct
				if minSend == 0 || r.sendTS < minSend {
					minSend = r.sendTS
				}
				if r.endTS > maxEnd {
					maxEnd = r.endTS
				}
			}
			if len(ttfts) > 0 {
				avgTTFTs = append(avgTTFTs, Mean(ttfts))
				p99s = append(p99s, Percentile(ttfts, 99))
				speeds = append(speeds, Mean(spds))
				if maxEnd > minSend {
					throughputs = append(throughputs, float64(sumCT)/(float64(maxEnd-minSend)/1000.0))
				}
			}
		}
	row.AvgTTFT = Median(avgTTFTs)
		row.P99TTFT = Median(p99s)
		row.AvgSpeed = Median(speeds)
		row.Throughput = Median(throughputs)
		row.Ttft = Dist(levelTtfts)
		row.Speed = Dist(levelSpeeds)
		if level > 0 {
			row.AvgPerReq = row.Throughput / float64(level)
		}
		if total > 0 {
			row.FailRate = float64(fail) / float64(total)
		}
		row.OkCount = total - fail
		row.FailCount = fail
		if row.FailRate > 0.2 {
			row.Note = "失败率过高"
		}
		out = append(out, row)
	}
	return out
}

// Cache 缓存结果:每档冷热 TTFT 与提升倍数(§6.3)
func Cache(st *store.Store, taskID, runNo int64) []CacheRow {
	rows, err := st.DB().Query(`SELECT tier,phase,ttft_ms,speed,tokens_estimated,COALESCE(usage_suspect,0),status,prompt_tokens,COALESCE(cached_tokens,0)
		FROM request_samples WHERE task_id=? AND run_no=? AND phase IN ('cold','hot') ORDER BY id`, taskID, runNo)
	if err != nil {
		return nil
	}
	defer rows.Close()
	cold := map[string][]float64{}
	hot := map[string][]float64{}
	speeds := map[string][]float64{}
	est := map[string]bool{}
	hitRate := map[string][]float64{}
	for rows.Next() {
		var tier, phase, status string
		var ttft, speed float64
		var e, us int
		var pt, ct int
		rows.Scan(&tier, &phase, &ttft, &speed, &e, &us, &status, &pt, &ct)
		if status != "ok" || us != 0 {
			continue
		}
		if phase == "hot" && pt > 0 && ct > 0 {
			hitRate[tier] = append(hitRate[tier], float64(ct)/float64(pt))
		}
		if e != 0 {
			est[tier] = true
			continue
		}
		if phase == "cold" {
			cold[tier] = append(cold[tier], ttft)
		} else {
			hot[tier] = append(hot[tier], ttft)
		}
		speeds[tier] = append(speeds[tier], speed)
	}
	tiers := map[string]bool{}
	for t := range cold {
		tiers[t] = true
	}
	for t := range hot {
		tiers[t] = true
	}
	var sorted []string
	for t := range tiers {
		sorted = append(sorted, t)
	}
	sort.Slice(sorted, func(i, j int) bool { return tierOrder(sorted[i]) < tierOrder(sorted[j]) })
	var out []CacheRow
	for _, t := range sorted {
		row := CacheRow{Tier: t}
		row.ColdTTFT = Median(cold[t])
		row.HotTTFT = Median(hot[t])
		row.Ttft = Dist(cold[t])
		row.Speed = Dist(speeds[t])
		if row.ColdTTFT > 0 && row.HotTTFT > 0 {
			row.Improvement = row.ColdTTFT / row.HotTTFT
			if row.Improvement < 1.0 {
				row.Improvement = 1.0
				row.Note = "缓存疑似未生效"
			}
		}
		if len(hitRate[t]) > 0 {
			row.HitRate = Mean(hitRate[t])
			if row.HitRate <= 0.01 && row.Note == "" {
				row.Note = "服务未报告缓存命中,建议重跑复核"
			}
		}
		if est[t] {
			row.Note = strings.TrimPrefix(row.Note+";估算", ";")
		}
		out = append(out, row)
	}
	return out
}

// ---------- NIAH 检索测试(V2.5):档位 × 深度 命中率矩阵 ----------

type NIAHCell struct {
	Tier       string  `json:"tier"`      // 点位档位标签(如 128K)
	Pct        int     `json:"pct"`       // 点位:verify=长度百分比;discover=K 数;legacy=埋针深度
	DepthPct   int     `json:"depth_pct"` // 埋针深度百分比
	Tokens     int     `json:"tokens"`    // 点位目标 tokens
	Mode       string  `json:"mode"`      // verify/discover/scan/legacy
	Probes     int     `json:"probes"`    // 有效探针数(命中+未命中;错误不计)
	Hits       int     `json:"hits"`
	Errors     int     `json:"errors"` // 错误/截断探针数(V2.13:不参与命中率)
	HitRate    float64 `json:"hit_rate"`
	TtftMedian float64 `json:"ttft_median"`
	Depth      int     `json:"depth"` // 兼容老页面:= Pct(scan 系)或埋针深度(legacy)
}

// niahMeta 探针样本的 meta(V2.13 新格式;旧数据仅有 pct/hit)
type niahMeta struct {
	Mode     string `json:"mode"`
	Pct      int    `json:"pct"`
	DepthPct int    `json:"depth_pct"`
	Tokens   int    `json:"tokens"`
	Outcome  string `json:"outcome"`
	Hit      bool   `json:"hit"`
}

// StabilityPoint 稳定性逐次调用点(V2.10 散点图:X=调用序号)
type StabilityPoint struct {
	I     int     `json:"i"`
	Ttft  float64 `json:"ttft"`
	Speed float64 `json:"speed"`
	OK    bool    `json:"ok"`
}

// StabilityPoints 稳定性 phase 全部请求的逐次数据
func StabilityPoints(st *store.Store, taskID, runNo int64) []StabilityPoint {
	rows, err := st.DB().Query(`SELECT ttft_ms,speed,status,tokens_estimated,output_incomplete,COALESCE(usage_suspect,0)
		FROM request_samples WHERE task_id=? AND run_no=? AND phase='stability' ORDER BY send_ts`, taskID, runNo)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []StabilityPoint
	i := 0
	for rows.Next() {
		var p StabilityPoint
		var est, inc, us int
		var status string
		rows.Scan(&p.Ttft, &p.Speed, &status, &est, &inc, &us)
		i++
		p.I = i
		p.OK = status == "ok" && est == 0 && inc == 0 && us == 0
		out = append(out, p)
	}
	return out
}

// NIAH 聚合 phase='niah' 样本(V2.13:按 meta 的 mode/pct/depth_pct 聚合;错误探针单列不计命中率;旧数据回退兼容)
func NIAH(st *store.Store, taskID, runNo int64) []NIAHCell {
	rows, err := st.DB().Query(`SELECT tier,concurrency,ttft_ms,status,tokens_estimated,COALESCE(usage_suspect,0),COALESCE(meta,'')
		FROM request_samples WHERE task_id=? AND run_no=? AND phase='niah'
		AND item_id IN (SELECT id FROM test_items WHERE task_id=? AND type='niah') ORDER BY id`, taskID, runNo, taskID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	type acc struct {
		probes, hits, errors int
		ttfts                []float64
	}
	type key struct {
		mode, tier       string
		pct, depth, toks int
	}
	cells := map[key]*acc{}
	var order []key
	for rows.Next() {
		var tier, meta, status string
		var conc, est, us int
		var ttft float64
		rows.Scan(&tier, &conc, &ttft, &status, &est, &us, &meta)
		// 解析 meta(新旧格式兼容)
		var m niahMeta
		json.Unmarshal([]byte(meta), &m)
		k := key{tier: tier}
		isErr := false
		switch {
		case m.Mode != "": // V2.13 新格式
			k.mode, k.pct, k.depth, k.toks = m.Mode, m.Pct, m.DepthPct, m.Tokens
			isErr = m.Outcome == "错误"
		case m.Pct > 0: // V2.7~V2.8 旧扫描:meta 只有 pct/hit,埋针深度固定
			k.mode, k.pct, k.depth, k.toks = "scan", m.Pct, 50, m.Tokens
			isErr = status != "ok"
		default: // V2.5 老矩阵:concurrency 列即埋针深度
			k.mode, k.pct, k.depth, k.toks = "legacy", conc, conc, 0
			isErr = status != "ok"
		}
		c := cells[k]
		if c == nil {
			c = &acc{}
			cells[k] = c
			order = append(order, k)
		}
		if isErr {
			c.errors++
			continue // 错误/截断探针不参与命中率
		}
		c.probes++
		hit := m.Hit || strings.Contains(meta, `"hit":true`)
		if hit {
			c.hits++
		}
		if status == "ok" && est == 0 && us == 0 {
			c.ttfts = append(c.ttfts, ttft)
		}
	}
	var out []NIAHCell
	for _, k := range order {
		c := cells[k]
		rate := 0.0
		if c.probes > 0 {
			rate = float64(c.hits) / float64(c.probes)
		}
		depthCompat := k.pct
		if k.mode == "legacy" {
			depthCompat = k.depth // 老矩阵页面把 depth 列当埋针深度
		}
		out = append(out, NIAHCell{
			Tier: k.tier, Pct: k.pct, DepthPct: k.depth, Tokens: k.toks, Mode: k.mode,
			Probes: c.probes, Hits: c.hits, Errors: c.errors, HitRate: rate,
			TtftMedian: Median(c.ttfts), Depth: depthCompat,
		})
	}
	return out
}

// Stability 稳定性窗口(§6.5):10 分钟窗口,速度中位,温/功耗均值
func Stability(st *store.Store, taskID, runNo int64, windowMin int) ([]StabilityWindow, float64, string) {
	if windowMin <= 0 {
		windowMin = 10
	}
	rows, err := st.DB().Query(`SELECT send_ts,speed,ttft_ms,status,tokens_estimated,output_incomplete,COALESCE(usage_suspect,0),prompt_tokens,completion_tokens
		FROM request_samples WHERE task_id=? AND run_no=? AND phase='stability' ORDER BY send_ts`, taskID, runNo)
	if err != nil {
		return nil, 0, ""
	}
	defer rows.Close()
	type rec struct {
		ts    int64
		speed float64
		ttft  float64
		ok    bool
		success bool
		pt    int64
		ct    int64
	}
	var recs []rec
	for rows.Next() {
		var r rec
		var est, inc, us int
		var status string
		rows.Scan(&r.ts, &r.speed, &r.ttft, &status, &est, &inc, &us, &r.pt, &r.ct)
		r.success = status == "ok"
		r.ok = status == "ok" && est == 0 && inc == 0 && us == 0
		recs = append(recs, r)
	}
	if len(recs) == 0 {
		return nil, 0, ""
	}
	t0 := recs[0].ts
	windowMs := int64(windowMin) * 60 * 1000
	windows := map[int][]rec{}
	maxW := 0
	for _, r := range recs {
		w := int((r.ts - t0) / windowMs)
		windows[w] = append(windows[w], r)
		if w > maxW {
			maxW = w
		}
	}
	var out []StabilityWindow
	for w := 0; w <= maxW; w++ {
		ws := windows[w]
		win := StabilityWindow{Index: w + 1, StartTS: t0 + int64(w)*windowMs, EndTS: t0 + int64(w+1)*windowMs}
		var speeds, ttfts []float64
		for _, r := range ws {
			if r.ok {
				speeds = append(speeds, r.speed)
				ttfts = append(ttfts, r.ttft)
			}
		}
		win.Samples = len(ws)
		win.SpeedMedian = Median(speeds)
		win.Speed = Dist(speeds)
		win.Ttft = Dist(ttfts)
		for _, r := range ws {
			if r.success {
				win.OkCount++
			} else {
				win.FailCount++
			}
		}
		for _, r := range ws {
			win.PromptSum += r.pt
			win.CompleteSum += r.ct
		}
		if len(out) > 0 {
			prev := out[len(out)-1].SpeedMedian
			switch {
			case prev > 0 && win.SpeedMedian > prev*1.02:
				win.Trend = "升"
			case win.SpeedMedian < prev*0.98:
				win.Trend = "降"
			default:
				win.Trend = "平"
			}
		}
		if len(ws) < 3 {
			win.Note = "窗口样本过少,仅供参考"
		}
		out = append(out, win)
	}
	// 衰减与降频判断(§6.5)
	var decay float64
	var note string
	if len(out) >= 2 {
		first := out[0].SpeedMedian
		last := out[len(out)-1].SpeedMedian
		decay = Degradation(first, last)
		if first > 0 && len(out) >= 3 {
			d1 := Degradation(first, out[len(out)-1].SpeedMedian)
			d2 := Degradation(first, out[len(out)-2].SpeedMedian)
			if d1 > 5 && d2 > 5 {
				note = "疑似降频(末两窗口速度较首窗口下降 > 5%)"
			}
		}
	}
	return out, decay, note
}

// QualityVerdict 质量抽查结论:人工标记优先,未标记按自动初筛(F-7.3 / §6.8)
func QualityVerdict(st *store.Store, taskID, runNo int64) (verdict string, total, abnormal, suspect int) {
	rows, err := st.DB().Query(`SELECT auto_flag,manual_flag FROM quality_answers WHERE task_id=? AND run_no=?`, taskID, runNo)
	if err != nil {
		return "未测试", 0, 0, 0
	}
	defer rows.Close()
	for rows.Next() {
		var auto, manual string
		rows.Scan(&auto, &manual)
		total++
		flag := auto
		if manual != "" {
			flag = manual
		}
		switch flag {
		case "abnormal":
			abnormal++
		case "suspect":
			suspect++
		}
	}
	if total == 0 {
		return "未测试", 0, 0, 0
	}
	switch {
	case abnormal > 0:
		return "异常", total, abnormal, suspect
	case suspect > 0:
		return "疑似异常", total, abnormal, suspect
	default:
		return "正常", total, abnormal, suspect
	}
}

// Summary 任务摘要卡片(F-7.3)
func Summary(st *store.Store, taskID int64) TaskSummary {
	var ts TaskSummary
	ts.TaskID = taskID
	task, err := st.GetTask(taskID)
	if err != nil {
		return ts
	}
	runNo := int64(task.RunNo)
	// 累计 token(全部有效样本,含预热/校准——真实成本口径)
	st.QueryRow(`SELECT COALESCE(SUM(prompt_tokens),0), COALESCE(SUM(completion_tokens),0)
		FROM request_samples WHERE task_id=? AND run_no=? AND status='ok'`, taskID, runNo).
		Scan(&ts.TotalPrompt, &ts.TotalCompletion)
	// 总调用次数(校准+预热+正式+压测全计,成功失败都计)
	st.QueryRow(`SELECT COUNT(*), COALESCE(SUM(CASE WHEN status='ok' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN status='ok' THEN 0 ELSE 1 END),0)
		FROM request_samples WHERE task_id=? AND run_no=?`, taskID, runNo).
		Scan(&ts.TotalCalls, &ts.OkCalls, &ts.FailCalls)
	ladder := Ladder(st, taskID, runNo, 0)
	// 1K 速度
	for _, r := range ladder {
		if r.Tier == "1K" && !r.Skipped {
			ts.Speed1K = r.SpeedMedian
		}
	}
	// 最高执行档 TTFT
	for _, r := range ladder {
		if !r.Skipped && r.Samples > 0 {
			ts.TopTier = r.Tier
			ts.TopTierTTFT = r.TtftMedian
		}
	}
	// 峰值吞吐
	for _, c := range Concurrency(st, taskID, runNo) {
		if !c.CircuitTrip && c.Throughput > ts.PeakThroughput {
			ts.PeakThroughput = c.Throughput
			ts.PeakConcurrency = c.Level
		}
	}
	// 衰减(≥2 窗口才算有效;0% 衰减与「未测试」要区分)
	wins, decay, dnote := Stability(st, taskID, runNo, 0)
	ts.Degradation = decay
	ts.DegradationNote = dnote
	ts.DecayTested = len(wins) >= 2
	// 质量
	verdict, _, _, _ := QualityVerdict(st, taskID, runNo)
	ts.Quality = verdict
	return ts
}

// ---------- V2.2 能力评分 ----------

// CapabilityCats 能力维度固定顺序(雷达图/表格列)
var CapabilityCats = []string{"common", "reasoning", "instruction", "code", "creative", "summary", "needle", "consistency", "doc"}

var CapabilityCatNames = map[string]string{
	"common": "常识", "reasoning": "推理", "instruction": "指令遵循", "code": "代码",
	"creative": "创作", "summary": "摘要", "needle": "长文找回", "consistency": "一致性", "doc": "文档",
}

// verdictScore 单题得分:人工 > LLM评判 > 规则/初筛
func verdictScore(manual, judgeScore, auto string) (float64, bool) {
	switch manual {
	case "normal":
		return 1, true
	case "abnormal":
		return 0, true
	}
	switch judgeScore {
	case "pass":
		return 1, true
	case "partial":
		return 0.5, true
	case "fail":
		return 0, true
	}
	switch auto {
	case "normal":
		return 1, true
	case "suspect":
		return 0.25, true
	}
	return 0, false
}

// Capability 任务能力评分:每维度平均分(0-1)
func Capability(st *store.Store, taskID, runNo int64) map[string]float64 {
	rows, err := st.DB().Query(`SELECT COALESCE(NULLIF(q.category,''),q.qtype,''),qa.auto_flag,qa.manual_flag,COALESCE(qa.judge_score,'')
		FROM quality_answers qa LEFT JOIN questions q ON q.id=qa.question_id
		WHERE qa.task_id=? AND qa.run_no=?`, taskID, runNo)
	if err != nil {
		return nil
	}
	defer rows.Close()
	type agg struct{ sum float64; n int }
	byCat := map[string]*agg{}
	for rows.Next() {
		var cat, auto, manual, js string
		rows.Scan(&cat, &auto, &manual, &js)
		score, ok := verdictScore(manual, js, auto)
		if !ok || cat == "" {
			continue
		}
		if byCat[cat] == nil {
			byCat[cat] = &agg{}
		}
		byCat[cat].sum += score
		byCat[cat].n++
	}
	out := map[string]float64{}
	for cat, a := range byCat {
		if a.n > 0 {
			out[cat] = a.sum / float64(a.n)
		}
	}
	return out
}
