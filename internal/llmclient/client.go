// Package llmclient OpenAI 兼容 SSE 客户端,计时/token 口径见文档 §6.1
package llmclient

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	hc *http.Client
}

// HTTPError 非 200 响应;429/5xx 为可重试错误
type HTTPError struct {
	Code int
	Body string
}

func (e *HTTPError) Error() string { return fmt.Sprintf("HTTP %d: %s", e.Code, e.Body) }

// Retryable 限流/过载/服务端错误可重试
func Retryable(err error) bool {
	if he, ok := err.(*HTTPError); ok {
		return he.Code == 429 || he.Code >= 500
	}
	return false
}

func New() *Client {
	tr := &http.Transport{
		MaxIdleConns:        128,
		MaxIdleConnsPerHost: 128, // 连接池上限 ≥64(§6.4)
		IdleConnTimeout:     90 * time.Second,
		DialContext: (&net.Dialer{
			Timeout:   15 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		DisableCompression: true, // SSE 不压缩,保证首 chunk 计时准确
	}
	return &Client{hc: &http.Client{Transport: tr}}
}

type ChatRequest struct {
	Model       string
	Prompt      string
	Temperature float64
	TopP        float64
	TopK        int
	MaxTokens   int
	IgnoreEOS   bool
	APIKey      string // 选填,非空时携带 Authorization: Bearer
	// 思考模式(V2.2):各家字段不统一,按厂商常见字段发送,不支持的服务会忽略或报错(记日志)
	ReasoningEffort string         // low/medium/high(OpenAI 系)
	EnableThinking  *bool          // false 关闭思考(Qwen/vLLM 系)
	Extra           map[string]any // 自定义请求参数兜底
	// Messages 多轮对话(非空时优先于 Prompt;V2.20 缓存测试对话式叠加)
	Messages []map[string]string
}

// Result 单请求完整计时与 token 数据(§6.1)
type Result struct {
	SendTS       int64 // 请求发出时刻(unix ms,TTFT 起点)
	FirstChunkTS int64 // 首个 content 非空 delta 到达
	EndTS        int64 // finish_reason chunk 或 [DONE]
	FirstAnyTS   int64 // 首个任何非空 delta(content 或 reasoning_content)
	Content      string
	Reasoning        string // 思维链(reasoning_content,思维链模型)
	PromptTokens     int
	CompletionTokens int
	ReasoningTokens  int // completion 中的思维链 token
	CachedTokens     int    // prompt 命中缓存的 token(服务返回时)
	TokensEstimated  bool
	FinishReason     string
	RequestBody      string // 实际发送的请求体(全量日志用)
}

func (r *Result) TtftMs() float64 {
	ts := r.FirstChunkTS
	if ts == 0 {
		ts = r.FirstAnyTS // 无正文时以首个思维链 chunk 兜底
	}
	if ts == 0 {
		return 0
	}
	return float64(ts - r.SendTS)
}

func (r *Result) TotalMs() float64 {
	if r.EndTS == 0 {
		return 0
	}
	return float64(r.EndTS - r.SendTS)
}

// Speed 输出速度 = completion_tokens ÷ (末 chunk − 首 chunk)(附录 A)
// 思维链模型:窗口从首个任何输出(含思维链)起算,否则思考 token 会被算进正文窗口导致虚高
func (r *Result) Speed() float64 {
	start := r.FirstChunkTS
	if r.FirstAnyTS != 0 && (start == 0 || r.FirstAnyTS < start) {
		start = r.FirstAnyTS
	}
	d := r.EndTS - start
	if start == 0 || d <= 0 || r.CompletionTokens <= 0 {
		return 0
	}
	return float64(r.CompletionTokens) / (float64(d) / 1000.0)
}

// ReasoningOnly 全思维链无正文输出
func (r *Result) ReasoningOnly() bool {
	return r.FirstChunkTS == 0 && r.FirstAnyTS != 0
}

type streamDelta struct {
	Choices []struct {
		Delta struct {
			Role             string `json:"role"`
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
			Reasoning        string `json:"reasoning"` // vLLM 新版思维链字段名(任务50事故:只认 reasoning_content 导致 TTFT 全 0)
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *usageBlock `json:"usage"`
}

type usageBlock struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	Details          *struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"completion_tokens_details"`
	PromptDetails *struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

func chatURL(base string) string {
	base = strings.TrimRight(base, "/")
	if strings.HasSuffix(base, "/v1") {
		return base + "/chat/completions"
	}
	return base + "/v1/chat/completions"
}

// ChatStream 发起流式请求,严格按 §6.1 口径计时
func (c *Client) ChatStream(ctx context.Context, baseURL string, req *ChatRequest) (*Result, error) {
	body := map[string]any{
		"model": req.Model,
		"temperature": req.Temperature,
		"top_p":       req.TopP,
		"stream":      true,
		"stream_options": map[string]any{
			"include_usage": true, // 显式携带(§6.1)
		},
	}
	if len(req.Messages) > 0 {
		body["messages"] = req.Messages
	} else {
		body["messages"] = []map[string]string{
			{"role": "user", "content": req.Prompt},
		}
	}
	// V2.20:max_tokens < 0 → 请求不携带该字段(质量题:模型自行输出到截断为止)
	if req.MaxTokens >= 0 {
		body["max_tokens"] = req.MaxTokens
	}
	if req.TopK > 0 {
		body["top_k"] = req.TopK
	}
	if req.IgnoreEOS {
		body["ignore_eos"] = true // vLLM/SGLang 系支持(§6.6)
	}
	if req.ReasoningEffort != "" {
		body["reasoning_effort"] = req.ReasoningEffort
	}
	if req.EnableThinking != nil {
		body["enable_thinking"] = *req.EnableThinking
	}
	for k, v := range req.Extra {
		body[k] = v
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, chatURL(baseURL), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if req.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)
	}

	sendTS := time.Now().UnixMilli() // 请求发出时刻:TTFT 起点(含连接/排队,口径贴近真实等待)
	resp, err := c.hc.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, &HTTPError{Code: resp.StatusCode, Body: strings.TrimSpace(string(b))}
	}

	res := &Result{SendTS: sendTS}
	res.RequestBody = string(payload)

	var content strings.Builder
	var reasoning strings.Builder
	reader := bufio.NewReaderSize(resp.Body, 8192)
	var dataLines []string
	gotUsage := false
	doneSeen := false

	flushEvent := func() {
		if len(dataLines) == 0 {
			return
		}
		data := strings.Join(dataLines, "\n")
		dataLines = dataLines[:0]
		if data == "[DONE]" {
			if res.EndTS == 0 {
				res.EndTS = time.Now().UnixMilli()
			}
			doneSeen = true
			return
		}
		var d streamDelta
		if err := json.Unmarshal([]byte(data), &d); err != nil {
			return // 忽略无法解析的行(心跳/注释)
		}
		if d.Usage != nil {
			gotUsage = true
			res.PromptTokens = d.Usage.PromptTokens
			res.CompletionTokens = d.Usage.CompletionTokens
			if d.Usage.Details != nil {
				res.ReasoningTokens = d.Usage.Details.ReasoningTokens
			}
			if d.Usage.PromptDetails != nil {
				res.CachedTokens = d.Usage.PromptDetails.CachedTokens
			}
		}
		for _, ch := range d.Choices {
			rc := ch.Delta.ReasoningContent
			if rc == "" {
				rc = ch.Delta.Reasoning
			}
			if rc != "" || ch.Delta.Content != "" {
				if res.FirstAnyTS == 0 {
					res.FirstAnyTS = time.Now().UnixMilli() // 首个任何非空输出
				}
			}
			if rc != "" {
				reasoning.WriteString(rc)
			}
			if ch.Delta.Content != "" {
				if res.FirstChunkTS == 0 {
					res.FirstChunkTS = time.Now().UnixMilli() // 首个 content 非空 delta
				}
				content.WriteString(ch.Delta.Content)
			}
			if ch.FinishReason != nil && *ch.FinishReason != "" {
				res.EndTS = time.Now().UnixMilli()
				res.FinishReason = *ch.FinishReason
			}
		}
	}

	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			trimmed := strings.TrimRight(line, "\r\n")
			switch {
			case trimmed == "":
				flushEvent()
			case strings.HasPrefix(trimmed, "data:"):
				dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(trimmed, "data:")))
			case strings.HasPrefix(trimmed, ":"):
				// SSE 注释/心跳,忽略
			}
		}
		// 收尾:[DONE] 或 (finish_reason 且 usage 已收)即退出。
		// 不再傻等 EOF——服务端发完不关连接时会一路阻塞到超时,把成功请求误判 timeout
		if doneSeen || (res.EndTS != 0 && gotUsage) {
			break
		}
		if err != nil {
			if err == io.EOF {
				flushEvent()
				if res.EndTS == 0 && (res.FirstChunkTS != 0 || res.FirstAnyTS != 0) {
					res.EndTS = time.Now().UnixMilli()
				}
				break
			}
			if ctx.Err() != nil {
				return res, ctx.Err()
			}
			return res, fmt.Errorf("读取流失败: %w", err)
		}
	}

	res.Content = content.String()
	res.Reasoning = reasoning.String()

	// token 优先取 usage;缺失时估算并标记(§6.1)
	if res.PromptTokens == 0 && res.CompletionTokens == 0 {
		res.TokensEstimated = true
		res.PromptTokens = EstimateTokens(req.Prompt)
		res.CompletionTokens = EstimateTokens(res.Content)
	} else if res.CompletionTokens == 0 && res.Content != "" {
		res.TokensEstimated = true
		res.CompletionTokens = EstimateTokens(res.Content)
	}
	return res, nil
}

// EstimateTokens 估算口径:中文密集文本约 1.5 字符/token(附录 B)
func EstimateTokens(s string) int {
	n := len([]rune(s))
	est := (n*2 + 2) / 3
	if est < 1 && n > 0 {
		est = 1
	}
	return est
}

// Tokenize 尝试框架 /tokenize 接口(§6.2,可选,不支持时返回错误)
func (c *Client) Tokenize(ctx context.Context, baseURL, model, text, apiKey string) (int, error) {
	base := strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(base, "/v1") {
		base = strings.TrimSuffix(base, "/v1")
	}
	payload, _ := json.Marshal(map[string]string{"model": model, "prompt": text})
	tctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(tctx, http.MethodPost, base+"/tokenize", bytes.NewReader(payload))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var out struct {
		Count  int   `json:"count"`
		Tokens []int `json:"tokens"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return 0, err
	}
	if out.Count > 0 {
		return out.Count, nil
	}
	return len(out.Tokens), nil
}

// Ping 前置检查:模型服务可达性(用一次极短请求探测)
func (c *Client) Ping(ctx context.Context, baseURL, model, apiKey string) error {
	tctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	base := strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(base, "/v1") {
		base = strings.TrimSuffix(base, "/v1")
	}
	req, err := http.NewRequestWithContext(tctx, http.MethodGet, base+"/v1/models", nil)
	if err != nil {
		return err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("连接失败: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			return fmt.Errorf("服务返回 HTTP %d(鉴权失败,请在任务配置中填写 API Key)", resp.StatusCode)
		}
		return fmt.Errorf("服务返回 HTTP %d", resp.StatusCode)
	}
	return nil
}

// ChatSync 非流式调用(裁判模型评判用):返回正文与 usage
func (c *Client) ChatSync(ctx context.Context, baseURL string, req *ChatRequest) (string, error) {
	body := map[string]any{
		"model": req.Model,
		"messages": []map[string]string{
			{"role": "user", "content": req.Prompt},
		},
		"stream": false,
	}
	if req.Temperature > 0 {
		body["temperature"] = req.Temperature
	}
	if req.MaxTokens > 0 {
		body["max_tokens"] = req.MaxTokens
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, chatURL(baseURL), bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if req.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)
	}
	resp, err := c.hc.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", &HTTPError{Code: resp.StatusCode, Body: strings.TrimSpace(string(b))}
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
				Reasoning        string `json:"reasoning"` // vLLM 新版字段名
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("响应无 choices")
	}
	content := out.Choices[0].Message.Content
	if strings.TrimSpace(content) == "" {
		content = out.Choices[0].Message.ReasoningContent
	}
	if strings.TrimSpace(content) == "" {
		content = out.Choices[0].Message.Reasoning
	}
	return content, nil
}

