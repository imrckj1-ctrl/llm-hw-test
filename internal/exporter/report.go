// 正式 Word 测试报告(V2.12):版式对齐《大模型推理硬件性能测试报告-模板》样例 v4。
// 只放表格不放图表;首章"测试结论"留空人工填写;统计表横改竖(TTFT 一排、速度一排);
// 产物 xlsx/pptx/docx/pdf 原文件 OLE 嵌入(双击打开),其余类型打 zip 后嵌入。
package exporter

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"llmhwtest/internal/stats"
	"llmhwtest/internal/store"
)

const blankFill = "待填" // 平台没有的信息,留待用户填写

func f1(v float64) string { return strconv.FormatFloat(v, 'f', 1, 64) }
func f2(v float64) string { return strconv.FormatFloat(v, 'f', 2, 64) }

func distCells(d stats.DistStats) []string {
	return []string{f1(d.Avg), f1(d.P50), f1(d.P95), f1(d.P99), f1(d.Min), f1(d.Max)}
}

// itl ITL(ms/token) = 1000 ÷ TPS(decode 口径,不含 TTFT;V2.16)
func itl(avgSpeed float64) string {
	if avgSpeed > 0 {
		return f1(1000 / avgSpeed)
	}
	return "-"
}

func dstr(ms int64) string {
	if ms == 0 {
		return blankFill
	}
	return time.UnixMilli(ms).Format("2006-01-02")
}

// fmtCtxLen 上下文长度 K/M 展示(1024 进制,整除才缩写)
func fmtCtxLen(n int) string {
	if n <= 0 {
		return blankFill
	}
	if n%(1024*1024) == 0 {
		return fmt.Sprintf("%dM", n/(1024*1024))
	}
	if n%1024 == 0 {
		return fmt.Sprintf("%dK", n/1024)
	}
	return strconv.Itoa(n)
}

