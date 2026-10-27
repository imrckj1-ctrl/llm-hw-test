---
name: hwtest-platform
description: 操作灵思微致测试平台(LLM 硬件/模型测试)——按四阶段流程引导用户完成一次完整测试:连接平台 → 创建项目(逐项收集信息或 SSH 自发现) → 选测试项/语料/题目(读后台资源,给默认模板) → 启动并跟进结果。当需要创建/执行测试任务、跑模型性能或质量测试、查结果、导出报告时使用。
---

# 灵思微致测试平台 · 智能体操作手册

平台 = 单进程服务(Go + 内嵌 SQLite + 内嵌前端)。
一切操作走 REST API(带 API Token);本目录 `hwtest-cli.mjs` 封装了常用命令,优先使用。

本手册按**四阶段流程**工作,不要跳步;每一步该问用户什么、该读什么后台,下面都有清单。

---

## 阶段一 · 连接平台

**必须向用户索取两样东西**:

1. **平台地址**:形如 `http://192.168.x.x:8080`(别猜,问);
2. **API Token**:平台「系统设置 → API Token」生成(只显示一次,让用户去复制)。

拿到后验证连通:

```bash
export HWTEST_URL=http://192.168.31.168:8080   # 换成用户给的地址
export HWTEST_TOKEN=hwt_xxx                    # 换成用户给的 token
cd <本技能目录>
node hwtest-cli.mjs check
```

`check` 通过会返回:平台版本、pi 版本、pi-goal 版本、`default_task_params`(全部默认参数)。
不通过:先检查地址/端口/token,不要继续往下走。

---

## 阶段二 · 创建项目(收集信息)

把下面这张清单列给用户,逐项收集。**带 ★ 的必须用户给,其余可省略(用默认)。**

### 必填信息清单

| 项 | 说明 | 获取方式 |
|---|---|---|
| ★ 项目名称 | 本次测试叫什么(如"4090D 双卡 Qwen3.8 全项") | 只能问用户 |
| ★ 被测服务 API 地址 | OpenAI 兼容端点,如 `http://192.168.31.169:3000/v1` | 用户给;也可帮用户拼 |
| ★ 模型名称 | 与服务端 `--served-model-name` 一致 | **优先自己拉**:`node hwtest-cli.mjs models <api地址> [key]` 列出可选模型让用户挑;或用户给 |
| ★ API Key | 服务端鉴权 key(敏感信息) | 必须用户给,不要编造 |
| ★ 语料版本 ID | 任务必绑一个语料版本 | **自己读后台**:`node hwtest-cli.mjs corpus`,取最新版 id |
| 测试人员 | 报告署名 | 问用户,可空 |
| 备注 | 项目备注 | 问用户,可空 |
| 量化精度 | FP16/AWQ/GPTQ/FP8… | 用户给,可空 |
| 上下文长度 | 模型宣称上下文,1024 进制(256K=262144,1M=1048576) | 用户给;默认 256K |
| 思考等级 | **选完模型后必须问**:default 默认(不加字段)/off 关闭思考/minimal 极简/low 低/medium 中/high 高/xhigh 极高/max 最高。**注意:只在质量验证(能力测试)里生效**,性能测试项不携带 | 用户给,默认 default |
| 测试类型 | 纯模型(云端 API)/ 本地硬件(实体机) | 问用户 |

### 方式 B:SSH 自发现(本地硬件 / 用户嫌填表麻烦时)

如果用户愿意把被测机的 SSH 给你(地址 + 账号),**你可以自己上去把信息摸齐再自动填**,不必逐项问:

```bash
# 摸 GPU/芯片
ssh user@host nvidia-smi --query-gpu=name,memory.total,driver_version --format=csv
# 摸推理服务与模型列表
curl -s http://<host>:<port>/v1/models -H "Authorization: Bearer <key>"
# 摸启动命令(含 max-model-len、max-num-seqs、kv-cache 等关键参数)
ssh user@host "ps aux | grep -E 'vllm|llama|sglang' | grep -v grep"
# 摸整机信息
ssh user@host "uname -a; free -h; df -h /; lscpu | head -20"
```

摸完把结果整理成"本地硬件"项目的硬件信息(AI 芯片/显存/整机形态/操作系统/推理框架/启动命令),
**项目名称、测试人员仍然要问用户**。摸不到的信息如实留空,不要编。

### 建任务命令

