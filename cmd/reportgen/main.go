// reportgen 独立报告生成器(不启动服务,直接读库):
// 用于在远端主程序运行期间,从同一份 SQLite(WAL 并发读安全)离线生成 Word 报告预览。
// 用法: reportgen -data ./data -project 66 -out /tmp/report.docx
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"llmhwtest/internal/exporter"
	"llmhwtest/internal/httpd"
	"llmhwtest/internal/store"
)

func main() {
	dataDir := flag.String("data", "data", "数据目录(含 platform.db)")
	projectID := flag.Int64("project", 0, "项目 ID")
	out := flag.String("out", "", "输出 docx 路径")
	flag.Parse()
	if *projectID == 0 || *out == "" {
		fmt.Fprintln(os.Stderr, "用法: reportgen -data ./data -project <id> -out <file.docx>")
		os.Exit(2)
	}
	st, err := store.Open(filepath.Join(*dataDir, "platform.db"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "打开数据库失败:", err)
		os.Exit(1)
	}
	defer st.Close()
	buf, name, err := exporter.ProjectReportDOCX(st, *dataDir, *projectID, httpd.PlatformVersion)
	if err != nil {
		fmt.Fprintln(os.Stderr, "生成报告失败:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, buf.Bytes(), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "写入失败:", err)
		os.Exit(1)
	}
	fmt.Printf("已生成: %s(%s,%d 字节)\n", *out, name, buf.Len())
}
