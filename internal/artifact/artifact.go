// Package artifact 产物落地器(确定性,不调 LLM):提取代码 → 写文件 → 结构校验(V2.2 §4.1)
// V2.5:文档类产物(xlsx Go 渲染 / pptx·docx·pdf 内嵌 python 渲染器)
package artifact

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/xuri/excelize/v2"
)

//go:embed render_doc.py
var renderDocPy string

// docFileName 文档类产物统一文件名
func docFileName(kind string) string { return "document." + kind }

// SchemaHint 文档类题目的输出契约(由平台追加到题干,模型只输出 JSON 围栏)
func SchemaHint(kind string) string {
	switch kind {
	case "xlsx":
		return "【输出格式要求】只输出一个 ```json 代码块,不要输出其他内容。结构:{\"sheets\":[{\"name\":\"表名\",\"headers\":[\"列1\",\"列2\"],\"rows\":[[\"值\",123]]}]}"
	case "pptx":
		return "【输出格式要求】只输出一个 ```json 代码块,不要输出其他内容。结构:{\"slides\":[{\"title\":\"页标题\",\"subtitle\":\"副标题(仅封面)\",\"bullets\":[\"要点1\",\"要点2\"]}]}"
	case "docx", "pdf":
		return "【输出格式要求】只输出一个 ```json 代码块,不要输出其他内容。结构:{\"title\":\"文档标题\",\"blocks\":[{\"type\":\"h1|h2|p|bullet\",\"text\":\"内容\"}]}"
	}
	return ""
}

// IsDoc 文档类产物类型
func IsDoc(kind string) bool {
	return kind == "xlsx" || kind == "pptx" || kind == "docx" || kind == "pdf"
}

