// NIAH 检索测试(V2.13 重构:宣称值验证 + 未知上限探测;V2.7 宣称扫描 / V2.5 老矩阵保留兼容)
//
// 两种主模式(与用户对齐的设计定稿):
//   填了上下文长度 → 宣称值验证:硬上限预检 + 长度{50%,75%,100%} × 埋针深度{25%,50%,75%} × 2 次,
//     全过判「与宣称一致」,否则报出从多少 K 开始丢针。
//   未填上下文 → 探测模式:8K 起逐档翻倍(默认上限 1M),首个失败点后二分细化(±5%),
//     边界点 3 次 × 3 深度复测确认。输出硬上限 + 有效上下文两个数。
//
// 探针三态(核心修复):命中 / 未命中(接收但没找到) / 错误(拒绝·截断·请求失败),
// 错误不参与命中判定——「装不下」和「找不到」不再混为一谈。
// 文本按 token 精确截取:校准实测 tokens/字符比,探针后用服务端实际 usage 自适应修正。
//
// 样本约定:phase="niah",concurrency=点位标识(verify=长度百分比;discover=K 数;legacy=埋针深度),
// meta={"mode","pct","depth_pct","tokens","needle","outcome","thinking_hit"}
package runner

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// 探针三态
const (
	outHit = iota
	outMiss
	outError
)

func outcomeCN(o int) string {
	switch o {
	case outHit:
		return "命中"
	case outMiss:
		return "未命中"
	default:
		return "错误"
	}
}

// embedAtDepth 在文本 depth% 处埋针(句读级:插入一个完整句子)
func embedAtDepth(text, needle string, depth int) string {
	rs := []rune(text)
	if len(rs) < 50 {
		return text + "\n本文隐藏的密钥是:" + needle + "。"
	}
	pos := len(rs) * depth / 100
	if pos < 10 {
		pos = 10
	}
	if pos > len(rs)-10 {
		pos = len(rs) - 10
	}
	return string(rs[:pos]) + "本文隐藏的密钥是:" + needle + "。" + string(rs[pos:])
}

const niahPrompt = "请通读以下文章,然后回答问题:本文隐藏的密钥是什么?请只回答密钥本身,不要输出任何其他内容。\n\n"

// niahReserve 提问句 + 输出预算的预留 tokens(避免点位文本顶满上下文导致截尾/拒收)
const niahReserve = 640

// niahTextSource token 精确的文本源:校准实测 tokens/字符比,探针后用实际 usage 自适应修正
type niahTextSource struct {
	base  []rune  // 语料全文(已剥掉压测续写指令)
	ratio float64 // tokens / rune
}

func newNiahTextSource(baseText string, ratio float64) *niahTextSource {
	base := []rune(strings.TrimSuffix(baseText, LoadTestSuffix))
	if ratio <= 0 {
		ratio = 1.0
	}
	return &niahTextSource{base: base, ratio: ratio}
}

// textFor 按目标 tokens 取文本(不足截取,超出循环拼接语料),返回文本与所用字符数
func (s *niahTextSource) textFor(tokens int) (string, int) {
	n := int(float64(tokens) / s.ratio)
	if n < 50 {
		n = 50
	}
	if n <= len(s.base) {
		return string(s.base[:n]), n
	}
	var b strings.Builder
	for utf8.RuneCountInString(b.String()) < n {
		b.WriteString(string(s.base))
	}
	rs := []rune(b.String())
	return string(rs[:n]), n
}

// refine 探针返回实际 prompt_tokens 后修正比例(截断/错误样本不用)
func (s *niahTextSource) refine(actualTokens, runesUsed int) {
	if actualTokens > 0 && runesUsed >= 50 {
		s.ratio = float64(actualTokens) / float64(runesUsed)
	}
}

