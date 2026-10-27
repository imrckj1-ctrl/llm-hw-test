package runner

import (
	"strings"
	"testing"
)

// embedNeedle 回归:语料恰好 600 字符不再除零崩溃;位置真随机且避开首尾
func TestEmbedNeedle(t *testing.T) {
	for _, n := range []int{50, 100, 599, 600, 601, 700, 15000} {
		corpus := strings.Repeat("文", n)
		out := embedNeedle(corpus, "LWZabc12")
		if !strings.Contains(out, "LWZabc12") {
			t.Fatalf("len=%d 针未埋入", n)
		}
	}
	// 长语料:多次埋针位置应分布在 10%~90% 区间且不全相同
	corpus := strings.Repeat("字", 15000)
	seen := map[int]bool{}
	for i := 0; i < 20; i++ {
		out := embedNeedle(corpus, "LWZzzzz9")
		idx := strings.Index(out, "本文隐藏的密钥是:LWZzzzz9")
		if idx < 0 {
			t.Fatal("针未埋入")
		}
		pos := len([]rune(out[:idx]))
		if pos < 1500 || pos > 13500 {
			t.Fatalf("埋针位置 %d 越出 10%%~90%% 区间", pos)
		}
		seen[pos] = true
	}
	if len(seen) < 2 {
		t.Fatalf("20 次埋针位置全部相同,随机性失效: %v", seen)
	}
}

// autoScreen 自动初筛(§6.8):空回答/乱码/重复刷屏应命中,正常回答不误伤
func TestAutoScreen(t *testing.T) {
	cases := []struct {
		name   string
		answer string
		want   string
	}{
		{"空回答", "", "suspect"},
		{"纯空白", "   \n\t  ", "suspect"},
		{"单字符刷屏", strings.Repeat("哈", 200), "suspect"},
		{"短串刷屏", strings.Repeat("测试输出", 50), "suspect"},
		{"乱码占比高", strings.Repeat("正常", 10) + strings.Repeat(string(rune(0xFFFD)), 30), "suspect"},
		{"控制字符乱码", strings.Repeat("ab", 20) + strings.Repeat("\x01\x02\x03", 5), "suspect"},
		{"正常中文回答", "根据题目所给条件,答案是 42。推导过程如下:首先计算前两项之和,再代入递推公式逐项求值,最终得到结果。", "normal"},
		{"正常英文回答", "The answer is 42. We derive it by applying the recurrence relation step by step until convergence.", "normal"},
	}
	for _, c := range cases {
		flag, reason := autoScreen(c.answer)
		if flag != c.want {
			t.Fatalf("%s: flag=%q want=%q reason=%q", c.name, flag, c.want, reason)
		}
	}
}