// tierKey 档位排序键(与 stats.tierOrder 同规则,支持 K/M 小数档)
func tierKey(t string) int {
	s := strings.ToUpper(strings.TrimSpace(t))
	mult := 1000
	if strings.HasSuffix(s, "M") {
		mult = 1000000
		s = strings.TrimSuffix(s, "M")
	} else {
		s = strings.TrimSuffix(s, "K")
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f <= 0 {
		return 1 << 30
	}
	return int(f * float64(mult))
}

// ---------- 任务参数快照(gen/exec/advanced 嵌套结构) ----------

type taskParams struct {
	Gen      map[string]any `json:"gen"`
	Exec     map[string]any `json:"exec"`
	Advanced map[string]any `json:"advanced"`
}

func parseParams(raw string) taskParams {
	var p taskParams
	json.Unmarshal([]byte(raw), &p)
	return p
}

func pval(m map[string]any, key string) string {
	if m == nil {
		return blankFill
	}
	v, ok := m[key]
	if !ok || v == nil {
		return blankFill
	}
	switch x := v.(type) {
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case string:
		if x == "" {
			return blankFill
		}
		return x
	case bool:
		return yesNo(x)
	default:
		return fmt.Sprint(x)
	}
}

func plist(m map[string]any, key string) string {
	if m == nil {
		return blankFill
	}
	v, ok := m[key].([]any)
	if !ok || len(v) == 0 {
		return blankFill
	}
	var parts []string
	for _, x := range v {
		parts = append(parts, fmt.Sprint(x))
	}
	return strings.Join(parts, " / ")
}

func cacheSwitchCN(s string) string {
	switch s {
	case "on":
		return "开"
	case "off":
		return "关"
	default:
		return "未知"
	}
}

// thinkingCN 思考等级展示(与任务表单选项一致;空/未知=默认)
func thinkingCN(s string) string {
	switch s {
	case "off":
		return "关闭(off)"
	case "minimal":
		return "极简(minimal)"
	case "low":
		return "低(low)"
	case "medium":
		return "中(medium)"
	case "high":
		return "高(high)"
	case "xhigh":
		return "极高(xhigh)"
	case "max":
		return "最高(max)"
	default:
		return "默认(不加字段)"
	}
}

// ProjectReportDOCX 生成项目正式测试报告(docx)。version 由调用方注入(避免包环)
func ProjectReportDOCX(st *store.Store, dataDir string, projectID int64, version string) (*bytes.Buffer, string, error) {
	p, err := st.GetProject(projectID)
	if err != nil {
		return nil, "", err
	}
	device, _ := st.GetDevice(p.DeviceID)
	tasks, _ := st.ListTasks(projectID, 0, "")

	d := newDocx()

	// ---------- 封面 ----------
	d.CoverTitle("大模型推理硬件性能测试报告")
	d.CoverSubtitle("模型 × 硬件平台 GPU / NPU 推理性能与质量摸底测试")
	devName := blankFill
	if device != nil {
		devName = device.Name
	}
	d.CoverInfo("编制部门", blankFill)
	d.CoverInfo("测试单位", blankFill)
	d.CoverInfo("硬件平台", devName)
	d.CoverInfo("测试日期", dstr(p.FinishedAt))
	d.CoverInfo("文档版本", "V1.0")
	d.CoverInfo("文档状态", "草稿")
	d.PageBreak()

	// ---------- 目录 ----------
	d.TOCTitle()
	chapters := []string{"一、测试结论", "二、测试概述", "三、硬件信息", "四、运行时栈与版本信息",
		"五、被测模型清单", "六、模型调用参数", "七、不同输入长度性能测试(阶梯)", "八、并发性能测试",
		"九、前缀缓存(开关缓存)测试", "十、NIAH 长上下文检索测试", "十一、长时稳定性测试",
		"十二、质量抽检", "附录 A 关键测试方法说明", "附录 B 版本记录"}
	d.RegisterChapters(chapters) // V2.17:目录可点击跳转
	for _, e := range chapters {
		d.TOCEntry(e)
	}
	d.PageBreak()

	// ---------- 一、测试结论(人工填写) ----------
	d.H1B("一、测试结论")
	d.PColor("(本章节由测试人员根据以下实测数据综合撰写)", "999999")
	d.BlankLines(6)

	// V2.18:交互式结果页网页直嵌(双击图标直接用浏览器打开可交互图表,无需解压)
	if html := buildResultsHTML(st, p, tasks); html != nil {
		d.Attach("交互式结果页(双击图标用浏览器打开)", "results.html", "html", html)
	}

	// ---------- 二、测试概述 ----------
	d.H1B("二、测试概述")
	d.P("本报告针对主流大模型在实际硬件平台上的推理表现进行系统测试。测试基于本硬件平台(被测模型版本)," +
		"通过统一的模型、统一的测试语料集群和统一的调用参数,检验硬件在不同上下文长度、不同并发压力" +
		"和不同缓存状态下的推理表现,为模型选型、容量规划和采购决策提供实测数据。")
	d.P("测试对象为不同输入并发下的推理响应能力,覆盖 1K / 10K / 20K / 30K 等档位输入长度," +
		"并对比前缀缓存(开关缓存)开启前后的差异。")
	d.Table([]string{"项目", "内容"}, [][]string{
		{"测试目的", "评估硬件平台的大模型推理性能(延迟 / 吞吐 / 长上下文)"},
		{"设备形态", orDash(deviceField(device, "form"))},
		{"编制部门", blankFill},
		{"测试人员", orDash(p.Tester)},
		{"测试地点", blankFill},
		{"环境说明", blankFill + "(机房温度 / 湿度 / 网络环境)"},
		{"被测设备", devName + "(设备型号 / 加速芯片型号及数量)"},
	})
	d.Caption("测试环境信息")

	d.H2("2.1 测试结果摘要")
	for _, t := range tasks {
		sum := stats.Summary(st, t.ID)
		decay := "-"
		if sum.DecayTested {
			decay = f2(sum.Degradation) + "%"
		}
		eff := niahEffective(st, t.ID, int64(t.RunNo))
		rows := [][]string{
			{"稳定运行最高输入档位", orDash(sum.TopTier)},
			{"1K 档生成速度(tokens/s)", f2(sum.Speed1K)},
			{fmt.Sprintf("最高档(%s)首 token 延迟 TTFT(ms)", orDash(sum.TopTier)), f1(sum.TopTierTTFT)},
			{"峰值总吞吐(tokens/s)@并发数", fmt.Sprintf("%s @ %d", f2(sum.PeakThroughput), sum.PeakConcurrency)},
			{"总调用次数(成功/失败)", fmt.Sprintf("%d(成功 %d / 失败 %d)", sum.TotalCalls, sum.OkCalls, sum.FailCalls)},
			{"长时压测性能衰减", decay},
			{"NIAH 有效上下文", eff},
			{"质量抽检结论", orDash(sum.Quality)},
			{"一句话结论", blankFill},
		}
		if len(tasks) > 1 {
			d.H3(fmt.Sprintf("任务 %d(%s)", t.ID, t.ModelName))
		}
		d.Table([]string{"指标", "数值"}, rows)
	}
	d.Caption("测试结果摘要")

	// ---------- 三、硬件信息 ----------
	// V2.12:hardware 项目读 hw_json 快照(算力设备 + 测试整机两区),老 device 档案项目照旧
	hw := parseHwJSON(p.HwJSON)
	d.H1B("三、硬件信息")
	if device == nil && hw == nil {
		d.P("被测设备的基础配置如下表所示。本项目未关联设备档案,请按实际形态填写。")
	} else {
		d.P("被测设备的基础配置如下表所示。")
	}
	if hw != nil {
		hv := func(k string) string { return orDash(hwStr(hw, k)) }
		d.H2("3.1 算力设备信息")
		d.Table([]string{"项目", "型号 / 规格", "备注"}, [][]string{
			{"设备名称", hv("name"), ""},
			{"设备形态", hv("form_factor"), "AI 盒子 / PCIe 加速卡 / M.2 算力卡等"},
			{"AI 加速芯片", hv("chip_model"), "型号,算力(TOPS@INT8 / TFLOPS@FP16)"},
			{"标称算力", fmt.Sprintf("%s TOPS@INT8 / %s TFLOPS@FP16", hwNum(hw, "tops_int8"), hwNum(hw, "tflops_fp16")), ""},
			{"显存", hwVram(hw), "显存类型(HBM / GDDR6 / LPDDR5)、带宽(GB/s)"},
			{"主控 CPU / SoC", hv("cpu_soc"), "AI 盒子填 SoC 型号;插卡设备填主机 CPU"},
			{"散热方式", hv("cooling"), "被动散热 / 主动风冷 / 液冷方案"},
			{"标称 TDP", hwNum(hw, "tdp_w") + " W", "电源额定功率 / 整机 TDP(W)"},
			{"固件版本", hv("firmware"), "AI 盒子 / 加速卡固件版本"},
		})
		d.Caption("算力设备信息")
		d.H2("3.2 测试整机信息")
		d.Table([]string{"项目", "型号 / 规格", "备注"}, [][]string{
			{"整机形态", hv("machine_form"), "台式机 / 工作站 / 服务器 / miniPC / 笔记本 / 其他"},
			{"整机型号 / 平台", hv("machine_model"), ""},
			{"整机 CPU", hv("machine_cpu"), ""},
			{"整机内存", orDash(hwStr(hw, "machine_memory", "memory")), "容量与类型(DDR5 / LPDDR5 等)、频率"},
			{"整机硬盘", orDash(hwStr(hw, "machine_disk", "storage")), "系统盘 / 数据盘容量与读写速度"},
			{"操作系统及版本", hv("os_version"), "发行版及内核版本"},
		})
		d.Caption("测试整机信息")
	} else {
		dv := deviceFields(device)
		d.Table([]string{"项目", "型号 / 规格", "数量", "备注"}, [][]string{
			{"主控 CPU / SoC", dv["cpu"], blankFill, "AI 盒子填 SoC 型号;插卡设备填主机 CPU"},
			{"AI 加速芯片", dv["chip"], dv["chipCount"], "型号、数量,算力(TOPS@INT8 / TFLOPS@FP16)"},
			{"显存 / 统一内存", dv["vram"], "-", "显存类型(HBM / GDDR6 / LPDDR5)、带宽(GB/s)"},
			{"显存形态", dv["vramForm"], "-", "独立显存 / 与内存共享(AI 盒子常见)"},
			{"宿主平台(插卡填写)", dv["host"], "-", "主板型号、PCIe 代数与通道数"},
			{"内存", dv["memory"], blankFill, "容量与类型(DDR5 / LPDDR5 等)、频率"},
			{"存储", dv["storage"], blankFill, "系统盘 / 数据盘容量与读写速度"},
			{"散热方式", dv["cooling"], "-", "被动散热 / 主动风冷 / 液冷方案"},
			{"电源与整机功耗", dv["tdp"], "-", "电源额定功率 / 整机 TDP(W)"},
			{"网络与接口", dv["network"], blankFill, "网口速率;AI 盒子注明 USB / 串口等接口"},
		})
		d.Caption("硬件配置信息")
	}

	// ---------- 四、运行时栈 ----------
	d.H1B("四、运行时栈与版本信息")
	d.P("推理性能对硬件驱动和推理平台高度敏感,实测时按下表如实记录版本信息。" +
		"推理版本差异会影响数据,做横向对比时请保持运行时栈一致。")
	hvStr := func(k string) string {
		if hw == nil {
			return blankFill
		}
		return orDash(hwStr(hw, k))
	}
	d.Table([]string{"组件", "版本 / 取值", "备注"}, [][]string{
		{"操作系统", hvStr("os_version"), "发行版及内核版本"},
		{"GPU / NPU 驱动", hvStr("driver_ver"), "驱动版本号"},
		{"推理框架及版本", hvStr("framework"), "如 vLLM / SGLang / Ollama / llama.cpp"},
		{"加速 SDK / 算子库", hvStr("sdk_ver"), "如 CANN / RKNN / TensorRT / 厂商私有 SDK 版本"},
		{"固件版本", hvStr("firmware"), "AI 盒子 / 加速卡固件版本"},
		{"压测工具", "灵思微致测试平台 " + version, "阶梯 / 并发 / 缓存 / 稳定性 / NIAH / 质量 六项"},
	})
	d.Caption("运行时栈信息")
	if hw != nil {
		if cmd := hwStr(hw, "startup_cmd"); cmd != "" {
			d.H3("模型启动命令")
			d.NoteBox(cmd)
		}
		if note := hwStr(hw, "env_note"); note != "" {
			d.H3("环境备注说明")
			d.P(note)
		}
	}

	// ---------- 五、被测模型清单 ----------
	d.H1B("五、被测模型清单")
	d.P("记录本次实测的全部模型,含确切版本号,及登记 Hugging Face 与魔搭社区(ModelScope)模型标号," +
		"验证测试可复现。量化版本同时注明权重量化格式(如 FP16 / INT8 / AWQ / GPTQ / GGUF 等)。")
	rows := [][]string{}
	for _, t := range tasks {
		pj := parseParams(t.ParamsJSON)
		rows = append(rows, []string{t.ModelName, thinkingCN(pval(pj.Advanced, "thinking_mode")), orDash(t.Quant), blankFill,
			fmtCtxLen(t.ContextLen), orDash(t.ModelHFID), orDash(t.ModelMSID)})
	}
	if len(rows) == 0 {
		rows = append(rows, []string{blankFill, blankFill, blankFill, blankFill, blankFill, blankFill, blankFill})
	}
	d.Table([]string{"模型名称(含版本)", "思考等级", "量化精度", "权重格式 / 格式", "上下文长度", "Hugging Face 模型 ID", "魔搭社区模型 ID"}, rows)
	d.Caption("被测模型清单(按实际模型数增删行)")
	// V2.23:思考等级口径说明
	d.PColor("注:思考等级仅在质量验证(能力测试)中启用,性能测试项不携带该参数;横向对比不同模型的质量成绩时须固定该等级。", "909399")

	// ---------- 六、模型调用参数 ----------
	d.H1B("六、模型调用参数")
	d.P("统一各被测模型的调用参数,排除采样参数对实测数据的干扰;每轮测试前执行预热请求," +
		"正式数据取有效样本统计。以下为任务的实际参数快照。")
	for _, t := range tasks {
		if len(tasks) > 1 {
			d.H2(fmt.Sprintf("任务 %d(%s)", t.ID, t.ModelName))
		}
		pj := parseParams(t.ParamsJSON)
		d.Table([]string{"参数", "取值", "说明"}, [][]string{
			{"temperature", pval(pj.Gen, "temperature"), "采样温度;对比测试须固定"},
			{"top_p", pval(pj.Gen, "top_p"), "核采样阈值"},
			{"top_k", pval(pj.Gen, "top_k"), "候选词截断"},
			{"max_tokens(输出上限)", pval(pj.Gen, "max_tokens"), "仅性能测试项统一固定,便于横向对比;质量抽查不限制输出长度"},
			{"API 接口形式", "OpenAI 兼容 /v1/chat/completions", "流式(SSE)采集 TTFT"},
			{"测试语料版本", orDash(t.CorpusVersionNo), "任务级快照:语料/题目后续编辑不影响本任务结果复现"},
			{"预热次数", pval(pj.Exec, "warmup"), "排除模型加载与冷启动影响"},
			{"阶梯档位", plist(pj.Exec, "ladder_tiers"), "输入长度档位序列"},
			{"阶梯每档重复", pval(pj.Exec, "ladder_repeats"), "取有效样本统计"},
			{"并发档位", plist(pj.Exec, "concurrency_levels"), "并发测试专用档位"},
			{"并发输入档位 / 每档重复", pval(pj.Exec, "concurrency_tier") + " / " + pval(pj.Exec, "concurrency_repeats"), "支持多输入档,逐档分组统计"},
			{"稳压时长 / 档位 / 并发", pval(pj.Exec, "stability_minutes") + " 分钟 / " + pval(pj.Exec, "stability_tier") + " / " + pval(pj.Exec, "stability_concurrency"), "长时稳定性压测参数"},
			{"缓存档位 / 每档重复", plist(pj.Exec, "cache_tiers") + " / " + pval(pj.Exec, "cache_repeats"), "冷热对比档位"},
			{"NIAH 每项探针数", pval(pj.Exec, "niah_repeats"), "宣称验证:长度点位 × 深度(最细 1%)复测;未填上下文:翻倍+二分探测"},
			{"缓存开关状态", cacheSwitchCN(t.CacheEnabled), "服务端前缀缓存"},
		})
	}
	d.Caption("模型调用参数(任务实际参数快照)")

	// ---------- 七、阶梯 ----------
	d.H1B("七、不同输入长度性能测试(阶梯)")
	d.P("本项为核心测试:在不同输入长度档位下,测量首 token 延迟(TTFT)与生成速度。" +
		"据此绘制生成速度和首 token 延迟趋势。")
	wrote := false
	for _, t := range tasks {
		lr := stats.Ladder(st, t.ID, int64(t.RunNo), 0)
		if len(lr) == 0 {
			continue
		}
		if !wrote {
			d.NoteBox("测试方法:流式采集;前缀缓存保持关闭(即每轮构建全新前缀,确保冷基线);" +
				"每档重复多次,统计平均 / P50 / P95 / P99 / 最短 / 最长;" +
				"若设备与模型不支持高档位,按实际能支持的档位测试,并在表中注明。")
		}
		wrote = true
		if len(tasks) > 1 {
			d.H2(fmt.Sprintf("任务 %d(%s)", t.ID, t.ModelName))
		}
		rows = [][]string{}
		for _, r := range lr {
			if r.Skipped {
				rows = append(rows, []string{r.Tier, "超出上下文限制", "-", "-", "-", "-", "-", "-", "-", "-", "-"})
				continue
			}
			row := []string{r.Tier, fmt.Sprintf("%d", r.PromptTokens),
				fmt.Sprintf("%d/%d", r.Samples, r.Planned), fmt.Sprintf("%d/%d", r.OkCount, r.Samples)}
			rows = append(rows, append(row, distCells(r.Ttft)...))
		}
		d.Table([]string{"输入档位", "实际长度(tokens)", "进度", "成功", "平均", "P50", "P95", "P99", "最短", "最长"}, rows)
		d.Caption(fmt.Sprintf("阶梯测试首 token 延迟 TTFT(ms)分布(模型:%s)", t.ModelName))
		rows = [][]string{}
		for _, r := range lr {
			if !r.Skipped {
				row := append([]string{r.Tier, strconv.Itoa(r.Samples)}, distCells(r.Speed)...)
				rows = append(rows, append(row, itl(r.Speed.Avg)))
			}
		}
		d.Table([]string{"输入档位", "样本数", "平均", "P50", "P95", "P99", "最短", "最长", "ITL(ms)"}, rows)
		d.Caption(fmt.Sprintf("阶梯测试生成速度(tokens/s)分布(模型:%s)", t.ModelName))
	}
	if !wrote {
		d.P("本次测试未执行该测试项。")
	}

	// ---------- 八、并发 ----------
	d.H1B("八、并发性能测试")
	d.P("按多个输入长度档位分别做并发阶梯:固定输出,逐步提升并发请求数," +
		"观察单请求速度与整机的吞吐拐点,评估硬件在不同输入规模下的并发承载能力;" +
		"单档连续失败触发熔断跳过,不影响其余档位继续测试。")
	wrote = false
	for _, t := range tasks {
		cr := stats.Concurrency(st, t.ID, int64(t.RunNo))
		if len(cr) == 0 {
			continue
		}
		wrote = true
		if len(tasks) > 1 {
			d.H2(fmt.Sprintf("任务 %d(%s)", t.ID, t.ModelName))
		}
		// V2.15:按输入档分组,每档一对表(TTFT 分布 + 速度分布)
		tierSet := map[string]bool{}
		var tierOrderList []string
		for _, c := range cr {
			if !tierSet[c.Tier] {
				tierSet[c.Tier] = true
				tierOrderList = append(tierOrderList, c.Tier)
			}
		}
		sort.Slice(tierOrderList, func(i, j int) bool { return tierKey(tierOrderList[i]) < tierKey(tierOrderList[j]) })
		for _, tier := range tierOrderList {
			var sub []stats.ConcurrencyRow
			for _, c := range cr {
				if c.Tier == tier {
					sub = append(sub, c)
				}
			}
			if len(tierOrderList) > 1 {
				d.H3(fmt.Sprintf("输入档 %s", tier))
			}
		rows = [][]string{}
		for _, c := range sub {
			if c.CircuitTrip {
				rows = append(rows, []string{fmt.Sprintf("%d @ %s", c.Level, c.Tier), "-", "熔断跳过", "-", "-", "-", "-", "-", "-", "-", "-"})
				continue
			}
			row := []string{fmt.Sprintf("%d @ %s", c.Level, c.Tier), fmt.Sprintf("%d/%d", c.Rounds, c.PlannedRounds),
				fmt.Sprintf("%d/%d", c.OkCount, c.FailCount), f1(c.FailRate * 100)}
			rows = append(rows, append(row, distCells(c.Ttft)...))
		}
		d.Table([]string{"并发数 @ 输入档", "进度(轮)", "成功/失败", "失败率(%)", "平均", "P50", "P95", "P99", "最短", "最长"}, rows)
		d.Caption(fmt.Sprintf("并发测试首 token 延迟 TTFT(ms)分布(模型:%s / 输入:%s)", t.ModelName, tier))
		rows = [][]string{}
			for _, c := range sub {
				if !c.CircuitTrip {
					row := append([]string{strconv.Itoa(c.Level)}, distCells(c.Speed)...)
					row = append(row, itl(c.Speed.Avg))
					rows = append(rows, append(row, f2(c.Throughput)))
				}
			}
			d.Table([]string{"并发数", "平均", "P50", "P95", "P99", "最短", "最长", "ITL(ms)", "总吞吐(tokens/s)"}, rows)
		d.Caption(fmt.Sprintf("并发测试单请求生成速度(tokens/s)分布与总吞吐(模型:%s / 输入:%s)", t.ModelName, tier))
		}
	}
	if !wrote {
		d.P("本次测试未执行该测试项。")
	}

	// ---------- 九、缓存 ----------
	d.H1B("九、前缀缓存(开关缓存)测试")
	d.P("前缀缓存(prefix caching)是推理服务的开销优化:当两次请求的开头部分完全一致时," +
		"服务端可直接复用已计算好的中间结果(KV cache),跳过重复计算,降低首 token 延迟。" +
		"该功能在服务端启动时开启,用户侧无需任何动作;是否命中取决于请求数据的开头是否重复。")
	// V2.20:对话式叠加(对齐真实多轮对话场景)
	d.P("本测试方法为对话式叠加:第 1 轮发送文档全文(冷,写缓存);之后每轮把上轮回答并入上下文再发同一文档," +
		"前缀逐轮累积。各轮 cached_tokens/prompt_tokens 即为命中率;热轮 TTFT 对比冷轮即缓存实际收益。")
	d.H2("9.1 缓存配置信息")
	cacheState := "未知"
	if len(tasks) > 0 {
		cacheState = cacheSwitchCN(tasks[0].CacheEnabled)
	}
	d.Table([]string{"项目", "取值 / 说明"}, [][]string{
		{"缓存机制", blankFill + "(按硬件平台确认)"},
		{"缓存开关参数", blankFill + "(如 vLLM: --enable-prefix-caching;SGLang:默认开启)"},
		{"测试时开关状态", cacheState},
		{"显存分配策略", blankFill + "(显存中分配给 KV cache 的比例或上限)"},
		{"冷轮语料构造方式", "固定开头前缀 / 每轮插入全新随机片段(防命中)"},
		{"热轮语料构造方式", "同一前缀文本重复请求,开头完全一致"},
	})
	d.Caption("缓存配置信息")
	wrote = false
	for _, t := range tasks {
		cr := stats.Cache(st, t.ID, int64(t.RunNo))
		if len(cr) == 0 {
			continue
		}
		if !wrote {
			d.H2("9.2 开关缓存效果对比")
		}
		wrote = true
		rows = [][]string{}
		for _, c := range cr {
			hit := "0(服务端未返回)"
			if c.HitRate > 0 {
				hit = f1(c.HitRate * 100)
			}
			row := []string{c.Tier, f1(c.ColdTTFT), f1(c.HotTTFT), f2(c.Improvement), hit}
			rows = append(rows, append(row, distCells(c.Ttft)...))
		}
		d.Table([]string{"档位", "冷TTFT(ms)", "热TTFT(ms)", "提升倍数", "命中率(%)", "冷TTFT平均", "P50", "P95", "P99", "最短", "最长"}, rows)
		d.Caption(fmt.Sprintf("开关缓存前后首 token 延迟对比(模型:%s)", t.ModelName))
			rows = [][]string{}
			for _, c := range cr {
				row := append([]string{c.Tier}, distCells(c.Speed)...)
				rows = append(rows, append(row, itl(c.Speed.Avg)))
			}
			d.Table([]string{"档位", "平均", "P50", "P95", "P99", "最短", "最长", "ITL(ms)"}, rows)
		d.Caption(fmt.Sprintf("缓存测试生成速度(tokens/s)分布(模型:%s)", t.ModelName))
	}
	if !wrote {
		d.P("本次测试未执行该测试项。")
	}

	// ---------- 十、NIAH ----------
	d.H1B("十、NIAH 长上下文检索测试")
	d.P("NIAH(Needle In A Haystack)在长上下文语料的不同深度埋入随机探针(数字密钥)," +
		"要求模型原样找回,检验模型对长上下文的真实利用能力。" +
		"填了上下文长度时按宣称值验证(多长度点位 × 多埋针深度复测,深度最细至 1%);" +
		"未填时从 8K 逐档翻倍探测,首个失败点二分细化,定位实打实的有效上下文。")
	wrote = false
	for _, t := range tasks {
		cells := stats.NIAH(st, t.ID, int64(t.RunNo))
		if len(cells) == 0 {
			continue
		}
		wrote = true
		if len(tasks) > 1 {
			d.H2(fmt.Sprintf("任务 %d(%s)", t.ID, t.ModelName))
		}
		mode := cells[0].Mode
		if mode == "verify" || mode == "legacy" {
			// 矩阵:行=输入长度点位,列=埋针深度(verify 用 depth_pct,legacy 兼容 depth 列)
			depthOf := func(c stats.NIAHCell) int {
				if mode == "verify" {
					return c.DepthPct
				}
				return c.Depth
			}
			depthSet, tierSet := map[int]bool{}, map[string]bool{}
			for _, c := range cells {
				depthSet[depthOf(c)] = true
				tierSet[c.Tier] = true
			}
			var depths []int
			for k := range depthSet {
				depths = append(depths, k)
			}
			sort.Ints(depths)
			var tiers []string
			for k := range tierSet {
				tiers = append(tiers, k)
			}
			sort.Slice(tiers, func(i, j int) bool { return tierKey(tiers[i]) < tierKey(tiers[j]) })
			byKey := map[string]stats.NIAHCell{}
			for _, c := range cells {
				byKey[fmt.Sprintf("%s|%d", c.Tier, depthOf(c))] = c
			}
			headers := []string{"输入长度点位"}
			for _, dp := range depths {
				headers = append(headers, fmt.Sprintf("埋针深度 %d%%", dp))
			}
			rows = [][]string{}
			for _, tier := range tiers {
				row := []string{tier}
				for _, dp := range depths {
					c, ok := byKey[fmt.Sprintf("%s|%d", tier, dp)]
					if !ok || c.Probes == 0 {
						row = append(row, "-")
						continue
					}
					cell := fmt.Sprintf("%s(%d/%d)", f1(c.HitRate*100), c.Hits, c.Probes)
					if c.Errors > 0 {
						cell += fmt.Sprintf(" 错误%d", c.Errors)
					}
					row = append(row, cell)
				}
				rows = append(rows, row)
			}
			d.Table(headers, rows)
			d.Caption(fmt.Sprintf("NIAH 长度点位 × 埋针深度命中率(%%)(模型:%s)", t.ModelName))
		} else {
			// 探测/扫描:点位表(按实际 tokens 排序,错误单列)
			sorted := append([]stats.NIAHCell(nil), cells...)
			sort.Slice(sorted, func(i, j int) bool {
				if sorted[i].Tokens != sorted[j].Tokens {
					return sorted[i].Tokens < sorted[j].Tokens
				}
				return sorted[i].Pct < sorted[j].Pct
			})
			rows = [][]string{}
			for _, c := range sorted {
				concl := "能找回"
				if c.Probes == 0 {
					concl = "错误/截断"
				} else if c.HitRate < 0.5 {
					concl = "找不回"
				}
				rows = append(rows, []string{c.Tier, commaInt(c.Tokens), fmt.Sprintf("%d%%", c.DepthPct),
					fmt.Sprintf("%s(%d/%d)", f1(c.HitRate*100), c.Hits, c.Probes),
					strconv.Itoa(c.Errors), f1(c.TtftMedian), concl})
			}
			d.Table([]string{"输入长度", "目标 tokens", "埋针深度", "命中率(%)", "错误数", "TTFT 中位(ms)", "结论"}, rows)
			d.Caption(fmt.Sprintf("NIAH 有效上下文探测点位(模型:%s)", t.ModelName))
		}
		if eff := niahEffective(st, t.ID, int64(t.RunNo)); eff != "-" {
			d.P("有效上下文结论:" + eff)
		}
	}
	if !wrote {
		d.P("本次测试未执行该测试项。")
	}

	// ---------- 十一、稳定性 ----------
	d.H1B("十一、长时稳定性测试")
	d.P("小算力设备散热受限,持续负载可能触发降频,生成速度随时间衰减。" +
		"本项连续压测,分段记录,观察生成速度衰减与失败情况。")
	// V2.20:稳定性三要素醒目标明(输入档位/并发/时长)
	if len(tasks) > 0 {
		if pj := parseParams(tasks[0].ParamsJSON); pval(pj.Exec, "stability_minutes") != "" {
			d.NoteBox(fmt.Sprintf("压测参数:输入档位 %s · 每波并发 %s · 压测时长 %s 分钟",
				orDash(pval(pj.Exec, "stability_tier")), orDash(pval(pj.Exec, "stability_concurrency")), pval(pj.Exec, "stability_minutes")))
		}
	}
	wrote = false
	for _, t := range tasks {
		wins, decay, dnote := stats.Stability(st, t.ID, int64(t.RunNo), 0)
		if len(wins) == 0 {
			continue
		}
		wrote = true
		if len(tasks) > 1 {
			d.H2(fmt.Sprintf("任务 %d(%s)", t.ID, t.ModelName))
		}
		rows = [][]string{}
		for _, w := range wins {
			row := []string{fmt.Sprintf("窗口%d(%s)", w.Index, ts(w.StartTS)), strconv.Itoa(w.OkCount), strconv.Itoa(w.FailCount)}
			rows = append(rows, append(row, distCells(w.Ttft)...))
		}
		d.Table([]string{"时间节点", "成功", "失败", "平均", "P50", "P95", "P99", "最短", "最长"}, rows)
		d.Caption(fmt.Sprintf("长时稳定性测试首 token 延迟 TTFT(ms)分布(模型:%s)", t.ModelName))
			rows = [][]string{}
			for _, w := range wins {
				row := []string{fmt.Sprintf("窗口%d(%s)", w.Index, ts(w.StartTS)), strconv.Itoa(w.OkCount), strconv.Itoa(w.FailCount)}
				row = append(row, distCells(w.Speed)...)
				row = append(row, itl(w.Speed.Avg))
				rows = append(rows, append(row, orDash(w.Trend)))
			}
			d.Table([]string{"时间节点", "成功", "失败", "平均", "P50", "P95", "P99", "最短", "最长", "ITL(ms)", "趋势"}, rows)
		d.Caption(fmt.Sprintf("长时稳定性测试生成速度(tokens/s)分布(模型:%s)", t.ModelName))
		decayStr := "-"
		if len(wins) >= 2 {
			decayStr = f2(decay) + "%"
		}
		d.P(fmt.Sprintf("性能衰减:%s(末窗口较首窗口速度变化;%s)", decayStr, orDash(dnote)))
	}
	if !wrote {
		d.P("本次测试未执行该测试项。")
	}

	// ---------- 十二、质量抽检 ----------
	d.H1B("十二、质量抽检")
	wrote = false
	for _, t := range tasks {
		type qa struct {
			id                                              int64
			title, auto, reason, manual, note, judgeScore   string
			judgeReason, artPath, artType                   string
			artOK                                           bool
		}
		qrows, _ := st.DB().Query(`SELECT id,question_title,auto_flag,COALESCE(auto_reason,''),manual_flag,COALESCE(manual_note,''),
			COALESCE(judge_score,''),COALESCE(judge_reason,''),COALESCE(artifact_path,''),COALESCE(artifact_ok,0),
			COALESCE(NULLIF(artifact_type,''),'none')
			FROM quality_answers WHERE task_id=? AND run_no=? ORDER BY id`, t.ID, t.RunNo)
		var list []qa
		if qrows != nil {
			for qrows.Next() {
				var q qa
				var ok int
				qrows.Scan(&q.id, &q.title, &q.auto, &q.reason, &q.manual, &q.note,
					&q.judgeScore, &q.judgeReason, &q.artPath, &ok, &q.artType)
				q.artOK = ok != 0
				list = append(list, q)
			}
			qrows.Close()
		}
		if len(list) == 0 {
			continue
		}
		if !wrote {
			d.NoteBox("测试方法:质量题由 pi 智能体执行,可自主调用文件与命令工具;" +
				"产物类题目启用 goal 契约(智能体持续工作直至声明完成,单题上限 1 小时);" +
				"不限制输出长度(不携带 max_tokens);裁判模型逐题判定通过 / 未通过。")
		}
		wrote = true
		verdict, total, abnormal, suspect := stats.QualityVerdict(st, t.ID, int64(t.RunNo))
		d.P(fmt.Sprintf("任务 %d(%s):共 %d 题,结论 %s(异常 %d,存疑 %d)。"+
			"由 pi 智能体在独立沙箱中实际作答并产出文件,裁判模型逐题评分。",
			t.ID, t.ModelName, total, verdict, abnormal, suspect))
		rows = [][]string{}
		for _, q := range list {
			flag := q.auto
			if q.manual != "" {
				flag = q.manual
			}
			mark := "-"
			if q.manual != "" {
				mark = "已复核"
			}
			desc := q.reason
			if desc == "" {
				desc = q.judgeReason
			}
			if len([]rune(desc)) > 40 {
				desc = string([]rune(desc)[:40])
			}
			rows = append(rows, []string{q.title, orDash(q.judgeScore), flagCN(flag), mark, orDash(desc)})
		}
		d.Table([]string{"题目", "裁判评分", "初筛结论", "人工复核", "异常描述"}, rows)
		d.Caption(fmt.Sprintf("质量抽检记录(模型:%s)", t.ModelName))
		// 产物附件:xlsx/pptx/docx/pdf 直嵌;其余打 zip 后嵌入
		hasArt := false
		for _, q := range list {
			if q.artOK && q.artPath != "" {
				if !hasArt {
					d.H3("产物附件(双击图标打开原文件)")
					hasArt = true
				}
				attachArtifact(d, dataDir, q.artPath, q.artType, q.title)
			}
		}
	}
	if !wrote {
		d.P("本次测试未执行该测试项。")
	}

	// ---------- 附录 A ----------
	d.H1B("附录 A 关键测试方法说明")
	d.Table([]string{"术语", "定义"}, [][]string{
		{"TTFT(首 token 延迟)", "从发送请求到收到第一个 token 的耗时,主要反映输入处理(prefill)速度,越小越好"},
		{"生成速度 TPS(decode)", "每秒生成的 token 数,窗口为「首个输出 token → 最后一个 token」,不含 TTFT(不含首字延迟);与 vLLM benchmark / genai-perf 口径一致"},
		{"ITL(Inter-Token Latency)", "相邻两个输出 token 的平均间隔,单位 ms/token,即 1000 ÷ TPS"},
		{"总吞吐", "全部并发请求合计每秒输出的 token 数,单位 tokens/s,反映整机容量"},
		{"总耗时", "从发送请求到接收完全部输出的端到端时间"},
		{"KV cache", "模型注意力计算时缓存在显存中的中间结果,复用可跳过重复计算"},
		{"前缀缓存(prefix caching)", "服务端开关:请求与缓存同开头时复用已算好的 KV cache,跳过重复计算"},
		{"缓存命中(热请求)", "请求开头与缓存完全一致,前缀缓存生效,首 token 延迟显著下降"},
		{"缓存未命中(冷请求)", "无缓存可复用,请求按完整前缀计算,反映真实硬件算力"},
		{"命中率", "缓存命中请求数占总请求数的比例,或服务端返回的 cached_tokens 占比"},
		{"P99 延迟", "99% 的请求慢于该延迟阈值,用于观察长尾延迟"},
		{"NIAH(大海捞针)", "在长上下文不同深度埋入探针并要求找回,衡量真实可用上下文长度"},
		{"有效上下文", "探针命中率开始显著下降的上下文位置,即模型实际可用的上下文边界"},
	})
	d.Caption("术语表")

	// ---------- 附录 B ----------
	d.H1B("附录 B 版本记录")
	d.Table([]string{"版本", "日期", "修改人", "修改说明"}, [][]string{
		{"V1.0", dstr(p.FinishedAt), orDash(p.Tester), fmt.Sprintf("首次发布(平台 %s 自动生成)", orDash(version))},
		{blankFill, blankFill, blankFill, blankFill},
	})
	d.Caption("版本记录")

	buf, err := d.Build()
	if err != nil {
		return nil, "", err
	}
	name := fmt.Sprintf("%s_%s_测试报告_%s.docx", orName(p.Name, "项目"), orName(deviceName(device), "设备"),
		time.Now().Format("20060102"))
	return buf, name, nil
}

// niahEffective 有效上下文结论(V2.13:与结果页同口径,按点位实际 tokens 取命中率≥50% 的最高点位)
func niahEffective(st *store.Store, taskID, runNo int64) string {
	cells := stats.NIAH(st, taskID, runNo)
	best, maxTested, minFail, mode := 0, 0, 0, ""
	for _, c := range cells {
		if c.Tokens > maxTested {
			maxTested = c.Tokens
		}
		if c.HitRate >= 0.5 && c.Probes > 0 && c.Tokens > best {
			best = c.Tokens
		}
		if c.Probes > 0 && c.HitRate < 0.5 && (minFail == 0 || c.Tokens < minFail) {
			minFail = c.Tokens
		}
		if c.Mode != "" {
			mode = c.Mode
		}
	}
	if best <= 0 {
		return "-"
	}
	t, _ := st.GetTask(taskID)
	if mode == "verify" && t != nil && t.ContextLen > 0 {
		if best >= maxTested {
			return fmt.Sprintf("与宣称一致:全部长度点位均命中(宣称 %s)", TokensToTierCN(t.ContextLen))
		}
		if minFail > 0 {
			return fmt.Sprintf("宣称不达标:从 %s(宣称值的 %d%%)开始丢针,实际可用上下文约 %s",
				TokensToTierCN(minFail), minFail*100/t.ContextLen, TokensToTierCN(best))
		}
	}
	if best >= maxTested {
		return fmt.Sprintf("有效上下文 ≥ %s(已达探测上限,各点位均命中)", TokensToTierCN(best))
	}
	return fmt.Sprintf("实测有效上下文约 %s,视为该模型的实际可用上下文边界", TokensToTierCN(best))
}

// TokensToTierCN 报告内 token 数 → K/M 文案(避免 runner 包依赖)
func TokensToTierCN(tokens int) string {
	if tokens >= 1000000 {
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", float64(tokens)/1000000), "0"), ".") + "M"
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", float64(tokens)/1000), "0"), ".") + "K"
}

func commaInt(n int) string {
	s := strconv.Itoa(n)
	var out []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, byte(c))
	}
	return string(out)
}

