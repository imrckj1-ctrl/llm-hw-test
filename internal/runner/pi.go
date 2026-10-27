// pi 智能体执行器(V2.8):质量题默认走 pi(pi-agent-core 工具循环),模型可用
// read/write/bash 等工具真实产出文件;失败回退直接问答
package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"llmhwtest/internal/artifact"
	"llmhwtest/internal/pisandbox"
	"llmhwtest/internal/store"
)

// piEnabled 质量题执行方式:默认 pi;设置 quality_engine=direct 关闭
func piEnabled(st *store.Store) bool {
	v, ok := st.GetSetting("quality_engine")
	if !ok {
		return true
	}
	return v != "direct"
}

type piPayload struct {
	APIBaseURL  string  `json:"api_base_url"`
	APIKey      string  `json:"api_key"`
	Model       string  `json:"model"`
	Question    string  `json:"question"`
	Workdir     string  `json:"workdir"`
	MaxTokens   int     `json:"max_tokens"`
	ContextLen  int     `json:"context_len"`
	Temperature float64 `json:"temperature"`
	TopP        float64 `json:"top_p"`
	TopK        int     `json:"top_k"`
	TimeoutSec  int     `json:"timeout_sec"`
	UseGoal     bool    `json:"use_goal"` // V2.11:目标驱动契约(产物题开,普通题关)
	// V2.22:思考等级透传(minimal/low/medium/high/xhigh/max;off=关闭;空=默认)
	ThinkingMode string `json:"thinking_mode,omitempty"`
}

type piResult struct {
	Answer       string   `json:"answer"`
	Files        []string `json:"files"`
	Turns        int      `json:"turns"`
	ToolCalls    int      `json:"tool_calls"`
	InputTokens  int      `json:"input_tokens"`
	OutputTokens int      `json:"output_tokens"`
	DurationMs   int64    `json:"duration_ms"`
	TtftMs       float64  `json:"ttft_ms"`
	Iterations   int      `json:"iterations"`   // goal 续跑轮数(pi-goal 薄适配)
	GoalStatus   string   `json:"goal_status"`  // complete/blocked/active(超迭代兜底)
	Error        string   `json:"error"`
}

// piInfraError 基础设施错误(执行器/依赖缺失、进程起不来)→ 回退直接问答
type piInfraError struct{ msg string }

func (e *piInfraError) Error() string { return e.msg }

// piRunnerPath 定位 scripts/pi/runner.mjs 并确认依赖已装
func (r *Runner) piRunnerPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	base := filepath.Dir(exe)
	p := filepath.Join(base, "scripts", "pi", "runner.mjs")
	if _, err := os.Stat(p); err != nil {
		return "", &piInfraError{"pi 执行器缺失: " + p}
	}
	if _, err := os.Stat(filepath.Join(base, "scripts", "pi", "node_modules")); err != nil {
		return "", &piInfraError{"pi 依赖未安装(scripts/pi 下执行 npm install)"}
	}
	return p, nil
}

// piUseContainer 容器沙箱判定:auto=镜像就绪就用容器(缺失时后台自动构建,本次宿主机兜底);
// on=强制(镜像未就绪报错);off=宿主机
func (r *Runner) piUseContainer() (bool, error) {
	mode, _ := r.st.GetSetting("pi_container")
	if mode == "" {
		mode = "auto"
	}
	if mode == "off" {
		return false, nil
	}
	st := pisandbox.Detect()
	if !st.Docker {
		if mode == "on" {
			return false, &piInfraError{"pi_container=on 但未找到 docker"}
		}
		return false, nil
	}
	if !st.Image {
		// 自动后台构建,本次先宿主机兜底(下一次质量题即走容器)
		pisandbox.EnsureBuiltAsync()
		if mode == "on" {
			return false, &piInfraError{"pi_container=on 但镜像 hwtest-pi:local 未就绪(已在后台自动构建,稍后重试)"}
		}
		return false, nil
	}
	return true, nil
}

// newestMtime 目录内最新文件修改时间(看门狗用)
func newestMtime(dir string) time.Time {
	latest := time.Time{}
	filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() && info.ModTime().After(latest) {
			latest = info.ModTime()
		}
		return nil
	})
	return latest
}