// niahProbe 三态探针:发问 → 判定(错误/截断不计入未命中) → 落 meta;返回三态与实际 prompt tokens
func (r *Runner) niahProbe(ec *execContext, text, needle, tier string, point, rep, targetTokens, depthPct int, mode string) (int, int) {
	res, err := r.doRequest(ec, niahPrompt+text, "niah", tier, point, rep, true, 512)
	outcome, thinkHit, actualPT := outMiss, false, 0
	switch {
	case err != nil:
		outcome = outError
		r.log(ec.task.ID, "warn", fmt.Sprintf("[%s] 点位 %v 第 %d 次请求失败(不计入未命中): %v", tier, pointLabel(point, mode), rep, err))
	case res != nil && targetTokens > 0 && res.PromptTokens > 0 &&
		float64(res.PromptTokens) < float64(targetTokens)*0.9:
		outcome = outError // 服务端截断:实际吃进的 tokens 远少于目标
		actualPT = res.PromptTokens
		r.log(ec.task.ID, "warn", fmt.Sprintf(
			"[%s] 点位 %v 疑似截断:实际 %d tokens < 目标 %d(记错误,不参与边界判定)", tier, pointLabel(point, mode), res.PromptTokens, targetTokens))
	case res != nil && strings.Contains(res.Content, needle):
		outcome = outHit
	case res != nil && strings.Contains(res.Reasoning, needle):
		outcome, thinkHit = outHit, true // 想到但没说出(预算耗尽),记命中并标注
		r.log(ec.task.ID, "warn", fmt.Sprintf("[%s] 点位 %v 第 %d 次思维链命中但正文为空(已计命中,标注)", tier, pointLabel(point, mode), rep))
	default:
		if res != nil {
			actualPT = res.PromptTokens
		}
		r.log(ec.task.ID, "warn", fmt.Sprintf("[%s] 点位 %v 第 %d 次未找回针 %s", tier, pointLabel(point, mode), rep, needle))
	}
	if res != nil && actualPT == 0 {
		actualPT = res.PromptTokens
	}
	meta, _ := json.Marshal(map[string]any{
		"mode": mode, "pct": point, "depth_pct": depthPct, "tokens": targetTokens,
		"needle": needle, "outcome": outcomeCN(outcome), "hit": outcome == outHit, "thinking_hit": thinkHit,
	})
	if ec.lastSampleID > 0 {
		r.st.Exec(`UPDATE request_samples SET meta=? WHERE id=?`, string(meta), ec.lastSampleID)
	}
	return outcome, actualPT
}

func pointLabel(point int, mode string) string {
	if mode == "discover" {
		return fmt.Sprintf("%dK", point)
	}
	return fmt.Sprintf("%d%%", point)
}

// runNIAH 入口:宣称值>0 走宣称扫描(V2.7 保留);填了上下文 → 宣称验证;未填 → 探测模式;否则老矩阵(兼容旧任务)
func (r *Runner) runNIAH(ec *execContext) error {
	if ec.params.Exec.NIAHClaimedK > 0 {
		return r.runNIAHScan(ec)
	}
	if ec.task.ContextLen > 0 {
		return r.runNIAHVerify(ec)
	}
	return r.runNIAHDiscover(ec)
}

// ---------- V2.13 模式一:宣称值验证(填了上下文长度) ----------

