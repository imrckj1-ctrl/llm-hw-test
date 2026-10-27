package httpd

import (
	"fmt"
	"net/http"
	"strings"
	"time"
	"llmhwtest/internal/runner"
	"llmhwtest/internal/stats"
)

// ---------- 结果展示(§5.8) ----------

func (s *Server) handleTaskSummary(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	t, err := s.st.GetTask(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "任务不存在")
		return
	}
	items, _ := s.st.ListTestItems(id, 0)
	sum := stats.Summary(s.st, id)
	proj, _ := s.st.GetProject(t.ProjectID) // 项目即任务:详情页需要项目层信息(类型/硬件/备注)
	// 历史 run 列表(F-5.4,前端切换器用)
	// V3.0.2:补全为 1..当前 run_no 的完整区间——仅按有样本的 run 出选项时,
	// 「当前 run 无数据」会导致下拉里没有当前值(显示 3 却只有 1 可选);
	// 无数据的 run 选中后前端显示空态,比不可选更直观
	maxRun := t.RunNo
	rrows, _ := s.st.DB().Query(`SELECT DISTINCT run_no FROM request_samples WHERE task_id=? ORDER BY run_no`, id)
	if rrows != nil {
		for rrows.Next() {
			var rn int
			rrows.Scan(&rn)
			if rn > maxRun {
				maxRun = rn
			}
		}
		rrows.Close()
	}
	if maxRun < 1 {
		maxRun = 1
	}
	runs := []int{}
	for rn := 1; rn <= maxRun; rn++ {
		runs = append(runs, rn)
	}
	writeJSON(w, map[string]any{
		"task": t, "items": items, "summary": sum, "project": proj, "runs": runs,
	})
}

func (s *Server) handleTaskItems(w http.ResponseWriter, r *http.Request) {
	items, err := s.st.ListTestItems(pathID(r, "id"), 0)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, items)
}

// handleTaskResults 按测试项聚合结果(表格 + 图表数据)
func (s *Server) handleTaskResults(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	t, err := s.st.GetTask(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "任务不存在")
		return
	}
	runNo := int64(t.RunNo)
	if rn := queryInt(r, "run_no"); rn > 0 {
		runNo = rn
	}
	wins, decay, dnote := stats.Stability(s.st, id, runNo, 0)
	niahCells := stats.NIAH(s.st, id, runNo)
	// V2.13 有效上下文结论:按点位实际 tokens 取命中率≥50% 的最高点位;触顶=最高测试点位仍全部命中
	var niahEff map[string]any
	if len(niahCells) > 0 {
		best, maxTested, mode := 0, 0, ""
		for _, c := range niahCells {
			if c.Tokens > maxTested {
				maxTested = c.Tokens
			}
			if c.HitRate >= 0.5 && c.Probes > 0 && c.Tokens > best {
				best = c.Tokens
			}
			if c.Mode != "" {
				mode = c.Mode
			}
		}
		if best > 0 {
			eff := map[string]any{"tokens": best, "at_ceiling": best >= maxTested, "mode": mode}
			if t.ContextLen > 0 {
				eff["pct"] = best * 100 / t.ContextLen
			}
			niahEff = eff
		}
	}
	writeJSON(w, map[string]any{
		"run_no":         runNo,
		"ladder":         stats.Ladder(s.st, id, runNo, 0),
		"concurrency":    stats.Concurrency(s.st, id, runNo),
		"cache":          stats.Cache(s.st, id, runNo),
		"niah":           niahCells,
		"niah_effective": niahEff,
		"stability":      map[string]any{"windows": wins, "decay": decay, "note": dnote, "points": stats.StabilityPoints(s.st, id, runNo)},
	})
}

