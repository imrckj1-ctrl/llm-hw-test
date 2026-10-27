package exporter

// 交互式结果页模板:__TITLE__/__ECHARTS__/__DATA__ 三处注入(见 results_html.go)
const resultsHTMLHead = `<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8">
<title>__TITLE__</title>
<style>
*{box-sizing:border-box;margin:0}
body{font-family:"Helvetica Neue","Microsoft YaHei",sans-serif;background:#f5f7fa;color:#303133;font-size:13px}
.page{max-width:1200px;margin:0 auto;padding:14px 18px 40px}
.hd{margin-bottom:12px}.hd h1{font-size:17px;color:#0B2545}
.hd .sub{color:#909399;font-size:12px;margin-top:2px}
.task-h{font-size:15px;font-weight:600;color:#2E74B5;margin:16px 0 10px}
.cards{display:flex;gap:10px;flex-wrap:wrap;margin-bottom:14px}
.metric{flex:1;min-width:140px;background:#fff;border:1px solid #e4e7ed;border-radius:6px;padding:12px;text-align:center}
.metric b{font-size:20px;color:#0B2545}.metric div{font-size:12px;color:#909399;margin-top:3px}
.card{background:#fff;border:1px solid #e4e7ed;border-radius:6px;margin-bottom:14px}
.card-h{padding:10px 14px;border-bottom:1px solid #ebeef5;font-weight:600;font-size:14px;display:flex;gap:8px;align-items:center}
.tag{font-size:11px;font-weight:400;background:#f0f9eb;color:#67c23a;border:1px solid #e1f3d8;border-radius:4px;padding:1px 6px}
.card-b{padding:12px 14px}
table{width:100%;border-collapse:collapse;font-size:12px;margin-bottom:10px}
th,td{border:1px solid #ebeef5;padding:5px 7px;text-align:center}
th{background:#f5f7fa;color:#606266;font-weight:600}
.charts{display:flex;gap:12px;flex-wrap:wrap}
.chart{flex:1;min-width:420px;height:280px}
.alert{background:#f0f9eb;border:1px solid #e1f3d8;color:#67c23a;border-radius:6px;padding:8px 12px;margin-bottom:10px;font-size:12px}
.muted{color:#909399;font-size:12px}
</style></head><body>
<div class="page">
  <div class="hd"><h1>__TITLE__</h1><div class="sub">灵思微致测试平台 · 交互式结果页(图表可悬停查看数值 / 点击图例开关曲线)</div></div>
  <div id="root"></div>
</div>
<script>__ECHARTS__</script>
<script>
const D = __DATA__;
const root = document.getElementById('root');
const el = (h) => { const d = document.createElement('div'); d.innerHTML = h; return d.firstElementChild; };
const fmt = (v, n = 1) => (v == null || isNaN(v)) ? '-' : (+v).toFixed(n);
const distCols = (o, n = 1) => ['avg','p50','p95','p99','min','max'].map(k => '<td>' + fmt(o?.[k], n) + '</td>').join('');
const distHead = '<th>平均</th><th>P50</th><th>P95</th><th>P99</th><th>最短</th><th>最长</th>';
const card = (t, inner) => el('<div class="card"><div class="card-h">' + t + '<span class="tag">完成</span></div><div class="card-b">' + inner + '</div></div>');
const mount = (node, opt) => { const c = echarts.init(node); c.setOption(opt); window.addEventListener('resize', () => c.resize()); };
const base = (t) => ({ title: { text: t, textStyle: { fontSize: 13 } }, tooltip: { trigger: 'axis' }, grid: { left: 60, right: 30, top: 50, bottom: 55 }, legend: { bottom: 0, type: 'scroll', textStyle: { fontSize: 11 } } });

(D.tasks || []).forEach((T, ti) => {
  if (D.tasks.length > 1) root.appendChild(el('<div class="task-h">' + T.title + '</div>'));
  const cards = el('<div class="cards"></div>');
  (T.cards || []).forEach(([v, l]) => cards.appendChild(el('<div class="metric"><b>' + v + '</b><div>' + l + '</div></div>')));
  root.appendChild(cards);
  const id = (n) => 't' + ti + '_' + n;

  // ---- 阶梯 ----
  if (T.ladder && T.ladder.length) {
    const rows = T.ladder.filter(r => !r.skipped);
    const tbl = '<table><tr><th rowspan="2">档位</th><th rowspan="2">实际 tokens</th><th colspan="6">TTFT(ms)</th><th colspan="6">速度(tokens/s)</th><th rowspan="2">样本数</th></tr><tr>' + distHead + distHead + '</tr>'
      + rows.map(r => '<tr><td><b>' + r.tier + '</b></td><td>' + r.prompt_tokens + '</td>' + distCols(r.ttft) + distCols(r.speed, 2) + '<td>' + r.samples + '</td></tr>').join('') + '</table>';
    const c = card('阶梯测试', tbl + '<div class="charts"><div class="chart" id="' + id('ls') + '"></div><div class="chart" id="' + id('lt') + '"></div></div>');
    root.appendChild(c);
    mount(c.querySelector('#' + id('ls')), { ...base('阶梯 - 输出速度'), xAxis: { type: 'category', data: rows.map(r => r.tier) }, yAxis: { type: 'value', name: 'tokens/s' }, series: [{ type: 'line', smooth: true, data: rows.map(r => +r.speed.avg.toFixed(2)) }] });
    mount(c.querySelector('#' + id('lt')), { ...base('阶梯 - TTFT'), xAxis: { type: 'category', data: rows.map(r => r.tier) }, yAxis: { type: 'value', name: 'ms' }, series: [{ type: 'line', smooth: true, data: rows.map(r => +r.ttft.avg.toFixed(1)) }] });
  }

  // ---- 并发 ----
  if (T.concurrency && T.concurrency.length) {
    const tiers = [...new Set(T.concurrency.map(r => r.tier || '1K'))];
    const levels = [...new Set(T.concurrency.filter(r => !r.circuit_trip).map(r => r.level))].sort((a, b) => a - b);
    const groupRows = tiers.map(t => {
      const g = T.concurrency.filter(r => (r.tier || '1K') === t);
      return g.map((r, i) => '<tr>' + (i === 0 ? '<td rowspan="' + g.length + '"><b>' + t + '</b></td>' : '') + '<td><b>' + r.level + '</b></td><td>' + r.rounds + '/' + r.planned_rounds + '</td><td>' + r.ok_count + '/' + r.fail_count + '</td>' + (r.circuit_trip ? '<td colspan="12" style="color:#f56c6c">熔断跳过</td>' : distCols(r.ttft) + distCols(r.speed, 2)) + '</tr>').join('');
    }).join('');
    const tbl = '<table><tr><th rowspan="2">输入档</th><th rowspan="2">并发档</th><th rowspan="2">进度</th><th rowspan="2">成功/失败</th><th colspan="6">TTFT(ms)</th><th colspan="6">单请求速度(tokens/s)</th></tr><tr>' + distHead + distHead + '</tr>' + groupRows + '</table>';
    const c = card('并发测试', tbl + '<div class="charts"><div class="chart" id="' + id('cs') + '"></div><div class="chart" id="' + id('ct') + '"></div></div>');
    root.appendChild(c);
    const mk = (pick) => tiers.map(t => ({ name: t, type: 'line', smooth: true, data: levels.map(l => { const r = T.concurrency.find(x => x.level === l && (x.tier || '1K') === t && !x.circuit_trip); const v = r ? pick(r) : null; return v == null || isNaN(v) ? null : v; }) }));
    mount(c.querySelector('#' + id('cs')), { ...base('并发 - 平均每请求速度 TPS(decode,不含 TTFT)'), xAxis: { type: 'category', data: levels.map(l => 'C' + l) }, yAxis: { type: 'value', name: 'tokens/s' }, series: mk(r => +r.speed.avg.toFixed(2)) });
    mount(c.querySelector('#' + id('ct')), { ...base('并发 - TTFT'), xAxis: { type: 'category', data: levels.map(l => 'C' + l) }, yAxis: { type: 'value', name: 'ms' }, series: mk(r => +r.ttft.avg.toFixed(1)) });
  }

  // ---- 缓存 ----
  if (T.cache && T.cache.length) {
    const tbl = '<table><tr><th>档位</th><th>冷TTFT(ms)</th><th>热TTFT(ms)</th><th>提升倍数</th><th>命中率(%)</th></tr>'
      + T.cache.map(r => '<tr><td><b>' + r.tier + '</b></td><td>' + fmt(r.cold_ttft) + '</td><td>' + fmt(r.hot_ttft) + '</td><td>' + fmt(r.improvement, 2) + '</td><td>' + (r.hit_rate ? (r.hit_rate * 100).toFixed(1) : '-') + '</td></tr>').join('') + '</table>';
    const c = card('缓存测试', tbl + '<div class="charts"><div class="chart" id="' + id('ca') + '"></div></div>');
    root.appendChild(c);
    mount(c.querySelector('#' + id('ca')), { ...base('缓存 - 各档位命中率'), xAxis: { type: 'category', data: T.cache.map(r => r.tier) }, yAxis: { type: 'value', name: '命中率%', max: 100, min: 0 }, series: [{ type: 'line', smooth: true, data: T.cache.map(r => +(r.hit_rate * 100).toFixed(1)) }] });
  }

  // ---- 稳定性 ----
  if (T.stability && T.stability.points && T.stability.points.length) {
    const pts = T.stability.points;
    // V2.23:压测参数行(输入档/每波并发/时长)与详情页一致
    const note = T.stability.note ? '<div class="muted" style="margin-bottom:8px">' + T.stability.note + '</div>' : '';
    const dec = T.stability.decay != null ? '<div class="alert">性能衰减 ' + (+T.stability.decay).toFixed(2) + '%</div>' : '';
    const c = card('稳定性测试', note + dec + '<div class="charts"><div class="chart" id="' + id('ss') + '"></div><div class="chart" id="' + id('st') + '"></div></div>');
    root.appendChild(c);
    const scat = (pick) => [{ type: 'scatter', symbolSize: 7, data: pts.filter(p => p.ok).map(p => [p.i, +pick(p).toFixed(2)]) }];
    mount(c.querySelector('#' + id('ss')), { ...base('稳定性 - 逐次输出速度'), xAxis: { type: 'value', name: '调用序号' }, yAxis: { type: 'value', name: 'tokens/s' }, series: scat(p => p.speed) });
    mount(c.querySelector('#' + id('st')), { ...base('稳定性 - 逐次 TTFT'), xAxis: { type: 'value', name: '调用序号' }, yAxis: { type: 'value', name: 'ms' }, series: scat(p => p.ttft) });
  }

  // ---- NIAH ----
  if (T.niah && T.niah.length) {
    const eff = T.niah_effective;
    const alert = eff ? '<div class="alert">实测有效上下文约 <b>' + eff.tokens.toLocaleString() + '</b> tokens' + (eff.pct ? '(任务上下文的 ' + eff.pct + '%)' : '') + '</div>' : '';
    const isMatrix = T.niah[0].mode === 'verify' || T.niah[0].mode === 'legacy';
    let tbl;
    let chart = '';
    if (isMatrix) {
      const depths = [...new Set(T.niah.map(r => r.depth_pct))].sort((a, b) => a - b);
      const tiers2 = [...new Set(T.niah.map(r => r.tier))];
      // V2.22:热力图(X=长度点位,Y=埋针深度,绿=命中/红=丢失)
      chart = '<div id="' + id('nh') + '" style="width:100%;height:' + Math.max(220, 90 + depths.length * 46) + 'px"></div>';
      tbl = '<table><tr><th>输入长度点位</th>' + depths.map(d => '<th>埋针深度 ' + d + '%</th>').join('') + '</tr>'
        + tiers2.map(t => '<tr><td><b>' + t + '</b></td>' + depths.map(d => { const c = T.niah.find(x => x.tier === t && x.depth_pct === d); return '<td>' + (c && c.probes > 0 ? (c.hit_rate * 100).toFixed(0) + '%(' + c.hits + '/' + c.probes + ')' : '-') + '</td>'; }).join('') + '</tr>').join('') + '</table>';
    } else {
      const rows = [...T.niah].sort((a, b) => (a.tokens || a.pct) - (b.tokens || b.pct));
      // V2.22:扫描模式改柱状图(绿=能找回/红=找不回)
      chart = '<div id="' + id('nb') + '" style="width:100%;height:280px"></div>';
      tbl = '<table><tr><th>输入长度</th><th>点位</th><th>埋针深度</th><th>命中率</th><th>错误数</th><th>TTFT中位(ms)</th></tr>'
        + rows.map(r => '<tr><td><b>' + r.tier + '</b></td><td>' + r.pct + (r.mode === 'discover' ? 'K' : '%') + '</td><td>' + r.depth_pct + '%</td><td>' + (r.hit_rate * 100).toFixed(0) + '%(' + r.hits + '/' + r.probes + ')</td><td>' + r.errors + '</td><td>' + fmt(r.ttft_median) + '</td></tr>').join('') + '</table>';
    }
    root.appendChild(card('NIAH 检索测试', alert + chart + tbl));
    // 图表挂载(div 已入 DOM)
    if (isMatrix) {
      const depths = [...new Set(T.niah.map(r => r.depth_pct))].sort((a, b) => a - b);
      const tiers2 = [...new Set(T.niah.map(r => r.tier))];
      const data = [];
      depths.forEach((d, y) => tiers2.forEach((t, x) => {
        const c = T.niah.find(v => v.tier === t && v.depth_pct === d);
        data.push([x, y, c && c.probes > 0 ? +(c.hit_rate * 100).toFixed(0) : '-']);
      }));
      mount(document.querySelector('#' + id('nh')), {
        tooltip: { formatter: (p) => tiers2[p.value[0]] + ' · 深度 ' + depths[p.value[1]] + '%<br/>命中率 ' + p.value[2] + '%' },
        grid: { left: 70, right: 80, top: 40, bottom: 44 },
        xAxis: { type: 'category', data: tiers2, name: '上下文长度' },
        yAxis: { type: 'category', data: depths.map(d => d + '%'), name: '埋针深度' },
        visualMap: { min: 0, max: 100, calculable: false, orient: 'vertical', right: 0, top: 'center',
          inRange: { color: ['#dc2626', '#f59e0b', '#16a34a'] }, text: ['命中', '丢失'], textStyle: { fontSize: 10 } },
        series: [{ type: 'heatmap', data, label: { show: true, fontSize: 10, formatter: (p) => p.value[2] } }],
        title: { text: 'NIAH 命中率热力图', left: 'center', textStyle: { fontSize: 13 } },
      });
    } else {
      const rows = [...T.niah].sort((a, b) => (a.tokens || a.pct) - (b.tokens || b.pct));
      mount(document.querySelector('#' + id('nb')), {
        tooltip: { trigger: 'axis' },
        grid: { left: 60, right: 30, top: 50, bottom: 60 },
        xAxis: { type: 'category', data: rows.map(r => r.tier), axisLabel: { interval: 0, fontSize: 10 } },
        yAxis: { type: 'value', name: '找回率%', max: 100, min: 0 },
        series: [{ type: 'bar', barMaxWidth: 42,
          data: rows.map(r => ({ value: +(r.hit_rate * 100).toFixed(1),
            itemStyle: { color: r.hit_rate >= 0.999 ? '#16a34a' : r.hit_rate >= 0.5 ? '#d97706' : '#dc2626' } })),
        }],
        title: { text: 'NIAH 有效上下文扫描', left: 'center', textStyle: { fontSize: 13 } },
      });
    }
  }

  // ---- 质量 ----
  if (T.quality && T.quality.list && T.quality.list.length) {
    const q = T.quality;
    const alert = '<div class="alert">结论:' + q.verdict + ';共 ' + q.total + ' 题,异常 ' + q.abnormal + ',存疑 ' + q.suspect + '</div>';
    const tbl = '<table><tr><th>题目</th><th>裁判评分</th><th>初筛</th></tr>'
      + q.list.map(r => '<tr><td style="text-align:left">' + r.t + '</td><td>' + r.s + '</td><td>' + r.f + '</td></tr>').join('') + '</table>';
    root.appendChild(card('质量抽查', alert + tbl));
  }
});
</script></body></html>`
