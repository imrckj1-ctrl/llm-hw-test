// hwtest-cli:智能体/CLI 操作测试平台的命令行工具(Bearer Token 认证)
// 环境: HWTEST_URL(默认 http://127.0.0.1:8080) HWTEST_TOKEN(设置页生成)
// 用法: node hwtest-cli.mjs <命令> [参数];详见 SKILL.md
const BASE = (process.env.HWTEST_URL || 'http://127.0.0.1:8080').replace(/\/$/, '') + '/api';
const TOKEN = process.env.HWTEST_TOKEN || '';

async function req(method, path, body) {
  const headers = { 'Content-Type': 'application/json' };
  if (TOKEN) headers.Authorization = 'Bearer ' + TOKEN;
  const resp = await fetch(BASE + path, { method, headers, body: body ? JSON.stringify(body) : undefined });
  const text = await resp.text();
  let data;
  try { data = JSON.parse(text); } catch { data = text; }
  if (!resp.ok) {
    console.error('HTTP ' + resp.status + ':', typeof data === 'string' ? data : JSON.stringify(data));
    process.exit(1);
  }
  return data;
}
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const out = (v) => console.log(JSON.stringify(v, null, 2));

// 阶段2/3 引导清单:`guide` 命令打印,照此向用户逐项收集/确认
const GUIDE = `== 阶段2 · 建任务信息清单(★必须用户给,其余可默认) ==
★ 项目名称 | ★ API地址(如 http://host:3000/v1) | ★ API Key(敏感,必须用户给)
★ 语料版本(用 corpus 命令读后台取最新) | 测试人员 | 备注
模型名称: 用 models <api地址> [key] 拉服务端列表给用户挑,别猜
量化精度(可空) | 上下文长度(1024进制,256K=262144,1M=1048576,默认256K)
思考等级 thinking_mode(仅质量验证生效): default / off / minimal / low / medium / high / xhigh / max
测试类型: model=云端纯模型 / hardware=本地硬件(本地硬件可 SSH 自发现: nvidia-smi / ps 启动命令 / 整机信息; 项目名仍问用户)

== 阶段3 · 逐项参数提问清单(禁止只问"跑哪些项"; 每项测不测+参数都要问, 或用户明确说"全部默认") ==
1 阶梯测试: 测不测? 哪些输入档(默认 1K/10K/20K/30K)? 每档几次(默认3)? 预热几次(默认3)?
2 并发测试: 测不测? 哪几个并发档(默认 1/4/8/16/32)? 用哪个输入档(默认1K)? 每档几轮(默认跟随)?
3 缓存测试: 测不测? 哪些档(默认跟随阶梯)? 冷热各几轮?
4 稳定性: 测不测? 压几分钟(默认1, 提醒别调大烧token)? 哪个输入档(默认10K)? 每波并发几(默认1)?
5 质量抽查: 测不测? 选题方式: all全部 / cats按维度 / group题组 / custom指定题(用 questions 命令把题库摆给用户挑)? 思考等级?
6 NIAH检索: 测不测? 勾选即自动扫描无需参数, 只需确认任务上下文长度填准
组装: items 勾选项 + exec/advanced/gen 对应参数, 全部可省(省=默认)`;

const USAGE = [
  '命令:',
  '  check                          验证连通(平台/pi/沙箱版本与状态)',
  '  projects | devices | corpus | questions   列表(建任务前先读)',
  '  create-task <JSON>             直建任务(项目壳自动生成);JSON 同 /tasks/direct',
  '  start <taskId>                 入队执行',
  '  status <taskId>                当前状态/进度',
  '  wait <taskId> [分钟=30]        阻塞等到终态',
  '  digest <taskId>                一页式结果摘要(指标/质量/产物链接)',
  '  quality <taskId>               质量抽查明细',
  '  cancel <taskId>                取消(已完成数据保留)',
  '  pause <taskId>                 暂停(释放名额,断点可续)',
  '  resume <taskId>                继续跑(从断点续跑)',
  '  skip-item <taskId>             跳过当前测试项(执行中)/下一项(暂停中)',
  '  rerun-item <taskId> <itemId>   单项重跑(itemId 见 status)',
  '  package <taskId>               数据包信息(物化目录绝对路径+文件清单)',
  '  clone <taskId>                 复制为新任务(终态重跑的唯一方式)',
  '  export <projectId> <out.docx>  导出项目 Word 正式报告',
  '  compare <id1,id2,...>          对比看板聚合数据(六测试项)',
  '  parallel <N>                   设置同时跑的任务条数(1-32)',
  '  models <api地址> [apiKey]      拉取被测服务的模型列表(选模型用)',
  '  guide                          打印建任务信息清单 + 逐项参数提问清单(阶段2/3照此问)',
  '  backup <out.db>                下载 SQLite 备份',
].join('\n');

