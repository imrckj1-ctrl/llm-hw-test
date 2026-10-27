package httpd

import (
	"encoding/json"
	"net/http"
	"llmhwtest/internal/exporter"
	"llmhwtest/internal/llmclient"
	"llmhwtest/internal/store"
)

// ---------- 项目(§5.4) ----------

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	f := store.ProjectFilter{
		DeviceID: queryInt(r, "device_id"),
		Tester:   queryStr(r, "tester"),
		Name:     queryStr(r, "name"),
		Status:   queryStr(r, "status"),
		FromMs:   queryInt(r, "from"),
		ToMs:     queryInt(r, "to"),
		Progress: queryStr(r, "progress"),
		Hidden:   queryStr(r, "hidden") == "1", // V3.0.1:默认只列未隐藏;hidden=1 只列已隐藏
	}
	list, err := s.st.ListProjects(f)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, list)
}

// 创建项目 = 设备 + 测试人员 + 备注 + 一组模型任务(F-4.1)
func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Type     string            `json:"type"` // hardware/model(V2.2)
		DeviceID int64              `json:"device_id"`
		Hw       map[string]any     `json:"hw"` // 本地硬件信息快照(V2.2)
		Name     string             `json:"name"`
		Tester   string             `json:"tester"`
		Note     string             `json:"note"`
		Tasks    []taskPayload      `json:"tasks"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if body.Type == "" {
		if body.DeviceID > 0 {
			body.Type = "hardware"
		} else {
			body.Type = "model"
		}
	}
	hwJSON := ""
	var devName string
	if body.Type == "hardware" {
		if body.DeviceID > 0 {
			// 老路径:引用已登记设备
			dev, err := s.st.GetDevice(body.DeviceID)
			if err != nil {
				writeErr(w, http.StatusBadRequest, "设备不存在")
				return
			}
			if dev.Archived {
				writeErr(w, http.StatusBadRequest, "设备已归档,不可新建项目(F-2.4)")
				return
			}
			devName = dev.Name
		} else {
			// 新路径:硬件信息内联快照
			b, _ := json.Marshal(body.Hw)
			hwJSON = string(b)
			if v, ok := body.Hw["name"].(string); ok && v != "" {
				devName = v
			}
		}
	}
	p := &store.Project{Type: body.Type, DeviceID: body.DeviceID, HwJSON: hwJSON, Name: body.Name, Tester: body.Tester, Note: body.Note}
	if p.Name == "" {
		if devName != "" {
			p.Name = devName + " 评估"
		} else {
			p.Name = "模型能力测试"
		}
	}
	if err := s.st.CreateProject(p); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	for i := range body.Tasks {
		if err := s.createTaskFromPayload(p, &body.Tasks[i]); err != nil {
			// 任务配置错误时回滚已建项目(级联删除已建任务),不留孤儿项目
			s.st.DeleteProject(p.ID)
			writeErr(w, http.StatusBadRequest, "任务配置错误: "+err.Error())
			return
		}
	}
	writeJSON(w, p)
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	p, err := s.st.GetProject(pathID(r, "id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "项目不存在")
		return
	}
	tasks, _ := s.st.ListTasks(p.ID, 0, "")
	writeJSON(w, map[string]any{"project": p, "tasks": tasks})
}

func (s *Server) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	p, err := s.st.GetProject(pathID(r, "id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "项目不存在")
		return
	}
	var body struct {
		Name   string         `json:"name"`
		Tester string         `json:"tester"`
		Note   string         `json:"note"`
		Hw     map[string]any `json:"hw"` // V3.0.1:编辑任务可改硬件信息快照;缺省不修改
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	// A3:全部任务「已完成」后项目锁定,不可再改
	if p.Status == "done" {
		writeErr(w, http.StatusBadRequest, "项目已全部完结,信息锁定不可修改")
		return
	}
	p.Name, p.Tester, p.Note = body.Name, body.Tester, body.Note
	if body.Hw != nil {
		b, _ := json.Marshal(body.Hw)
		p.HwJSON = string(b)
	}
	if err := s.st.UpdateProject(p); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	if err := s.st.DeleteProject(pathID(r, "id")); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// handleSetProjectHidden 隐藏/恢复项目(V3.0.1):隐藏≠删除,数据完整保留;
// 隐藏后项目列表/对比分析/首页不可见。执行中的项目须先暂停或取消才允许隐藏。
func (s *Server) handleSetProjectHidden(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	p, err := s.st.GetProject(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "项目不存在")
		return
	}
	var body struct {
		Hidden bool `json:"hidden"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if body.Hidden && (p.Status == "running" || p.Status == "queued") {
		writeErr(w, http.StatusBadRequest, "项目正在执行/排队,请先暂停或取消后再隐藏")
		return
	}
	if err := s.st.SetProjectHidden(id, body.Hidden); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleStartProject(w http.ResponseWriter, r *http.Request) {
	n, err := s.sched.EnqueueProject(pathID(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true, "enqueued": n})
}

