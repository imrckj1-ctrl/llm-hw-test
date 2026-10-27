package httpd

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
	"llmhwtest/internal/pisandbox"
	"llmhwtest/internal/store"
)

// ---------- 系统设置(§5.11) ----------

// sandboxStatus pi 沙箱状态;docker 已装但镜像缺失时自动后台构建(V2.23,无手动中间态)
func sandboxStatus() pisandbox.Status {
	st := pisandbox.Detect()
	if st.Docker && !st.Image {
		pisandbox.EnsureBuiltAsync()
		st.Building = true
	}
	return st
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	parallel, _ := s.st.GetSetting("parallel_limit")
	defParams := store.DefaultTaskParams().JSON()
	if v, ok := s.st.GetSetting("default_task_params"); ok {
		defParams = v
	}
	baseline, _ := s.st.GetSetting("baseline_wait_sec")
	judgeURL, _ := s.st.GetSetting("judge_api_url")
	judgeModel, _ := s.st.GetSetting("judge_model")
	_, judgeKeySet := s.st.GetSetting("judge_key_enc")
	qualityEngine, ok := s.st.GetSetting("quality_engine")
	if !ok {
		qualityEngine = "pi"
	}
	piContainer, _ := s.st.GetSetting("pi_container")
	if piContainer == "" {
		piContainer = "auto"
	}
	tokenSet, tokenCreated := s.au.APITokenInfo()
	writeJSON(w, map[string]any{
		"parallel_limit":      parallel,
		"default_task_params": defParams,
		"baseline_wait_sec":   baseline,
		"judge_api_url":       judgeURL,
		"judge_model":         judgeModel,
		"judge_key_set":       judgeKeySet,
		"quality_engine":      qualityEngine,
		"pi_container":        piContainer,
		"pi_sandbox":          sandboxStatus(),
		"platform_version":    PlatformVersion,
		"pi_version":          readPkgVersion(s.dataDir, "pi-ai"),
		"pi_goal_version":     readPkgVersion(s.dataDir, "pi-goal"),
		"api_token_set":       tokenSet,
		"api_token_created":   tokenCreated,
	})
}

// PlatformVersion 平台版本(可用 -ldflags "-X llmhwtest/internal/httpd.PlatformVersion=vX" 注入)
var PlatformVersion = "v3.0.2"

// readPkgVersion 读 scripts/pi/node_modules 里包的版本(随可执行文件同目录)
func readPkgVersion(dataDir, shortName string) string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	base := filepath.Dir(exe)
	var pkg string
	switch shortName {
	case "pi-ai":
		pkg = filepath.Join(base, "scripts", "pi", "node_modules", "@earendil-works", "pi-agent-core", "package.json")
	case "pi-goal":
		pkg = filepath.Join(base, "scripts", "pi", "node_modules", "@narumitw", "pi-goal", "package.json")
	default:
		return ""
	}
	b, err := os.ReadFile(pkg)
	if err != nil {
		return ""
	}
	var meta struct {
		Version string `json:"version"`
	}
	if json.Unmarshal(b, &meta) != nil {
		return ""
	}
	return meta.Version
}

// handleGenAPIToken 生成长期 API Token(明文仅此一次返回)
func (s *Server) handleGenAPIToken(w http.ResponseWriter, r *http.Request) {
	tok, err := s.au.GenerateAPIToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true, "token": tok, "note": "请立即保存,token 只显示这一次"})
}

