<template>
  <div>
    <BackBar title="项目">
      <span style="margin-right: 14px; font-size: 13px; color: #606266; display: inline-flex; align-items: center; gap: 6px">
        同时跑
        <el-input-number v-model="parallelLimit" :min="1" :max="32" size="small" style="width: 92px" @change="saveParallel" />
        条
      </span>
      <el-button @click="openHidden">
        <el-icon><Hide /></el-icon>隐藏任务
      </el-button>
      <el-button type="primary" @click="$router.push('/projects/create')">
        <el-icon><Plus /></el-icon>新建测试任务
      </el-button>
    </BackBar>
    <div class="filter-bar">
      <el-input v-model="filterName" placeholder="项目名称" clearable prefix-icon="Search" style="width: 170px" @change="load" />
      <el-select v-model="filterDevice" placeholder="全部设备" clearable style="width: 200px" @change="load">
        <el-option v-for="d in devices" :key="d.id" :label="d.name" :value="d.id" />
      </el-select>
      <el-input v-model="filterTester" placeholder="测试人员" clearable style="width: 130px" @change="load" />
      <el-select v-model="filterStatus" placeholder="完成状态" clearable style="width: 130px" @change="load">
        <el-option label="待启动" value="pending" />
        <el-option label="排队中" value="queued" />
        <el-option label="进行中" value="running" />
        <el-option label="已完成" value="done" />
        <el-option label="部分失败" value="partial_failed" />
      </el-select>
      <el-select v-model="filterProgress" placeholder="完成进度" clearable style="width: 130px" @change="load">
        <el-option label="未开始(0%)" value="notstarted" />
        <el-option label="进行中(部分完成)" value="running" />
        <el-option label="全部完成(100%)" value="alldone" />
      </el-select>
      <el-date-picker v-model="filterRange" type="daterange" unlink-panels range-separator="至"
        start-placeholder="创建开始" end-placeholder="创建结束" style="width: 240px" @change="load" />
      <el-button @click="load"><el-icon><Refresh /></el-icon>刷新</el-button>
    </div>
    <el-table :data="projects" v-loading="loading" border stripe>
      <el-table-column label="任务ID" width="80" align="center">
        <template #default="{ row }">
          <el-link v-if="row.task_id" type="primary" @click="goDetail(row)">{{ row.task_id }}</el-link>
          <span v-else>-</span>
        </template>
      </el-table-column>
      <el-table-column label="项目名称" min-width="240" align="left">
        <template #default="{ row }">
          <el-link type="primary" @click="goDetail(row)">{{ row.name }}</el-link>
        </template>
      </el-table-column>
      <el-table-column label="模型" min-width="130" align="left">
        <template #default="{ row }">{{ row.model_name || '-' }}</template>
      </el-table-column>
      <el-table-column label="设备/来源" min-width="120" align="left">
        <template #default="{ row }">
          {{ row.device_name || hwName(row) || '云端' }}
          <el-tag size="small" effect="plain" style="margin-left: 4px">{{ row.type === 'model' ? '纯模型' : '硬件' }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="测试项" min-width="130" align="center">
        <template #default="{ row }">
          <el-tag v-for="t in row.tags || []" :key="t" size="small" effect="plain" style="margin: 1px 2px">{{ t }}</el-tag>
          <span v-if="!(row.tags || []).length">-</span>
        </template>
      </el-table-column>
      <el-table-column prop="tester" label="测试人员" width="90" align="center">
        <template #default="{ row }">{{ row.tester || '-' }}</template>
      </el-table-column>
      <el-table-column label="状态" width="90" align="center">
        <template #default="{ row }">
          <el-tag :type="projectStatus[row.status]?.type || 'info'">{{ projectStatus[row.status]?.text || row.status }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="任务进度" width="130" align="center">
        <template #default="{ row }">
          <el-progress :percentage="row.status === 'done' ? 100 : (row.progress || 0)" :stroke-width="8" />
        </template>
      </el-table-column>
      <el-table-column prop="task_total" label="任务数" width="70" align="center" />
      <el-table-column label="创建时间" width="150" align="center">
        <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
      </el-table-column>
      <el-table-column label="完成时间" width="150" align="center">
        <template #default="{ row }">{{ row.status === 'done' ? fmtTime(row.finished_at) : '-' }}</template>
      </el-table-column>
      <el-table-column label="操作" width="280" fixed="right" align="center">
        <template #default="{ row }">
          <el-tooltip content="一键启动" placement="top">
            <el-button size="small" type="success" circle :disabled="row.task_total === 0 || row.status === 'done'" @click="start(row)"><el-icon><VideoPlay /></el-icon></el-button>
          </el-tooltip>
          <el-tooltip content="暂停任务(释放执行名额,可断点续跑)" placement="top">
            <el-button size="small" circle type="warning" :disabled="!row.task_id || row.status !== 'running'" @click="pauseRow(row)"><el-icon><VideoPause /></el-icon></el-button>
          </el-tooltip>
          <el-tooltip content="继续跑(断点续跑)" placement="top">
            <el-button size="small" circle type="success" plain :disabled="!row.task_id || row.status !== 'paused'" @click="resumeRow(row)"><el-icon><CaretRight /></el-icon></el-button>
          </el-tooltip>
          <el-tooltip content="导出 Word 正式报告" placement="top">
            <el-button size="small" circle type="warning" plain @click="exportWord(row)"><el-icon><Document /></el-icon></el-button>
          </el-tooltip>
          <el-tooltip content="复制为新任务" placement="top">
            <el-button size="small" circle plain type="primary" @click="copyRow(row)"><el-icon><CopyDocument /></el-icon></el-button>
          </el-tooltip>
          <el-tooltip content="隐藏(列表/对比分析不可见,数据保留,可从「隐藏任务」恢复)" placement="top">
            <el-button size="small" circle plain type="info" :disabled="['running','queued'].includes(row.status)" @click="hideRow(row)"><el-icon><Hide /></el-icon></el-button>
          </el-tooltip>
        </template>
      </el-table-column>
      <template #empty>
        <el-empty description="暂无项目,点击右上角「新建项目」开始一次评估" />
      </template>
    </el-table>

    <!-- V3.0.1:隐藏项目清单(恢复/删除) -->
    <el-dialog v-model="hiddenOpen" :title="`隐藏任务(共 ${hiddenList.length} 条)`" width="900px" top="6vh">
      <el-table :data="hiddenList" v-loading="hiddenLoading" border stripe size="small">
        <el-table-column prop="task_id" label="任务ID" width="70" align="center" />
        <el-table-column prop="name" label="项目名称" min-width="200" align="left" />
        <el-table-column label="模型" min-width="120" align="left">
          <template #default="{ row }">{{ row.model_name || '-' }}</template>
        </el-table-column>
        <el-table-column label="状态" width="90" align="center">
          <template #default="{ row }">
            <el-tag :type="projectStatus[row.status]?.type || 'info'" size="small">{{ projectStatus[row.status]?.text || row.status }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="创建时间" width="150" align="center">
          <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="150" align="center">
          <template #default="{ row }">
            <el-button size="small" type="success" plain @click="unhideRow(row)">恢复</el-button>
            <el-button size="small" type="danger" plain @click="deleteHiddenRow(row)">删除</el-button>
          </template>
        </el-table-column>
        <template #empty><el-empty description="没有隐藏的项目" /></template>
      </el-table>
    </el-dialog>

  </div>
</template>

<script setup>
import { onMounted, onUnmounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useRouter } from 'vue-router'
import api from '../api'
import BackBar from '../components/BackBar.vue'
import { fmtTime, projectStatus } from '../utils'

const router = useRouter()
const projects = ref([])
const devices = ref([])
const loading = ref(false)
const filterName = ref('')
const filterDevice = ref(null)
const filterTester = ref('')
const filterStatus = ref('')
const filterProgress = ref('')
const filterRange = ref(null)
// 并行任务数快捷调节(实时生效,调度器每次派发前都会读这个设置)
const parallelLimit = ref(5)
const saveParallel = async (v) => {
  await api.put('/settings', { parallel_limit: String(v) })
  ElMessage.success(`并行任务数已调整为 ${v}(立即生效)`)
}
let timer = null

// 项目即任务:点项目名直跳该任务的详情页
const goDetail = (row) => {
  if (row.task_id) router.push(`/tasks/${row.task_id}`)
  else ElMessage.warning('该项目还没有任务')
}
const hwName = (row) => {
  try { return JSON.parse(row.hw_json || '{}').name || '' } catch { return '' }
}
const load = async () => {
  loading.value = true
  try {
    const params = {}
    if (filterName.value) params.name = filterName.value
    if (filterDevice.value) params.device_id = filterDevice.value
    if (filterTester.value) params.tester = filterTester.value
    if (filterStatus.value) params.status = filterStatus.value
    if (filterProgress.value) params.progress = filterProgress.value
    if (filterRange.value && filterRange.value.length === 2) {
      params.from = +new Date(filterRange.value[0])
      params.to = +new Date(filterRange.value[1]) + 86399999
    }
    const { data } = await api.get('/projects', { params })
    projects.value = data || []
  } finally {
    loading.value = false
  }
}
const start = async (row) => {
  const { data } = await api.post(`/projects/${row.id}/start`)
  ElMessage.success(`已入队 ${data.enqueued} 个任务`)
  load()
}
const exportWord = (row) => { window.open(`/api/projects/${row.id}/export.docx`, '_blank') }
// 复制为新任务(新项目壳+待执行,含加密 Key),跳到新任务详情可改配置
const copyRow = async (row) => {
  if (!row.task_id) { ElMessage.warning('该项目还没有任务'); return }
  const { data } = await api.post(`/tasks/${row.task_id}/clone`)
  ElMessage.success('已复制为新任务(待执行)')
  if (data && data.id) router.push(`/tasks/${data.id}`)
  else load()
}
// V3.0.1:项目隐藏(不删除)——列表/对比分析/首页不可见,数据保留;「隐藏任务」里可恢复或真删
const hiddenOpen = ref(false)
const hiddenList = ref([])
const hiddenLoading = ref(false)
const loadHidden = async () => {
  hiddenLoading.value = true
  try {
    const { data } = await api.get('/projects', { params: { hidden: 1 } })
    hiddenList.value = data || []
  } finally {
    hiddenLoading.value = false
  }
}
const openHidden = () => { hiddenOpen.value = true; loadHidden() }
const hideRow = async (row) => {
  await ElMessageBox.confirm('隐藏后项目列表与对比分析不再显示该项目,数据完整保留,可随时从「隐藏任务」恢复。确定隐藏?', '隐藏项目', { type: 'warning' })
  await api.post(`/projects/${row.id}/hidden`, { hidden: true })
  ElMessage.success('已隐藏')
  load()
}
const unhideRow = async (row) => {
  await api.post(`/projects/${row.id}/hidden`, { hidden: false })
  ElMessage.success('已恢复')
  loadHidden()
  load()
}
const deleteHiddenRow = async (row) => {
  await ElMessageBox.confirm(`删除将级联删除项目「${row.name}」及其全部任务与测试数据,不可恢复。确定删除?`, '删除项目', { type: 'error', confirmButtonText: '确定删除', cancelButtonText: '取消' })
  await api.delete(`/projects/${row.id}`)
  ElMessage.success('已删除')
  loadHidden()
  load()
}
// V2.19:列表行内执行控制(暂停/继续跑)
const pauseRow = async (row) => {
  await ElMessageBox.confirm('暂停后任务中止当前请求并释放执行名额,点「继续跑」从断点续跑。确定暂停?', '暂停任务', { type: 'warning' })
  await api.post(`/tasks/${row.task_id}/pause`)
  ElMessage.success('已请求暂停')
  load()
}
const resumeRow = async (row) => {
  await api.post(`/tasks/${row.task_id}/resume`)
  ElMessage.success('继续跑已开始')
  load()
}

onMounted(async () => {
  load()
  const { data } = await api.get('/devices', { params: { all: 1 } })
  devices.value = data || []
  const st = await api.get('/settings')
  parallelLimit.value = parseInt(st.data.parallel_limit || '5', 10)
  timer = setInterval(load, 5000)
})
onUnmounted(() => clearInterval(timer))
</script>
