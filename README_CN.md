# 大模型推理硬件自动化测试平台：Go 后端 + Vue 3 前端

[English](README.md) | 简体中文

[![MIT License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

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

## 远程访问与 API

- Web 控制台地址：`http://<服务器地址>:8899`，部署时可通过 `--addr` 修改端口。
- 在「系统设置 → API Token」生成平台 Token，Token 只显示一次；公开仓库不保存任何真实 Token、SSH 密码或模型密钥。
- 被测模型服务使用 OpenAI 兼容接口，任务中配置 `http://<模型服务地址>:<端口>/v1`，平台通过 `/chat/completions` 发起测试。
- 任务页的「数据包」会给出服务器上的物化目录绝对路径，里面包含日志、请求/响应、明细 CSV、语料快照和产物文件。
- 本地硬件项目可在用户提供 SSH 信息后自动发现 GPU、推理服务、启动参数和整机信息；SSH 仅用于测试环境采集，不写入仓库。

## CLI Skills 智能体接入

平台提供 `skills/hwtest-platform` 技能包和 `hwtest-cli.mjs`，可以让智能体按四阶段流程完成测试：

1. 设置连接参数并验证：

```bash
export HWTEST_URL=http://<平台地址>:8899
export HWTEST_TOKEN=<API Token>
node hwtest-cli.mjs check
```

2. 读取资源并创建任务：

```bash
node hwtest-cli.mjs corpus
node hwtest-cli.mjs questions
node hwtest-cli.mjs models http://<模型服务地址>:<端口>/v1
node hwtest-cli.mjs create-task '{ "name": "...", "task": { ... } }'
```

3. 跟进执行与结果：

```bash
node hwtest-cli.mjs status 9
node hwtest-cli.mjs wait 9
node hwtest-cli.mjs digest 9
node hwtest-cli.mjs quality 9
node hwtest-cli.mjs package 9
```

4. 导出报告或进行横向对比：

```bash
node hwtest-cli.mjs export 9 task-9-sample.docx
node hwtest-cli.mjs compare 9,12
```

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

## 核心功能

- **测试项目与任务管理**：创建本地硬件或纯模型测试项目，配置 API 地址、模型、量化精度、上下文长度、语料版本和测试人员。
- **六类测试项**：阶梯测试、并发测试、前缀缓存冷热测试、长时稳定性测试、质量抽查和 NIAH 长上下文检索。
- **自动执行与恢复**：设备级调度、全局并行上限、暂停/继续、跳过、单项重跑，以及中断后的整任务重跑。
- **指标统计**：TTFT、生成速度、ITL、吞吐、Token 消耗、P50/P95/P99、性能衰减和能力总分。
- **质量验证**：pi 智能体可调用文件与命令工具，真实生成 Excel、PPT、Word、PDF 和网页产物，并支持裁判模型与人工复核。
- **报告与导出**：生成 Word 正式报告，支持 OLE 内嵌产物和交互式结果页；支持对比分析、Excel 导出和任务数据包物化。
- **语料与题库**：内置可版本化语料和质量题库，任务绑定语料快照，历史任务保持原测试口径。
- **安全与接入**：bcrypt 登录、会话鉴权、API Token、加密存储模型密钥，并提供 Web API 与 CLI Skills 接入。

## 验收口径对照(§10)

| # | 标准 | 验证 |
|---|---|---|
| 1 | 无人值守跑完全部勾选测试项 | e2e 任务1 五项全自动完成 |
| 2 | 30K 档 prompt_tokens 偏差 ≤1% | e2e 实测 pt=30000(0.00%) |
| 3 | 缓存热 TTFT ≤ 冷 × 50% | e2e 实测 40ms vs 140ms(3.5x) |
| 4 | 语料版本切换 | 版本化快照,任务记录版本号,历史任务按旧口径 |
| 5 | Excel 各 sheet 完整、未执行标「未测试」;对比两视角+叠加图+导出 | openpyxl 校验 8 sheet;对比导出 4 sheet |
| 6 | 重启中断任务可整任务重跑,历史 run 可查 | interrupt-test 通过(run_no 递增) |

## 真实报告示例

仓库提供一份来自任务 9 的真实 Word 正式测试报告：[下载任务 9 测试报告](docs/reports/task-9-sample.docx)。

报告由平台自动生成，包含：

- 测试项目、设备和推理软件栈信息；
- 阶梯、并发、稳定性、缓存和 NIAH 结果；
- 指标分布、P50/P95/P99 和性能衰减；
- pi 智能体质量抽查记录；
- OLE 内嵌的 Excel、PPT、Word、PDF 和 HTML 产物；
- 交互式 `results.html` 结果页。

为适合公开发布，报告中的测试服务密钥已脱敏。

---

## 公开仓库说明

本仓库公开核心产品源码、默认题库、52 档公开仿真语料及语料生成器。

出于安全与版权考虑，公开版本暂不包含：

- `scripts/`：内部验收与调试脚本，其中包含测试环境凭据；
- `语料库/*.txt` 与 `语料库/context_bench/*.txt`：《三体》正文切分语料；
- `release-*/`、`render_out/`、根目录中的报告/预览文件、`*.tar.gz`、`sandbox/`、`mockups/`：发布产物、报告模板、压缩包和实验材料。

默认题库位于：

- `internal/store/seed/questions.json`
- `语料库/灵思微数测试平台-实战题库.md`
- `语料库/灵思微数测试平台-文档生成题库20题.md`

公开默认语料位于：

- `internal/store/seed/corpus.json.gz`：`corpus-public-v1`，52 档；
- `语料库/build_public_corpus.py`：可复现生成上述语料；
- `语料库/gen_corpus.py`：业务仿真文本生成器。

## 开源协议

本项目源码采用 [MIT License](LICENSE) 开源。仓库中内嵌或依赖的第三方组件仍遵循其各自的原始许可证，相关权利归原权利人所有。
