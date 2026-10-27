# 大模型推理硬件自动化测试平台

按《大模型硬件自动化测试平台-产品与技术设计文档-V2.0.md》实现。Go 单二进制 + 内嵌 SQLite + Vue3 前端(静态资源内嵌),部署于测试内网。

## 构建

```bash
# 前端
cd web && npm install && npm run build && cd ..
# 后端(内嵌前端产物,纯 Go 驱动免 CGO)
CGO_ENABLED=0 go build -ldflags="-s -w" -o hwtest .
```

## 运行

```bash
./hwtest --addr 127.0.0.1:8080 --data ./data
```

- 首次启动在控制台打印 **12 位随机初始密码**,首次登录强制修改;
- 丢失密码:`./hwtest --reset-password --data ./data` 重置;
- 生产建议 `--addr` 仅绑内网地址;systemd 示例见 `deploy/hwtest.service`;
- 浏览器访问 http://\<服务器\>:8080 。

## 架构(文档 §7.2)

```
main
 ├─ httpd        chi 路由 / 会话鉴权 / embed 静态资源
 ├─ auth         bcrypt 密码、7 天会话、5 次锁定 10 分钟
 ├─ scheduler    设备级 FIFO + 全局并行上限 + 取消/中断恢复
├─ runner       执行引擎(预热/阶梯/并发/缓存/稳定性/质量抽查/NIAH)
│   ├─ llmclient  OpenAI 兼容 SSE 客户端(§6.1 计时/token 口径)
│   └─ calibrator 输入长度校准(§6.2,±1% 冻结复用)
├─ stats        中位数/分位/窗口聚合/能效/衰减(附录 A)
 ├─ exporter     Word 正式报告(docx,OLE 内嵌产物+交互式结果页)/ 对比 Excel 导出 / 任务数据包物化
 └─ store        SQLite(modernc 纯 Go,单连接串行写)
```

## 自测工具(scripts/)

- `mock-llm.js` — OpenAI 兼容 mock 推理服务(SSE + usage + 前缀缓存模拟),用于无真机时端到端验证;
- `pi/` — 质量题智能体执行器(pi-agent-core + pi-ai,仅需 `npm install` 一次):质量抽查默认由 pi 带
  read/write/bash 工具执行,产物题(PPT/Excel/Word/HTML)由模型真实生成文件并登记;设置页可切回直接问答;
- `niah-auto-verify.js` — NIAH 自动边界扫描验收(30%→10%→1% 三级收敛,实测有效上下文);
- `fake-nvidia-smi.sh` — 模拟 nvidia-smi 输出(历史遗留,监控链路已于 V2.8 下线);
- `e2e.js` — 40 项端到端验收断言(认证/语料/设备/执行/结果/导出/对比/重跑/备份);
- `interrupt-test.js` — 中断恢复验收(§10 #7)。

## 验收口径对照(§10)

| # | 标准 | 验证 |
|---|---|---|
| 1 | 无人值守跑完全部勾选测试项 | e2e 任务1 五项全自动完成 |
| 2 | 30K 档 prompt_tokens 偏差 ≤1% | e2e 实测 pt=30000(0.00%) |
| 3 | 缓存热 TTFT ≤ 冷 × 50% | e2e 实测 40ms vs 140ms(3.5x) |
| 4 | ~~SSH 监控曲线与能效~~(V2.8 起下线:不监控远程设备,不做 SSH 采样) | 相关代码已移除 |
| 5 | 语料版本切换 | 版本化快照,任务记录版本号,历史任务按旧口径 |
| 6 | Excel 各 sheet 完整、未执行标「未测试」;对比两视角+叠加图+导出 | openpyxl 校验 8 sheet;对比导出 4 sheet |
| 7 | 重启中断任务可整任务重跑,历史 run 可查 | interrupt-test 通过(run_no 递增) |

---

## 公开仓库说明

本仓库公开核心产品源码、默认题库、52 档公开仿真语料及语料生成器。

出于安全与版权考虑，公开版本暂不包含：

- `scripts/`：内部验收与调试脚本，其中包含测试环境凭据；
- `语料库/*.txt` 与 `语料库/context_bench/*.txt`：《三体》正文切分语料；
- `release-*/`、`render_out/`、`*.docx`、`*.tar.gz`、`sandbox/`、`mockups/`：发布产物、报告、压缩包和实验材料。

默认题库位于：

- `internal/store/seed/questions.json`
- `语料库/灵思微数测试平台-实战题库.md`
- `语料库/灵思微数测试平台-文档生成题库20题.md`

公开默认语料位于：

- `internal/store/seed/corpus.json.gz`：`corpus-public-v1`，52 档；
- `语料库/build_public_corpus.py`：可复现生成上述语料；
- `语料库/gen_corpus.py`：业务仿真文本生成器。

公开语料完全由生成器产生，不包含《三体》正文。