// deviceField / deviceFields 设备档案安全取值(device 可空)
func deviceField(d *store.Device, _ string) string {
	if d == nil {
		return ""
	}
	return d.FormFactor
}

func deviceFields(d *store.Device) map[string]string {
	m := map[string]string{
		"cpu": blankFill, "chip": blankFill, "chipCount": blankFill, "vram": blankFill,
		"vramForm": blankFill, "host": blankFill, "memory": blankFill, "storage": blankFill,
		"cooling": blankFill, "tdp": blankFill, "network": blankFill,
	}
	if d == nil {
		return m
	}
	m["cpu"] = orDash(d.CPUSoC)
	m["chip"] = orDash(d.ChipModel)
	if d.ChipCount > 0 {
		m["chipCount"] = strconv.Itoa(d.ChipCount)
	}
	m["vram"] = fmt.Sprintf("%.1f GB %s %.0f GB/s", d.VramGB, d.VramType, d.VramBwGbps)
	if d.VramShared {
		m["vramForm"] = "共享"
	} else {
		m["vramForm"] = "独立"
	}
	m["host"] = orDash(d.HostPlatform)
	m["memory"] = orDash(d.Memory)
	m["storage"] = orDash(d.Storage)
	m["cooling"] = orDash(d.Cooling)
	if d.TdpW > 0 {
		m["tdp"] = fmt.Sprintf("%.0f W", d.TdpW)
	}
	m["network"] = orDash(d.Network)
	return m
}

