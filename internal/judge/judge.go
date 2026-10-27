// Package judge LLM 评判器:调用裁判模型判定回答质量(V2.2 设计 §4.2)
package judge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"llmhwtest/internal/llmclient"
	"llmhwtest/internal/store"
)

// Verdict 评判结论
const (
	VerdictPass    = "pass"    // 对
	VerdictPartial = "partial" // 部分对
	VerdictFail    = "fail"    // 错
)

type Judge struct {
	URL   string
	Model string
	Key   string
}

func (j *Judge) Enabled() bool { return j.URL != "" && j.Model != "" }

// FromSettings 从系统设置加载裁判配置
func FromSettings(st *store.Store) *Judge {
	j := &Judge{}
	j.URL, _ = st.GetSetting("judge_api_url")
	j.Model, _ = st.GetSetting("judge_model")
	if enc, ok := st.GetSetting("judge_key_enc"); ok {
		j.Key, _ = st.DecryptSecret(enc)
	}
	return j
}

const promptTemplate = `你是严格的评测裁判。给定一道题、参考答案/评分要点和一个模型的回答,请判定回答质量。

【题目】
%s

【参考答案/评分要点】
%s

【被测模型的回答】
%s

要求:
1. 仅依据参考答案/评分要点判定,不要引入额外知识苛责;
2. 无参考答案时,按常识与逻辑自洽性判定;
3. 只输出 JSON,不要任何其他文字:{"verdict":"pass|partial|fail","reason":"一句话理由"}`

// Evaluate 评判一条回答,返回 (verdict, reason, error)
func (j *Judge) Evaluate(ctx context.Context, question, expected, answer string) (string, string, error) {
	if !j.Enabled() {
		return "", "", fmt.Errorf("裁判模型未配置")
	}
	if expected == "" {
		expected = "(未提供)"
	}
	prompt := fmt.Sprintf(promptTemplate, question, expected, truncateRunes(answer, 4000))
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	content, err := llmclient.New().ChatSync(ctx, j.URL, &llmclient.ChatRequest{
		Model: j.Model, Prompt: prompt, APIKey: j.Key, MaxTokens: 4096, // 思维链裁判需要足够预算(512 会被思考耗尽,任务54事故)
	})
	if err != nil {
		return "", "", err
	}
	return parseVerdict(content)
}

func parseVerdict(content string) (string, string, error) {
	// 容忍模型在 JSON 外包裹文字/围栏
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end <= start {
		return "", "", fmt.Errorf("裁判输出非 JSON: %s", truncateRunes(content, 120))
	}
	var v struct {
		Verdict string `json:"verdict"`
		Reason  string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(content[start:end+1]), &v); err != nil {
		return "", "", fmt.Errorf("裁判 JSON 解析失败: %v", err)
	}
	switch v.Verdict {
	case VerdictPass, VerdictPartial, VerdictFail:
	default:
		return "", "", fmt.Errorf("裁判 verdict 非法: %s", v.Verdict)
	}
	return v.Verdict, v.Reason, nil
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