// Save 从回答中提取内容并落盘,返回相对数据目录的路径与校验结果
// kind: html(必须有 HTML 标签证据)/code(任意代码围栏)/xlsx·pptx·docx·pdf(JSON 规格渲染)
func Save(dataDir string, taskID, answerID int64, answer, kind string) (relPath string, ok bool, note string) {
	if IsDoc(kind) {
		return saveDoc(dataDir, taskID, answerID, answer, kind)
	}
	code := extractCode(answer, kind == "html")
	if code == "" {
		return "", false, "未产出可落地文件(回答中无代码块/HTML)"
	}
	dir := filepath.Join(dataDir, "artifacts", fmt.Sprintf("task_%d", taskID), fmt.Sprintf("q_%d", answerID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", false, "目录创建失败: " + err.Error()
	}
	// HTML 嗅探补全:片段补骨架,保证浏览器可打开
	if !strings.Contains(strings.ToLower(code), "<html") {
		code = "<!DOCTYPE html>\n<html lang=\"zh-CN\"><head><meta charset=\"UTF-8\"></head><body>\n" + code + "\n</body></html>"
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(code), 0o644); err != nil {
		return "", false, "写入失败: " + err.Error()
	}
	rel := fmt.Sprintf("task_%d/q_%d", taskID, answerID) // 相对 artifacts 根(服务侧再拼 artifacts/)
	return rel, true, fmt.Sprintf("已落地 index.html(%d 字符)", len([]rune(code)))
}

// ---------- 文档类产物 ----------

var jsonFenceRe = regexp.MustCompile("(?s)```(?:json|JSON)?\\s*\\n(.*?)```")

// extractSpec 提取 JSON 规格:优先 json 围栏,其次整段 JSON 嗅探
func extractSpec(answer string) (map[string]any, error) {
	for _, m := range jsonFenceRe.FindAllStringSubmatch(answer, -1) {
		var spec map[string]any
		if json.Unmarshal([]byte(strings.TrimSpace(m[1])), &spec) == nil {
			return spec, nil
		}
	}
	// 整段 JSON 嗅探(模型没加围栏时兜底)
	if i := strings.Index(answer, "{"); i >= 0 {
		if j := strings.LastIndex(answer, "}"); j > i {
			var spec map[string]any
			if json.Unmarshal([]byte(answer[i:j+1]), &spec) == nil {
				return spec, nil
			}
		}
	}
	return nil, fmt.Errorf("回答中未找到合法 JSON 规格")
}

func saveDoc(dataDir string, taskID, answerID int64, answer, kind string) (string, bool, string) {
	spec, err := extractSpec(answer)
	if err != nil {
		return "", false, "未产出可落地文件(" + err.Error() + ")"
	}
	dir := filepath.Join(dataDir, "artifacts", fmt.Sprintf("task_%d", taskID), fmt.Sprintf("q_%d", answerID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", false, "目录创建失败: " + err.Error()
	}
	// 规格留档(排查用)
	specBytes, _ := json.MarshalIndent(spec, "", "  ")
	os.WriteFile(filepath.Join(dir, "spec.json"), specBytes, 0o644)

	out := filepath.Join(dir, docFileName(kind))
	if kind == "xlsx" {
		return renderXlsx(dir, out, spec, taskID, answerID)
	}
	return renderViaPython(dataDir, dir, out, kind, spec, taskID, answerID)
}

// renderXlsx Go 原生渲染(excelize,零外部依赖)
func renderXlsx(dir, out string, spec map[string]any, taskID, answerID int64) (string, bool, string) {
	sheets, _ := spec["sheets"].([]any)
	if len(sheets) == 0 {
		return "", false, "JSON 规格缺少 sheets"
	}
	f := excelize.NewFile()
	defer f.Close()
	f.DeleteSheet("Sheet1")
	totalRows := 0
	for i, s := range sheets {
		sm, _ := s.(map[string]any)
		name, _ := sm["name"].(string)
		if name == "" {
			name = fmt.Sprintf("Sheet%d", i+1)
		}
		fi, err := f.NewSheet(name)
		if err != nil {
			continue
		}
		headers, _ := sm["headers"].([]any)
		for c, h := range headers {
			cell, _ := excelize.CoordinatesToCellName(c+1, 1)
			f.SetCellValue(name, cell, fmt.Sprint(h))
		}
		rows, _ := sm["rows"].([]any)
		for ri, row := range rows {
			rCells, _ := row.([]any)
			for ci, val := range rCells {
				cell, _ := excelize.CoordinatesToCellName(ci+1, ri+2)
				f.SetCellValue(name, cell, val)
			}
			totalRows++
		}
		f.SetActiveSheet(fi)
	}
	if totalRows == 0 {
		return "", false, "JSON 规格无任何数据行"
	}
	if err := f.SaveAs(out); err != nil {
		return "", false, "xlsx 写入失败: " + err.Error()
	}
	rel := fmt.Sprintf("task_%d/q_%d", taskID, answerID)
	return rel, true, fmt.Sprintf("已落地 %s(%d 表 %d 行)", docFileName("xlsx"), len(sheets), totalRows)
}

// renderViaPython pptx/docx/pdf:内嵌 python 渲染器(单二进制不依赖仓库目录)
func renderViaPython(dataDir, dir, out, kind string, spec map[string]any, taskID, answerID int64) (string, bool, string) {
	py, err := exec.LookPath("python3")
	if err != nil {
		return "", false, "未找到 python3,无法渲染 " + kind
	}
	script := filepath.Join(dataDir, "render_doc.py")
	if _, err := os.Stat(script); err != nil {
		if werr := os.WriteFile(script, []byte(renderDocPy), 0o755); werr != nil {
			return "", false, "渲染器释放失败: " + werr.Error()
		}
	}
	specPath := filepath.Join(dir, "spec.json")
	cmd := exec.Command(py, script, kind, specPath, out)
	stdout, serr := cmd.CombinedOutput()
	if serr != nil {
		return "", false, fmt.Sprintf("渲染失败: %v(%s)", serr, strings.TrimSpace(string(stdout)))
	}
	st, err := os.Stat(out)
	if err != nil || st.Size() < 500 {
		return "", false, "渲染产物异常(文件缺失或过小)"
	}
	rel := fmt.Sprintf("task_%d/q_%d", taskID, answerID)
	return rel, true, fmt.Sprintf("已落地 %s(%s)", docFileName(kind), strings.TrimSpace(string(stdout)))
}

var fenceRe = regexp.MustCompile("(?s)```([a-zA-Z]*)\\s*\\n(.*?)```")

// hasTagEvidence 判断内容是否含真实 HTML 标签(区别于思维链中的规划散文)
func hasTagEvidence(s string) bool {
	lower := strings.ToLower(s)
	for _, sig := range []string{"<html", "<!doctype", "<head", "<body", "<div", "<span", "<script", "<style", "<table", "<button", "<input", "<ul", "<p ", "<p>", "<h1", "<section", "<template"} {
		if strings.Contains(lower, sig) {
			return true
		}
	}
	return false
}

// extractCode 优先整段 HTML 嗅探,其次带标签证据的围栏;strictHTML 时拒绝无标签内容
func extractCode(answer string, strictHTML bool) string {
	lower := strings.ToLower(answer)
	if i := strings.Index(lower, "<!doctype html"); i >= 0 {
		return strings.TrimSpace(answer[i:])
	}
	if i := strings.Index(lower, "<html"); i >= 0 {
		end := strings.LastIndex(lower, "</html>")
		if end > i {
			return strings.TrimSpace(answer[i : end+7])
		}
		return strings.TrimSpace(answer[i:])
	}
	for _, m := range fenceRe.FindAllStringSubmatch(answer, -1) {
		body := strings.TrimSpace(m[2])
		if body == "" {
			continue
		}
		if !strictHTML {
			return body // 代码文件:第一个非空围栏
		}
		if hasTagEvidence(body) {
			return body
		}
	}
	return ""
}