// handleExportProjectDocx 正式 Word 测试报告(V2.11)
func (s *Server) handleExportProjectDocx(w http.ResponseWriter, r *http.Request) {
	buf, name, err := exporter.ProjectReportDOCX(s.st, s.dataDir, pathID(r, "id"), PlatformVersion)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+urlEscape(name))
	w.Write(buf.Bytes())
}

// ---------- 任务(§5.5) ----------

type taskPayload struct {
	APIBaseURL      string             `json:"api_base_url"`
	ModelName       string             `json:"model_name"`
	ModelHFID       string             `json:"model_hf_id"`
	ModelMSID       string             `json:"model_ms_id"`
	Quant           string             `json:"quant"`
	ContextLen      int                `json:"context_len"`
	CacheEnabled    string             `json:"cache_enabled"`
	CacheNote       string             `json:"cache_note"`
	APIKey          string             `json:"api_key"`       // 明文仅传输,入库即加密
	APIKeyClear     bool               `json:"api_key_clear"` // 编辑时显式清除密钥
	CorpusVersionID int64              `json:"corpus_version_id"`
	Params          *store.TaskParams  `json:"params"`
}

func (s *Server) createTaskFromPayload(p *store.Project, tp *taskPayload) error {
	if tp.APIBaseURL == "" || tp.ModelName == "" {
		return errString("API 地址与模型名称必填")
	}
	params := store.DefaultTaskParams()
	if tp.Params != nil {
		params = *tp.Params
	}
	cvID := tp.CorpusVersionID
	if cvID <= 0 {
		// V2.6:激活概念下线,未选版本时回退到最新版
		if latest, err := s.st.LatestCorpusVersion(); err == nil {
			cvID = latest.ID
		}
	}
	cacheEnabled := tp.CacheEnabled
	if cacheEnabled == "" {
		cacheEnabled = "unknown"
	}
	keyEnc := ""
	if tp.APIKey != "" {
		var err error
		keyEnc, err = s.st.EncryptSecret(tp.APIKey)
		if err != nil {
			return err
		}
	}
	t := &store.Task{
		ProjectID: p.ID, DeviceID: p.DeviceID,
		APIBaseURL: tp.APIBaseURL, ModelName: tp.ModelName, ModelHFID: tp.ModelHFID, ModelMSID: tp.ModelMSID,
		Quant: tp.Quant, ContextLen: tp.ContextLen, CacheEnabled: cacheEnabled, CacheNote: tp.CacheNote,
		APIKeyEnc: keyEnc, ParamsJSON: params.JSON(), CorpusVersionID: cvID,
	}
	return s.st.CreateTask(t)
}

type errString string

func (e errString) Error() string { return string(e) }

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	p, err := s.st.GetProject(pathID(r, "id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "项目不存在")
		return
	}
	var tp taskPayload
	if err := decodeJSON(r, &tp); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if err := s.createTaskFromPayload(p, &tp); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	tasks, _ := s.st.ListTasks(p.ID, 0, "")
	writeJSON(w, tasks[len(tasks)-1])
}

func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	list, err := s.st.ListTasks(queryInt(r, "project_id"), queryInt(r, "device_id"), queryStr(r, "status"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, list)
}

func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	t, err := s.st.GetTask(pathID(r, "id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "任务不存在")
		return
	}
	writeJSON(w, t)
}