// handleTaskSamples 请求级明细:分页 + 筛选(F-7.4)
func (s *Server) handleTaskSamples(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	q := r.URL.Query()
	where := "task_id=?"
	args := []any{id}
	if v := q.Get("phase"); v != "" {
		where += " AND phase=?"
		args = append(args, v)
	}
	if v := q.Get("tier"); v != "" {
		where += " AND tier=?"
		args = append(args, v)
	}
	if v := q.Get("status"); v != "" {
		where += " AND status=?"
		args = append(args, v)
	}
	if v := q.Get("item_id"); v != "" {
		where += " AND item_id=?"
		args = append(args, v)
	}
	if v := q.Get("run_no"); v != "" {
		where += " AND run_no=?"
		args = append(args, v)
	}
	page := int(queryInt(r, "page"))
	if page <= 0 {
		page = 1
	}
	size := int(queryInt(r, "page_size"))
	if size <= 0 || size > 500 {
		size = 50
	}
	var total int
	s.st.QueryRow(`SELECT COUNT(*) FROM request_samples WHERE `+where, args...).Scan(&total)
	rows, err := s.st.DB().Query(`SELECT id,run_no,phase,tier,concurrency,seq,send_ts,first_chunk_ts,end_ts,
		ttft_ms,total_ms,prompt_tokens,completion_tokens,tokens_estimated,output_incomplete,length_mismatch,
		speed,status,error,COALESCE(item_id,0),SUBSTR(COALESCE(raw_response,''),1,8000)
		FROM request_samples WHERE `+where+` ORDER BY id DESC LIMIT ? OFFSET ?`,
		append(args, size, (page-1)*size)...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	var list []map[string]any
	for rows.Next() {
		var sid, sendTS, firstTS, endTS, itemID int64
		var runNo, conc, seq, pt, ct, est, inc, mm int
		var phase, tier, status, errMsg, raw string
		var ttft, total, speed float64
		rows.Scan(&sid, &runNo, &phase, &tier, &conc, &seq, &sendTS, &firstTS, &endTS,
			&ttft, &total, &pt, &ct, &est, &inc, &mm, &speed, &status, &errMsg, &itemID, &raw)
		list = append(list, map[string]any{
			"id": sid, "run_no": runNo, "phase": phase, "tier": tier, "concurrency": conc, "seq": seq,
			"send_ts": sendTS, "first_chunk_ts": firstTS, "end_ts": endTS,
			"ttft_ms": ttft, "total_ms": total, "prompt_tokens": pt, "completion_tokens": ct,
			"tokens_estimated": est != 0, "output_incomplete": inc != 0, "length_mismatch": mm != 0,
			"speed": speed, "status": status, "error": errMsg, "item_id": itemID, "raw_response": raw,
		})
	}
	writeJSON(w, map[string]any{"total": total, "page": page, "page_size": size, "list": list})
}

// handleTaskQuality 质量抽查结果 + 人工标记
func (s *Server) handleTaskQuality(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	t, err := s.st.GetTask(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "任务不存在")
		return
	}
	runNo := t.RunNo
	if rn := queryInt(r, "run_no"); rn > 0 {
		runNo = int(rn)
	}
	rows, err := s.st.DB().Query(`SELECT qa.id,qa.question_id,qa.question_title,qa.answer_text,qa.auto_flag,qa.auto_reason,
		qa.manual_flag,qa.manual_note,COALESCE(q.qtype,''),COALESCE(q.category,''),
		COALESCE(qa.judge_score,''),COALESCE(qa.judge_reason,''),COALESCE(qa.artifact_path,''),COALESCE(qa.artifact_ok,0),
		COALESCE(NULLIF(qa.artifact_type,''), q.artifact_type, 'none') /* 优先运行时快照,老数据回退题库当前值 */
		FROM quality_answers qa LEFT JOIN questions q ON q.id=qa.question_id
		WHERE qa.task_id=? AND qa.run_no=? ORDER BY qa.id`, id, runNo)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	var list []map[string]any
	for rows.Next() {
		var qid, questionID int64
		var title, answer, auto, reason, manual, note, qtype string
		var category, judgeScore, judgeReason, artifactPath, artifactType string
		var artifactOK int
		rows.Scan(&qid, &questionID, &title, &answer, &auto, &reason, &manual, &note, &qtype,
			&category, &judgeScore, &judgeReason, &artifactPath, &artifactOK, &artifactType)
		list = append(list, map[string]any{
			"id": qid, "question_id": questionID, "question_title": title, "qtype": qtype,
			"answer_text": answer, "auto_flag": auto, "auto_reason": reason,
			"manual_flag": manual, "manual_note": note,
			"category": category, "judge_score": judgeScore, "judge_reason": judgeReason,
			"artifact_path": artifactPath, "artifact_ok": artifactOK != 0, "artifact_type": artifactType,
		})
	}
	verdict, total, abnormal, suspect := stats.QualityVerdict(s.st, id, int64(runNo))
	writeJSON(w, map[string]any{
		"run_no": runNo, "list": list, "verdict": verdict, "total": total, "abnormal": abnormal, "suspect": suspect,
	})
}

