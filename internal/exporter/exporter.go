// Package exporter Excel 生成(excelize,§7.7)。未执行/未采集项标注,不留空白。
package exporter

import (
	"bytes"
	"fmt"
	"time"
	"github.com/xuri/excelize/v2"
	"llmhwtest/internal/stats"
	"llmhwtest/internal/store"
)

const (
	notTested    = "未测试"
	notCollected = "未采集"
)

func num(v float64, digits int) any {
	if v <= 0 {
		return notTested
	}
	s := fmt.Sprintf("%.*f", digits, v)
	var f float64
	fmt.Sscanf(s, "%g", &f)
	return f
}

// numIf 有数据时保留 0 值(如失败率 0%/衰减 0%),无数据才标「未测试」
// 修复:num() 把合法的 0 一律显示成「未测试」,语义相反
func numIf(v float64, digits int, hasData bool) any {
	if !hasData {
		return notTested
	}
	s := fmt.Sprintf("%.*f", digits, v)
	var f float64
	fmt.Sscanf(s, "%g", &f)
	return f
}

func ts(ms int64) string {
	if ms <= 0 {
		return "-"
	}
	return time.UnixMilli(ms).Format("2006-01-02 15:04:05")
}

func headerStyle(f *excelize.File) int {
	st, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"#E8EEF7"}, Pattern: 1},
		Alignment: &excelize.Alignment{Vertical: "center", WrapText: true},
		Border: []excelize.Border{
			{Type: "left", Color: "B0B0B0", Style: 1}, {Type: "right", Color: "B0B0B0", Style: 1},
			{Type: "top", Color: "B0B0B0", Style: 1}, {Type: "bottom", Color: "B0B0B0", Style: 1},
		},
	})
	return st
}

func writeHeader(f *excelize.File, sheet string, headers []string) {
	st := headerStyle(f)
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
		f.SetCellStyle(sheet, cell, cell, st)
	}
	f.SetRowHeight(sheet, 1, 22)
}

func setRow(f *excelize.File, sheet string, row int, vals []any) {
	for i, v := range vals {
		cell, _ := excelize.CoordinatesToCellName(i+1, row)
		f.SetCellValue(sheet, cell, v)
	}
}

func statusCN(s string) string {
	switch s {
	case "pending":
		return "待执行"
	case "queued":
		return "排队中"
	case "running":
		return "执行中"
	case "done":
		return "已完成"
	case "failed":
		return "失败"
	case "cancelled":
		return "已取消"
	case "interrupted":
		return "中断"
	case "skipped":
		return "跳过"
	}
	return s
}

// ---------- 对比导出(§7.7) ----------

type compareCol struct {
	t   *store.Task
	sum stats.TaskSummary
}