func (s *Server) handleUpdateTask(w http.ResponseWriter, r *http.Request) {
	t, err := s.st.GetTask(pathID(r, "id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "任务不存在")
		return
	}
	var tp taskPayload
	if err := decodeJSON(r, &tp); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if tp.APIBaseURL != "" {
		t.APIBaseURL = tp.APIBaseURL
	}
	if tp.ModelName != "" {
		t.ModelName = tp.ModelName
	}
	t.ModelHFID, t.ModelMSID = tp.ModelHFID, tp.ModelMSID
	t.Quant = tp.Quant
	t.ContextLen = tp.ContextLen
	if tp.CacheEnabled != "" {
		t.CacheEnabled = tp.CacheEnabled
	}
	t.CacheNote = tp.CacheNote
	if tp.Params != nil {
		t.ParamsJSON = tp.Params.JSON()
	}
	if tp.CorpusVersionID > 0 {
		t.CorpusVersionID = tp.CorpusVersionID
	}
	// API Key:留空不修改;api_key_clear 显式清除
	if tp.APIKeyClear {
		t.APIKeyEnc = ""
	} else if tp.APIKey != "" {
		enc, err := s.st.EncryptSecret(tp.APIKey)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		t.APIKeyEnc = enc
	}
	if err := s.st.UpdateTaskConfig(t); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleDeleteTask(w http.ResponseWriter, r *http.Request) {
	if err := s.st.DeleteTask(pathID(r, "id")); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleStartTask(w http.ResponseWriter, r *http.Request) {
	if err := s.sched.EnqueueTask(pathID(r, "id")); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleCancelTask(w http.ResponseWriter, r *http.Request) {
	if err := s.sched.CancelTask(pathID(r, "id")); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleRerunItem(w http.ResponseWriter, r *http.Request) {
	if err := s.sched.RerunItem(pathID(r, "id"), pathID(r, "itemId")); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// 复制为新任务(A3 + V2.4):已完成任务配置锁定;复制 = 新项目壳 + 待执行新任务,含加密的 API Key
func (s *Server) handleCloneTask(w http.ResponseWriter, r *http.Request) {
	t, err := s.st.GetTask(pathID(r, "id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "任务不存在")
		return
	}
	// V2.4 项目即任务:复制需新建项目壳,保持一项目一任务
	src, _ := s.st.GetProject(t.ProjectID)
	np := &store.Project{Type: "model", Name: t.ModelName + " 测试", Tester: "", Note: ""}
	if src != nil {
		np.Type = src.Type
		np.DeviceID = src.DeviceID // 保留设备关联(修复克隆丢设备)
		np.HwJSON = src.HwJSON
		np.Name = src.Name + " 副本"
		np.Tester = src.Tester
		np.Note = src.Note
	}
	if err := s.st.CreateProject(np); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	nt := &store.Task{
		ProjectID: np.ID, DeviceID: t.DeviceID,
		APIBaseURL: t.APIBaseURL, ModelName: t.ModelName, ModelHFID: t.ModelHFID, ModelMSID: t.ModelMSID,
		Quant: t.Quant, ContextLen: t.ContextLen, CacheEnabled: t.CacheEnabled, CacheNote: t.CacheNote,
		APIKeyEnc: t.APIKeyEnc, ParamsJSON: t.ParamsJSON, CorpusVersionID: t.CorpusVersionID,
	}
	if err := s.st.CreateTask(nt); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	nt.APIKeySet = nt.APIKeyEnc != ""
	writeJSON(w, nt)
}

// 继续跑(修改清单-1):未完成测试项继续执行
func (s *Server) handleResumeTask(w http.ResponseWriter, r *http.Request) {
	if err := s.sched.ResumeTask(pathID(r, "id")); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// 暂停任务(V2.19):执行中→落「已暂停」释放名额;排队中→出队;点「继续跑」断点续跑
func (s *Server) handlePauseTask(w http.ResponseWriter, r *http.Request) {
	if err := s.sched.PauseTask(pathID(r, "id")); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// 跳过当前测试项(V2.19):该项标「已跳过」,任务继续下一项;之后可单项补测
func (s *Server) handleSkipItem(w http.ResponseWriter, r *http.Request) {
	if err := s.sched.SkipItem(pathID(r, "id")); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// 按项跳过(V2.19 测试项级控制):执行中的项中止本项;暂停/待执行时直接标记跳过,可补测
func (s *Server) handleSkipItemByID(w http.ResponseWriter, r *http.Request) {
	if err := s.sched.SkipItemByID(pathID(r, "id"), pathID(r, "itemId")); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// 直建任务(修改清单-5):无需先建项目,后端自动生成项目壳;备注必填
func (s *Server) handleDirectTask(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Type   string         `json:"type"`
		Name   string         `json:"name"` // V2.6:用户命名,原样作为项目名
		Tester string         `json:"tester"`
		Note   string         `json:"note"`
		Hw     map[string]any `json:"hw"`
		Task   taskPayload    `json:"task"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if body.Note == "" {
		writeErr(w, http.StatusBadRequest, "备注必填:请写明本次测试目的(测什么能力/什么场景)")
		return
	}
	if body.Name == "" {
		writeErr(w, http.StatusBadRequest, "项目名称必填")
		return
	}
	if body.Type == "" {
		body.Type = "model"
	}
	hwJSON := ""
	if body.Type == "hardware" && len(body.Hw) > 0 {
		b, _ := json.Marshal(body.Hw)
		hwJSON = string(b)
	}
	// V2.6:项目名 = 用户填写,原样显示;模型名在列表单独一列
	p := &store.Project{Type: body.Type, HwJSON: hwJSON, Name: body.Name, Tester: body.Tester, Note: body.Note}
	if err := s.st.CreateProject(p); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.createTaskFromPayload(p, &body.Task); err != nil {
		writeErr(w, http.StatusBadRequest, "任务配置错误: "+err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true, "project_id": p.ID})
}

// 查询模型(OpenAI 标准 GET /v1/models,自动带 Bearer)
func (s *Server) handleFetchModels(w http.ResponseWriter, r *http.Request) {
	var body struct {
		APIBaseURL string `json:"api_base_url"`
		APIKey     string `json:"api_key"`
		TaskID     int64  `json:"task_id"` // 编辑场景:用已存密钥
	}
	if err := decodeJSON(r, &body); err != nil || body.APIBaseURL == "" {
		writeErr(w, http.StatusBadRequest, "请填写 API 地址")
		return
	}
	key := body.APIKey
	if key == "" && body.TaskID > 0 {
		if t, err := s.st.GetTask(body.TaskID); err == nil {
			key, _ = s.st.DecryptSecret(t.APIKeyEnc)
		}
	}
	ids, err := llmclient.New().ListModels(r.Context(), body.APIBaseURL, key)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "查询失败: "+err.Error())
		return
	}
	writeJSON(w, map[string]any{"models": ids})
}
