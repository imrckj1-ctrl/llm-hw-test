package httpd

import (
	"net/http"

	"llmhwtest/internal/store"
)

// ---------- 题组(V2.4:自定义编组,全局共享;语料与题库页维护,建任务按组选题) ----------

func (s *Server) handleListQuestionGroups(w http.ResponseWriter, r *http.Request) {
	list, err := s.st.ListQuestionGroups()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []*store.QuestionGroup{}
	}
	writeJSON(w, list)
}

type groupPayload struct {
	Name        string  `json:"name"`
	Note        string  `json:"note"`
	QuestionIDs []int64 `json:"question_ids"`
}

func (s *Server) handleCreateQuestionGroup(w http.ResponseWriter, r *http.Request) {
	var p groupPayload
	if err := decodeJSON(r, &p); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if p.Name == "" {
		writeErr(w, http.StatusBadRequest, "题组名称必填")
		return
	}
	if len(p.QuestionIDs) == 0 {
		writeErr(w, http.StatusBadRequest, "题组至少包含 1 道题")
		return
	}
	g := &store.QuestionGroup{Name: p.Name, Note: p.Note, QuestionIDs: p.QuestionIDs}
	if err := s.st.CreateQuestionGroup(g); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, g)
}

func (s *Server) handleUpdateQuestionGroup(w http.ResponseWriter, r *http.Request) {
	var p groupPayload
	if err := decodeJSON(r, &p); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if p.Name == "" {
		writeErr(w, http.StatusBadRequest, "题组名称必填")
		return
	}
	if len(p.QuestionIDs) == 0 {
		writeErr(w, http.StatusBadRequest, "题组至少包含 1 道题")
		return
	}
	g := &store.QuestionGroup{ID: pathID(r, "id"), Name: p.Name, Note: p.Note, QuestionIDs: p.QuestionIDs}
	if err := s.st.UpdateQuestionGroup(g); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleDeleteQuestionGroup(w http.ResponseWriter, r *http.Request) {
	if err := s.st.DeleteQuestionGroup(pathID(r, "id")); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}