// attachArtifact 产物嵌入:文档类(xlsx/pptx/docx/pdf)原文件直嵌;其余类型整目录打 zip 嵌入
func attachArtifact(d *docx, dataDir, artPath, artType, title string) {
	dir := filepath.Join(dataDir, "artifacts", filepath.FromSlash(artPath))
	entries, err := os.ReadDir(dir)
	if err != nil {
		d.PColor(fmt.Sprintf("产物:《%s》文件缺失(%s)", title, artPath), "999999")
		return
	}
	isDoc := artType == "xlsx" || artType == "pptx" || artType == "docx" || artType == "pdf"
	if isDoc {
		want := "document." + artType
		var picked string
		for _, e := range entries {
			if !e.IsDir() && e.Name() == want {
				picked = e.Name()
				break
			}
		}
		if picked == "" { // 规范名不存在时退找同扩展名文件
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), "."+artType) {
					picked = e.Name()
					break
				}
			}
		}
		if picked != "" {
			data, err := os.ReadFile(filepath.Join(dir, picked))
			if err == nil && len(data) > 0 {
				d.Attach(title, picked, artType, data)
				return
			}
		}
	}
	// V2.20:网站文件(html)不再打 zip——原始文件直接嵌入,双击浏览器打开(同 results.html)
	if artType == "html" || artType == "code" {
		pickOrder := []string{"index.html"}
		var picked string
		for _, want := range pickOrder {
			for _, e := range entries {
				if !e.IsDir() && e.Name() == want {
					picked = e.Name()
					break
				}
			}
			if picked != "" {
				break
			}
		}
		if picked == "" { // 无规范名时退找首个 .html
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".html") {
					picked = e.Name()
					break
				}
			}
		}
		if picked != "" {
			data, err := os.ReadFile(filepath.Join(dir, picked))
			if err == nil && len(data) > 0 {
				d.Attach(title, picked, "html", data)
				return
			}
		}
	}
	// 其余类型或 html 缺失:整目录打 zip 兜底
	zbuf := &bytes.Buffer{}
	zw := zip.NewWriter(zbuf)
	n := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		w, err := zw.Create(e.Name())
		if err != nil {
			continue
		}
		w.Write(data)
		n++
	}
	zw.Close()
	if n == 0 {
		d.PColor(fmt.Sprintf("产物:《%s》无可用文件", title), "999999")
		return
	}
	d.Attach(title, sanitizeFilename(title)+".zip", "zip", zbuf.Bytes())
}