async function download(path, file) {
  const resp = await fetch(BASE + path, { headers: TOKEN ? { Authorization: 'Bearer ' + TOKEN } : {} });
  if (!resp.ok) { console.error('HTTP', resp.status); process.exit(1); }
  const { writeFileSync } = await import('node:fs');
  writeFileSync(file, Buffer.from(await resp.arrayBuffer()));
  console.log('已保存:', file);
}

(async () => {
  const args = process.argv.slice(2);
  const cmd = args[0];
  if (cmd === 'check') {
    const st = await req('GET', '/settings');
    const sb = st.pi_sandbox || {};
    const sbText = sb.image ? '容器沙箱已就绪'
      : sb.building ? '沙箱镜像构建中'
      : sb.docker ? '有 Docker 但镜像未就绪' : '未检测到 Docker(宿主机执行)';
    return out({
      platform_version: st.platform_version, pi_version: st.pi_version,
      pi_goal_version: st.pi_goal_version, quality_engine: st.quality_engine,
      pi_container: st.pi_container, pi_sandbox: sbText,
      parallel_limit: st.parallel_limit, default_task_params: st.default_task_params,
    });
  }
  if (cmd === 'projects') return out(await req('GET', '/projects'));
  if (cmd === 'devices') return out(await req('GET', '/devices'));
  if (cmd === 'corpus') return out(await req('GET', '/corpus-versions'));
  if (cmd === 'questions') return out(await req('GET', '/questions'));
  if (cmd === 'create-task') return out(await req('POST', '/tasks/direct', JSON.parse(args[1])));
  if (cmd === 'start') return out(await req('POST', '/tasks/' + args[1] + '/start'));
  if (cmd === 'status') return out(await req('GET', '/tasks/' + args[1]));
  if (cmd === 'digest') return out(await req('GET', '/tasks/' + args[1] + '/digest'));
  if (cmd === 'quality') return out(await req('GET', '/tasks/' + args[1] + '/quality'));
  if (cmd === 'cancel') return out(await req('POST', '/tasks/' + args[1] + '/cancel'));
  if (cmd === 'pause') return out(await req('POST', '/tasks/' + args[1] + '/pause'));
  if (cmd === 'resume') return out(await req('POST', '/tasks/' + args[1] + '/resume'));
  if (cmd === 'skip-item') return out(await req('POST', '/tasks/' + args[1] + '/skip-item'));
  if (cmd === 'rerun-item') return out(await req('POST', '/tasks/' + args[1] + '/items/' + args[2] + '/rerun'));
  if (cmd === 'package') return out(await req('GET', '/tasks/' + args[1] + '/package-info'));
  if (cmd === 'clone') return out(await req('POST', '/tasks/' + args[1] + '/clone'));
  if (cmd === 'export') return download('/projects/' + args[1] + '/export.docx', args[2] || 'report.docx');
  if (cmd === 'compare') return out(await req('GET', '/compare/board?ids=' + args[1]));
  if (cmd === 'parallel') return out(await req('PUT', '/settings', { parallel_limit: String(args[1]) }));
  if (cmd === 'models') {
    // 借平台的 fetch-models 转发:GET {addr}/v1/models(带 key)
    return out(await req('POST', '/tasks/fetch-models', { api_base_url: args[1], api_key: args[2] || '' }));
  }
  if (cmd === 'guide') {
    return console.log(GUIDE);
  }
  if (cmd === 'backup') return download('/backup', args[1] || 'backup.db');
  if (cmd === 'wait') {
    const tid = args[1];
    const timeoutMs = parseInt(args[2] || '30', 10) * 60000;
    const t0 = Date.now();
    while (Date.now() - t0 < timeoutMs) {
      const t = await req('GET', '/tasks/' + tid);
      if (['done', 'failed', 'cancelled', 'interrupted'].includes(t.status)) return out(t);
      await sleep(5000);
    }
    console.error('等待超时');
    process.exit(1);
  }
  console.log(USAGE);
  process.exit(cmd ? 1 : 0);
})().catch((e) => { console.error('错误:', e.message || e); process.exit(1); });