// runPiQuestion 子进程执行一道质量题;ctx 取消 = 杀进程(任务取消即中断)
func (r *Runner) runPiQuestion(ec *execContext, q *store.Question, prompt string, maxTokens int) (*piResult, error) {
	runner, err := r.piRunnerPath()
	if err != nil {
		return nil, err
	}
	workdir := filepath.Join(r.dataDir, "artifacts", "pi", fmt.Sprintf("task_%d", ec.task.ID), fmt.Sprintf("q_%d", q.ID))
	p := ec.params
	// 目标驱动开关:题目显式开启,或产物题默认开启;时限:开=1小时(真干活),关=10分钟(答完即收)
	useGoal := q.UseGoal || artifact.IsDoc(q.ArtifactType) || q.ArtifactType == "html" || q.ArtifactType == "code"
	timeout := 600
	if useGoal {
		timeout = 3600
	}
	payload := piPayload{
		APIBaseURL: ec.task.APIBaseURL, APIKey: ec.apiKey, Model: ec.task.ModelName,
		Question: prompt, MaxTokens: maxTokens, ContextLen: ec.task.ContextLen,
		Temperature: p.Gen.Temperature, TopP: p.Gen.TopP, TopK: p.Gen.TopK,
		TimeoutSec: timeout, UseGoal: useGoal, ThinkingMode: p.Advanced.ThinkingMode,
	}
	// V2.22 容器沙箱:auto=有 docker+镜像就走容器(文件系统隔离,防翻旧任务残留);
	// 否则宿主机执行并告警。容器内 workdir 固定 /work(宿主机目录挂载)
	useDocker, derr := r.piUseContainer()
	if derr != nil {
		return nil, derr
	}
	if !useDocker {
		r.log(ec.task.ID, "warn", "pi 容器沙箱未启用(无 docker 或镜像 hwtest-pi:local 未构建),本次在宿主机执行")
	}
	payload.Workdir = workdir
	if useDocker {
		payload.Workdir = "/work"
	}
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		return nil, &piInfraError{"工作目录创建失败: " + err.Error()}
	}
	payloadPath := filepath.Join(workdir, "pi-payload.json")
	pf, err := os.Create(payloadPath)
	if err != nil {
		return nil, &piInfraError{"payload 文件创建失败: " + err.Error()}
	}
	if err := json.NewEncoder(pf).Encode(payload); err != nil {
		pf.Close()
		return nil, &piInfraError{"payload 序列化失败: " + err.Error()}
	}
	pf.Close()

	// V2.22:硬超时兜底——runner.mjs 自律超时之外宽限 5 分钟,node 进程卡死时强制 kill,
	// 否则任务线程永久挂住(只能整任务取消)
	runCtx, runCancel := context.WithTimeout(ec.ctx, time.Duration(timeout+300)*time.Second)
	defer runCancel()
	var cmd *exec.Cmd
	if useDocker {
		// docker -v 只接受绝对路径;dataDir 可能是相对路径(如 ./data)
		piDir, _ := filepath.Abs(filepath.Dir(runner))
		workdir, _ = filepath.Abs(workdir)
		cmd = exec.CommandContext(runCtx, "docker", "run", "--rm", "--network", "host",
			"-v", piDir+":/pi:ro", "-v", workdir+":/work", "-w", "/work",
			"hwtest-pi:local", "node", "/pi/runner.mjs", "/work/pi-payload.json")
	} else {
		cmd = exec.CommandContext(runCtx, "node", runner, payloadPath)
	}
	// V2.22:pi.log 持久化——stdout/stderr 同时写内存与日志文件,超时/看门狗终止后可复盘
	logFile, lerr := os.Create(filepath.Join(workdir, "pi.log"))
	if lerr != nil {
		return nil, &piInfraError{"pi.log 创建失败: " + lerr.Error()}
	}
	defer logFile.Close()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = io.MultiWriter(&stdout, logFile)
	cmd.Stderr = io.MultiWriter(&stderr, logFile)
	if err := cmd.Start(); err != nil {
		return nil, &piInfraError{"pi 进程启动失败: " + err.Error()}
	}
	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()
	// V2.22:看门狗——15 分钟工作目录无任何文件变动(卡死特征)即强制终止
	const stallLimit = 15 * time.Minute
	startTS := time.Now()
	watchdog := time.NewTicker(30 * time.Second)
	defer watchdog.Stop()
	var runErr error
