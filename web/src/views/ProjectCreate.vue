<template>
  <div>
    <BackBar title="新建测试任务" />
    <el-card shadow="never" style="margin-bottom: 14px">
      <div class="type-row">
        <div :class="['type-card', form.type === 'model' && 'active']" @click="form.type = 'model'">
          <el-icon :size="24"><Cloudy /></el-icon>
          <div class="type-name">纯模型测试</div>
          <div class="type-desc">云端大模型 API,不提交硬件信息</div>
        </div>
        <div :class="['type-card', form.type === 'hardware' && 'active']" @click="form.type = 'hardware'">
          <el-icon :size="24"><Cpu /></el-icon>
          <div class="type-name">本地硬件</div>
          <div class="type-desc">算力卡/整机上的模型,附硬件信息与规格书</div>
        </div>
      </div>
      <el-form label-position="top" style="margin-top: 16px; max-width: 880px">
        <el-row :gutter="12">
          <el-col :span="12">
            <el-form-item label="项目名称" required>
              <el-input v-model="form.name" placeholder="必填:项目名原样显示,模型名在列表单独一列" />
            </el-form-item>
          </el-col>
          <el-col :span="6">
            <el-form-item label="测试人员"><el-input v-model="form.tester" placeholder="选填" /></el-form-item>
          </el-col>
        </el-row>
        <el-row :gutter="12">
          <el-col :span="12">
            <el-form-item label="备注(本次测什么)" required>
              <el-input v-model="form.note" placeholder="必填:如「k3-256k 全维度能力评测」「智凯100 INT4 吞吐压测」" />
            </el-form-item>
          </el-col>
        </el-row>
        <template v-if="form.type === 'hardware'">
          <HwForm v-model="hw">
            <template #after-name>
              <el-form-item label="从历史项目复制">
                <el-select v-model="copyFrom" placeholder="一键带出历史硬件信息" clearable style="width: 100%" @change="applyCopy">
                  <el-option v-for="p in hwProjects" :key="p.id" :label="`${p.id} ${p.name}`" :value="p.id" />
                </el-select>
              </el-form-item>
            </template>
          </HwForm>
          <el-form-item label="规格书 / 说明书(入库,可下载)">
            <el-upload :auto-upload="false" :on-change="onPickFile" :show-file-list="false" drag>
              <div style="padding: 6px 0; color: #909399; font-size: 13px">
                <el-icon><UploadFilled /></el-icon> 点击或拖拽上传(≤32MB,可多次选择)
              </div>
            </el-upload>
            <div v-for="(f, i) in specFiles" :key="i" class="spec-item">
              <el-icon><Document /></el-icon>
              <span style="flex: 1">{{ f.name }}</span>
              <el-button size="small" text type="danger" @click="specFiles.splice(i, 1)">移除</el-button>
            </div>
          </el-form-item>
        </template>
      </el-form>
    </el-card>

    <el-card shadow="never">
      <template #header><b>模型任务配置</b></template>
      <TaskForm v-model="task" :corpus-versions="corpusVersions" />
      <div style="display: flex; justify-content: flex-end; gap: 10px; margin-top: 14px">
        <el-button @click="$router.push('/projects')">取消</el-button>
        <el-button :loading="creating" @click="create(false)">创建任务</el-button>
        <el-button type="primary" :loading="creating" @click="create(true)">创建并启动</el-button>
      </div>
    </el-card>
  </div>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import api from '../api'
import BackBar from '../components/BackBar.vue'
import TaskForm from '../components/TaskForm.vue'
import HwForm from '../components/HwForm.vue'
import { blankHw, normalizeTaskParams } from '../utils'

const router = useRouter()
const corpusVersions = ref([])
const creating = ref(false)
const specFiles = ref([])
const hwProjects = ref([])
const copyFrom = ref(null)


const hw = ref(blankHw())