func (r *Runner) runNIAHVerify(ec *execContext) error {
	// V2.20:只下浮不上浮——目标按宣称值 95% 封顶,留足 tokenizer 差异与输出预算
	// (任务111 事故:宣称 262144,预检发 257333+max_tokens 被服务端 400 拒)
	ceiling := ec.task.ContextLen
	targetCeiling := int(float64(ceiling) * 0.95)
	ceilingTier := TokensToTier(targetCeiling)
	r.log(ec.task.ID, "info", fmt.Sprintf(
		"NIAH 宣称值验证:宣称上下文 %d tokens;预检/探测目标下浮至 %d(%s);长度{50%%,75%%,100%%} × 深度{25%%,50%%,75%%} × 2 次",
		ceiling, targetCeiling, ceilingTier))

	// 硬上限预检:校准上限长度(校准即一次真实上限请求)
	ct, err := ec.cali.Calibrate(ec.ctx, ceilingTier)
	if err != nil {
		// V2.20:预检失败标 failed 并写明原因(不再标 done 留空数据)
		return fmt.Errorf("NIAH 硬上限预检失败:服务端无法接收 %s(%v);宣称上下文可能不达标", ceilingTier, err)
	}
	if ct.mismatch && ct.tokens < int(float64(targetCeiling)*0.95) {
		return fmt.Errorf("NIAH 硬上限预检截断:目标 %d tokens,服务端实际只接受 %d tokens;宣称上下文可能不达标", targetCeiling, ct.tokens)
	}
	r.log(ec.task.ID, "info", fmt.Sprintf("硬上限预检通过:服务端接受 %d tokens", ct.tokens))

	src := newNiahTextSource(ct.text, float64(ct.tokens)/float64(utf8.RuneCountInString(ct.text)))
	lengths := []int{50, 75, 100}
	depths := []int{25, 50, 75}
	const reps = 2
	ec.progTotal = len(lengths) * len(depths) * reps

	failAt := map[int]bool{}
	for _, L := range lengths {
		target := targetCeiling * L / 100
		text, used := src.textFor(target - niahReserve)
		for _, dp := range depths {
			for rep := 1; rep <= reps; rep++ {
				if ec.ctx.Err() != nil {
					return ec.ctx.Err()
				}
				needle := "LWZ" + randomPrefix(5)
				out, apt := r.niahProbe(ec, embedAtDepth(text, needle, dp), needle,
					TokensToTier(target), L, rep, target, dp, "verify")
				if out != outError && apt > 0 {
					src.refine(apt, used)
				}
				if out != outHit {
					failAt[L] = true
				}
				ec.progCnt++
				r.progress(ec, "NIAH 检索测试")
				select {
				case <-ec.ctx.Done():
					return ec.ctx.Err()
				case <-time.After(300 * time.Millisecond):
				}
			}
		}
	}
	if len(failAt) == 0 {
		r.log(ec.task.ID, "info", fmt.Sprintf(
			"NIAH 宣称值验证结论:与宣称一致——%s 全部长度点位 × 深度均命中", ceilingTier))
	} else {
		minFail := 101
		for L := range failAt {
			if L < minFail {
				minFail = L
			}
		}
		r.log(ec.task.ID, "warn", fmt.Sprintf(
			"NIAH 宣称值验证结论:宣称不达标——从 %s(宣称值的 %d%%)开始丢针",
			TokensToTier(ceiling*minFail/100), minFail))
	}
	return nil
}

// ---------- V2.13 模式二:探测模式(未填上下文,最高百万级) ----------