```bash
node hwtest-cli.mjs create-task '{
  "type": "model",            # model=纯模型 / hardware=本地硬件
  "name": "用户给的项目名",
  "tester": "测试人员(可空)", "note": "备注(可空)",
  "task": {
    "api_base_url": "http://192.168.31.169:3000/v1",
    "model_name": "Qwen3.6-27B-AWQ",
    "quant": "AWQ",
    "context_len": 262144,
    "api_key": "sk-用户给的key",
    "corpus_version_id": 10,
    "params": { "gen": {}, "exec": {}, "advanced": {}, "items": {} }
  }
}'
# 返回里有 task id(记为 N),下一阶段用
```

---

## 阶段三 · 选测试项(读后台,给选择,给默认模板)

**先把后台资源读出来摆给用户,不要凭印象报。**

```bash
node hwtest-cli.mjs corpus      # 语料版本列表(id/版本号/档位/备注)
node hwtest-cli.mjs questions   # 题库列表(id/维度/标题/产物类型)
node hwtest-cli.mjs projects    # 已有项目(避免重名/可参考克隆)
```

再打印一次提问清单照着问:`node hwtest-cli.mjs guide`

**铁律:不允许只问"跑哪些项"。** 每个测试项都要按下面的清单逐项确认参数;
用户明确说"全部默认 / 按默认模板来"时,才可以整项走默认。

### 逐项确认清单(和平台网站表单一一对应)

**1. 阶梯测试**(不同输入长度下的 TTFT/速度)
- 测不测?
- 测的话:哪些输入档?(默认 `1K/10K/20K/30K`,支持 2.5K/1M)每档测几次?(默认 3)每档预热几次?(默认 3)

**2. 并发测试**(多并发吞吐与延迟)
- 测不测?
- 测的话:哪几个并发档?(默认 `1/4/8/16/32`,实体机别超服务端 max-num-seqs)用哪个输入档?(默认 1K)每档几轮?(默认跟随全局 repeats)

**3. 缓存测试**(前缀缓存冷热对比)
- 测不测?
- 测的话:哪些档?(默认跟随阶梯档)冷热各几轮?(默认跟随全局 repeats)

**4. 稳定性测试**(长时压测看衰减)
- 测不测?
- 测的话:压几分钟?(默认 1,**必须提醒用户:调大会烧 token**)哪个输入档?(默认 10K)每波并发几?(默认 1=背靠背)

**5. 质量抽查**(题库抽题 + pi 智能体生成产物)
- 测不测?
- 测的话:选题方式——`all` 全部启用题 / `cats` 按维度 / `group` 题组 / `custom` 指定题;用 `questions` 命令把题库摆给用户挑
- 质量题的思考等级?(default/off/minimal/low/medium/high/xhigh/max,**只在质量验证里生效**)

**6. NIAH 检索**(探针实测有效上下文)
- 测不测?
- 勾选即自动扫描,**无需任何参数**;只需跟用户确认任务的上下文长度填准了(扫描上限按它走,只下浮不上浮)

### 默认模板(用户说"按默认来"就用这套,开箱即测)

| 测试项 | 干什么 | 默认配置 |
|---|---|---|
| 阶梯测试 | 不同输入长度下的 TTFT/速度 | 档位 1K/10K/20K/30K,预热 3,每档 3 次 |
| 并发测试 | 多并发吞吐与延迟 | 并发 1/4/8/16/32,输入 1K |
| 缓存测试 | 前缀缓存冷热对比 | 跟随阶梯档 |
| 稳定性测试 | 长时压测看衰减 | 10K 输入,**1 分钟**(不调大,烧 token) |
| 质量抽查 | 题库抽题+pi 智能体生成产物 | 全部启用题 |
| NIAH 检索(可选) | 探针实测有效上下文 | 勾选即自动扫描,无需参数;开了上下文要填准 |

用户要定制:只改对应块,其余保持默认。示例(加 NIAH、阶梯改档):

```json
"items": {"ladder": true, "concurrency": true, "cache": true, "stability": true, "quality": true, "niah": true},
"exec": {"ladder_tiers": ["1K", "10K", "64K", "128K"], "stability_minutes": 1}
```

---

## 阶段四 · 启动与跟进

```bash
node hwtest-cli.mjs start N     # 入队执行
```

**启动后明确告诉用户**:
「任务已入队,平台后台自动执行,你不需要盯着。六个测试项按序执行,预计 X 分钟(小任务几分钟,大任务几十分钟)。」

跟进:

```bash
node hwtest-cli.mjs status N    # 状态/进度%/当前测试项
node hwtest-cli.mjs pause N     # 暂停(释放名额,断点可续)
node hwtest-cli.mjs resume N    # 继续跑(从断点续跑)
node hwtest-cli.mjs skip-item N # 跳过当前/下一测试项
node hwtest-cli.mjs wait N 60   # 等终态(60=最长等待分钟数)
node hwtest-cli.mjs digest N    # 终态后拿一页式结果摘要(核心指标/质量结论/产物链接)
node hwtest-cli.mjs package N   # 数据包信息(服务器物化目录绝对路径+文件清单)
```

