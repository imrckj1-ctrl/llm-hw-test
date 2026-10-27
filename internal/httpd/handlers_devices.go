package httpd

import (
	"net/http"
	"llmhwtest/internal/store"
)

func (s *Server) handleListDevices(w http.ResponseWriter, r *http.Request) {
	all := r.URL.Query().Get("all") == "1"
	list, err := s.st.ListDevices(all)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, list)
}

type devicePayload struct {
	*store.Device
	SSHSecret string `json:"ssh_secret"` // 明文仅在传输中,入库即加密(F-2.2)
}

func (s *Server) resolveSecret(d *store.Device, plain string, keepID int64) (string, error) {
	if plain != "" {
		return s.st.EncryptSecret(plain)
	}
	if keepID > 0 {
		old, err := s.st.GetDevice(keepID)
		if err == nil {
			return old.SSHSecretEnc, nil
		}
	}
	return "", nil
}

func (s *Server) handleCreateDevice(w http.ResponseWriter, r *http.Request) {
	var p devicePayload
	if err := decodeJSON(r, &p); err != nil || p.Device == nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	d := p.Device
	if d.Name == "" {
		writeErr(w, http.StatusBadRequest, "设备名称必填")
		return
	}
	enc, err := s.resolveSecret(d, p.SSHSecret, 0)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	d.SSHSecretEnc = enc
	if err := s.st.CreateDevice(d); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, d)
}

func (s *Server) handleGetDevice(w http.ResponseWriter, r *http.Request) {
	d, err := s.st.GetDevice(pathID(r, "id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "设备不存在")
		return
	}
	writeJSON(w, d)
}

func (s *Server) handleUpdateDevice(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	old, err := s.st.GetDevice(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "设备不存在")
		return
	}
	var p devicePayload
	if err := decodeJSON(r, &p); err != nil || p.Device == nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	d := p.Device
	d.ID = id
	enc, err := s.resolveSecret(d, p.SSHSecret, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	d.SSHSecretEnc = enc
	d.CreatedAt = old.CreatedAt
	if err := s.st.UpdateDevice(d); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleDeleteDevice(w http.ResponseWriter, r *http.Request) {
	if err := s.st.DeleteDevice(pathID(r, "id")); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleArchiveDevice(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Archived bool `json:"archived"`
	}
	decodeJSON(r, &body)
	if err := s.st.ArchiveDevice(pathID(r, "id"), body.Archived); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}
