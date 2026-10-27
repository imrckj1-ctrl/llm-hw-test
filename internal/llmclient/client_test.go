package llmclient

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// 回归:vLLM 0.27 思维链字段名是 delta.reasoning(非 reasoning_content),
// 不识别会导致 FirstAnyTS/TTFT 永远为 0(任务50事故)
func TestChatStreamVLLMReasoningField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		chunks := []string{
			`{"choices":[{"delta":{"role":"assistant","content":""}}]}`,
			`{"choices":[{"delta":{"reasoning":"想一想"}}]}`, // vLLM 新字段名
			`{"choices":[{"delta":{"reasoning_content":"老字段"}}]}`,
			`{"choices":[{"delta":{"content":"答案"}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
			`{"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":5}}`,
		}
		for _, c := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", c)
			fl.Flush()
			time.Sleep(5 * time.Millisecond)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	res, err := New().ChatStream(context.Background(), srv.URL+"/v1", &ChatRequest{Model: "m", Prompt: "hi", MaxTokens: 16})
	if err != nil {
		t.Fatal(err)
	}
	if res.FirstAnyTS == 0 {
		t.Fatal("FirstAnyTS 未记录(reasoning 字段未识别)")
	}
	if res.FirstChunkTS == 0 {
		t.Fatal("FirstChunkTS 未记录(content delta 未识别)")
	}
	if res.Reasoning != "想一想老字段" {
		t.Fatalf("Reasoning=%q,两种字段名都应采集", res.Reasoning)
	}
	if res.Content != "答案" {
		t.Fatalf("Content=%q", res.Content)
	}
	if res.PromptTokens != 10 || res.CompletionTokens != 5 {
		t.Fatalf("usage 未解析: pt=%d ct=%d", res.PromptTokens, res.CompletionTokens)
	}
	if res.TtftMs() <= 0 {
		t.Fatalf("TTFT 应 >0,得 %v", res.TtftMs())
	}
}