完成后**把 digest 里的关键数字回报给用户**(1K 速度/最高档 TTFT/峰值吞吐/衰减/质量通过率/调用次数),
不要只说"跑完了"。Word 正式报告:项目页点「生成 Word」,或
`node hwtest-cli.mjs export <projectId> out.docx`。

### CLI 命令总表

| 命令 | 作用 |
|---|---|
| `check` | 验连通:平台/pi/pi-goal 版本、沙箱状态、并行上限、默认参数 |
| `projects` / `devices` / `corpus` / `questions` | 读后台资源(建任务前必用) |
| `create-task <JSON>` | 直建任务 |
| `start N` / `pause N` / `resume N` / `cancel N` | 启动/暂停/继续/取消 |
| `skip-item N` / `rerun-item N <itemId>` | 跳过测试项/单项重跑(itemId 在 status 里看) |
| `status N` / `wait N [分钟]` | 查状态/等终态 |
| `digest N` / `quality N` | 结果摘要/质量明细 |
| `package N` | 数据包(物化目录路径+文件清单) |
| `clone N` | 复制为新任务 |
| `export <projectId> out.docx` | 项目 Word 正式报告 |
| `compare <id1,id2>` | 对比看板聚合数据 |
| `parallel <N>` | 同时跑的任务条数(1-32) |
| `backup out.db` | 整库 SQLite 备份 |

---

## 附录 A · 参数全表(params 四块,全部可省略走默认)

### items(勾哪些测试项,默认勾前 5 项)

`ladder / concurrency / cache / stability / quality / niah`

### exec(执行参数)

| 参数 | 含义 | 默认 |
|---|---|---|
| `ladder_tiers` | 阶梯输入档位,如 `["1K","10K","30K"]`,支持 2.5K/1M | `["1K","10K","20K","30K"]` |
| `warmup` | 每档预热次数 | 3 |
| `repeats` | 每档正式重复次数(全局) | 3 |
| `ladder_repeats` | 仅阶梯重复(0=用 repeats) | 0 |
| `concurrency_levels` | 并发档,如 `[1,4,8]` | `[1,4,8,16,32]` |
| `concurrency_tier` | 并发请求输入档 | `"1K"` |
| `cache_tiers` | 缓存测试档位(空=跟随阶梯) | 跟随 ladder_tiers |
| `stability_minutes` | 稳定性压测时长(分钟)。**保持 1** | 1 |
| `stability_tier` | 压测输入档 | `"10K"` |
| `stability_concurrency` | 每波并发数 | 1 |

### 质量抽查(items 内子参数)

| 参数 | 含义 | 默认 |
|---|---|---|
| `quality_mode` | 选题:`all` 全部 / `cats` 按维度 / `group` 题组 / `custom` 指定题目 | all |
| `capability_cats` | 维度筛选,如 `["reasoning","code","doc","creative"]` | 全部 |
| `quality_question_ids` | 指定题目 ID(mode=custom,先读 questions) | 全部 |

### advanced(高级)

| 参数 | 含义 | 默认 |
|---|---|---|
| `thinking_mode` | 思考等级(**仅质量验证生效**,性能项不携带):`default` 不加字段 / `off` 关闭 / `minimal` 极简 / `low` / `medium` / `high` / `xhigh` 极高 / `max` 最高 | default |
| `request_timeout_sec` | 单请求超时(秒) | 300 |
| `ignore_eos` | 忽略 EOS 标志 | false |
| `extra_params` | 自定义请求参数 JSON 文本 | 空 |

### gen(生成参数,横向对比须固定)

`temperature` 0.7 / `top_p` 0.9 / `top_k` 20 / `max_tokens` 1024(性能项固定;质量题不限)

---

## 附录 B · 避坑清单

- **稳定性时长保持 1 分钟**,长时间压测烧 token;用户明确要求才调大;
- 实体机并发档不要超过服务端 `--max-num-seqs`,否则排队失真;
- NIAH 只下浮不上浮:任务上下文填 256K,探针不会超 256K 校准,填准它;
- `api_key` 敏感:必须向用户索取,平台加密存储,日志不回显,回答里不要贴完整 key;
- 涉及 ID 的(语料/题目/设备/题组)一律先读后台,读不到就问用户;
- 次数/时长类参数用户不知道就用默认,不要追问细节;
- 取消任务 `node hwtest-cli.mjs cancel N`(已完成数据保留);终态重跑用 `clone N`;
- 项目页"数据包"按钮给的是服务器物化目录绝对路径,任务全部原始数据都在那里。
