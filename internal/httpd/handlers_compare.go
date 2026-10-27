package httpd

import (
	"net/http"
	"net/url"
	"strings"
	"llmhwtest/internal/exporter"
	"llmhwtest/internal/stats"
	"llmhwtest/internal/store"
)

func urlEscape(s string) string { return url.PathEscape(s) }

// ---------- 对比分析(§5.9,V1.2) ----------

// handleCompareModels 已完成任务中出现过的 模型+精度 组合(对比视角一)
func (s *Server) handleCompareModels(w http.ResponseWriter, r *http.Request) {
	rows, err := s.st.DB().Query(`SELECT t.model_name, t.quant, COUNT(*) FROM tasks t
		LEFT JOIN projects p ON p.id=t.project_id
		WHERE t.status='done' AND COALESCE(p.hidden,0)=0 GROUP BY t.model_name, t.quant ORDER BY t.model_name`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	var list []map[string]any
	for rows.Next() {
		var name, quant string
		var n int
		rows.Scan(&name, &quant, &n)
		list = append(list, map[string]any{"model_name": name, "quant": quant, "count": n})
	}
	writeJSON(w, list)
}

// handleCompareTasks 自由筛选对比(F-8.1/F-8.2):模型视角 / 设备视角 + 条件筛选
func (s *Server) handleCompareTasks(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	where := "t.status='done' AND COALESCE(p.hidden,0)=0"
	var args []any
	if v := q.Get("model_name"); v != "" {
		where += " AND t.model_name=?"
		args = append(args, v)
	}
	if v := q.Get("quant"); v != "" {
		where += " AND t.quant=?"
		args = append(args, v)
	}
	if v := q.Get("device_ids"); v != "" {
		var ids []string
		for _, part := range strings.Split(v, ",") {
			if part = strings.TrimSpace(part); part != "" {
				ids = append(ids, part)
			}
		}
		if len(ids) > 0 {
			where += " AND t.device_id IN (" + strings.Repeat("?,", len(ids)-1) + "?)"
			for _, id := range ids {
				args = append(args, id)
			}
		}
	}
	if v := q.Get("form_factor"); v != "" {
		where += " AND d.form_factor=?"
		args = append(args, v)
	}
	if v := q.Get("project_id"); v != "" {
		where += " AND t.project_id=?"
		args = append(args, v)
	}
	if v := q.Get("tester"); v != "" {
		where += " AND p.tester LIKE ?"
		args = append(args, "%"+v+"%")
	}
	if v := q.Get("from"); v != "" {
		where += " AND t.finished_at>=?"
		args = append(args, v)
	}
	if v := q.Get("to"); v != "" {
		where += " AND t.finished_at<=?"
		args = append(args, v)
	}
	rows, err := s.st.DB().Query(`SELECT t.id FROM tasks t
		LEFT JOIN devices d ON d.id=t.device_id
		LEFT JOIN projects p ON p.id=t.project_id
		WHERE `+where+` ORDER BY t.finished_at DESC`, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	var ids []int64
	for rows.Next() {
		var id int64
		rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	type taskWithSummary struct {
		Task    any `json:"task"`
		Summary any `json:"summary"`
		Project any `json:"project_name"`
		Tester  any `json:"tester"`
		Tags    []string `json:"tags"` // 测试项(能力/阶梯/并发/缓存/稳定性/NIAH)
	}
	var out []taskWithSummary
	for _, id := range ids {
		t, err := s.st.GetTask(id)
		if err != nil {
			continue
		}
		var projName, tester string
		s.st.QueryRow(`SELECT COALESCE(name,''), COALESCE(tester,'') FROM projects WHERE id=?`, t.ProjectID).Scan(&projName, &tester)
		items := store.ParseTaskParams(t.ParamsJSON).Items
		tags := []string{}
		if items.Quality {
			tags = append(tags, "能力")
		}
		if items.Ladder {
			tags = append(tags, "阶梯")
		}
		if items.Concurrency {
			tags = append(tags, "并发")
		}
		if items.Cache {
			tags = append(tags, "缓存")
		}
		if items.Stability {
			tags = append(tags, "稳定性")
		}
		if items.NIAH {
			tags = append(tags, "NIAH")
		}
		out = append(out, taskWithSummary{t, stats.Summary(s.st, id), projName, tester, tags})
	}
	writeJSON(w, out)
}

// handleCompareCurves 叠加图数据(F-8.3):阶梯速度/并发吞吐/缓存 TTFT/稳定性/NIAH
func (s *Server) handleCompareCurves(w http.ResponseWriter, r *http.Request) {
	ids := parseIDs(r.URL.Query().Get("ids"))
	type curves struct {
		TaskID      int64                  `json:"task_id"`
		Label       string                 `json:"label"`
		Ladder      []stats.LadderRow      `json:"ladder"`
		Concurrency []stats.ConcurrencyRow `json:"concurrency"`
		Cache       []stats.CacheRow       `json:"cache"`
		// V2.9:稳定性与 NIAH 补齐(对比内容六项测试项全覆盖)
		Stability   []stats.StabilityWindow `json:"stability"`
		Decay       float64                 `json:"decay"`        // 衰减%(0=无衰减;未测看 decay_tested)
		DecayTested bool                    `json:"decay_tested"`
		NIAH        []stats.NIAHCell        `json:"niah"`
		ContextLen  int                     `json:"context_len"`
	}
	var out []curves
	for _, id := range ids {
		t, err := s.st.GetTask(id)
		if err != nil {
			continue
		}
		wins, decay, _ := stats.Stability(s.st, id, int64(t.RunNo), 0)
		out = append(out, curves{
			TaskID: id,
			Label:  itoa(id) + " " + t.DeviceName + "/" + t.ModelName,
			Ladder: stats.Ladder(s.st, id, int64(t.RunNo), 0),
			Concurrency: stats.Concurrency(s.st, id, int64(t.RunNo)),
			Cache:       stats.Cache(s.st, id, int64(t.RunNo)),
			Stability:   wins,
			Decay:       decay,
			DecayTested: len(wins) > 1,
			NIAH:        stats.NIAH(s.st, id, int64(t.RunNo)),
			ContextLen:  t.ContextLen,
		})
	}
	writeJSON(w, out)
}

func (s *Server) handleCompareExport(w http.ResponseWriter, r *http.Request) {
	ids := parseIDs(r.URL.Query().Get("ids"))
	if len(ids) == 0 {
		writeErr(w, http.StatusBadRequest, "请先勾选要对比的任务")
		return
	}
	buf, name, err := exporter.ComparePackage(s.st, ids)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+urlEscape(name))
	w.Write(buf.Bytes())
}

// ---------- 对比看板(V2.22):/compare/board ----------
// 每个任务聚合:总均值/稳定性汇总/阶梯/并发/缓存/NIAH,前端看板直接消费

type boardLadder struct {
	Tier string  `json:"tier"`
	TTFT float64 `json:"ttft"`
	TPS  float64 `json:"tps"`
}
type boardConc struct {
	Tier string  `json:"tier"`
	C    int     `json:"c"`
	TTFT float64 `json:"ttft"`
	TPS  float64 `json:"tps"`
}
type boardCache struct {
	Tier   string  `json:"tier"`
	Hit    float64 `json:"hit"`
	Saving float64 `json:"saving"`
}
type boardTask struct {
	TaskID      int64             `json:"task_id"`
	AvgTTFT     float64           `json:"avg_ttft"`
	AvgTPS      float64           `json:"avg_tps"`
	StabOK      int               `json:"stab_ok"`
	StabFail    int               `json:"stab_fail"`
	StabAvgTTFT float64           `json:"stab_avg_ttft"`
	StabAvgTPS  float64           `json:"stab_avg_tps"`
	Ladder      []boardLadder     `json:"ladder"`
	Conc        []boardConc       `json:"conc"`
	Cache       []boardCache      `json:"cache"`
	NIAH        []stats.NIAHCell  `json:"niah"`
}

func (s *Server) handleCompareBoard(w http.ResponseWriter, r *http.Request) {
	ids := parseIDs(r.URL.Query().Get("ids"))
	out := make([]boardTask, 0, len(ids))
	for _, id := range ids {
		t, err := s.st.GetTask(id)
		if err != nil {
			continue
		}
		runNo := int64(t.RunNo)
		bt := boardTask{TaskID: id}
		// 总均值(正式样本,有效)
		s.st.QueryRow(`SELECT COALESCE(AVG(ttft_ms),0), COALESCE(AVG(speed),0)
			FROM request_samples WHERE task_id=? AND run_no=? AND phase='formal' AND status='ok'`, id, runNo).
			Scan(&bt.AvgTTFT, &bt.AvgTPS)
		// 稳定性汇总
		s.st.QueryRow(`SELECT COALESCE(SUM(CASE WHEN status='ok' THEN 1 ELSE 0 END),0),
			COALESCE(SUM(CASE WHEN status='ok' THEN 0 ELSE 1 END),0),
			COALESCE(AVG(CASE WHEN status='ok' THEN ttft_ms END),0),
			COALESCE(AVG(CASE WHEN status='ok' THEN speed END),0)
			FROM request_samples WHERE task_id=? AND run_no=? AND phase='stability'`, id, runNo).
			Scan(&bt.StabOK, &bt.StabFail, &bt.StabAvgTTFT, &bt.StabAvgTPS)
		for _, r := range stats.Ladder(s.st, id, runNo, 0) {
			if r.Skipped || r.Samples == 0 {
				continue
			}
			bt.Ladder = append(bt.Ladder, boardLadder{Tier: r.Tier, TTFT: r.TtftMedian, TPS: r.SpeedMedian})
		}
		for _, c := range stats.Concurrency(s.st, id, runNo) {
			if c.CircuitTrip {
				continue
			}
			tier := c.Tier
			if tier == "" {
				tier = "1K"
			}
			bt.Conc = append(bt.Conc, boardConc{Tier: tier, C: c.Level, TTFT: c.AvgTTFT, TPS: c.AvgSpeed})
		}
		for _, c := range stats.Cache(s.st, id, runNo) {
			bt.Cache = append(bt.Cache, boardCache{Tier: c.Tier, Hit: c.HitRate * 100, Saving: c.Improvement})
		}
		bt.NIAH = stats.NIAH(s.st, id, runNo)
		out = append(out, bt)
	}
	writeJSON(w, out)
}

// handleCompareCapability 能力域对比:每任务八维度得分 + 总分(V2.2)
func (s *Server) handleCompareCapability(w http.ResponseWriter, r *http.Request) {
	ids := parseIDs(r.URL.Query().Get("ids"))
	type row struct {
		TaskID  int64              `json:"task_id"`
		Label   string             `json:"label"`
		Source  string             `json:"source"` // 云端/设备名
		Quant   string             `json:"quant"`
		Scores  map[string]float64 `json:"scores"`
		Overall float64            `json:"overall"`
	}
	var out []row
	for _, id := range ids {
		t, err := s.st.GetTask(id)
		if err != nil {
			continue
		}
		source := t.DeviceName
		if source == "" {
			source = "云端"
		}
		scores := stats.Capability(s.st, id, int64(t.RunNo))
		var sum float64
		var n int
		for _, c := range stats.CapabilityCats {
			if v, ok := scores[c]; ok {
				sum += v
				n++
			}
		}
		overall := 0.0
		if n > 0 {
			overall = sum / float64(n)
		}
		out = append(out, row{
			TaskID: id,
			Label:  itoa(id) + " " + t.ModelName,
			Source: source, Quant: t.Quant, Scores: scores, Overall: overall,
		})
	}
	writeJSON(w, map[string]any{
		"cats": stats.CapabilityCats, "cat_names": stats.CapabilityCatNames, "rows": out,
	})
}

func parseIDs(v string) []int64 {
	var out []int64
	for _, part := range strings.Split(v, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		var id int64
		for _, ch := range part {
			if ch < '0' || ch > '9' {
				id = 0
				break
			}
			id = id*10 + int64(ch-'0')
		}
		if id > 0 {
			out = append(out, id)
		}
	}
	return out
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