// ProbeCacheHit 非流式缓存探测(V2.20):发一次 max_tokens=1 的同前缀请求,
// 返回 usage.prompt_tokens_details.cached_tokens 与 prompt_tokens。
// 背景:SSE 流式尾帧在部分网关上丢 prompt_tokens_details(任务111/128 实测 llmclient 流式收不到,
// 非流式一定有),缓存命中率测量改走非流式探测兜底。
func (c *Client) ProbeCacheHit(ctx context.Context, baseURL, model string, messages []map[string]string, apiKey string) (cached, prompt int, err error) {
	body := map[string]any{
		"model":      model,
		"messages":   messages,
		"max_tokens": 1,
		"stream":     false,
	}
	payload, _ := json.Marshal(body)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, chatURL(baseURL), bytes.NewReader(payload))
	if err != nil {
		return 0, 0, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := c.hc.Do(httpReq)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return 0, 0, &HTTPError{Code: resp.StatusCode, Body: strings.TrimSpace(string(b))}
	}
	var out struct {
		Usage struct {
			PromptTokens  int `json:"prompt_tokens"`
			PromptDetails *struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return 0, 0, err
	}
	if out.Usage.PromptDetails != nil {
		cached = out.Usage.PromptDetails.CachedTokens
	}
	return cached, out.Usage.PromptTokens, nil
}

// ListModels 按 OpenAI 标准拉取模型清单(「查询模型」按钮)
func (c *Client) ListModels(ctx context.Context, baseURL, apiKey string) ([]string, error) {
	tctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	base := strings.TrimRight(baseURL, "/")
	if !strings.HasSuffix(base, "/v1") {
		base += "/v1"
	}
	req, err := http.NewRequestWithContext(tctx, http.MethodGet, base+"/models", nil)
	if err != nil {
		return nil, err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("连接失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&out); err != nil {
		return nil, err
	}
	var ids []string
	for _, m := range out.Data {
		if m.ID != "" {
			ids = append(ids, m.ID)
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("服务返回的模型列表为空")
	}
	return ids, nil
}