func sanitizeFilename(s string) string {
	r := strings.NewReplacer("/", "_", "\\", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_")
	s = r.Replace(strings.TrimSpace(s))
	if len([]rune(s)) > 40 {
		s = string([]rune(s)[:40])
	}
	if s == "" {
		s = "artifact"
	}
	return s
}

// ---------- hw_json 快照读取(V2.12:hardware 项目内联硬件信息) ----------

func parseHwJSON(raw string) map[string]any {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var m map[string]any
	if json.Unmarshal([]byte(raw), &m) != nil || len(m) == 0 {
		return nil
	}
	return m
}

// hwStr 读 hw 字段,支持多个候选键(表单字段名与报告语义名的兼容映射)
func hwStr(hw map[string]any, keys ...string) string {
	for _, key := range keys {
		v, ok := hw[key]
		if !ok || v == nil {
			continue
		}
		if s, ok := v.(string); ok {
			if strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
			continue
		}
		return fmt.Sprint(v)
	}
	return ""
}

func hwNum(hw map[string]any, key string) string {
	v, ok := hw[key]
	if !ok || v == nil {
		return blankFill
	}
	if f, ok := v.(float64); ok {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	return fmt.Sprint(v)
}

func hwVram(hw map[string]any) string {
	gb, typ, bw := hwStr(hw, "vram_gb"), hwStr(hw, "vram_type"), hwStr(hw, "vram_bw_gbps")
	if gb == "" && typ == "" {
		return blankFill
	}
	shared := ""
	if hwStr(hw, "vram_shared") == "true" {
		shared = "(共享)"
	}
	return strings.TrimSpace(fmt.Sprintf("%sGB %s %sGB/s %s", gb, typ, bw, shared))
}
