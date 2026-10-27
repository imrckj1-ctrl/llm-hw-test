package runner

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// niahTextSource 按 token 比例截取/扩展语料(V2.13)
func TestNIAHTextSource(t *testing.T) {
	base := strings.Repeat("灵思微致测试语料。", 1000) // 9000 字符
	src := newNiahTextSource(base+LoadTestSuffix, 0.5) // 0.5 token/字符
	if len(src.base) != 9000 {
		t.Fatalf("压测指令未剥掉: %d", len(src.base))
	}
	// 截取:1000 tokens → 2000 字符
	text, used := src.textFor(1000)
	if used != 2000 || utf8.RuneCountInString(text) != 2000 {
		t.Fatalf("截取错误: used=%d len=%d", used, utf8.RuneCountInString(text))
	}
	// 扩展:20000 tokens → 40000 字符(循环拼接)
	text, used = src.textFor(20000)
	if used != 40000 || utf8.RuneCountInString(text) != 40000 {
		t.Fatalf("扩展错误: used=%d len=%d", used, utf8.RuneCountInString(text))
	}
	// 自适应修正:实际 1500 tokens / 2000 字符 → 0.75
	src.refine(1500, 2000)
	if src.ratio != 0.75 {
		t.Fatalf("refine 错误: %v", src.ratio)
	}
	// 错误样本不参与修正
	src.refine(0, 2000)
	if src.ratio != 0.75 {
		t.Fatalf("refine 应忽略 0: %v", src.ratio)
	}
}

// embedAtDepth 埋针位置与完整性
func TestEmbedAtDepth(t *testing.T) {
	text := strings.Repeat("甲", 1000)
	out := embedAtDepth(text, "LWZ12345", 50)
	if !strings.Contains(out, "LWZ12345") {
		t.Fatal("针未埋入")
	}
	idx := utf8.RuneCountInString(out[:strings.Index(out, "LWZ12345")])
	if idx < 400 || idx > 600 { // rune 50% 附近
		t.Fatalf("埋针位置偏离: %d", idx)
	}
	// 超短文本兜底
	short := embedAtDepth("短文本", "LWZ99", 50)
	if !strings.Contains(short, "LWZ99") {
		t.Fatal("短文本埋针失败")
	}
}
