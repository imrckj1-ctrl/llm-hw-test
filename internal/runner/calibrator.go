// 输入长度精确控制:校准算法(§6.2)。禁止按字符数估算 token。
package runner

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"llmhwtest/internal/llmclient"
	"llmhwtest/internal/store"
)

// TierTokens 档位目标 token 数("1K"→1000,"2.5K"→2500;V2.5 小数档;V2.7 M 后缀:1M=1000000)
func TierTokens(tier string) (int, error) {
	t := strings.ToUpper(strings.TrimSpace(tier))
	if strings.HasSuffix(t, "M") {
		f, err := strconv.ParseFloat(strings.TrimSuffix(t, "M"), 64)
		if err != nil || f <= 0 {
			return 0, fmt.Errorf("档位数值非法: %s", tier)
		}
		return int(math.Round(f * 1000000)), nil
	}
	if strings.HasSuffix(t, "K") {
		f, err := strconv.ParseFloat(strings.TrimSuffix(t, "K"), 64)
		if err != nil || f <= 0 {
			return 0, fmt.Errorf("档位数值非法: %s", tier)
		}
		return int(math.Round(f * 1000)), nil
	}
	return 0, fmt.Errorf("档位格式应为 数字+K/M,如 1K/2.5K/1M")
}

// TokensToTier token 数 → 档位标签(V2.7 扫描点位命名;与 TierTokens 互逆)
func TokensToTier(tokens int) string {
	trim := func(f float64, unit string) string {
		s := strconv.FormatFloat(f, 'f', 2, 64)
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
		return s + unit
	}
	if tokens >= 1000000 {
		return trim(float64(tokens)/1000000, "M")
	}
	return trim(float64(tokens)/1000, "K")
}

type calibratedText struct {
	text     string
	tokens   int
	mismatch bool // 3 次迭代仍未达标(§6.2)
}

// LoadTestSuffix 压测续写指令(V2.6):无指令长输入下思维链模型会提前收尾,
// 导致输出未满测不到速度;校准时并入文本,prompt_tokens 口径含本句
const LoadTestSuffix = "\n\n请接着上面的文字继续往下写,不要总结,不要评论,不要停下来提问。"

type calibrator struct {
	client *llmclient.Client
	st     *store.Store
	task   *store.Task
	params store.TaskParams
	apiKey string
	logf   func(level, msg string)
	cache  map[string]*calibratedText
}

func newCalibrator(c *llmclient.Client, st *store.Store, t *store.Task, p store.TaskParams, apiKey string, logf func(string, string)) *calibrator {
	return &calibrator{client: c, st: st, task: t, params: p, apiKey: apiKey, logf: logf, cache: map[string]*calibratedText{}}
}

// corpusForTier 取该档语料;自定义档位取不小于目标的最接近档,否则最大档
func (c *calibrator) corpusForTier(tier string) (string, error) {
	if c.task.CorpusVersionID <= 0 {
		return "", fmt.Errorf("任务未选择语料版本")
	}
	if txt, err := c.st.CorpusText(c.task.CorpusVersionID, tier); err == nil && txt != "" {
		return txt, nil
	}
	target, err := TierTokens(tier)
	if err != nil {
		return "", err
	}
	texts, err := c.st.CorpusTexts(c.task.CorpusVersionID, true)
	if err != nil || len(texts) == 0 {
		return "", fmt.Errorf("语料版本 %d 无任何档位文本", c.task.CorpusVersionID)
	}
	var best *store.CorpusText
	for _, t := range texts {
		tk, err := TierTokens(t.Tier)
		if err != nil {
			continue
		}
		if tk >= target && (best == nil) {
			best = t
			break
		}
	}
	if best == nil {
		best = texts[len(texts)-1] // 最大档
	}
	c.logf("info", fmt.Sprintf("档位 %s 无对应语料,复用 %s 档语料进行校准", tier, best.Tier))
	return best.Content, nil
}