// handleRevokeAPIToken 吊销长期 API Token
func (s *Server) handleRevokeAPIToken(w http.ResponseWriter, r *http.Request) {
	if err := s.au.RevokeAPIToken(); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// handleSkillZip 下载智能体技能包(V2.24):SKILL.md + hwtest-cli.mjs 打成 zip
func (s *Server) handleSkillZip(w http.ResponseWriter, r *http.Request) {
	exe, err := os.Executable()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	dir := filepath.Join(filepath.Dir(exe), "skills", "hwtest-platform")
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	n := 0
	err = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		f, err := zw.Create(filepath.ToSlash(rel))
		if err != nil {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		f.Write(data)
		n++
		return nil
	})
	zw.Close()
	if err != nil || n == 0 {
		writeErr(w, http.StatusNotFound, "技能包不存在(部署目录缺 skills/hwtest-platform)")
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=hwtest-platform-skill.zip")
	w.Write(buf.Bytes())
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ParallelLimit     string `json:"parallel_limit"`
		DefaultTaskParams string `json:"default_task_params"`
		BaselineWaitSec   string `json:"baseline_wait_sec"`
		JudgeAPIURL       string `json:"judge_api_url"`
		JudgeModel        string `json:"judge_model"`
		JudgeAPIKey       string `json:"judge_api_key"` // 留空不修改;"-"清除
		QualityEngine     string `json:"quality_engine"` // pi(智能体)| direct(直接问答)
		PIContainer       string `json:"pi_container"`   // auto|on|off(pi 沙箱)
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if body.ParallelLimit != "" {
		var n int
		if _, err := fmt.Sscanf(body.ParallelLimit, "%d", &n); err != nil || n < 1 || n > 32 {
			writeErr(w, http.StatusBadRequest, "并行上限须为 1-32")
			return
		}
		s.st.SetSetting("parallel_limit", body.ParallelLimit)
	}
	if body.DefaultTaskParams != "" {
		// 校验 JSON 可解析为 TaskParams
		p := store.ParseTaskParams(body.DefaultTaskParams)
		s.st.SetSetting("default_task_params", p.JSON())
	}
	if body.BaselineWaitSec != "" {
		s.st.SetSetting("baseline_wait_sec", body.BaselineWaitSec)
	}
	if body.QualityEngine != "" {
		if body.QualityEngine != "pi" && body.QualityEngine != "direct" {
			writeErr(w, http.StatusBadRequest, "质量题执行方式仅支持 pi / direct")
			return
		}
		s.st.SetSetting("quality_engine", body.QualityEngine)
	}
	if body.PIContainer != "" {
		if body.PIContainer != "auto" && body.PIContainer != "on" && body.PIContainer != "off" {
			writeErr(w, http.StatusBadRequest, "pi 沙箱仅支持 auto / on / off")
			return
		}
		s.st.SetSetting("pi_container", body.PIContainer)
	}
	if body.JudgeAPIURL != "" {
		s.st.SetSetting("judge_api_url", body.JudgeAPIURL)
	}
	if body.JudgeModel != "" {
		s.st.SetSetting("judge_model", body.JudgeModel)
	}
	if body.JudgeAPIKey == "-" {
		s.st.SetSetting("judge_key_enc", "")
	} else if body.JudgeAPIKey != "" {
		enc, err := s.st.EncryptSecret(body.JudgeAPIKey)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.st.SetSetting("judge_key_enc", enc)
	}
	writeJSON(w, map[string]any{"ok": true})
}

// handleBackup 一键下载 SQLite 一致性备份(§7.8:Online Backup / VACUUM INTO)
func (s *Server) handleBackup(w http.ResponseWriter, r *http.Request) {
	name := fmt.Sprintf("backup-%s.db", time.Now().Format("20060102-150405"))
	dest := filepath.Join(s.dataDir, name)
	if err := s.st.BackupTo(dest); err != nil {
		writeErr(w, http.StatusInternalServerError, "备份失败: "+err.Error())
		return
	}
	defer os.Remove(dest)
	f, err := os.Open(dest)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename="+name)
	st, _ := f.Stat()
	http.ServeContent(w, r, name, st.ModTime(), f)
}

// handleOverview 首页大屏数据总览
func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{}
	var devTotal, devArchived int
	s.st.QueryRow(`SELECT COUNT(*), COALESCE(SUM(archived),0) FROM devices`).Scan(&devTotal, &devArchived)
	out["device_total"] = devTotal
	out["device_archived"] = devArchived
	// 任务按状态计数
	taskCounts := map[string]int{}
	trows, _ := s.st.DB().Query(`SELECT status, COUNT(*) FROM tasks GROUP BY status`)
	if trows != nil {
		for trows.Next() {
			var st string
			var c int
			trows.Scan(&st, &c)
			taskCounts[st] = c
		}
		trows.Close()
	}
	out["task_counts"] = taskCounts
	// 项目派生状态计数
	projects, _ := s.st.ListProjects(store.ProjectFilter{})
	projCounts := map[string]int{}
	for _, p := range projects {
		projCounts[p.Status]++
	}
	out["project_total"] = len(projects)
	out["project_counts"] = projCounts
	// 进行中/排队任务实时列表
	live := []map[string]any{}
	lrows, _ := s.st.DB().Query(`SELECT t.id,t.project_id,t.model_name,t.quant,t.progress,t.current_item,t.status,
		COALESCE(d.name,''),t.run_no FROM tasks t LEFT JOIN devices d ON d.id=t.device_id
		LEFT JOIN projects p ON p.id=t.project_id
		WHERE t.status IN ('running','queued') AND COALESCE(p.hidden,0)=0 ORDER BY t.status='queued', t.id`)
	if lrows != nil {
		for lrows.Next() {
			var id, pid, progress, runNo int64
			var model, quant, item, status, devName string
			lrows.Scan(&id, &pid, &model, &quant, &progress, &item, &status, &devName, &runNo)
			live = append(live, map[string]any{
				"task_id": id, "project_id": pid, "model_name": model, "quant": quant,
				"progress": progress, "current_item": item, "status": status,
				"device_name": devName, "run_no": runNo,
			})
		}
		lrows.Close()
	}
	out["live"] = live
	// 近 7 天每日完成任务数
	trend := []map[string]any{}
	trows2, _ := s.st.DB().Query(`SELECT date(finished_at/1000,'unixepoch','+8 hours') AS d, COUNT(*)
		FROM tasks WHERE status='done' AND finished_at > 0
		GROUP BY d ORDER BY d DESC LIMIT 7`)
	if trows2 != nil {
		for trows2.Next() {
			var d string
			var c int
			trows2.Scan(&d, &c)
			trend = append(trend, map[string]any{"date": d, "count": c})
		}
		trows2.Close()
	}
	out["trend"] = trend
	// 最近完成任务(首页表格)
	recent := []map[string]any{}
	rrows, _ := s.st.DB().Query(`SELECT t.id,t.project_id,t.model_name,t.quant,COALESCE(d.name,'云端'),t.finished_at,
		COALESCE(t.started_at,0),COALESCE(p.name,'') FROM tasks t
		LEFT JOIN devices d ON d.id=t.device_id LEFT JOIN projects p ON p.id=t.project_id
		WHERE t.status='done' AND t.finished_at>0 AND COALESCE(p.hidden,0)=0 ORDER BY t.finished_at DESC, t.id DESC LIMIT 8`)
	if rrows != nil {
		for rrows.Next() {
			var id, pid, fin, start int64
			var model, quant, devName, projName string
			rrows.Scan(&id, &pid, &model, &quant, &devName, &fin, &start, &projName)
			recent = append(recent, map[string]any{
				"task_id": id, "project_id": pid, "model_name": model, "quant": quant,
				"device_name": devName, "project_name": projName, "finished_at": fin, "duration_sec": (fin - start) / 1000,
			})
		}
		rrows.Close()
	}
	out["recent_done"] = recent
	// 质量标注统计
	var qaTotal, qaAbnormal, qaSuspect int
	s.st.QueryRow(`SELECT COUNT(*),
		COALESCE(SUM(CASE WHEN COALESCE(NULLIF(manual_flag,''),auto_flag)='abnormal' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN COALESCE(NULLIF(manual_flag,''),auto_flag)='suspect' THEN 1 ELSE 0 END),0)
		FROM quality_answers`).Scan(&qaTotal, &qaAbnormal, &qaSuspect)
	out["quality_total"] = qaTotal
	out["quality_abnormal"] = qaAbnormal
	out["quality_suspect"] = qaSuspect
	writeJSON(w, out)
}
