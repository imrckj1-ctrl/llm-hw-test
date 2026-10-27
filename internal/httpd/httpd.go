// Package httpd 路由/鉴权中间件/静态资源(embed)
package httpd

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"llmhwtest/internal/auth"
	"llmhwtest/internal/scheduler"
	"llmhwtest/internal/store"
	"llmhwtest/web"
)

type Server struct {
	st      *store.Store
	au      *auth.Auth
	sched   *scheduler.Scheduler
	dataDir string
}

func New(st *store.Store, au *auth.Auth, sched *scheduler.Scheduler, dataDir string) *Server {
	return &Server{st: st, au: au, sched: sched, dataDir: dataDir}
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Route("/api", func(api chi.Router) {
		// 认证(开放)
		api.Post("/login", s.handleLogin)
		api.Post("/logout", s.handleLogout)
		// 以下全部需登录
		api.Group(func(pr chi.Router) {
			pr.Use(s.au.Middleware)
			pr.Get("/me", s.handleMe)
			pr.Post("/password", s.handleChangePassword)
			pr.Get("/devices", s.handleListDevices)
			pr.Post("/devices", s.handleCreateDevice)
			pr.Get("/devices/{id}", s.handleGetDevice)
			pr.Put("/devices/{id}", s.handleUpdateDevice)
			pr.Delete("/devices/{id}", s.handleDeleteDevice)
			pr.Post("/devices/{id}/archive", s.handleArchiveDevice)
			pr.Get("/devices/{id}/files", s.handleListDeviceFiles)
			pr.Post("/devices/{id}/files", s.handleUploadDeviceFile)
			pr.Get("/device-files/{id}", s.handleDownloadDeviceFile)
			pr.Delete("/device-files/{id}", s.handleDeleteDeviceFile)
			pr.Get("/projects/{id}/files", s.handleListProjectFiles)
			pr.Post("/projects/{id}/files", s.handleUploadProjectFile)
			pr.Get("/project-files/{id}", s.handleDownloadProjectFile)
			pr.Delete("/project-files/{id}", s.handleDeleteProjectFile)
			pr.Get("/artifacts/*", s.handleArtifact)
			pr.Get("/corpus-versions", s.handleListCorpusVersions)
			pr.Post("/corpus-versions", s.handleCreateCorpusVersion)
			pr.Get("/corpus-versions/{id}/texts", s.handleGetCorpusTexts)
			pr.Post("/corpus-versions/{id}/activate", s.handleActivateCorpus)
			pr.Put("/corpus-versions/{id}", s.handleUpdateCorpusVersion)
			pr.Delete("/corpus-versions/{id}", s.handleDeleteCorpusVersion)
			pr.Put("/corpus-versions/{id}/texts/{tier}", s.handleReplaceCorpusTier)
			pr.Get("/questions", s.handleListQuestions)
			pr.Post("/questions", s.handleCreateQuestion)
			pr.Put("/questions/{id}", s.handleUpdateQuestion)
			pr.Delete("/questions/{id}", s.handleDeleteQuestion)
			pr.Get("/question-groups", s.handleListQuestionGroups)
			pr.Post("/question-groups", s.handleCreateQuestionGroup)
			pr.Put("/question-groups/{id}", s.handleUpdateQuestionGroup)
			pr.Delete("/question-groups/{id}", s.handleDeleteQuestionGroup)
			pr.Get("/projects", s.handleListProjects)
			pr.Post("/projects", s.handleCreateProject)
			pr.Get("/projects/{id}", s.handleGetProject)
			pr.Put("/projects/{id}", s.handleUpdateProject)
			pr.Delete("/projects/{id}", s.handleDeleteProject)
			pr.Post("/projects/{id}/hidden", s.handleSetProjectHidden)
			pr.Post("/projects/{id}/start", s.handleStartProject)
			pr.Get("/projects/{id}/export.docx", s.handleExportProjectDocx)
			pr.Post("/projects/{id}/tasks", s.handleCreateTask)
			pr.Get("/tasks", s.handleListTasks)
			pr.Get("/tasks/{id}", s.handleGetTask)
			pr.Put("/tasks/{id}", s.handleUpdateTask)
			pr.Delete("/tasks/{id}", s.handleDeleteTask)
			pr.Post("/tasks/{id}/start", s.handleStartTask)
			pr.Post("/tasks/{id}/cancel", s.handleCancelTask)
			pr.Post("/tasks/{id}/clone", s.handleCloneTask)
			pr.Post("/tasks/{id}/resume", s.handleResumeTask)
			pr.Post("/tasks/{id}/pause", s.handlePauseTask)
			pr.Post("/tasks/{id}/skip-item", s.handleSkipItem)
			pr.Post("/tasks/direct", s.handleDirectTask)
			pr.Post("/tasks/{id}/items/{itemId}/rerun", s.handleRerunItem)
			pr.Post("/tasks/{id}/items/{itemId}/skip", s.handleSkipItemByID)
			pr.Post("/tasks/fetch-models", s.handleFetchModels)
			pr.Get("/tasks/{id}/summary", s.handleTaskSummary)
			pr.Get("/tasks/{id}/items", s.handleTaskItems)
			pr.Get("/tasks/{id}/results", s.handleTaskResults)
			pr.Get("/tasks/{id}/samples", s.handleTaskSamples)
			pr.Get("/tasks/{id}/quality", s.handleTaskQuality)
			pr.Put("/quality-answers/{id}", s.handleMarkQuality)
			pr.Get("/tasks/{id}/logs", s.handleTaskLogs)
			pr.Get("/tasks/{id}/bundle.zip", s.handleTaskBundle)
			pr.Get("/tasks/{id}/package-info", s.handlePackageInfo)
			pr.Get("/tasks/{id}/events", s.handleTaskEvents)
			pr.Get("/tasks/{id}/digest", s.handleTaskDigest)
			pr.Get("/compare/models", s.handleCompareModels)
			pr.Get("/compare/tasks", s.handleCompareTasks)
			pr.Get("/compare/curves", s.handleCompareCurves)
			pr.Get("/compare/board", s.handleCompareBoard)
			pr.Get("/compare/capability", s.handleCompareCapability)
			pr.Get("/compare/capability-detail", s.handleCompareCapabilityDetail)
			pr.Get("/compare/export.xlsx", s.handleCompareExport)
			pr.Get("/settings", s.handleGetSettings)
			pr.Put("/settings", s.handlePutSettings)
			pr.Post("/settings/api-token", s.handleGenAPIToken)
			pr.Delete("/settings/api-token", s.handleRevokeAPIToken)
			pr.Get("/settings/skill.zip", s.handleSkillZip)
			pr.Get("/backup", s.handleBackup)
			pr.Get("/overview", s.handleOverview)
		})
	})
	// 静态资源(embed.FS),SPA 路由回退 index.html
	dist, _ := fs.Sub(web.Dist, "dist")
	fileSrv := http.FileServer(http.FS(dist))
	r.NotFound(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasPrefix(req.URL.Path, "/api/") {
			writeErr(w, http.StatusNotFound, "接口不存在")
			return
		}
		path := strings.TrimPrefix(req.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		// HTML 入口禁缓存(避免老前端残留);assets 带 hash 可长缓存
		if strings.HasSuffix(path, ".html") {
			w.Header().Set("Cache-Control", "no-cache")
		}
		if _, err := fs.Stat(dist, path); err != nil {
			req.URL.Path = "/"
		}
		fileSrv.ServeHTTP(w, req)
	})
	return r
}

// ---------- 通用工具 ----------

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func decodeJSON(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}

func pathID(r *http.Request, name string) int64 {
	id, _ := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	return id
}

func queryInt(r *http.Request, name string) int64 {
	v, _ := strconv.ParseInt(r.URL.Query().Get(name), 10, 64)
	return v
}

func queryStr(r *http.Request, name string) string {
	return strings.TrimSpace(r.URL.Query().Get(name))
}
