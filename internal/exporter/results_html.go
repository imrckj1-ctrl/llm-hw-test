// 交互式结果页生成器(V2.17):单文件 HTML(内嵌 echarts + 真实数据),
// 打 zip 后作为 OLE 附件嵌入 Word 报告「一、测试结论」下方
package exporter

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"

	"llmhwtest/internal/stats"
	"llmhwtest/internal/store"
)

type resultsPayload struct {
	Title string        `json:"title"`
	Tasks []resultsTask `json:"tasks"`
}

type resultsTask struct {
	Title string                 `json:"title"`
	Cards [][2]string            `json:"cards"`
	Lad   []stats.LadderRow      `json:"ladder"`
	Conc  []stats.ConcurrencyRow `json:"concurrency"`
	Cac   []stats.CacheRow       `json:"cache"`
	Stab  stabPayload            `json:"stability"`
	NIAH  []stats.NIAHCell       `json:"niah"`
	Eff   *effPayload            `json:"niah_effective,omitempty"`
	Q     qualityPayload         `json:"quality"`
}

type stabPayload struct {
	Decay  float64                `json:"decay"`
	Points []stats.StabilityPoint `json:"points"`
	// V2.23:压测三要素(输入档/每波并发/时长),与详情页参数行一致
	Note   string                 `json:"note"`
}

type effPayload struct {
	Tokens int  `json:"tokens"`
	Pct    int  `json:"pct"`
	Top    bool `json:"at_ceiling"`
}

type qualityPayload struct {
	Verdict  string      `json:"verdict"`
	Total    int         `json:"total"`
	Abnormal int         `json:"abnormal"`
	Suspect  int         `json:"suspect"`
	List     []qualityIt `json:"list"`
}

type qualityIt struct {
	T string `json:"t"`
	S string `json:"s"`
	F string `json:"f"`
}

// buildResultsHTML 生成项目交互式结果页;无任务时返回 nil
// decayCard 性能衰减卡:未测显示 '-',0% 衰减与未测试区分
func decayCard(s stats.TaskSummary) string {
	if !s.DecayTested {
		return "-"
	}
	return f2(s.Degradation) + "%"
}

func buildResultsHTML(st *store.Store, p *store.Project, tasks []*store.Task) []byte {
	payload := resultsPayload{Title: p.Name}
	for _, t := range tasks {
		rt := resultsTask{Title: fmt.Sprintf("任务 %d(%s)", t.ID, t.ModelName)}
		sum := stats.Summary(st, t.ID)
		rt.Cards = [][2]string{
			{f2(sum.Speed1K), "1K 输出速度 (tokens/s)"},
			{f1(sum.TopTierTTFT), fmt.Sprintf("%s TTFT (ms)", orDash(sum.TopTier))},
			{f2(sum.PeakThroughput), fmt.Sprintf("峰值总吞吐(并发 %d)", sum.PeakConcurrency)},
			{commaInt64(int64(sum.TotalCalls)), fmt.Sprintf("总调用次数(成功 %s / 失败 %s)", commaInt64(int64(sum.OkCalls)), commaInt64(int64(sum.FailCalls)))},
			{commaInt64(sum.TotalPrompt), "总输入 tokens"},
			{commaInt64(sum.TotalCompletion), "总输出 tokens"},
			{decayCard(sum), "性能衰减"},
			{orDash(sum.Quality), "质量结论"},
		}
		rt.Lad = stats.Ladder(st, t.ID, int64(t.RunNo), 0)
		rt.Conc = stats.Concurrency(st, t.ID, int64(t.RunNo))
		rt.Cac = stats.Cache(st, t.ID, int64(t.RunNo))
		_, decay, _ := stats.Stability(st, t.ID, int64(t.RunNo), 0)
		rt.Stab = stabPayload{Decay: decay, Points: stats.StabilityPoints(st, t.ID, int64(t.RunNo))}
		// V2.23:稳定性压测参数行(与详情页一致)
		ep := store.ParseTaskParams(t.ParamsJSON)
		stabTier := ep.Exec.StabilityTier
		if stabTier == "" {
			stabTier = "10K"
		}
		stabConc := ep.Exec.StabilityConcurrency
		if stabConc <= 0 {
			stabConc = 1
		}
		rt.Stab.Note = fmt.Sprintf("压测参数:输入档 %s · 每波并发 %d · 时长 %d 分钟", stabTier, stabConc, ep.Exec.StabilityMinutes)
		rt.NIAH = stats.NIAH(st, t.ID, int64(t.RunNo))
		if best, maxTested := 0, 0; len(rt.NIAH) > 0 {
			for _, c := range rt.NIAH {
				if c.Tokens > maxTested {
					maxTested = c.Tokens
				}
				if c.HitRate >= 0.5 && c.Probes > 0 && c.Tokens > best {
					best = c.Tokens
				}
			}
			if best > 0 {
				rt.Eff = &effPayload{Tokens: best, Top: best >= maxTested}
				if t.ContextLen > 0 {
					rt.Eff.Pct = best * 100 / t.ContextLen
				}
			}
		}
		verdict, total, abnormal, suspect := stats.QualityVerdict(st, t.ID, int64(t.RunNo))
		rt.Q = qualityPayload{Verdict: verdict, Total: total, Abnormal: abnormal, Suspect: suspect}
		qrows, _ := st.DB().Query(`SELECT question_title,COALESCE(judge_score,''),auto_flag
			FROM quality_answers WHERE task_id=? AND run_no=? ORDER BY id`, t.ID, t.RunNo)
		if qrows != nil {
			for qrows.Next() {
				var it qualityIt
				qrows.Scan(&it.T, &it.S, &it.F)
				if it.S == "" {
					it.S = "-"
				}
				rt.Q.List = append(rt.Q.List, it)
			}
			qrows.Close()
		}
		payload.Tasks = append(payload.Tasks, rt)
	}
	if len(payload.Tasks) == 0 {
		return nil
	}
	data, _ := json.Marshal(payload)
	out := resultsHTMLHead
	out = replaceAll(out, "__TITLE__", payload.Title)
	out = replaceAll(out, "__ECHARTS__", string(echartsJS))
	out = replaceAll(out, "__DATA__", string(data))
	return []byte(out)
}

func replaceAll(s, old, new string) string {
	return bytes.NewBuffer(bytes.ReplaceAll([]byte(s), []byte(old), []byte(new))).String()
}

// zipSingle 单文件打包
func zipSingle(name string, content []byte) []byte {
	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)
	w, err := zw.Create(name)
	if err == nil {
		w.Write(content)
	}
	zw.Close()
	return buf.Bytes()
}

func commaInt64(n int64) string {
	s := fmt.Sprintf("%d", n)
	var out []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, byte(c))
	}
	return string(out)
}
