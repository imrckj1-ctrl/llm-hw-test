<template>
  <div>
    <BackBar title="对比分析" />
    <el-card shadow="never" style="margin-bottom: 14px">
      <div class="filter-bar" style="margin-bottom: 0">
        <el-radio-group v-model="domain">
          <el-radio-button value="perf">性能对比</el-radio-button>
          <el-radio-button value="capability">能力对比</el-radio-button>
        </el-radio-group>
        <el-radio-group v-model="view" @change="loadTasks">
          <el-radio-button value="model">按模型对比</el-radio-button>
          <el-radio-button value="device">按设备对比</el-radio-button>
        </el-radio-group>
        <template v-if="view === 'model'">
          <el-select v-model="filters.model" placeholder="模型名称(默认全部)" clearable style="width: 220px" @change="loadTasks">
            <el-option v-for="m in modelOptions" :key="m" :label="m" :value="m" />
          </el-select>
        </template>
        <template v-else>
          <el-select v-model="filters.device_id" placeholder="选择设备" style="width: 200px" @change="loadTasks">
            <el-option v-for="d in devices" :key="d.id" :label="d.name" :value="d.id" />
          </el-select>
        </template>
        <el-date-picker v-model="filters.range" type="datetimerange" unlink-panels range-separator="至"
          start-placeholder="开始时间" end-placeholder="结束时间" style="width: 300px" @change="loadTasks" />
        <el-input v-model="filters.tester" placeholder="测试人员" clearable style="width: 120px" @change="loadTasks" />
        <el-button type="primary" :disabled="!selected.length" @click="exportCompare">
          <el-icon><Download /></el-icon>导出对比 Excel({{ selected.length }})
        </el-button>
      </div>
    </el-card>
    <el-card shadow="never" style="margin-bottom: 14px">
      <template #header><b>已完成任务(勾选加入对比)</b></template>
      <!-- 确认后列表收起为一行摘要,空间留给对比结果;可再展开修改 -->
      <div v-if="confirmed" style="display: flex; align-items: center; gap: 10px; flex-wrap: wrap">
        <span>已选 <b>{{ selected.length }}</b> 项:{{ selectedIds.join('、') }}</span>
        <el-button size="small" @click="confirmed = false">修改选择</el-button>
      </div>
      <template v-else>
        <el-table ref="taskTable" :data="pagedTasks" border stripe size="small" row-key="task.id"
          @selection-change="onSelChange">
          <el-table-column type="selection" width="45" />
          <el-table-column label="任务ID" width="80" align="center">
            <template #default="{ row }">
              <el-link type="primary" @click="$router.push(`/tasks/${row.task.id}`)">{{ row.task.id }}</el-link>
            </template>
          </el-table-column>
          <el-table-column label="项目名称" min-width="150" show-overflow-tooltip>
            <template #default="{ row }">{{ row.project_name || '-' }}</template>
          </el-table-column>
          <el-table-column label="模型" min-width="130" show-overflow-tooltip>
            <template #default="{ row }">{{ row.task.model_name }} <span style="color:#909399">{{ row.task.quant }}</span></template>
          </el-table-column>
          <el-table-column label="设备/来源" min-width="110" show-overflow-tooltip>
            <template #default="{ row }">{{ row.task.device_name || '云端' }}</template>
          </el-table-column>
          <el-table-column label="测试项" min-width="150" align="center">
            <template #default="{ row }">
              <el-tag v-for="t in row.tags" :key="t" size="small" effect="plain" style="margin-right: 4px">{{ t }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="测试人员" width="90" align="center">
            <template #default="{ row }">{{ row.tester || '-' }}</template>
          </el-table-column>
        </el-table>
        <el-pagination v-model:current-page="page" v-model:page-size="pageSize" :total="tasks.length"
          :page-sizes="[10, 20, 50]" layout="total, sizes, prev, pager, next" style="margin-top: 10px; justify-content: flex-end" />
      </template>
    </el-card>
    <!-- 已选悬浮提示条:跨页勾选不丢,点确认收起列表 -->
    <div v-if="selected.length && !confirmed" class="sel-float">
      <span>已选 <b>{{ selected.length }}</b> 项:{{ selectedIds.join('、') }}</span>
      <el-button type="primary" size="small" @click="confirmed = true">确认对比</el-button>
    </div>
    <!-- 对比内容二级勾选(能力对比):默认全不勾,勾哪个出哪个(V2.9);性能对比直接出看板(V2.22) -->
    <el-card v-if="selected.length && domain === 'capability'" shadow="never" style="margin-bottom: 14px">
      <div style="display: flex; align-items: center; gap: 10px; flex-wrap: wrap">
        <b>对比内容</b>
        <el-checkbox-group v-model="capPanels">
          <el-checkbox value="score">能力得分表</el-checkbox>
          <el-checkbox value="radar">能力雷达图</el-checkbox>
          <el-checkbox value="detail">逐题明细对比</el-checkbox>
        </el-checkbox-group>
        <span class="muted">已选 {{ selected.length }} 个任务;勾选内容项后下方出图</span>
      </div>
    </el-card>
    <!-- 性能对比:V2.22 看板(项目增删/速览卡/六板块) -->
    <CompareBoard v-if="selected.length && domain === 'perf'" :tasks="selected" @remove="removeTask" />
    <template v-if="selected.length && domain === 'capability'">
      <el-card v-if="capPanels.includes('score')" shadow="never" style="margin-bottom: 14px">
        <template #header><b>能力得分对比(对=1 / 部分对=0.5 / 错=0,维度内取均值)</b></template>
        <el-table :data="capRows" border size="small">
          <el-table-column label="任务" min-width="170" fixed>
            <template #default="{ row }">
              <el-link type="primary" @click="$router.push(`/tasks/${row.task_id}`)">任务 {{ row.task_id }}</el-link>
              <div class="muted">{{ row.source }} · {{ row.quant || '-' }}</div>
            </template>
          </el-table-column>
          <el-table-column v-for="c in capCats" :key="c" :label="capCatNames[c] || c" width="90" align="center">
            <template #default="{ row }">
              <span v-if="row.scores[c] !== undefined" :style="{ color: scoreColor(row.scores[c]) }">
                {{ row.scores[c].toFixed(2) }}
              </span>
              <span v-else class="muted">-</span>
            </template>
          </el-table-column>
          <el-table-column label="总分" width="90" align="center" fixed="right">
            <template #default="{ row }">
              <b :style="{ color: scoreColor(row.overall) }">{{ row.overall.toFixed(2) }}</b>
            </template>
          </el-table-column>
        </el-table>
      </el-card>
      <el-card v-if="capPanels.includes('radar')" shadow="never">
        <template #header><b>能力雷达叠加</b></template>
        <Chart :option="radarChart" :height="420" />
      </el-card>
      <el-card v-if="capPanels.includes('detail')" shadow="never" style="margin-top: 14px">
        <template #header><b>逐题明细对比(回答全文与产物)</b></template>
        <el-table :data="capQuestions" border size="small">
          <el-table-column label="题目" width="170" fixed>
            <template #default="{ row }">
              <el-tag size="small" effect="plain" style="margin-right: 4px">{{ catNameOf(row.category) }}</el-tag>
              {{ row.title }}
            </template>
          </el-table-column>
          <el-table-column v-for="t in capDetail" :key="t.task_id" :label="`任务 ${t.task_id}`" min-width="260">
            <template #default="{ row }">
              <template v-if="answerOf(t, row.id)">
                <div style="display: flex; align-items: center; gap: 6px; flex-wrap: wrap">
                  <el-tag v-if="answerOf(t, row.id).judge_score" size="small"
                    :type="{ pass: 'success', partial: 'warning', fail: 'danger' }[answerOf(t, row.id).judge_score]">
                    {{ { pass: '对', partial: '部分对', fail: '错' }[answerOf(t, row.id).judge_score] }}
                  </el-tag>
                  <el-popover width="520" trigger="click">
                    <template #reference>
                      <el-link type="primary">查看回答</el-link>
                    </template>
                    <pre style="white-space: pre-wrap; font-size: 12.5px; max-height: 420px; overflow-y: auto">{{ answerOf(t, row.id).answer }}</pre>
                  </el-popover>
                  <el-link v-if="answerOf(t, row.id).artifact_path" type="success" target="_blank"
                    :href="`/api/artifacts/${answerOf(t, row.id).artifact_path}/index.html`">打开产物</el-link>
                </div>
                <div v-if="answerOf(t, row.id).judge_reason" class="muted" style="font-size: 11px; margin-top: 3px">
                  {{ answerOf(t, row.id).judge_reason }}
                </div>
              </template>
              <span v-else class="muted">未测</span>
            </template>
          </el-table-column>
        </el-table>
      </el-card>
    </template>
  </div>
</template>

<script setup>
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import api from '../api'
import BackBar from '../components/BackBar.vue'
import Chart from '../components/Chart.vue'
import CompareBoard from '../components/CompareBoard.vue'

const view = ref('model')
const domain = ref('perf')
const filters = ref({ model: '', device_id: null, range: null, tester: '' })
const modelOptions = ref([])
const devices = ref([])
const tasks = ref([])
const selected = ref([])
const capPanels = ref([])  // 能力对比内容:score/radar/detail
// 分页 + 跨页勾选 + 确认收起(V2.9)
const page = ref(1)
const pageSize = ref(10)
const confirmed = ref(false)
const pagedTasks = computed(() => tasks.value.slice((page.value - 1) * pageSize.value, page.value * pageSize.value))
const selectedIds = computed(() => selected.value.map((t) => t.task.id).sort((a, b) => a - b))
// 看板 chip 与表格勾选双向同步(V2.22):不用 reserve-selection,selected 为唯一事实源
// 翻页/展开会重建勾选态;写表时挂 syncing 标志,防止 selection-change 回写冲掉 chips
const taskTable = ref(null)
let syncing = false
const onSelChange = (rows) => { if (!syncing) selected.value = rows }
const applySelection = async () => {
  await nextTick()
  // el-table 挂载与数据就绪晚于 v-else 渲染一帧,等一拍再对齐勾选态
  await new Promise((r) => setTimeout(r, 80))
  const tbl = taskTable.value
  if (!tbl) return
  syncing = true
  tbl.clearSelection()
  const ids = new Set(selected.value.map((x) => x.task.id))
  for (const row of pagedTasks.value) {
    if (ids.has(row.task.id)) tbl.toggleRowSelection(row, true)
  }
  syncing = false
}
const removeTask = (t) => {
  selected.value = selected.value.filter((x) => x.task.id !== t.task.id)
  const row = pagedTasks.value.find((x) => x.task.id === t.task.id)
  if (row && taskTable.value) {
    syncing = true
    taskTable.value.toggleRowSelection(row, false)
    syncing = false
  }
}
// 翻页/改每页条数/展开「修改选择」→ 按 selected 重建当前页勾选态
watch([page, pageSize, confirmed], () => { if (!confirmed.value) applySelection() })
const capRows = ref([])
const capCats = ref([])
const capCatNames = ref({})
const capDetail = ref([])

const catNameOf = (c) => ({ common: '常识', reasoning: '推理', instruction: '指令', code: '代码',
  creative: '创作', summary: '摘要', needle: '找回', consistency: '一致性' }[c] || c || '-')

// 逐题明细:所有勾选任务的题目并集作为行
const capQuestions = computed(() => {
  const map = new Map()
  for (const t of capDetail.value) {
    for (const a of t.answers || []) {
      if (!map.has(a.question_id)) {
        map.set(a.question_id, { id: a.question_id, title: a.question, category: a.category })
      }
    }
  }
  return [...map.values()]
})
const answerOf = (t, qid) => (t.answers || []).find((a) => a.question_id === qid)

const loadCapDetail = async () => {
  if (!selected.value.length) { capDetail.value = []; return }
  const ids = selected.value.map((t) => t.task.id).join(',')
  const { data } = await api.get('/compare/capability-detail', { params: { ids } })
  capDetail.value = data || []
}

const scoreColor = (v) => (v >= 0.8 ? '#67C23A' : v >= 0.5 ? '#E6A23C' : '#F56C6C')

const loadCapability = async () => {
  if (!selected.value.length) { capRows.value = []; return }
  const ids = selected.value.map((t) => t.task.id).join(',')
  const { data } = await api.get('/compare/capability', { params: { ids } })
  capRows.value = data.rows || []
  capCats.value = data.cats || []
  capCatNames.value = data.cat_names || {}
}

const radarChart = computed(() => {
  const cats = capCats.value
  return {
    tooltip: {},
    legend: { top: 6, type: 'scroll' },
    radar: {
      indicator: cats.map((c) => ({ name: capCatNames.value[c] || c, max: 1 })),
      radius: '68%',
      splitArea: { areaStyle: { color: ['#fafbfc', '#f0f2f5'] } },
    },
    series: [{
      type: 'radar',
      data: capRows.value.map((row, i) => ({
        name: '任务' + row.task_id,
        value: cats.map((c) => row.scores[c] ?? 0),
        lineStyle: { color: colors[i % colors.length], width: 2 },
        itemStyle: { color: colors[i % colors.length] },
        areaStyle: { opacity: 0.08 },
      })),
    }],
  }
})

const loadTasks = async () => {
  const params = {}
  if (view.value === 'model') {
    if (filters.value.model) params.model_name = filters.value.model
  } else if (filters.value.device_id) {
    params.device_ids = String(filters.value.device_id)
  }
  if (filters.value.tester) params.tester = filters.value.tester
  if (filters.value.range && filters.value.range.length === 2) {
    params.from = String(+filters.value.range[0])
    params.to = String(+filters.value.range[1])
  }
  const { data } = await api.get('/compare/tasks', { params })
  tasks.value = data || []
  selected.value = []
  page.value = 1
  confirmed.value = false
}

const colors = ['#409EFF', '#E6A23C', '#67C23A', '#F56C6C', '#909399', '#9B59B6', '#16BAAA', '#D2691E']
const exportCompare = () => {
  const ids = selected.value.map((t) => t.task.id).join(',')
  window.open(`/api/compare/export.xlsx?ids=${ids}`, '_blank')
}

// 能力对比数据懒加载:勾了对应内容才拉接口(V2.9);性能看板由组件自取数(V2.22)
watch([selected, capPanels], () => {
  if (capPanels.value.some((p) => ['score', 'radar'].includes(p))) loadCapability()
  if (capPanels.value.includes('detail')) loadCapDetail()
}, { deep: true })

onMounted(async () => {
  const [m, d] = await Promise.all([api.get('/compare/models'), api.get('/devices', { params: { all: 1 } })])
  const names = [...new Set((m.data || []).map((x) => x.model_name))]
  modelOptions.value = names
  devices.value = d.data || []
  loadTasks()
})
</script>

<style scoped>
.muted { color: #909399; font-size: 12px; }
/* 已选悬浮条(跨页勾选提示) */
.sel-float {
  position: fixed;
  bottom: 18px;
  left: 50%;
  transform: translateX(-50%);
  z-index: 100;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 8px 16px;
  background: #303133;
  color: #fff;
  border-radius: 8px;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.25);
  font-size: 13px;
}
</style>
