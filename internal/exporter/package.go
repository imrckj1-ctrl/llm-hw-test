// 任务数据包(V2.23 重构):
// 内容构建一次产出文件清单,可 zip(浏览器下载)也可物化为真实目录(数据包对话框给绝对路径)
// 物化时机:任务终态(done/failed)由 runner 后台调用;或数据包对话框首次点击时现建
package exporter

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"llmhwtest/internal/store"
)

// PkgFile 数据包内一个文件
type PkgFile struct {
	Name string
	Data []byte
}

func pkgOrEmpty(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// pkgSafeName 档位名转安全文件名(去路径分隔符)
func pkgSafeName(s string) string {
	r := strings.NewReplacer("/", "_", "\\", "_", ":", "_")
	return r.Replace(s)
}

// TaskPackageFiles 构建任务数据包全部内容(运行日志/明细/全文/meta/请求对/产物/语料快照/题目快照/结果页)
func TaskPackageFiles(st *store.Store, dataDir string, t *store.Task) []PkgFile {
	id := t.ID
	var files []PkgFile
	add := func(name, content string) { files = append(files, PkgFile{name, []byte(content)}) }

	// 1. 运行日志
	var logSb strings.Builder
	lrows, _ := st.DB().Query(`SELECT ts,level,message FROM task_logs WHERE task_id=? ORDER BY id`, id)
	if lrows != nil {
		for lrows.Next() {
			var ts int64
			var level, msg string
			lrows.Scan(&ts, &level, &msg)
			logSb.WriteString(fmt.Sprintf("[%s] [%s] %s\n", time.UnixMilli(ts).Format("2006-01-02 15:04:05.000"), level, msg))
		}
		lrows.Close()
	}
	add("run_log.txt", logSb.String())

	// 2. 请求明细 CSV
	var csvSb strings.Builder
	csvSb.WriteString("id,run_no,phase,tier,concurrency,seq,send_ts,ttft_ms,total_ms,prompt_tokens,completion_tokens,speed,status,tokens_estimated,output_incomplete,length_mismatch,usage_suspect,reasoning_only,error\n")
	srows, _ := st.DB().Query(`SELECT id,run_no,phase,tier,concurrency,seq,send_ts,ttft_ms,total_ms,prompt_tokens,completion_tokens,
		speed,status,tokens_estimated,output_incomplete,length_mismatch,COALESCE(usage_suspect,0),COALESCE(reasoning_only,0),error
		FROM request_samples WHERE task_id=? ORDER BY id`, id)
	if srows != nil {
		for srows.Next() {
			var sid, sendTS int64
			var runNo, conc, seq, pt, ct, est, inc, mm, us, ro int
			var phase, tier, status, errMsg string
			var ttft, total, speed float64
			srows.Scan(&sid, &runNo, &phase, &tier, &conc, &seq, &sendTS, &ttft, &total, &pt, &ct, &speed, &status,
				&est, &inc, &mm, &us, &ro, &errMsg)
			csvSb.WriteString(fmt.Sprintf("%d,%d,%s,%s,%d,%d,%d,%.1f,%.1f,%d,%d,%.2f,%s,%d,%d,%d,%d,%d,%q\n",
				sid, runNo, phase, tier, conc, seq, sendTS, ttft, total, pt, ct, speed, status, est, inc, mm, us, ro, errMsg))
		}
		srows.Close()
	}
	add("samples.csv", csvSb.String())

	// 3. 请求/响应全文 JSONL
	type line struct {
		ID               int64   `json:"id"`
		RunNo            int     `json:"run_no"`
		Phase            string  `json:"phase"`
		Tier             string  `json:"tier,omitempty"`
		Seq              int     `json:"seq"`
		SendTS           int64   `json:"send_ts"`
		TtftMs           float64 `json:"ttft_ms"`
		TotalMs          float64 `json:"total_ms"`
		PromptTokens     int     `json:"prompt_tokens"`
		CompletionTokens int     `json:"completion_tokens"`
		Speed            float64 `json:"speed"`
		Status           string  `json:"status"`
		Error            string  `json:"error,omitempty"`
		Response         string  `json:"response"`
	}
	var jl strings.Builder
	rrows, _ := st.DB().Query(`SELECT id,run_no,phase,tier,seq,send_ts,ttft_ms,total_ms,prompt_tokens,completion_tokens,
		speed,status,error,COALESCE(raw_response,'') FROM request_samples WHERE task_id=? ORDER BY id`, id)
	if rrows != nil {
		for rrows.Next() {
			var l line
			rrows.Scan(&l.ID, &l.RunNo, &l.Phase, &l.Tier, &l.Seq, &l.SendTS, &l.TtftMs, &l.TotalMs,
				&l.PromptTokens, &l.CompletionTokens, &l.Speed, &l.Status, &l.Error, &l.Response)
			b, _ := json.Marshal(l)
			jl.Write(b)
			jl.WriteByte('\n')
		}
		rrows.Close()
	}
	add("responses.jsonl", jl.String())

	// 4. 任务元信息
	meta, _ := json.MarshalIndent(map[string]any{
		"task_id": t.ID, "project_id": t.ProjectID, "model": t.ModelName, "quant": t.Quant,
		"api_base_url": t.APIBaseURL, "run_no": t.RunNo, "status": t.Status,
		"corpus_version": t.CorpusVersionNo, "params": json.RawMessage(t.ParamsJSON),
		"exported_at": time.Now().Format(time.RFC3339),
	}, "", "  ")
	add("task_meta.json", string(meta))

	// 5. 全量请求/响应对
	prows, _ := st.DB().Query(`SELECT id,COALESCE(request_id,0),COALESCE(raw_response,'') FROM request_samples WHERE task_id=? ORDER BY id`, id)
	type pair struct {
		id    int64
		reqID int64
		resp  string
	}
	var pairs []pair
	if prows != nil {
		for prows.Next() {
			var p pair
			prows.Scan(&p.id, &p.reqID, &p.resp)
			pairs = append(pairs, p)
		}
		prows.Close()
	}
	for _, p := range pairs {
		if p.reqID > 0 {
			add(fmt.Sprintf("requests/%05d-request.json", p.id), st.GetRequestBody(p.reqID))
		}
		if p.resp != "" {
			add(fmt.Sprintf("requests/%05d-response.txt", p.id), p.resp)
		}
	}

	// 6. 产物原文件
	arows, _ := st.DB().Query(`SELECT artifact_path FROM quality_answers WHERE task_id=? AND artifact_path<>''`, id)
	if arows != nil {
		defer arows.Close()
		for arows.Next() {
			var rel string
			arows.Scan(&rel)
			dir := filepath.Join(dataDir, "artifacts", filepath.FromSlash(rel))
			entries, err := os.ReadDir(dir)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				// pi-payload.json 含被测服务 API key,绝不进数据包
				if e.Name() == "pi-payload.json" {
					continue
				}
				data, err := os.ReadFile(filepath.Join(dir, e.Name()))
				if err != nil {
					continue
				}
				files = append(files, PkgFile{filepath.ToSlash(filepath.Join("artifacts", rel, e.Name())), data})
			}
		}
	}

	// 7. 语料快照(终态冻结优先,老任务回退语料现值)
	crows, _ := st.DB().Query(`SELECT tier,content,version_no FROM task_corpus_snapshot WHERE task_id=? AND run_no=? ORDER BY id`, id, t.RunNo)
	csnapEmpty := true
	if crows != nil {
		for crows.Next() {
			var tier, content, vno string
			crows.Scan(&tier, &content, &vno)
			add(fmt.Sprintf("corpus/%s/%s.txt", pkgOrEmpty(vno, t.CorpusVersionNo), pkgSafeName(tier)), content)
			csnapEmpty = false
		}
		crows.Close()
	}
	if csnapEmpty && t.CorpusVersionID > 0 {
		lrows2, _ := st.DB().Query(`SELECT tier,content FROM corpus_texts WHERE version_id=? ORDER BY id`, t.CorpusVersionID)
		if lrows2 != nil {
			for lrows2.Next() {
				var tier, content string
				lrows2.Scan(&tier, &content)
				add(fmt.Sprintf("corpus/%s/%s.txt", pkgOrEmpty(t.CorpusVersionNo, "corpus"), pkgSafeName(tier)), content)
			}
			lrows2.Close()
		}
	}

	// 8. 题目快照(同上回退)
	type qsnap struct {
		QuestionID     int64  `json:"question_id"`
		Title          string `json:"title"`
		Prompt         string `json:"prompt"`
		LongText       string `json:"long_text,omitempty"`
		QType          string `json:"qtype"`
		Category       string `json:"category"`
		ExpectedAnswer string `json:"expected_answer"`
		JudgeMode      string `json:"judge_mode"`
		ArtifactType   string `json:"artifact_type"`
		UseGoal        bool   `json:"use_goal"`
	}
	var qs []qsnap
	qrows, _ := st.DB().Query(`SELECT question_id,title,prompt,long_text,qtype,category,expected_answer,judge_mode,artifact_type,use_goal
		FROM task_question_snapshot WHERE task_id=? AND run_no=? ORDER BY id`, id, t.RunNo)
	if qrows != nil {
		for qrows.Next() {
			var q qsnap
			var ug int
			qrows.Scan(&q.QuestionID, &q.Title, &q.Prompt, &q.LongText, &q.QType, &q.Category, &q.ExpectedAnswer, &q.JudgeMode, &q.ArtifactType, &ug)
			q.UseGoal = ug != 0
			qs = append(qs, q)
		}
		qrows.Close()
	}
	if len(qs) == 0 {
		lq, _ := st.DB().Query(`SELECT q.id,q.title,q.prompt,COALESCE(q.long_text,''),q.qtype,COALESCE(q.category,''),
			COALESCE(q.expected_answer,''),COALESCE(q.judge_mode,''),COALESCE(q.artifact_type,''),COALESCE(q.use_goal,0)
			FROM quality_answers qa JOIN questions q ON q.id=qa.question_id
			WHERE qa.task_id=? AND qa.run_no=? GROUP BY q.id ORDER BY q.id`, id, t.RunNo)
		if lq != nil {
			for lq.Next() {
				var q qsnap
				var ug int
				lq.Scan(&q.QuestionID, &q.Title, &q.Prompt, &q.LongText, &q.QType, &q.Category, &q.ExpectedAnswer, &q.JudgeMode, &q.ArtifactType, &ug)
				q.UseGoal = ug != 0
				qs = append(qs, q)
			}
			lq.Close()
		}
	}
	if len(qs) > 0 {
		qb, _ := json.MarshalIndent(map[string]any{"task_id": id, "run_no": t.RunNo, "questions": qs}, "", "  ")
		add("questions.json", string(qb))
	}

	// 9. 交互式结果页(单任务版)
	if p, err := st.GetProject(t.ProjectID); err == nil {
		if html := buildResultsHTML(st, p, []*store.Task{t}); html != nil {
			files = append(files, PkgFile{"results.html", html})
		}
	}
	return files
}

