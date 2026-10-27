<template>
  <div>
    <BackBar title="首页">
      <span class="refresh-note">每 3 秒自动刷新 · {{ clock }}</span>
    </BackBar>

    <el-row :gutter="12" class="stat-row">
      <el-col :span="4" v-for="c in statCards" :key="c.label">
        <el-card shadow="never" class="stat-card">
          <div class="stat-num" :style="{ color: c.color }">{{ c.value }}</div>
          <div class="stat-label">{{ c.label }}</div>
        </el-card>
      </el-col>
    </el-row>

    <el-row :gutter="12">
      <el-col :span="10">
        <el-card shadow="never">
          <template #header><b>近 7 日任务完成趋势</b></template>
          <Chart :option="trendChart" :height="230" />
        </el-card>
      </el-col>
      <el-col :span="7">
        <el-card shadow="never">
          <template #header><b>任务状态分布</b></template>
          <Chart :option="statusChart" :height="230" />
        </el-card>
      </el-col>
      <el-col :span="7">
        <el-card shadow="never">
          <template #header><b>任务完成率</b></template>
          <Chart :option="rateChart" :height="230" />
        </el-card>
      </el-col>
    </el-row>

    <el-row :gutter="12" style="margin-top: 12px">
      <el-col :span="12">
        <el-card shadow="never">
          <template #header>
            <div class="tbl-head">
              <b>正在执行</b>
              <el-tag v-if="(data.live || []).length" type="primary" size="small" effect="plain">{{ data.live.length }} 项</el-tag>
            </div>
          </template>
          <el-table :data="data.live || []" size="small" stripe>
            <el-table-column label="任务" width="90" align="center">
              <template #default="{ row }">
                <el-link type="primary" @click="$router.push(`/tasks/${row.task_id}`)">{{ row.task_id }}</el-link>
                <span class="muted"> run{{ row.run_no }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="device_name" label="设备/来源" min-width="120" show-overflow-tooltip />
            <el-table-column label="模型" min-width="120" show-overflow-tooltip>
              <template #default="{ row }">{{ row.model_name }} <span class="muted">{{ row.quant }}</span></template>
            </el-table-column>
            <el-table-column label="状态" width="80" align="center">
              <template #default="{ row }">
                <el-tag size="small" :type="row.status === 'running' ? 'primary' : 'warning'">
                  {{ row.status === 'running' ? '执行中' : '排队中' }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="current_item" label="阶段" width="100" align="center">
              <template #default="{ row }">{{ row.current_item || '排队等待' }}</template>
            </el-table-column>
            <el-table-column label="进度" min-width="130" align="center">
              <template #default="{ row }">
                <el-progress :percentage="row.progress" :stroke-width="8" />
              </template>
            </el-table-column>
            <template #empty><el-empty description="当前没有执行中的任务" :image-size="60" /></template>
          </el-table>
        </el-card>
      </el-col>
      <el-col :span="12">
        <el-card shadow="never">
          <template #header><b>最近完成</b></template>
          <el-table :data="data.recent_done || []" size="small" stripe>
            <el-table-column label="任务" width="80" align="center">
              <template #default="{ row }">
                <el-link type="primary" @click="$router.push(`/tasks/${row.task_id}`)">{{ row.task_id }}</el-link>
              </template>
            </el-table-column>
            <el-table-column label="项目" min-width="140" show-overflow-tooltip>
              <template #default="{ row }">
                <el-link v-if="row.project_name" type="primary" @click="$router.push('/projects')">{{ row.project_name }}</el-link>
                <span v-else>-</span>
              </template>
            </el-table-column>
            <el-table-column label="模型" min-width="130" show-overflow-tooltip>
              <template #default="{ row }">{{ row.model_name }} <span class="muted">{{ row.quant }}</span></template>
            </el-table-column>
            <el-table-column prop="device_name" label="设备/来源" min-width="110" show-overflow-tooltip />
            <el-table-column label="耗时" width="90" align="center">
              <template #default="{ row }">{{ fmtDuration(row.duration_sec) }}</template>
            </el-table-column>
            <el-table-column label="完成时间" width="150" align="center">
              <template #default="{ row }">{{ fmtTime(row.finished_at) }}</template>
            </el-table-column>
            <template #empty><el-empty description="暂无完成任务" :image-size="60" /></template>
          </el-table>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
import api from '../api'
import BackBar from '../components/BackBar.vue'
import Chart from '../components/Chart.vue'
import { fmtTime } from '../utils'

const data = ref({})
const clock = ref('')
let timer = null

const load = async () => {
  const { data: d } = await api.get('/overview')
  data.value = d || {}
}
const fmtDuration = (sec) => {
  if (!sec || sec <= 0) return '-'
  if (sec < 60) return sec + 's'
  return Math.floor(sec / 60) + 'm' + (sec % 60) + 's'
}

const taskTotal = computed(() => Object.values(data.value.task_counts || {}).reduce((a, b) => a + b, 0))
const doneCount = computed(() => (data.value.task_counts || {}).done || 0)
const runningCount = computed(() => {
  const c = data.value.task_counts || {}
  return (c.running || 0) + (c.queued || 0)
})
const statCards = computed(() => [
  { label: '设备总数', value: data.value.device_total ?? '-', color: '#303133' },
  { label: '项目总数', value: data.value.project_total ?? '-', color: '#303133' },
  { label: '任务总数', value: taskTotal.value, color: '#303133' },
  { label: '进行中', value: runningCount.value, color: '#409EFF' },
  { label: '已完成', value: doneCount.value, color: '#67C23A' },
  { label: '质量异常/疑似', value: (data.value.quality_abnormal || 0) + (data.value.quality_suspect || 0), color: '#E6A23C' },
])

const trendChart = computed(() => {
  const t = [...(data.value.trend || [])].reverse()
  return {
    tooltip: { trigger: 'axis' },
    grid: { left: 40, right: 16, top: 20, bottom: 24 },
    xAxis: { type: 'category', data: t.map((x) => x.date.slice(5)), boundaryGap: false },
    yAxis: { type: 'value', minInterval: 1, splitLine: { lineStyle: { color: '#f0f2f5' } } },
    series: [{
      type: 'line', smooth: true, data: t.map((x) => x.count),
      symbolSize: 7, itemStyle: { color: '#409EFF' }, lineStyle: { width: 3 },
      areaStyle: { color: { type: 'linear', x: 0, y: 0, x2: 0, y2: 1, colorStops: [
        { offset: 0, color: 'rgba(64,158,255,0.25)' }, { offset: 1, color: 'rgba(64,158,255,0.02)' }] } },
    }],
  }
})
const statusChart = computed(() => {
  const c = data.value.task_counts || {}
  const items = [
    { name: '已完成', value: c.done || 0, itemStyle: { color: '#67C23A' } },
    { name: '执行中', value: c.running || 0, itemStyle: { color: '#409EFF' } },
    { name: '排队中', value: c.queued || 0, itemStyle: { color: '#B3C6E7' } },
    { name: '待执行', value: c.pending || 0, itemStyle: { color: '#C0C4CC' } },
    { name: '失败', value: c.failed || 0, itemStyle: { color: '#F56C6C' } },
    { name: '已取消', value: c.cancelled || 0, itemStyle: { color: '#909399' } },
    { name: '中断', value: c.interrupted || 0, itemStyle: { color: '#E6A23C' } },
  ].filter((x) => x.value > 0)
  return {
    tooltip: { trigger: 'item' },
    legend: { bottom: 0, textStyle: { fontSize: 11 }, itemWidth: 12, itemHeight: 8 },
    series: [{
      type: 'pie', radius: ['48%', '70%'], center: ['50%', '44%'],
      label: { show: false }, itemStyle: { borderColor: '#fff', borderWidth: 2 },
      data: items.length ? items : [{ name: '暂无任务', value: 1, itemStyle: { color: '#f0f2f5' } }],
    }],
  }
})
const rateChart = computed(() => {
  const total = taskTotal.value
  const rate = total ? Math.round((doneCount.value / total) * 100) : 0
  return {
    series: [{
      type: 'gauge', startAngle: 210, endAngle: -30, min: 0, max: 100,
      radius: '95%', center: ['50%', '58%'],
      progress: { show: true, width: 12, itemStyle: { color: '#67C23A' } },
      axisLine: { lineStyle: { width: 12, color: [[1, '#EBEEF5']] } },
      axisTick: { show: false }, splitLine: { show: false }, axisLabel: { show: false },
      pointer: { show: false }, anchor: { show: false },
      detail: { valueAnimation: true, fontSize: 30, color: '#303133', offsetCenter: [0, '-5%'], formatter: '{value}%' },
      data: [{ value: rate }],
    }],
  }
})

onMounted(() => {
  load()
  const tick = () => { clock.value = new Date().toLocaleString('zh-CN', { hour12: false }) }
  tick()
  timer = setInterval(() => { tick(); load() }, 3000)
})
onUnmounted(() => clearInterval(timer))
</script>

<style scoped>
.refresh-note { color: #909399; font-size: 12px; }
.stat-row { margin-bottom: 12px; }
.stat-card { text-align: center; }
.stat-card :deep(.el-card__body) { padding: 14px 8px; }
.stat-num { font-size: 26px; font-weight: 700; font-variant-numeric: tabular-nums; }
.stat-label { color: #909399; font-size: 12px; margin-top: 4px; }
.tbl-head { display: flex; justify-content: space-between; align-items: center; }
.muted { color: #909399; font-size: 12px; }
</style>
