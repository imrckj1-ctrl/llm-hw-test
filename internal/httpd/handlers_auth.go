package httpd

import (
	"encoding/json"
	"math"
	"net/http"
	"llmhwtest/internal/auth"
)

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	token, remain, err := s.au.Login(body.Password)
	if err != nil {
		if remain > 0 {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]any{"error": err.Error(), "locked": true, "remain_sec": int(math.Ceil(remain.Seconds()))})
			return
		}
		writeErr(w, http.StatusUnauthorized, err.Error())
		return
	}
	auth.SetCookie(w, token)
	writeJSON(w, map[string]any{"ok": true, "force_password_change": s.au.ForceChange()})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.au.Logout(auth.TokenFrom(r))
	auth.ClearCookie(w)
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"ok": true, "force_password_change": s.au.ForceChange()})
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if err := s.au.ChangePassword(body.OldPassword, body.NewPassword); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	auth.ClearCookie(w) // 旧会话全部失效(F-1.3)
	writeJSON(w, map[string]any{"ok": true})
}