// PackageDir 物化目录路径
func PackageDir(dataDir string, taskID int64) string {
	return filepath.Join(dataDir, "packages", fmt.Sprintf("task_%d", taskID))
}

// MaterializeTaskPackage 物化为真实目录(先清后写,保证与当次构建一致)
func MaterializeTaskPackage(st *store.Store, dataDir string, t *store.Task) (string, error) {
	dir := PackageDir(dataDir, t.ID)
	if err := os.RemoveAll(dir); err != nil {
		return "", err
	}
	for _, f := range TaskPackageFiles(st, dataDir, t) {
		fp := filepath.Join(dir, filepath.FromSlash(f.Name))
		if err := os.MkdirAll(filepath.Dir(fp), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(fp, f.Data, 0o644); err != nil {
			return "", err
		}
	}
	return dir, nil
}

// ZipPackageFiles 文件清单 → zip
func ZipPackageFiles(files []PkgFile) *bytes.Buffer {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	for _, f := range files {
		w, err := zw.Create(f.Name)
		if err != nil {
			continue
		}
		w.Write(f.Data)
	}
	zw.Close()
	return buf
}

// ZipPackageDir 物化目录 → zip(逐文件读,低内存)
func ZipPackageDir(dir string) (*bytes.Buffer, error) {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		w, err := zw.Create(filepath.ToSlash(rel))
		if err != nil {
			return nil
		}
		f, err := os.Open(p)
		if err != nil {
			return nil
		}
		_, err = io.Copy(w, f)
		f.Close()
		return err
	})
	zw.Close()
	return buf, err
}

// PackageFileList 物化目录文件清单(相对路径+大小)
func PackageFileList(dir string) []map[string]any {
	var out []map[string]any
	filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		out = append(out, map[string]any{"path": filepath.ToSlash(rel), "size": info.Size()})
		return nil
	})
	return out
}