// Calibrate §6.2:每(任务×档位)校准一次,冻结复用
func (c *calibrator) Calibrate(ctx context.Context, tier string) (*calibratedText, error) {
	if ct, ok := c.cache[tier]; ok {
		return ct, nil
	}
	target, err := TierTokens(tier)
	if err != nil {
		return nil, err
	}
	corpus, err := c.corpusForTier(tier)
	if err != nil {
		return nil, err
	}
	corpus += LoadTestSuffix // V2.6:压测请求统一带续写指令,并纳入 token 校准

	text := corpus
	var best *calibratedText
	bestDev := math.MaxFloat64

	// /tokenize 预截短:减少迭代(§6.2 可选)
	if n, err := c.client.Tokenize(ctx, c.task.APIBaseURL, c.task.ModelName, text, c.apiKey); err == nil && n > 0 {
		ratio := float64(target) / float64(n)
		if ratio < 1.0 {
			text = truncateRunes(text, int(float64(utf8.RuneCountInString(text))*ratio))
		}
		c.logf("info", fmt.Sprintf("[%s] /tokenize 预估 %d tokens,预调整后进入校准迭代", tier, n))
	}

	for i := 1; i <= 3; i++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		res, err := c.send(ctx, text, 5, "calibration", tier, i) // 校准只取极少输出(§6.2)
		if err != nil {
			return nil, fmt.Errorf("校准请求失败: %w", err)
		}
		if res.TokensEstimated {
			return nil, fmt.Errorf("被测服务未返回 usage,无法校准输入长度(§6.2)")
		}
		pt := res.PromptTokens
		// V2.22:prompt_tokens=0 时缩放比例为无穷大,resize 循环会挂起;直接判校准失败
		if pt <= 0 {
			return nil, fmt.Errorf("被测服务返回 prompt_tokens=0,无法校准输入长度(§6.2)")
		}
		dev := math.Abs(float64(pt-target)) / float64(target)
		c.logf("info", fmt.Sprintf("[%s] 校准迭代 %d: prompt_tokens=%d 目标=%d 偏差=%.2f%%", tier, i, pt, target, dev*100))
		if dev < bestDev {
			bestDev = dev
			best = &calibratedText{text: text, tokens: pt, mismatch: dev > 0.01}
		}
		if dev <= 0.01 { // 偏差 ≤1% 冻结(§6.2)
			c.cache[tier] = best
			return best, nil
		}
		ratio := float64(target) / float64(pt)
		// V2.22:钳制缩放比例,异常 usage 不再产生极端文本长度
		if ratio > 20 {
			ratio = 20
		} else if ratio < 0.05 {
			ratio = 0.05
		}
		text = resizeWithCorpus(text, corpus, ratio)
	}
	c.logf("warn", fmt.Sprintf("[%s] 3 次迭代未收敛,取最接近文本(偏差 %.2f%%),该档样本标注「长度未达标」", tier, bestDev*100))
	c.cache[tier] = best
	return best, nil
}

// send 单发请求并落样本(校准/预热/正式共用)
func (c *calibrator) send(ctx context.Context, text string, maxTokens int, phase, tier string, seq int) (*llmclient.Result, error) {
	p := c.params
	req := &llmclient.ChatRequest{
		Model: c.task.ModelName, Prompt: text,
		Temperature: p.Gen.Temperature, TopP: p.Gen.TopP, TopK: p.Gen.TopK,
		MaxTokens: maxTokens, IgnoreEOS: p.Advanced.IgnoreEOS, APIKey: c.apiKey,
	}
	to := time.Duration(p.Advanced.RequestTimeoutSec) * time.Second
	// V2.20:request_timeout_sec=0(API 直建未传时)不能 WithTimeout(0) 立刻死,回退默认 300s
	if to <= 0 {
		to = 300 * time.Second
	}
	// 校准请求与正式请求同超时口径(过载网关上 60s 不够)
	// 限流/过载/超时退避重试:校准不是测量,重试安全
	var res *llmclient.Result
	var err error
	backoffs := []time.Duration{2 * time.Second, 5 * time.Second}
	for attempt := 0; ; attempt++ {
		tctx, cancel := context.WithTimeout(ctx, to)
		res, err = c.client.ChatStream(tctx, c.task.APIBaseURL, req)
		cancel()
		retryable := (llmclient.Retryable(err) || isTimeout(err)) && !isServerDead(err)
		if err == nil || !retryable || attempt >= len(backoffs) || ctx.Err() != nil {
			break
		}
		c.logf("warn", fmt.Sprintf("校准请求失败(%s),%v 后重试(第 %d 次)", err, backoffs[attempt], attempt+1))
		select {
		case <-ctx.Done():
			return res, ctx.Err()
		case <-time.After(backoffs[attempt]):
		}
	}
	sm := &store.Sample{
		TaskID: c.task.ID, RunNo: c.task.RunNo, Phase: phase, Tier: tier, Concurrency: 1, Seq: seq,
		Status: "ok",
	}
	if res != nil {
		sm.SendTS, sm.FirstChunkTS, sm.EndTS = res.SendTS, res.FirstChunkTS, res.EndTS
		sm.TtftMs, sm.TotalMs = res.TtftMs(), res.TotalMs()
		sm.PromptTokens, sm.CompletionTokens = res.PromptTokens, res.CompletionTokens
		sm.TokensEstimated = res.TokensEstimated
		sm.Speed = res.Speed()
		sm.RawResponse = res.Content
	}
	if err != nil {
		sm.Status = "error"
		if isTimeout(err) {
			sm.Status = "timeout"
		}
		sm.Error = err.Error()
	}
	if _, ierr := c.st.InsertSample(sm); ierr != nil {
		c.logf("error", "样本入库失败: "+ierr.Error())
	}
	return res, err
}

func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "deadline exceeded") || strings.Contains(s, "Client.Timeout") || strings.Contains(s, "context deadline")
}

// resizeWithCorpus 按比例截断或循环拼接语料(§6.2:边界直接拼接,允许断句)
func resizeWithCorpus(text, corpus string, ratio float64) string {
	runes := []rune(text)
	target := int(float64(len(runes)) * ratio)
	if target < 1 {
		target = 1
	}
	if target <= len(runes) {
		return string(runes[:target])
	}
	corpusRunes := []rune(corpus)
	if len(corpusRunes) == 0 {
		return text
	}
	var b strings.Builder
	b.WriteString(text)
	i := 0
	// 计数推进,避免循环内反复 RuneCount 整个 builder(O(n²))
	for n := utf8.RuneCountInString(text); n < target; n++ {
		b.WriteRune(corpusRunes[i%len(corpusRunes)])
		i++
	}
	return b.String()
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if n >= len(r) {
		return s
	}
	return string(r[:n])
}
