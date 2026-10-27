<template>
  <div class="board">
    <!-- 项目标签条:chips 增删 + 归一化开关 -->
    <div class="bar">
      <span v-for="(t, i) in tasks" :key="t.task.id" class="tag"
        :style="{ background: COLORS[i] + '18', color: COLORS[i] }"
        :title="`${t.project_name || ''}(${t.task.model_name})`">
        <span class="dot" :style="{ background: COLORS[i] }"></span>
        <span class="nm">{{ labelOf(i) }} · {{ t.task.model_name }}</span>
        <span class="x" @click="$emit('remove', t)">×</span>
      </span>
      <label class="switch">
        <input type="checkbox" v-model="normalized"> 归一化视图(以 {{ labelOf(0) }} 为 100%,柱状类图生效)
      </label>
    </div>

    <!-- 速览卡 -->
    <div class="cards">
      <div v-for="d in cardDefs" :key="d.t" class="card">
        <div class="t">{{ d.t }}</div>
        <div v-for="(row, i) in cardRows(d)" :key="i" class="row">
          <span class="who" :style="{ background: COLORS[i] }"></span>
          <span class="v">{{ row.v == null ? '—' : row.v.toFixed(1) }}</span>
          <span class="u">{{ d.u }}</span>
          <span v-if="row.best" class="bestTag">最优</span>
          <span v-if="i > 0" class="diff" :class="row.diffCls">{{ row.diffTxt }}</span>
        </div>
      </div>
    </div>

    <!-- 阶梯测试 -->
    <div class="sec">
      <h2>阶梯测试</h2>
      <div class="note">X=输入档位,一个项目一条线,末端标数值。没测某档的项目该档自动留空(线断开)。
        <label class="miniSwitch"><input type="checkbox" v-model="ladderLog"> TTFT 对数轴(高值压缩,低档位看得清)</label>
      </div>
      <div v-if="!ladderTiers.length" class="empty">所选项目均无阶梯测试数据</div>
      <div v-else class="grid2">
        <Chart :option="ladderOpt('ttft', ladderLog ? '首字延迟 TTFT(对数轴)' : '首字延迟 TTFT', 'ms', ladderLog)" :height="300" />
        <Chart :option="ladderOpt('tps', '输出速度', 'tokens/s', false)" :height="300" />
      </div>
    </div>

    <!-- 并发测试 -->
    <div class="sec">
      <h2>并发测试</h2>
      <div class="note">先选输入档,再看并发:X=并发档,一个项目一条线,直接比「同输入下谁扛得住并发」。</div>
      <div v-if="!concTiers.length" class="empty">所选项目均无并发测试数据</div>
      <template v-else>
        <div class="tabsRow">
          <el-select v-if="concTiers.length > 8" v-model="concTier" size="small" style="width: 200px">
            <el-option v-for="t in concTiers" :key="t" :label="`输入 ${t}(${concCount(t)}组)`" :value="t" />
          </el-select>
          <template v-else>
            <button v-for="t in concTiers" :key="t" class="tabBtn" :class="{ on: t === concTier }"
              @click="concTier = t">输入 {{ t }}({{ concCount(t) }}组)</button>
          </template>
        </div>
        <div class="grid2">
          <Chart :option="concOpt('tps', '输出速度', 'tokens/s')" :height="300" />
          <Chart :option="concOpt('ttft', '首字延迟 TTFT', 'ms')" :height="300" />
        </div>
      </template>
    </div>

    <!-- 缓存测试 -->
    <div class="sec">
      <h2>缓存测试</h2>
      <div class="note">柱状:各档缓存命中率与 TTFT 节省率。只有测过缓存项的项目才出柱子。</div>
      <div v-if="!cacheTiers.length" class="empty">所选项目均无缓存测试数据</div>
      <div v-else class="grid2">
        <Chart :option="cacheOpt('hit', '缓存命中率')" :height="300" />
        <Chart :option="cacheOpt('saving', 'TTFT 节省率')" :height="300" />
      </div>
    </div>

    <!-- 稳定性测试 -->
    <div class="sec">
      <h2>稳定性测试</h2>
      <div class="note">成功率柱上标「X成功/Y失败」;TTFT 与速度分开两根轴,量纲不混。</div>
      <div v-if="!stabTasks.length" class="empty">均无稳定性数据</div>
      <div v-else class="grid3">
        <Chart :option="stabRateOpt" :height="300" />
        <Chart :option="stabPerfOpt('stab_avg_ttft', '平均 TTFT', 'ms')" :height="300" />
        <Chart :option="stabPerfOpt('stab_avg_tps', '平均速度', 'tokens/s')" :height="300" />
      </div>
    </div>

    <!-- NIAH 检索 -->
    <div class="sec">
      <h2>NIAH 检索</h2>
      <div class="note">热力图:X=上下文长度,Y=探针深度,绿=命中/红=丢失,所有项目同刻度,整片红=有效上下文边界。</div>
      <div v-if="!niahTasks.length" class="empty">均无 NIAH 数据</div>
      <div v-else class="grid2">
        <Chart v-for="(t, i) in niahTasks" :key="t.task_id" :option="niahOpt(t, i)" :height="340" />
      </div>
    </div>

    <!-- 质量抽查 -->
    <div class="sec">
      <h2>质量抽查</h2>
      <div class="note">通过率汇总条形 + 逐题通过判定对照(行=题目,列=项目,未测显示 —)。</div>
      <div v-if="!qualityTasks.length" class="empty">均无质量抽查数据</div>
      <template v-else>
        <Chart :option="qualityRateOpt" :height="220" />
        <div class="qwrap" style="margin-top: 14px">
          <table class="qtable">
            <thead>
              <tr><th>题目</th><th v-for="(t, i) in qualityTasks" :key="t.task_id" :title="qTitleOf(t)">{{ labelOf(taskPos(t.task_id)) }}</th></tr>
            </thead>
            <tbody>
              <tr v-for="q in qualityQuestions" :key="q.id">
                <td>{{ q.title }}</td>
                <td v-for="t in qualityTasks" :key="t.task_id" :class="qCellCls(t, q.id)">{{ qCellTxt(t, q.id) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import api from '../api'
import Chart from './Chart.vue'

const props = defineProps({ tasks: { type: Array, default: () => [] } })
defineEmits(['remove'])

const COLORS = ['#1a73e8', '#e8591a', '#16a34a', '#9333ea', '#d97706', '#0891b2', '#e11d48', '#65a30d']
const normalized = ref(false)
const ladderLog = ref(false)
const board = ref([])
const capDetail = ref([])
const concTier = ref('')

const labelOf = (i) => 'P' + (i + 1)
const taskPos = (taskId) => props.tasks.findIndex((t) => t.task.id === taskId)
const boardOf = (taskId) => board.value.find((b) => b.task_id === taskId) || {}

watch(() => props.tasks.map((t) => t.task.id).join(','), async (ids) => {
  if (!ids) { board.value = []; capDetail.value = []; return }
  const [b, q] = await Promise.all([
    api.get('/compare/board', { params: { ids } }),
    api.get('/compare/capability-detail', { params: { ids } }),
  ])
  board.value = b.data || []
  capDetail.value = q.data || []
}, { immediate: true })

// ---------- 速览卡 ----------
const stabRate = (b) => (b.stab_ok + b.stab_fail) ? 100 * b.stab_ok / (b.stab_ok + b.stab_fail) : null
const qRateOf = (t) => {
  const ans = (capDetail.value.find((x) => x.task_id === t.task.id) || {}).answers || []
  if (!ans.length) return null
  const score = ans.reduce((n, a) => n + (a.judge_score === 'pass' ? 1 : a.judge_score === 'partial' ? 0.5 : 0), 0)
  return 100 * score / ans.length
}
const cardDefs = [
  { t: '平均输出速度', u: 'tokens/s', get: (t) => { const v = boardOf(t.task.id).avg_tps; return v > 0 ? v : null }, lower: false, pp: false },
  { t: '平均 TTFT', u: 'ms', get: (t) => { const v = boardOf(t.task.id).avg_ttft; return v > 0 ? v : null }, lower: true, pp: false },
  { t: '稳定性成功率', u: '%', get: (t) => stabRate(boardOf(t.task.id)), lower: false, pp: true },
  { t: '质量通过率', u: '%', get: (t) => qRateOf(t), lower: false, pp: true },
]
const cardRows = (d) => {
  const vals = props.tasks.map(d.get)
  const valid = vals.filter((v) => v != null)
  const best = valid.length > 1 ? (d.lower ? Math.min(...valid) : Math.max(...valid)) : null
  const base = vals[0]
  return props.tasks.map((t, i) => {
    const v = vals[i]
    let diffTxt = '—', diffCls = 'na'
    if (i > 0 && v != null && base != null) {
      if (d.pp) {
        const dd = v - base
        if (Math.abs(dd) < 0.05) { diffTxt = '持平' } else { diffTxt = (dd > 0 ? '+' : '') + dd.toFixed(1) + 'pt'; diffCls = dd > 0 ? 'up' : 'down' }
      } else if (base) {
        const dd = (v / base - 1) * 100
        const good = d.lower ? dd < 0 : dd > 0
        diffTxt = (dd > 0 ? '+' : '') + dd.toFixed(1) + '%'
        diffCls = good ? 'up' : 'down'
      }
    }
    return { v, best: v != null && v === best, diffTxt, diffCls }
  })
}

// ---------- 通用 ----------
const tierNum = (t) => {
  const m = /^([\d.]+)(K|M)?$/i.exec(String(t || ''))
  if (!m) return 0
  return parseFloat(m[1]) * ((m[2] || '').toUpperCase() === 'M' ? 1000 : 1)
}
const unionKeys = (arrs, keyFn, sorter) => {
  const keys = [...new Set(arrs.flatMap((a) => (a || []).map(keyFn)))]
  if (sorter) keys.sort(sorter)
  else keys.sort()
  return keys
}
const legendFor = () => ({ bottom: 0, textStyle: { fontSize: 11 } })
const tip = { trigger: 'axis' }
// V3.0.1:tooltip 数值精度与任务详情口径统一(速度 2 位小数 / TTFT 1 位 / 百分比 1 位)
const tipFmt = (kind) => ({
  trigger: 'axis',
  valueFormatter: (v) => {
    if (v == null || v === '' || v === '-') return '-'
    const n = +v
    if (Number.isNaN(n)) return String(v)
    if (kind === 'pct') return n.toFixed(1) + '%'
    return n.toFixed(kind === 'ttft' ? 1 : 2)
  },
})
const normV = (v, base) => (normalized.value && v != null && base) ? +(v / base * 100).toFixed(1) : v
// 标签数值格式化:速度/比率 1 位小数,TTFT 取整
const fmtLbl = (v, dec = 1) => (v.value == null ? '' : (+v.value).toFixed(dec))

// ---------- 阶梯 ----------
const ladderTiers = computed(() => unionKeys(board.value.map((b) => b.ladder), (x) => x.tier, (a, b) => tierNum(a) - tierNum(b)))
const ladderOpt = (k, name, unit, useLog) => ({
  tooltip: tipFmt(k === 'ttft' ? 'ttft' : 'tps'), legend: legendFor(),
  grid: { left: 60, right: 60, top: 34, bottom: 52 },
  xAxis: { type: 'category', data: ladderTiers.value, name: '档位' },
  yAxis: useLog ? { type: 'log', name: unit } : { type: 'value', name: unit, scale: true },
  series: props.tasks.map((t, i) => {
    const rows = boardOf(t.task.id).ladder || []
    return {
      name: labelOf(i), type: 'line', color: COLORS[i], connectNulls: false, symbolSize: 6,
      data: ladderTiers.value.map((tier) => { const r = rows.find((x) => x.tier === tier); return r ? r[k] : null }),
      endLabel: { show: true, formatter: (v) => fmtLbl(v, k === 'ttft' ? 0 : 1), fontSize: 10 },
      labelLayout: { hideOverlap: true },
    }
  }),
  title: { text: name, left: 'center', textStyle: { fontSize: 13 } },
})

// ---------- 并发 ----------
const concTiers = computed(() => unionKeys(board.value.map((b) => b.conc), (x) => x.tier, (a, b) => tierNum(a) - tierNum(b)))
const concCount = (tier) => board.value.reduce((n, b) => n + (b.conc || []).filter((x) => x.tier === tier).length, 0)
watch(concTiers, (tiers) => {
  if (!tiers.includes(concTier.value)) {
    concTier.value = [...tiers].sort((a, b) => concCount(b) - concCount(a))[0] || ''
  }
}, { immediate: true })
const concLevels = computed(() => unionKeys(
  board.value.map((b) => (b.conc || []).filter((x) => x.tier === concTier.value)), (x) => x.c, (a, b) => a - b))
const concOpt = (k, name, unit) => ({
  tooltip: tipFmt(k === 'ttft' ? 'ttft' : 'tps'), legend: legendFor(),
  grid: { left: 60, right: 60, top: 34, bottom: 52 },
  xAxis: { type: 'category', data: concLevels.value.map((c) => c + ' 并发'), name: '并发档' },
  yAxis: { type: 'value', name: unit, scale: true },
  series: props.tasks.map((t, i) => {
    const rows = (boardOf(t.task.id).conc || []).filter((x) => x.tier === concTier.value)
    return {
      name: labelOf(i), type: 'line', color: COLORS[i], symbolSize: 7, connectNulls: true, // V3.0.1:并发档并集下跨空档连线,便于直接对比
      data: concLevels.value.map((c) => { const r = rows.find((x) => x.c === c); return r ? r[k] : null }),
      endLabel: { show: true, formatter: (v) => fmtLbl(v, k === 'ttft' ? 0 : 1), fontSize: 10 },
      labelLayout: { hideOverlap: true },
    }
  }),
  title: { text: `${name} · 输入 ${concTier.value}`, left: 'center', textStyle: { fontSize: 13 } },
})

// ---------- 缓存 ----------
const cacheTiers = computed(() => {
  const has = board.value.some((b) => (b.cache || []).some((x) => x.hit != null || x.saving != null))
  return has ? unionKeys(board.value.map((b) => b.cache), (x) => x.tier, (a, b) => tierNum(a) - tierNum(b)) : []
})
const cacheOpt = (k, name) => {
  const baseB = boardOf(props.tasks[0]?.task.id)
  return {
    tooltip: tipFmt('pct'), legend: legendFor(),
    grid: { left: 55, right: 16, top: 34, bottom: 52 },
    xAxis: { type: 'category', data: cacheTiers.value },
    yAxis: { type: 'value', name: normalized.value ? '%(相对P1)' : '%', max: normalized.value ? null : 100 },
    series: props.tasks.map((t, i) => {
      const rows = boardOf(t.task.id).cache || []
      return {
        name: labelOf(i), type: 'bar', color: COLORS[i],
        data: cacheTiers.value.map((tier) => {
          const r = rows.find((x) => x.tier === tier)
          const br = (baseB.cache || []).find((x) => x.tier === tier)
          return r ? normV(r[k], br && br[k]) : null
        }),
        label: { show: true, position: 'top', fontSize: 9, formatter: (v) => fmtLbl(v, 1) },
      }
    }),
    title: { text: name, left: 'center', textStyle: { fontSize: 13 } },
  }
}

// ---------- 稳定性 ----------
const stabTasks = computed(() => props.tasks.filter((t) => {
  const b = boardOf(t.task.id)
  return (b.stab_ok || 0) + (b.stab_fail || 0) > 0
}))
const stabRateOpt = computed(() => ({
  tooltip: tipFmt('pct'),
  grid: { left: 48, right: 12, top: 34, bottom: 52 },
  xAxis: { type: 'category', data: stabTasks.value.map((t) => labelOf(taskPos(t.task.id))) },
  yAxis: { type: 'value', max: 100, name: '%' },
  series: [{
    type: 'bar',
    data: stabTasks.value.map((t) => {
      const r = stabRate(boardOf(t.task.id))
      return { value: +r.toFixed(1), itemStyle: { color: r >= 99 ? '#16a34a' : r >= 90 ? '#d97706' : '#dc2626' } }
    }),
    label: {
      show: true, position: 'top', fontSize: 10,
      formatter: (v) => `${v.value}% (${boardOf(stabTasks.value[v.dataIndex].task.id).stab_ok}成功/${boardOf(stabTasks.value[v.dataIndex].task.id).stab_fail}失败)`,
    },
  }],
  title: { text: '成功率', left: 'center', textStyle: { fontSize: 13 } },
}))
const stabPerfOpt = (k, name, unit) => {
  const baseB = stabTasks.value.length ? boardOf(stabTasks.value[0].task.id) : {}
  return {
    tooltip: tipFmt(normalized.value ? 'pct' : (k === 'stab_avg_ttft' ? 'ttft' : 'tps')),
    grid: { left: 55, right: 12, top: 34, bottom: 52 },
    xAxis: { type: 'category', data: stabTasks.value.map((t) => labelOf(taskPos(t.task.id))) },
    yAxis: { type: 'value', name: normalized.value ? '%(相对P1)' : unit },
    series: [{
      type: 'bar',
      data: stabTasks.value.map((t) => ({
        value: normV(boardOf(t.task.id)[k], baseB[k]),
        itemStyle: { color: COLORS[taskPos(t.task.id)] },
      })),
      label: { show: true, position: 'top', fontSize: 9, formatter: (v) => fmtLbl(v, k === 'stab_avg_ttft' ? 0 : 1) },
    }],
    title: { text: name, left: 'center', textStyle: { fontSize: 13 } },
  }
}

// ---------- NIAH ----------
const niahTasks = computed(() => board.value.filter((b) => (b.niah || []).length))
const niahOpt = (b, idx) => {
  const cells = b.niah || []
  const lens = unionKeys([cells], (x) => x.tier, (a, bb) => tierNum(a) - tierNum(bb))
  const depths = [...new Set(cells.map((x) => x.depth_pct))].sort((a, bb) => a - bb)
  const data = []
  depths.forEach((dp, y) => lens.forEach((ln, x) => {
    const c = cells.find((v) => v.tier === ln && v.depth_pct === dp)
    data.push([x, y, c ? +(c.hit_rate * 100).toFixed(0) : '-'])
  }))
  return {
    tooltip: {},
    grid: { left: 60, right: idx === 0 ? 70 : 20, top: 36, bottom: 40 },
    xAxis: { type: 'category', data: lens, name: '上下文' },
    yAxis: { type: 'category', data: depths.map((d) => d + '%'), name: '深度' },
    visualMap: {
      show: idx === 0, min: 0, max: 100, calculable: false, orient: 'vertical', right: 0, top: 'center',
      inRange: { color: ['#dc2626', '#f59e0b', '#16a34a'] }, text: ['命中', '丢失'], textStyle: { fontSize: 10 },
    },
    series: [{ type: 'heatmap', data, label: { show: true, fontSize: 9, formatter: (p) => p.value[2] } }],
    title: { text: `${labelOf(taskPos(b.task_id))} · 命中率`, left: 'center', textStyle: { fontSize: 13 } },
  }
}

// ---------- 质量 ----------
const qualityTasks = computed(() => capDetail.value.filter((t) => (t.answers || []).length))
const qTitleOf = (t) => {
  const row = props.tasks.find((x) => x.task.id === t.task_id)
  return row ? `${row.project_name || ''}(${row.task.model_name})` : ''
}
const qualityQuestions = computed(() => {
  const map = new Map()
  for (const t of qualityTasks.value) {
    for (const a of t.answers) {
      if (!map.has(a.question_id)) map.set(a.question_id, { id: a.question_id, title: a.question })
    }
  }
  return [...map.values()]
})
const qAnswer = (t, qid) => (t.answers || []).find((a) => a.question_id === qid)
const qCellCls = (t, qid) => {
  const a = qAnswer(t, qid)
  if (!a) return 'miss'
  return { pass: 'pass', partial: 'partial', fail: 'fail' }[a.judge_score] || 'miss'
}
const qCellTxt = (t, qid) => {
  const a = qAnswer(t, qid)
  if (!a) return '—'
  if (!a.judge_score) return '未判定'
  return { pass: '通过', partial: '部分对', fail: '未通过' }[a.judge_score] || '—'
}
const qualityRateOpt = computed(() => ({
  tooltip: tipFmt('pct'),
  grid: { left: 55, right: 16, top: 34, bottom: 30 },
  xAxis: { type: 'category', data: qualityTasks.value.map((t) => labelOf(taskPos(t.task_id))) },
  yAxis: { type: 'value', max: 100, name: '%' },
  series: [{
    type: 'bar', barWidth: 46,
    data: qualityTasks.value.map((t) => {
      const ans = t.answers
      const score = ans.reduce((n, a) => n + (a.judge_score === 'pass' ? 1 : a.judge_score === 'partial' ? 0.5 : 0), 0)
      const rate = +(100 * score / ans.length).toFixed(1)
      const passCnt = ans.filter((a) => a.judge_score === 'pass').length
      return {
        value: rate,
        itemStyle: { color: COLORS[taskPos(t.task_id)] },
        label: { show: true, position: 'top', formatter: `${rate}% (${passCnt}/${ans.length})` },
      }
    }),
  }],
  title: { text: '质量通过率', left: 'center', textStyle: { fontSize: 13 } },
}))
</script>

<style scoped>
.board { margin-top: 14px; }
.bar { background: #fff; border: 1px solid #e4e9f0; border-radius: 8px; padding: 14px 18px; margin-bottom: 16px; display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
.tag { padding: 4px 10px; border-radius: 6px; font-size: 12px; font-weight: 600; display: inline-flex; align-items: center; gap: 6px; }
.tag .nm { max-width: 200px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.tag .x { cursor: pointer; font-weight: 700; opacity: .55; margin-left: 2px; }
.tag .x:hover { opacity: 1; }
.dot { width: 10px; height: 10px; border-radius: 2px; display: inline-block; flex: none; }
.switch { margin-left: auto; display: flex; align-items: center; gap: 8px; font-size: 13px; color: #5a6472; }
.cards { display: grid; grid-template-columns: repeat(4, 1fr); gap: 12px; margin-bottom: 16px; }
.card { background: #fff; border: 1px solid #e4e9f0; border-radius: 8px; padding: 12px 14px; }
.card .t { font-size: 12px; color: #7a8699; margin-bottom: 6px; }
.card .row { display: flex; align-items: baseline; gap: 6px; margin-top: 3px; font-size: 13px; }
.card .who { width: 10px; height: 10px; border-radius: 2px; flex: none; align-self: center; }
.card .v { font-size: 15px; font-weight: 700; }
.card .u { font-size: 11px; color: #9aa5b1; }
.bestTag { font-size: 10px; font-weight: 700; color: #b45309; background: #fef3c7; border-radius: 8px; padding: 1px 7px; margin-left: 2px; }
.diff { font-size: 11px; font-weight: 600; margin-left: auto; padding: 1px 7px; border-radius: 9px; }
.up { background: #e6f7ec; color: #16a34a; }
.down { background: #fdeaea; color: #dc2626; }
.na { background: #f0f2f5; color: #9aa5b1; }
.sec { background: #fff; border: 1px solid #e4e9f0; border-radius: 8px; padding: 18px; margin-bottom: 16px; }
.sec h2 { font-size: 16px; margin-bottom: 2px; }
.sec .note { font-size: 12px; color: #8a94a3; margin-bottom: 12px; }
.miniSwitch { font-size: 12px; color: #5a6472; display: inline-flex; align-items: center; gap: 4px; margin-left: 10px; font-weight: 400; }
.grid2 { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }
.grid3 { display: grid; grid-template-columns: 1fr 1fr 1fr; gap: 16px; }
.tabsRow { display: flex; gap: 6px; margin-bottom: 12px; flex-wrap: wrap; }
.tabBtn { padding: 4px 14px; border: 1px solid #e0e5ec; border-radius: 14px; background: #fff; font-size: 12px; cursor: pointer; color: #5a6472; }
.tabBtn.on { background: #1a73e8; border-color: #1a73e8; color: #fff; }
.empty { display: flex; align-items: center; justify-content: center; height: 120px; color: #9aa5b1; font-size: 13px; background: #fafbfc; border-radius: 6px; }
.qwrap { max-height: 380px; overflow: auto; border: 1px solid #eef1f5; border-radius: 6px; }
.qtable { width: 100%; border-collapse: collapse; font-size: 13px; }
.qtable th, .qtable td { padding: 7px 10px; border-bottom: 1px solid #eef1f5; text-align: left; }
.qtable thead th { position: sticky; top: 0; background: #f7f9fb; color: #5a6472; font-weight: 600; z-index: 1; }
.pass { color: #16a34a; font-weight: 700; }
.partial { color: #d97706; font-weight: 700; }
.fail { color: #dc2626; font-weight: 700; }
.miss { color: #b6bec9; }
</style>
