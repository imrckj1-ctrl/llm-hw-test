package httpd

import (
	"net/http"
	"os"
	"path/filepath"
	"llmhwtest/internal/exporter"
)

// 任务数据包(V2.23):物化为真实目录 data/packages/task_<id>/,对话框给绝对路径+文件清单
// 终态任务内容冻结,物化一次终身有效;执行中任务点击则临时构建(不缓存,数据未定型)

// handlePackageInfo 数据包信息:绝对路径 + 文件清单(终态任务自动物化;执行中临时构建不落盘)
func (s *Server) handlePackageInfo(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	t, err := s.st.GetTask(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "任务不存在")
		return
	}
	frozen := t.Status == "done" || t.Status == "failed"
	if !frozen {
		// 执行中/暂停等:临时构建清单,不落盘
		var list []map[string]any
		for _, f := range exporter.TaskPackageFiles(s.st, s.dataDir, t) {
			list = append(list, map[string]any{"path": f.Name, "size": len(f.Data)})
		}
		writeJSON(w, map[string]any{"dir": "", "frozen": false, "files": list})
		return
	}
	dir := exporter.PackageDir(s.dataDir, id)
	if _, err := os.Stat(dir); err != nil {
		if _, err := exporter.MaterializeTaskPackage(s.st, s.dataDir, t); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	abs, _ := filepath.Abs(dir)
	writeJSON(w, map[string]any{"dir": abs, "frozen": true, "files": exporter.PackageFileList(dir)})
}

// handleTaskBundle 兼容老接口:bundle.zip 下载(内容构建统一走 exporter)
func (s *Server) handleTaskBundle(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	t, err := s.st.GetTask(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "任务不存在")
		return
	}
	buf := exporter.ZipPackageFiles(exporter.TaskPackageFiles(s.st, s.dataDir, t))
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=task-bundle.zip")
	w.Write(buf.Bytes())
}

// 能力明细对比(修改清单-2):勾选任务的逐题回答全文 + 评判 + 产物
func (s *Server) handleCompareCapabilityDetail(w http.ResponseWriter, r *http.Request) {
	ids := parseIDs(r.URL.Query().Get("ids"))
	type answer struct {
		QuestionID   int64  `json:"question_id"`
		Question     string `json:"question"`
		Category     string `json:"category"`
		Answer       string `json:"answer"`
		JudgeScore   string `json:"judge_score"`
		JudgeReason  string `json:"judge_reason"`
		AutoFlag     string `json:"auto_flag"`
		ArtifactPath string `json:"artifact_path"`
		ArtifactOK   bool   `json:"artifact_ok"`
	}
	type taskCol struct {
		TaskID  int64    `json:"task_id"`
		Label   string   `json:"label"`
		Answers []answer `json:"answers"`
	}
	var out []taskCol
	for _, id := range ids {
		t, err := s.st.GetTask(id)
		if err != nil {
			continue
		}
		tc := taskCol{TaskID: id, Label: itoa(id) + " " + t.ModelName}
		rows, _ := s.st.DB().Query(`SELECT qa.question_id,qa.question_title,COALESCE(q.category,''),qa.answer_text,
			COALESCE(qa.judge_score,''),COALESCE(qa.judge_reason,''),qa.auto_flag,COALESCE(qa.artifact_path,''),COALESCE(qa.artifact_ok,0)
			FROM quality_answers qa LEFT JOIN questions q ON q.id=qa.question_id
			WHERE qa.task_id=? AND qa.run_no=? ORDER BY qa.id`, id, t.RunNo)
		if rows != nil {
			for rows.Next() {
				var a answer
				var ok int
				rows.Scan(&a.QuestionID, &a.Question, &a.Category, &a.Answer, &a.JudgeScore, &a.JudgeReason,
					&a.AutoFlag, &a.ArtifactPath, &ok)
				a.ArtifactOK = ok != 0
				tc.Answers = append(tc.Answers, a)
			}
			rows.Close()
		}
		out = append(out, tc)
	}
	writeJSON(w, out)
}
