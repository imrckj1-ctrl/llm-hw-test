package runner

import (
	"os"
	"path/filepath"
	"testing"
)

// normalizePiArtifact:登记目录口径 + 规范化文件名(document.<ext> / index.html)
func TestNormalizePiArtifact(t *testing.T) {
	dir := t.TempDir()
	wd := filepath.Join(dir, "artifacts", "pi", "task_9", "q_3")
	sub := filepath.Join(wd, "data", "out")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(sub, "工资表.xlsx"), []byte("fake-xlsx"), 0o644)
	os.WriteFile(filepath.Join(wd, "gen.py"), []byte("print(1)"), 0o644)

	// 文档类:优先匹配 xlsx,规范为 document.xlsx,返回目录路径
	rel, ok := normalizePiArtifact(dir, 9, 3, []string{"gen.py", "data/out/工资表.xlsx"}, "xlsx")
	if !ok || rel != "pi/task_9/q_3" {
		t.Fatalf("rel=%q ok=%v", rel, ok)
	}
	if _, err := os.Stat(filepath.Join(wd, "document.xlsx")); err != nil {
		t.Fatal("document.xlsx 未生成")
	}

	// html:规范为 index.html
	os.WriteFile(filepath.Join(sub, "page.html"), []byte("<html></html>"), 0o644)
	rel, ok = normalizePiArtifact(dir, 9, 3, []string{"data/out/page.html"}, "html")
	if !ok || rel != "pi/task_9/q_3" {
		t.Fatalf("html: rel=%q ok=%v", rel, ok)
	}
	if _, err := os.Stat(filepath.Join(wd, "index.html")); err != nil {
		t.Fatal("index.html 未生成")
	}

	// 空清单 / 源文件丢失
	if _, ok := normalizePiArtifact(dir, 9, 3, nil, "xlsx"); ok {
		t.Fatal("空清单不应 ok")
	}
	if _, ok := normalizePiArtifact(dir, 9, 3, []string{"不存在.xlsx"}, "xlsx"); ok {
		t.Fatal("源文件丢失不应 ok")
	}
}
