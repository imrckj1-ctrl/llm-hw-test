package httpd

import (
	"net/http"
	"strconv"
	"github.com/go-chi/chi/v5"
	"llmhwtest/internal/store"
)

// ---------- 语料(§5.3) ----------

func (s *Server) handleListCorpusVersions(w http.ResponseWriter, r *http.Request) {
	list, err := s.st.ListCorpusVersions()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// 附带每版本各档字数
	type versionWithTexts struct {
		*store.CorpusVersion
		Texts []*store.CorpusText `json:"texts"`
	}
	var out []versionWithTexts
	for _, v := range list {
		texts, _ := s.st.CorpusTexts(v.ID, false)
		out = append(out, versionWithTexts{v, texts})
	}
	writeJSON(w, out)
}

// 创建版本:可一次提交多档;未提交档从基准版本(默认当前激活)复制(F-3.2)
func (s *Server) handleCreateCorpusVersion(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Uploader      string            `json:"uploader"`
		Note          string            `json:"note"`
		BaseVersionID int64             `json:"base_version_id"`
		Texts         map[string]string `json:"texts"` // tier → content
		Activate      bool              `json:"activate"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if len(body.Texts) == 0 && body.BaseVersionID <= 0 {
		writeErr(w, http.StatusBadRequest, "至少提交一个档位文本")
		return
	}
	if body.Note == "" {
		writeErr(w, http.StatusBadRequest, "备注必填(含约 200 字来源说明,附录 B)")
		return
	}
	v := &store.CorpusVersion{
		VersionNo: s.st.NextCorpusVersionNo(),
		Uploader:  body.Uploader,
		Note:      body.Note,
	}
	if err := s.st.CreateCorpusVersion(v); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// 复制基准档位
	baseID := body.BaseVersionID
	if baseID <= 0 {
		// V2.6:激活概念下线,未指定基准时从最新版复制
		if latest, err := s.st.LatestCorpusVersion(); err == nil {
			baseID = latest.ID
		}
	}
	if baseID > 0 {
		baseTexts, _ := s.st.CorpusTexts(baseID, true)
		for _, bt := range baseTexts {
			if _, overridden := body.Texts[bt.Tier]; !overridden {
				s.st.UpsertCorpusText(v.ID, bt.Tier, bt.Content)
			}
		}
	}
	for tier, content := range body.Texts {
		if !validTier(tier) {
			continue
		}
		s.st.UpsertCorpusText(v.ID, tier, content)
	}
	if body.Activate {
		s.st.ActivateCorpusVersion(v.ID)
	}
	writeJSON(w, v)
}

func (s *Server) handleGetCorpusTexts(w http.ResponseWriter, r *http.Request) {
	withContent := r.URL.Query().Get("content") == "1"
	texts, err := s.st.CorpusTexts(pathID(r, "id"), withContent)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, texts)
}

func (s *Server) handleActivateCorpus(w http.ResponseWriter, r *http.Request) {
	if err := s.st.ActivateCorpusVersion(pathID(r, "id")); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// 修改版本备注/上传人(修改清单-7)
func (s *Server) handleUpdateCorpusVersion(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Uploader string `json:"uploader"`
		Note     string `json:"note"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if body.Note == "" {
		writeErr(w, http.StatusBadRequest, "备注不能为空")
		return
	}
	if err := s.st.UpdateCorpusVersion(pathID(r, "id"), body.Uploader, body.Note); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// 删除版本(带引用/激活保护,修改清单-7)
func (s *Server) handleDeleteCorpusVersion(w http.ResponseWriter, r *http.Request) {
	if err := s.st.DeleteCorpusVersion(pathID(r, "id")); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// 替换档位 = 生成新版本(F-3.2):以 :id 为基准复制其余档
func (s *Server) handleReplaceCorpusTier(w http.ResponseWriter, r *http.Request) {
	baseID := pathID(r, "id")
	tier := chi.URLParam(r, "tier")
	if !validTier(tier) {
		writeErr(w, http.StatusBadRequest, "档位须为 数字+K(可含小数),如 1K/50K/2.5K")
		return
	}
	var body struct {
		Content  string `json:"content"`
		Uploader string `json:"uploader"`
		Note     string `json:"note"`
	}
	if err := decodeJSON(r, &body); err != nil || body.Content == "" {
		writeErr(w, http.StatusBadRequest, "文本内容不能为空")
		return
	}
	base, err := s.st.GetCorpusVersion(baseID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "基准版本不存在")
		return
	}
	note := body.Note
	if note == "" {
		note = base.Note + "(替换 " + tier + " 档)"
	}
	v := &store.CorpusVersion{VersionNo: s.st.NextCorpusVersionNo(), Uploader: body.Uploader, Note: note}
	if err := s.st.CreateCorpusVersion(v); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	baseTexts, _ := s.st.CorpusTexts(baseID, true)
	for _, bt := range baseTexts {
		if bt.Tier != tier {
			s.st.UpsertCorpusText(v.ID, bt.Tier, bt.Content)
		}
	}
	s.st.UpsertCorpusText(v.ID, tier, body.Content)
	writeJSON(w, v)
}

// 档位支持自定义(任意 数字+K/M 格式,如 1K/50K/2.5K/1M;V2.5 放开小数档,V2.7 放开 M 档)
func validTier(t string) bool {
	if len(t) < 2 || len(t) > 7 {
		return false
	}
	suffix := t[len(t)-1]
	if suffix != 'K' && suffix != 'M' {
		return false
	}
	f, err := strconv.ParseFloat(t[:len(t)-1], 64)
	return err == nil && f > 0
}

// ---------- 题库(F-3.6) ----------

func (s *Server) handleListQuestions(w http.ResponseWriter, r *http.Request) {
	list, err := s.st.ListQuestions(false)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, list)
}

func (s *Server) handleCreateQuestion(w http.ResponseWriter, r *http.Request) {
	var q store.Question
	if err := decodeJSON(r, &q); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if q.Prompt == "" || !validQType(q.QType) {
		writeErr(w, http.StatusBadRequest, "类型(常识/推理/长文摘要)与题干必填")
		return
	}
	if err := s.st.CreateQuestion(&q); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, &q)
}

func (s *Server) handleUpdateQuestion(w http.ResponseWriter, r *http.Request) {
	var q store.Question
	if err := decodeJSON(r, &q); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	q.ID = pathID(r, "id")
	if err := s.st.UpdateQuestion(&q); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleDeleteQuestion(w http.ResponseWriter, r *http.Request) {
	if err := s.st.DeleteQuestion(pathID(r, "id")); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func validQType(t string) bool {
	switch t {
	case "common", "reasoning", "summary":
		return true
	}
	return false
}