func ComparePackage(st *store.Store, taskIDs []int64) (*bytes.Buffer, string, error) {
	f := excelize.NewFile()
	defer f.Close()
	var cols []compareCol
	for _, id := range taskIDs {
		t, err := st.GetTask(id)
		if err != nil {
			continue
		}
		cols = append(cols, compareCol{t, stats.Summary(st, id)})
	}
	label := func(t *store.Task) string {
		return fmt.Sprintf("%d %s/%s(%s)", t.ID, t.DeviceName, t.ModelName, orDash(t.Quant))
	}
	// sheet 1 对比汇总(行=指标,列=任务)
	f.SetSheetName("Sheet1", "对比汇总")
	hdr := []any{"指标"}
	for _, c := range cols {
		hdr = append(hdr, label(c.t))
	}
	setRow(f, "对比汇总", 1, hdr)
	metrics := []struct {
		name string
		get  func(c compareCol) any
	}{
		{"设备", func(c compareCol) any { return c.t.DeviceName }},
		{"模型", func(c compareCol) any { return c.t.ModelName }},
		{"量化精度", func(c compareCol) any { return orDash(c.t.Quant) }},
		{"上下文长度", func(c compareCol) any { return c.t.ContextLen }},
		{"1K 输出速度(tokens/s)", func(c compareCol) any { return num(c.sum.Speed1K, 2) }},
		{"最高档", func(c compareCol) any { return orDash(c.sum.TopTier) }},
		{"最高档 TTFT(ms)", func(c compareCol) any { return num(c.sum.TopTierTTFT, 1) }},
		{"峰值总吞吐(tokens/s)", func(c compareCol) any { return num(c.sum.PeakThroughput, 2) }},
		{"峰值并发", func(c compareCol) any { return intOrDash(c.sum.PeakConcurrency) }},
		{"性能衰减(%)", func(c compareCol) any { return numIf(c.sum.Degradation, 2, c.sum.DecayTested) }},
		{"质量抽查", func(c compareCol) any { return orDash(c.sum.Quality) }},
	}
	for i, m := range metrics {
		vals := []any{m.name}
		for _, c := range cols {
			vals = append(vals, m.get(c))
		}
		setRow(f, "对比汇总", i+2, vals)
	}
	f.SetColWidth("对比汇总", "A", "A", 24)
	for i := range cols {
		col, _ := excelize.ColumnNumberToName(i + 2)
		f.SetColWidth("对比汇总", col, col, 28)
	}
	// sheet 2~4 阶梯/并发/缓存合并长表(首列标注来源任务)
	f.NewSheet("阶梯明细")
	writeHeader(f, "阶梯明细", []string{"来源任务", "档位", "实际 prompt tokens", "TTFT 中位(ms)", "速度中位(tokens/s)", "ITL(ms/token)", "标注"})
	r := 2
	for _, c := range cols {
		rows := stats.Ladder(st, c.t.ID, int64(c.t.RunNo), 0)
		if len(rows) == 0 {
			setRow(f, "阶梯明细", r, []any{label(c.t), notTested, "", "", "", "", ""})
			r++
			continue
		}
		for _, lr := range rows {
			if lr.Skipped {
				setRow(f, "阶梯明细", r, []any{label(c.t), lr.Tier, "超出上下文限制", "", "", "", lr.Note})
			} else {
				setRow(f, "阶梯明细", r, []any{label(c.t), lr.Tier, lr.PromptTokens, num(lr.TtftMedian, 1), num(lr.SpeedMedian, 2), numIf(1000/lr.SpeedMedian, 1, lr.SpeedMedian > 0), orDash(lr.Note)})
			}
			r++
		}
	}
	autowidth(f, "阶梯明细", 7)
	f.NewSheet("并发明细")
	writeHeader(f, "并发明细", []string{"来源任务", "输入档", "并发档", "平均 TTFT(ms)", "P99 TTFT(ms)", "单请求速度", "ITL(ms/token)", "总吞吐", "失败率(%)", "标注"})
	r = 2
	for _, c := range cols {
		rows := stats.Concurrency(st, c.t.ID, int64(c.t.RunNo))
		if len(rows) == 0 {
			setRow(f, "并发明细", r, []any{label(c.t), notTested, "", "", "", "", "", "", "", ""})
			r++
			continue
		}
		for _, cr := range rows {
			if cr.CircuitTrip {
				setRow(f, "并发明细", r, []any{label(c.t), cr.Tier, cr.Level, "", "", "", "", "", "", cr.Note})
			} else {
				setRow(f, "并发明细", r, []any{label(c.t), cr.Tier, cr.Level, num(cr.AvgTTFT, 1), num(cr.P99TTFT, 1),
					num(cr.AvgSpeed, 2), numIf(1000/cr.AvgSpeed, 1, cr.AvgSpeed > 0), num(cr.Throughput, 2), numIf(cr.FailRate*100, 1, cr.Rounds > 0), orDash(cr.Note)})
			}
			r++
		}
	}
	autowidth(f, "并发明细", 10)
	f.NewSheet("缓存明细")
	writeHeader(f, "缓存明细", []string{"来源任务", "档位", "冷 TTFT(ms)", "热 TTFT(ms)", "提升倍数", "标注"})
	r = 2
	for _, c := range cols {
		rows := stats.Cache(st, c.t.ID, int64(c.t.RunNo))
		if len(rows) == 0 {
			setRow(f, "缓存明细", r, []any{label(c.t), notTested, "", "", "", ""})
			r++
			continue
		}
		for _, cr := range rows {
			setRow(f, "缓存明细", r, []any{label(c.t), cr.Tier, num(cr.ColdTTFT, 1), num(cr.HotTTFT, 1), num(cr.Improvement, 2), orDash(cr.Note)})
			r++
		}
	}
	autowidth(f, "缓存明细", 6)
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, "", err
	}
	return buf, fmt.Sprintf("对比结果_%s.xlsx", time.Now().Format("20060102_150405")), nil
}

func autowidth(f *excelize.File, sheet string, ncols int) {
	for i := 1; i <= ncols; i++ {
		col, _ := excelize.ColumnNumberToName(i)
		f.SetColWidth(sheet, col, col, 16)
	}
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func orName(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func deviceName(d *store.Device) string {
	if d == nil {
		return ""
	}
	return d.Name
}

func sharedCN(shared bool) string {
	if shared {
		return "共享"
	}
	return "独立"
}

func flagCN(f string) string {
	switch f {
	case "normal":
		return "正常"
	case "suspect":
		return "疑似异常"
	case "abnormal":
		return "异常"
	}
	return "-"
}

func yesNo(b bool) string {
	if b {
		return "是"
	}
	return "否"
}

func intOrDash(n int) any {
	if n <= 0 {
		return notTested
	}
	return n
}