func (r *Runner) runNIAHDiscover(ec *execContext) error {
	capK := ec.params.Exec.NIAHMaxK
	if capK <= 0 {
		capK = 1000 // 默认探到 1M
	}
	capTokens := capK * 1000

	// 语料底:最大档;token 比例:8K 档位校准一次(便宜),探针中自适应修正
	corpus, err := ec.cali.corpusForTier(TokensToTier(capTokens))
	if err != nil || corpus == "" {
		return fmt.Errorf("NIAH 探测无可用语料: %v", err)
	}
	ct8k, err := ec.cali.Calibrate(ec.ctx, "8K")
	if err != nil {
		return fmt.Errorf("NIAH 探测基线校准失败: %w", err)
	}
	src := newNiahTextSource(corpus, float64(ct8k.tokens)/float64(utf8.RuneCountInString(ct8k.text)))
	r.log(ec.task.ID, "info", fmt.Sprintf(
		"NIAH 探测模式:8K 起逐档翻倍,上限 %s;首个失败点二分细化(±5%%),边界点 3 次 × 3 深度复测", TokensToTier(capTokens)))

	// 点位用 K 数记入 concurrency(discover 无上下文百分比基准)
	probeAt := func(tok int) int {
		text, used := src.textFor(tok - niahReserve)
		needle := "LWZ" + randomPrefix(5)
		out, apt := r.niahProbe(ec, embedAtDepth(text, needle, 50), needle,
			TokensToTier(tok), tok/1000, 1, tok, 50, "discover")
		if out != outError && apt > 0 {
			src.refine(apt, used)
		}
		ec.progCnt++
		r.progress(ec, "NIAH 检索测试")
		return out
	}

	// 1. 翻倍摸底
	ec.progTotal = 8 + 6 + 9 // 摸底约 8 + 二分约 6 + 确认 9(上界)
	passTok, failTok, failKind := 0, 0, ""
	for tok := 8000; tok <= capTokens; tok *= 2 {
		if ec.ctx.Err() != nil {
			return ec.ctx.Err()
		}
		switch probeAt(tok) {
		case outHit:
			passTok = tok
		case outMiss:
			failTok, failKind = tok, "miss"
		default:
			failTok, failKind = tok, "error"
		}
		if failTok > 0 {
			break
		}
		select {
		case <-ec.ctx.Done():
			return ec.ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
	if failTok == 0 {
		r.log(ec.task.ID, "info", fmt.Sprintf(
			"NIAH 探测结论:直至上限 %s 仍全部命中,有效上下文 ≥ %s(已达探测上限,可调大 niah_max_k 再探)",
			TokensToTier(capTokens), TokensToTier(capTokens)))
		return nil
	}
	if failKind == "error" {
		r.log(ec.task.ID, "warn", fmt.Sprintf(
			"硬上限:服务端在 %s 处拒绝/截断。有效上下文受硬上限限制,不超过该值", TokensToTier(failTok)))
	} else {
		r.log(ec.task.ID, "info", fmt.Sprintf(
			"翻倍摸底:最后通过 %s,首个丢针 %s,开始二分细化", TokensToTier(passTok), TokensToTier(failTok)))
	}

	// 2. 二分细化(±5% 或 ±8K 收敛)
	lo, hi := passTok, failTok
	for hi-lo > 8000 && float64(hi-lo) > float64(lo)*0.05 {
		if ec.ctx.Err() != nil {
			return ec.ctx.Err()
		}
		mid := (lo + hi) / 2
		if probeAt(mid) == outHit {
			lo = mid
		} else {
			hi = mid
		}
		select {
		case <-ec.ctx.Done():
			return ec.ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}

	// 3. 边界点复测确认:3 次 × 深度 {25%,50%,75%}
	depths := []int{25, 50, 75}
	hits, total := 0, 0
	for rep := 1; rep <= 3; rep++ {
		for _, dp := range depths {
			if ec.ctx.Err() != nil {
				return ec.ctx.Err()
			}
			text, used := src.textFor(lo - niahReserve)
			needle := "LWZ" + randomPrefix(5)
			out, apt := r.niahProbe(ec, embedAtDepth(text, needle, dp), needle,
				TokensToTier(lo), lo/1000, rep, lo, dp, "discover")
			if out != outError && apt > 0 {
				src.refine(apt, used)
			}
			if out != outError {
				total++
				if out == outHit {
					hits++
				}
			}
			ec.progCnt++
			r.progress(ec, "NIAH 检索测试")
			select {
			case <-ec.ctx.Done():
				return ec.ctx.Err()
			case <-time.After(300 * time.Millisecond):
			}
		}
	}
	if failKind == "error" {
		r.log(ec.task.ID, "info", fmt.Sprintf(
			"NIAH 探测结论:硬上限约 %s;有效上下文 ≥ %s(受硬上限限制,边界点复测 %d/%d 命中)",
			TokensToTier(failTok), TokensToTier(lo), hits, total))
	} else {
		r.log(ec.task.ID, "info", fmt.Sprintf(
			"NIAH 探测结论:实测有效上下文约 %s(边界点复测 %d/%d 命中;深度 25%%/50%%/75%%)",
			TokensToTier(lo), hits, total))
	}
	return nil
}

// ---------- V2.7 宣称扫描(保留兼容:填了 NIAH 宣称 K 值时) ----------

func (r *Runner) runNIAHScan(ec *execContext) error {
	claimed := ec.params.Exec.NIAHClaimedK // 单位 K
	step := ec.params.Exec.NIAHStepPct
	if step <= 0 {
		step = 1
	}
	depth := ec.params.Exec.NIAHDepthPct
	if depth <= 0 || depth > 100 {
		depth = 50
	}
	repeats := ec.params.Exec.NIAHRepeats
	if repeats <= 0 {
		repeats = 3
	}
	var pcts []int
	for p := step; p <= 100; p += step {
		pcts = append(pcts, p)
	}
	pcts = append(pcts, 110)
	ec.progTotal = len(pcts) * repeats
	r.log(ec.task.ID, "info", fmt.Sprintf(
		"NIAH 宣称扫描:宣称 %dK,步进 %d%%,共 %d 点位 × %d 次,埋针深度 %d%%",
		claimed, step, len(pcts), repeats, depth))

	for _, pct := range pcts {
		if ec.ctx.Err() != nil {
			return ec.ctx.Err()
		}
		targetTokens := claimed * 1000 * pct / 100
		tier := TokensToTier(targetTokens)
		if ec.task.ContextLen > 0 && targetTokens+512 > ec.task.ContextLen {
			r.log(ec.task.ID, "warn", fmt.Sprintf("[%s] 点位 %d%% 超出任务上下文设置 %d,跳过", tier, pct, ec.task.ContextLen))
			continue
		}
		ct, err := ec.cali.Calibrate(ec.ctx, tier)
		if err != nil {
			r.log(ec.task.ID, "error", fmt.Sprintf("[%s] 校准失败,跳过点位 %d%%: %v", tier, pct, err))
			continue
		}
		baseText := strings.TrimSuffix(ct.text, LoadTestSuffix)
		for rep := 1; rep <= repeats; rep++ {
			if ec.ctx.Err() != nil {
				return ec.ctx.Err()
			}
			needle := "LWZ" + randomPrefix(5)
			text := embedAtDepth(baseText, needle, depth)
			r.niahProbe(ec, text, needle, tier, pct, rep, ct.tokens, depth, "scan")
			ec.progCnt++
			r.progress(ec, "NIAH 检索测试")
			select {
			case <-ec.ctx.Done():
				return ec.ctx.Err()
			case <-time.After(300 * time.Millisecond):
			}
		}
	}
	return nil
}

// ---------- V2.5 老矩阵(深度×档位,兼容旧任务) ----------

func (r *Runner) runNIAHLegacy(ec *execContext) error {
	tiers := ec.params.Exec.NIAHTiers
	if len(tiers) == 0 {
		tiers = ec.params.Exec.LadderTiers
	}
	depths := ec.params.Exec.NIAHDepths
	if len(depths) == 0 {
		depths = []int{10, 30, 50, 70, 90}
	}
	repeats := ec.params.Exec.NIAHRepeats
	if repeats <= 0 {
		repeats = 3
	}
	ec.progTotal = len(tiers) * len(depths) * repeats
	r.log(ec.task.ID, "info", fmt.Sprintf("NIAH 检索测试(旧版矩阵):%d 档 × %d 深度 × %d 次", len(tiers), len(depths), repeats))

	for _, tier := range tiers {
		if ec.ctx.Err() != nil {
			return ec.ctx.Err()
		}
		target, err := TierTokens(tier)
		if err != nil {
			r.log(ec.task.ID, "warn", err.Error())
			continue
		}
		if ec.task.ContextLen > 0 && target+512 > ec.task.ContextLen {
			r.log(ec.task.ID, "warn", fmt.Sprintf("[%s] NIAH 目标 %d tokens 超出上下文 %d,跳过", tier, target, ec.task.ContextLen))
			continue
		}
		ct, err := ec.cali.Calibrate(ec.ctx, tier)
		if err != nil {
			r.log(ec.task.ID, "error", fmt.Sprintf("[%s] 校准失败,跳过 NIAH 该档: %v", tier, err))
			continue
		}
		baseText := strings.TrimSuffix(ct.text, LoadTestSuffix)
		for _, depth := range depths {
			for rep := 1; rep <= repeats; rep++ {
				if ec.ctx.Err() != nil {
					return ec.ctx.Err()
				}
				needle := "LWZ" + randomPrefix(5)
				text := embedAtDepth(baseText, needle, depth)
				r.niahProbe(ec, text, needle, tier, depth, rep, ct.tokens, depth, "legacy")
				ec.progCnt++
				r.progress(ec, "NIAH 检索测试")
				select {
				case <-ec.ctx.Done():
					return ec.ctx.Err()
				case <-time.After(300 * time.Millisecond):
				}
			}
		}
	}
	return nil
}
