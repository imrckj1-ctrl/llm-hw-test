package httpd

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"github.com/go-chi/chi/v5"
	"llmhwtest/internal/store"
)

const maxSpecFileSize = 32 << 20 // 32MB

// 设备规格书/说明书附件:上传入库、列表、下载、删除

func (s *Server) handleListDeviceFiles(w http.ResponseWriter, r *http.Request) {
	list, err := s.st.ListDeviceFiles(pathID(r, "id"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, list)
}

func (s *Server) handleUploadDeviceFile(w http.ResponseWriter, r *http.Request) {
	deviceID := pathID(r, "id")
	if _, err := s.st.GetDevice(deviceID); err != nil {
		writeErr(w, http.StatusNotFound, "设备不存在")
		return
	}
	if err := r.ParseMultipartForm(maxSpecFileSize); err != nil {
		writeErr(w, http.StatusBadRequest, "表单解析失败: "+err.Error())
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "缺少文件字段 file")
		return
	}
	defer file.Close()
	if header.Size > maxSpecFileSize {
		writeErr(w, http.StatusBadRequest, "文件超过 32MB 限制")
		return
	}
	data, err := io.ReadAll(file)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	ct := header.Header.Get("Content-Type")
	id, err := s.st.AddDeviceFile(deviceID, header.Filename, ct, data)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, &store.DeviceFile{
		ID: id, DeviceID: deviceID, Filename: header.Filename,
		ContentType: ct, Size: int64(len(data)), CreatedAt: store.NowMs(),
	})
}

func (s *Server) handleDownloadDeviceFile(w http.ResponseWriter, r *http.Request) {
	f, data, err := s.st.GetDeviceFile(pathID(r, "id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "附件不存在")
		return
	}
	ct := f.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+urlEscape(f.Filename))
	http.ServeContent(w, r, f.Filename, time.UnixMilli(f.CreatedAt), bytes.NewReader(data))
}

func (s *Server) handleDeleteDeviceFile(w http.ResponseWriter, r *http.Request) {
	if err := s.st.DeleteDeviceFile(pathID(r, "id")); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// ---------- 项目规格书附件(V2.2) ----------

func (s *Server) handleListProjectFiles(w http.ResponseWriter, r *http.Request) {
	list, err := s.st.ListProjectFiles(pathID(r, "id"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, list)
}

func (s *Server) handleUploadProjectFile(w http.ResponseWriter, r *http.Request) {
	projectID := pathID(r, "id")
	if _, err := s.st.GetProject(projectID); err != nil {
		writeErr(w, http.StatusNotFound, "项目不存在")
		return
	}
	if err := r.ParseMultipartForm(maxSpecFileSize); err != nil {
		writeErr(w, http.StatusBadRequest, "表单解析失败: "+err.Error())
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "缺少文件字段 file")
		return
	}
	defer file.Close()
	if header.Size > maxSpecFileSize {
		writeErr(w, http.StatusBadRequest, "文件超过 32MB 限制")
		return
	}
	data, err := io.ReadAll(file)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	ct := header.Header.Get("Content-Type")
	id, err := s.st.AddProjectFile(projectID, header.Filename, ct, data)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, &store.ProjectFile{
		ID: id, ProjectID: projectID, Filename: header.Filename,
		ContentType: ct, Size: int64(len(data)), CreatedAt: store.NowMs(),
	})
}

func (s *Server) handleDownloadProjectFile(w http.ResponseWriter, r *http.Request) {
	f, data, err := s.st.GetProjectFile(pathID(r, "id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "附件不存在")
		return
	}
	ct := f.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+urlEscape(f.Filename))
	http.ServeContent(w, r, f.Filename, time.UnixMilli(f.CreatedAt), bytes.NewReader(data))
}

func (s *Server) handleDeleteProjectFile(w http.ResponseWriter, r *http.Request) {
	if err := s.st.DeleteProjectFile(pathID(r, "id")); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// handleArtifact 产物文件在线打开(能力题生成物)
func (s *Server) handleArtifact(w http.ResponseWriter, r *http.Request) {
	rel := chi.URLParam(r, "*")
	clean := filepath.Clean(rel)
	if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		writeErr(w, http.StatusBadRequest, "非法路径")
		return
	}
	// V2.22:pi 载荷文件含 API key,不对外提供下载
	if filepath.Base(clean) == "pi-payload.json" {
		writeErr(w, http.StatusNotFound, "产物不存在")
		return
	}
	full := filepath.Join(s.dataDir, "artifacts", clean)
	if _, err := os.Stat(full); err != nil {
		writeErr(w, http.StatusNotFound, "产物不存在")
		return
	}
	http.ServeFile(w, r, full)
}