func (s *Server) handleMarkQuality(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ManualFlag string `json:"manual_flag"` // normal/abnormal/""(清除)
		ManualNote string `json:"manual_note"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if body.ManualFlag != "" && body.ManualFlag != "normal" && body.ManualFlag != "abnormal" {
		writeErr(w, http.StatusBadRequest, "标记须为 normal/abnormal")
		return
	}
	if err := s.st.MarkQualityAnswer(pathID(r, "id"), body.ManualFlag, body.ManualNote); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// handleTaskLogs 运行日志(F-5.6):增量拉取 + 下载
func (s *Server) handleTaskLogs(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	if r.URL.Query().Get("download") == "1" {
		rows, err := s.st.DB().Query(`SELECT ts,level,message FROM task_logs WHERE task_id=? ORDER BY id`, id)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()
		var b strings.Builder
		for rows.Next() {
			var ts int64
			var level, msg string
			rows.Scan(&ts, &level, &msg)
			b.WriteString(fmt.Sprintf("[%s] [%s] %s\n", time.UnixMilli(ts).Format("2006-01-02 15:04:05.000"), level, msg))
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=task-%d.log", id))
		w.Write([]byte(b.String()))
		return
	}
	afterID := queryInt(r, "after_id")
	rows, err := s.st.DB().Query(`SELECT id,ts,level,message FROM task_logs WHERE task_id=? AND id>? ORDER BY id LIMIT 500`, id, afterID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	var list []map[string]any
	for rows.Next() {
		var lid, ts int64
		var level, msg string
		rows.Scan(&lid, &ts, &level, &msg)
		list = append(list, map[string]any{"id": lid, "ts": ts, "level": level, "message": msg})
	}
	writeJSON(w, map[string]any{"list": list})
}

// handleTaskEvents SSE 实时进度(§7.1,断线前端退化 2s 轮询)
func (s *Server) handleTaskEvents(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "不支持 SSE")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// 先推送当前状态快照
	if t, err := s.st.GetTask(id); err == nil {
		fmt.Fprintf(w, "event: snapshot\ndata: {\"status\":%q,\"progress\":%d,\"item\":%q}\n\n", t.Status, t.Progress, t.CurrentItem)
		flusher.Flush()
	}
	ch := s.sched.Events().Subscribe(id)
	defer s.sched.Events().Unsubscribe(id, ch)
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case ev := <-ch:
			writeSSE(w, ev)
			flusher.Flush()
		case <-heartbeat.C:
			fmt.Fprintf(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

func writeSSE(w http.ResponseWriter, ev runner.Event) {
	var b strings.Builder
	b.WriteString("event: " + ev.Type + "\n")
	fmt.Fprintf(w, "%s", b.String())
	fmt.Fprintf(w, "data: {\"type\":%q,\"message\":%q,\"level\":%q,\"progress\":%d,\"status\":%q,\"item\":%q,\"ts\":%d}\n\n",
		ev.Type, ev.Message, ev.Level, ev.Progress, ev.Status, ev.Item, ev.TS)
}

// handleTaskDigest 智能体视角的一页式任务摘要(V2.9):一次调用拿全任务最终结论
func (s *Server) handleTaskDigest(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	t, err := s.st.GetTask(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "任务不存在")
		return
	}
	runNo := int64(t.RunNo)
	items, _ := s.st.ListTestItems(id, t.RunNo)
	itemList := []map[string]any{}
	for _, it := range items {
		itemList = append(itemList, map[string]any{"type": it.Type, "status": it.Status, "error": it.Error})
	}
	verdict, qaTotal, qaAbnormal, qaSuspect := stats.QualityVerdict(s.st, id, runNo)
	// 产物清单
	artifacts := []map[string]any{}
	arows, _ := s.st.DB().Query(`SELECT question_title, artifact_path FROM quality_answers WHERE task_id=? AND run_no=? AND artifact_path<>''`, id, runNo)
	if arows != nil {
		for arows.Next() {
			var title, path string
			arows.Scan(&title, &path)
			artifacts = append(artifacts, map[string]any{"question": title, "url": "/api/artifacts/" + path})
		}
		arows.Close()
	}
	writeJSON(w, map[string]any{
		"task_id": t.ID, "project_id": t.ProjectID, "model": t.ModelName, "quant": t.Quant,
		"status": t.Status, "run_no": t.RunNo, "error": t.Error, "progress": t.Progress,
		"started_at": t.StartedAt, "finished_at": t.FinishedAt,
		"summary": stats.Summary(s.st, id), "items": itemList,
		"quality": map[string]any{"verdict": verdict, "total": qaTotal, "abnormal": qaAbnormal, "suspect": qaSuspect},
		"artifacts": artifacts,
	})
}
