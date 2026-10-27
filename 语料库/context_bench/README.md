# 模型上下文压测语料集

基于《三体》三部曲正文切分的 48 档测试语料，用于测试大模型的**上下文承载能力、并发吞吐、处理速度**。

## 语料规格

| 项目 | 取值 |
|---|---|
| 分词口径 | Qwen2.5（与 DeepSeek、GLM 等国产模型一致，中文 1.49 字符/token） |
| 来源 | 《三体》三部曲正文（已剔除丛书序、附录制作信息） |
| 正文 token 池 | 602,416 |
| 文件结构 | 正文 + 空行 + 固定提问 |
| 提问句 | 请简单介绍一下这段文字描述的什么。50字内。 |
| 计数方式 | **总长精确等于档位值**（正文 = 档位值 − 15 token 提问） |
| 前缀一致性 | 各档位内容前缀相同，小档位是大档位的严格前缀 |
| 复读策略 | 超过 60.2 万 token 的档位循环复读原文（600K 起共 9 档需复读，1M 档复读 1.74 遍） |
| 档位数 | 48 |
| 语料总 token | 15,835,136 |
| 磁盘占用 | 67 MB |

## 档位清单

- 起步：`1K`
- 细粒度：`10K` `15K` `20K` `25K` `30K` `35K` `40K` `50K` `60K` `70K` `80K` `90K`
- 常规步进：`100K` 起每 `20K` 递增至 `500K`
- 对齐常见窗口：`32K` `64K` `256K`
- 高分段：`550K` `600K` `650K` `700K` `750K` `800K` `850K` `900K` `950K`
- 超大档：`512K` `1M`

K = 1024，1M = 1,048,576。完整清单见 `manifest.csv`。

## 文件命名

文件名即档位名，直接可用：

```
1K.txt    10K.txt    32K.txt    128K.txt
256K.txt  550K.txt   600K.txt   950K.txt  1M.txt
```

每个文件 = 该档位精确 token 数的正文 + 空行 + 末尾提问句。

注意：文件名是字符串序（`1K` 后面排 `1M` 再 `10K`），排序不等于按长度排序。要按 token 从小到大遍历请读 `manifest.csv`。

## 执行压测

```bash
# 串行扫描全部档位，测 TTFT / 耗时 / 输出速度，并自动捕获超长报错
python bench_runner.py scan \
  --base-url http://192.168.31.169:3000/v1 \
  --model Qwen3-8B --repeat 3

# 只测几个档位
python bench_runner.py scan --model Qwen3-8B --levels 1K,32K,128K

# 并发压测（固定档位，8 并发 × 20 次）
python bench_runner.py concurrent \
  --model Qwen3-8B --levels 32K --concurrency 8 --repeat 20
```

输出 CSV 列：档位、输入 token、并发数、TTFT、端到端耗时、输出 token、输出速度、状态、错误信息、回答内容。

模型装不下的档位会记为 `FAIL` 并保留 HTTP 报错原文——这是判断真实上下文上限的关键数据。

## 复现与校验

```bash
python build_context_bench.py                    # 重新生成全部语料
python build_context_bench.py --only 600K,650K   # 只生成指定档位，manifest 自动合并
python verify_tokens.py                          # 独立校验落盘文件的 token 数
```

当前状态：48 个文件全部精确命中目标 token 数，校验 0 失败。