watch:
	for {
		select {
		case runErr = <-waitCh:
			break watch
		case <-watchdog.C:
			latest := newestMtime(workdir)
			if latest.Before(startTS) {
				latest = startTS
			}
			if time.Since(latest) > stallLimit {
				r.log(ec.task.ID, "warn", "pi 看门狗:15 分钟无任何文件产出,强制终止(复盘见产物目录 pi.log)")
				cmd.Process.Kill()
				<-waitCh
				return nil, &piInfraError{"pi 看门狗终止:15 分钟无任何文件产出(日志见 pi.log)"}
			}
		}
	}
	if ec.ctx.Err() != nil {
		return nil, ec.ctx.Err()
	}
	if runCtx.Err() == context.DeadlineExceeded {
		return nil, &piInfraError{fmt.Sprintf("pi 执行超过硬超时 %d 秒,已强制终止", timeout+300)}
	}
	out := strings.TrimSpace(stdout.String())
	if out == "" {
		if runErr != nil {
			return nil, &piInfraError{fmt.Sprintf("pi 进程失败: %v; %s", runErr, tailStr(stderr.String(), 300))}
		}
		return nil, fmt.Errorf("pi 无输出; %s", tailStr(stderr.String(), 300))
	}
	var res piResult
	lines := strings.Split(out, "\n")
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &res); err != nil {
		return nil, &piInfraError{fmt.Sprintf("pi 输出解析失败: %v; stderr: %s", err, tailStr(stderr.String(), 300))}
	}
	if res.Error != "" {
		// 服务端不认 tools 参数等兼容性问题 → 回退直接问答更划算
		if strings.Contains(strings.ToLower(res.Error), "tool") {
			return &res, &piInfraError{"被测服务疑似不支持工具调用: " + res.Error}
		}
		return &res, fmt.Errorf("%s", res.Error)
	}
	return &res, nil
}

func tailStr(s string, n int) string {
	if len(s) > n {
		return "…" + s[len(s)-n:]
	}
	return s
}

// piArtifactRel 从 pi 产物清单中挑选登记文件:优先匹配题目产物类型扩展名,否则第一个文件
// piArtifactExt 认可为产物的文件扩展名
var piArtifactExt = map[string]bool{
	".pptx": true, ".xlsx": true, ".docx": true, ".pdf": true,
	".html": true, ".htm": true, ".py": true, ".js": true, ".csv": true, ".md": true,
}

// normalizePiArtifact 挑选 pi 产物并规范命名:
// 文档类 → document.<type>(前端下载约定);html → index.html(在线预览约定)
// artifact_path 登记的是目录(打包导出按目录遍历),与直接问答模式口径一致
func normalizePiArtifact(dataDir string, taskID, questionID int64, files []string, artifactType string) (rel string, ok bool) {
	if len(files) == 0 {
		return "", false
	}
	want := "." + artifactType
	pick := ""
	for _, f := range files {
		ext := strings.ToLower(filepath.Ext(f))
		if ext == want {
			pick = f
			break
		}
		if pick == "" && piArtifactExt[ext] {
			pick = f
		}
	}
	if pick == "" {
		pick = files[0]
	}
	dir := filepath.Join(dataDir, "artifacts", "pi", fmt.Sprintf("task_%d", taskID), fmt.Sprintf("q_%d", questionID))
	src := filepath.Join(dir, filepath.FromSlash(pick))
	data, err := os.ReadFile(src)
	if err != nil {
		return "", false
	}
	ext := strings.ToLower(filepath.Ext(pick))
	target := ""
	switch {
	case ext == ".html" || ext == ".htm":
		target = "index.html" // 在线预览固定入口
	case artifact.IsDoc(artifactType):
		target = "document." + artifactType // 下载固定文件名
	default:
		target = filepath.Base(pick)
	}
	if filepath.Base(pick) != target {
		if err := os.WriteFile(filepath.Join(dir, target), data, 0o644); err != nil {
			return "", false
		}
	}
	return filepath.ToSlash(filepath.Join("pi", fmt.Sprintf("task_%d", taskID), fmt.Sprintf("q_%d", questionID))), true
}