const defaultParams = () => ({
  gen: { temperature: 0.7, top_p: 0.9, top_k: 20, max_tokens: 1024 },
  exec: { warmup: 3, repeats: 3, ladder_tiers: ['1K', '10K', '20K', '30K'], concurrency_levels: [1, 4, 8, 16, 32],
    stability_minutes: 30, stability_tier: '10K', stability_concurrency: 1,
    ladder_repeats: 0, concurrency_repeats: 0, cache_repeats: 0,
    niah_tiers: [], niah_depths: [10, 30, 50, 70, 90], niah_repeats: 2 },
  advanced: { request_timeout_sec: 300, ignore_eos: false, thinking_mode: 'default', extra_params: '' },
  items: { ladder: false, concurrency: false, cache: false, stability: false, quality: true, niah: false,
    capability_cats: [], quality_mode: 'all', quality_group_id: null, quality_question_ids: [] },
})
const task = ref({
  api_base_url: '', model_name: '', model_hf_id: '', model_ms_id: '', quant: '', context_len: 262144, // 默认 256K(1024 进制)
  cache_enabled: 'unknown', cache_note: '', api_key: '', api_key_set: false, api_key_clear: false,
  corpus_version_id: null, params: defaultParams(),
})
const form = ref({ type: 'model', name: '', tester: '', note: '' })

const onPickFile = (uploadFile) => {
  const raw = uploadFile.raw
  if (raw.size > 32 << 20) { ElMessage.warning('文件超过 32MB 限制'); return }
  specFiles.value.push(raw)
}
const applyCopy = (pid) => {
  const p = hwProjects.value.find((x) => x.id === pid)
  if (!p || !p.hw_json) return
  try {
    hw.value = { ...blankHw(), ...JSON.parse(p.hw_json) }
    ElMessage.success('已带出历史硬件信息')
  } catch { /* 忽略 */ }
}

const create = async (startNow) => {
  if (!form.value.name.trim()) { ElMessage.warning('项目名称必填'); return }
  if (!form.value.note.trim()) { ElMessage.warning('备注必填:写清本次测试目的'); return }
  if (form.value.type === 'hardware' && !hw.value.name) { ElMessage.warning('本地硬件需填写硬件名称'); return }
  if (!task.value.api_base_url || !task.value.model_name) { ElMessage.warning('API 地址与模型名称必填'); return }
  creating.value = true
  try {
    const payload = {
      type: form.value.type, name: form.value.name.trim(), tester: form.value.tester, note: form.value.note.trim(),
      task: { ...task.value, corpus_version_id: task.value.corpus_version_id || undefined,
        params: normalizeTaskParams(task.value.params) },
    }
    if (form.value.type === 'hardware') payload.hw = hw.value
    const { data } = await api.post('/tasks/direct', payload)
    const pid = data.project_id
    for (const f of specFiles.value) {
      const fd = new FormData()
      fd.append('file', f)
      await api.post(`/projects/${pid}/files`, fd, { headers: { 'Content-Type': 'multipart/form-data' } })
    }
    if (startNow) {
      const detail = await api.get(`/projects/${pid}`)
      const tid = (detail.data.tasks || [])[0]?.id
      if (tid) await api.post(`/tasks/${tid}/start`)
    }
    ElMessage.success(startNow ? '任务已创建并启动' : '任务已创建')
    router.push(`/projects/${pid}`)
  } finally {
    creating.value = false
  }
}

onMounted(async () => {
  const [cv, st, pj] = await Promise.all([
    api.get('/corpus-versions'), api.get('/settings'), api.get('/projects'),
  ])
  corpusVersions.value = cv.data || []
  hwProjects.value = (pj.data || []).filter((p) => p.hw_json)
  try {
    const dp = JSON.parse(st.data.default_task_params)
    task.value.params = { ...defaultParams(), ...JSON.parse(JSON.stringify(dp)) }
  } catch { /* 保留内置默认 */ }
})
</script>

<style scoped>
.type-row { display: flex; gap: 14px; }
.type-card { flex: 1; max-width: 320px; border: 2px solid #e4e7ed; border-radius: 10px; padding: 14px 18px;
  cursor: pointer; transition: all .15s; }
.type-card:hover { border-color: #a0cfff; }
.type-card.active { border-color: #409EFF; background: #ecf5ff; }
.type-name { font-size: 15px; font-weight: 600; margin-top: 6px; }
.type-desc { color: #909399; font-size: 12px; margin-top: 2px; }
.spec-item { display: flex; align-items: center; gap: 8px; padding: 7px 10px; border: 1px solid #e4e7ed;
  border-radius: 6px; margin-top: 8px; background: #fafafa; width: 100%; }
</style>
