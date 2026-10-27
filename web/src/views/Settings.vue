<template>
  <div>
    <BackBar title="系统设置" />
    <el-alert v-if="forceChange" type="warning" :closable="false" title="首次登录,请先修改初始密码" style="margin-bottom: 14px" />
    <el-row :gutter="14">
      <el-col :span="12">
        <el-card shadow="never" style="margin-bottom: 14px">
          <template #header><b>修改登录密码</b></template>
          <el-form label-width="90px" style="max-width: 420px">
            <el-form-item label="原密码"><el-input v-model="pwd.old" type="password" show-password /></el-form-item>
            <el-form-item label="新密码"><el-input v-model="pwd.new1" type="password" show-password placeholder="至少 8 位" /></el-form-item>
            <el-form-item label="确认新密码"><el-input v-model="pwd.new2" type="password" show-password /></el-form-item>
            <el-button type="primary" :loading="pwdSaving" @click="changePwd">修改密码</el-button>
          </el-form>
        </el-card>
        <el-card shadow="never" style="margin-bottom: 14px">
          <template #header><b>调度</b></template>
          <el-form label-width="160px" style="max-width: 460px">
            <el-form-item label="全局并行任务数上限">
              <el-input-number v-model="parallelLimit" :min="1" :max="32" />
            </el-form-item>
            <el-form-item label="基线空闲采样时长(秒)">
              <el-input-number v-model="baselineWait" :min="0" :max="60" />
              <div class="hint">任务启动后、预热开始前的空闲采样窗口(§6.7)</div>
            </el-form-item>
            <el-button type="primary" @click="saveSettings">保存</el-button>
          </el-form>
        </el-card>
        <el-card shadow="never">
          <template #header><b>数据备份</b></template>
          <p style="color: #606266; font-size: 13px; margin-top: 0">
            一键下载 SQLite 一致性备份文件(含全部设备、项目、任务、请求级样本与监控数据)。
          </p>
          <el-button @click="backup"><el-icon><Download /></el-icon>下载备份</el-button>
        </el-card>
        <el-card shadow="never" style="margin-top: 14px">
          <template #header><b>API Token(智能体 / CLI 接入)</b></template>
          <div style="max-width: 520px; font-size: 13px">
            <p style="color: #606266; margin-top: 0">长期有效,请求头携带 <code>Authorization: Bearer &lt;token&gt;</code> 即可调用全部接口,供智能体和自动化使用。</p>
            <div style="display: flex; align-items: center; gap: 10px; flex-wrap: wrap">
              <el-tag v-if="apiTokenSet" type="success">已设置(创建于 {{ fmtTime(apiTokenCreated) }})</el-tag>
              <el-tag v-else type="info">未设置</el-tag>
              <el-button type="primary" size="small" @click="genToken">{{ apiTokenSet ? '重新生成' : '生成 Token' }}</el-button>
              <el-button size="small" @click="downloadSkill"><el-icon><Download /></el-icon>下载技能</el-button>
              <el-button v-if="apiTokenSet" type="danger" size="small" plain @click="revokeToken">吊销</el-button>
            </div>
            <el-alert v-if="newToken" type="warning" :closable="false" style="margin-top: 10px">
              <div style="word-break: break-all; font-family: monospace; font-size: 12px">{{ newToken }}</div>
              <div style="margin-top: 4px; font-size: 12px">Token 只显示这一次,请立即复制保存。</div>
            </el-alert>
          </div>
        </el-card>
        <el-card shadow="never" style="margin-top: 14px">
          <template #header><b>关于</b></template>
          <div style="font-size: 13px; color: #606266; line-height: 1.9">
            <div>测试平台版本:<b>{{ versions.platform || '-' }}</b></div>
            <div>pi 智能体核心(pi-agent-core):<b>{{ versions.pi || '未安装' }}</b></div>
            <div>pi-goal 目标驱动插件:<b>{{ versions.pigoal || '未安装' }}</b></div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="12">
        <el-card shadow="never" style="margin-bottom: 14px">
          <template #header><b>全局默认测试参数(仅影响新建任务)</b></template>
          <el-form label-width="130px" style="max-width: 480px">
            <el-form-item label="temperature"><el-input-number v-model="dp.gen.temperature" :min="0" :max="2" :step="0.1" /></el-form-item>
            <el-form-item label="top_p"><el-input-number v-model="dp.gen.top_p" :min="0" :max="1" :step="0.05" /></el-form-item>
            <el-form-item label="top_k"><el-input-number v-model="dp.gen.top_k" :min="0" /></el-form-item>
            <el-form-item label="max_tokens"><el-input-number v-model="dp.gen.max_tokens" :min="1" :step="128" /></el-form-item>
            <el-form-item label="预热次数"><el-input-number v-model="dp.exec.warmup" :min="0" /></el-form-item>
            <el-form-item label="每组重复"><el-input-number v-model="dp.exec.repeats" :min="1" /></el-form-item>
            <el-form-item label="稳定性时长(分)"><el-input-number v-model="dp.exec.stability_minutes" :min="1" /></el-form-item>
            <el-form-item label="单请求超时(s)"><el-input-number v-model="dp.advanced.request_timeout_sec" :min="10" /></el-form-item>
            <el-form-item label="质量题执行方式">
              <el-radio-group v-model="qualityEngine">
                <el-radio value="pi">智能体(pi,带工具调用,可真实生成文件)</el-radio>
                <el-radio value="direct">直接问答</el-radio>
              </el-radio-group>
            </el-form-item>
            <el-form-item label="pi 沙箱">
              <div style="display: flex; flex-direction: column; gap: 8px">
                <div style="display: flex; align-items: center; gap: 10px">
                  <el-tag v-if="piSandbox.image" type="success" effect="plain">已启用容器沙箱(hwtest-pi:local 已就绪)</el-tag>
                  <el-tag v-else-if="piSandbox.docker" type="primary" effect="plain">正在自动构建沙箱镜像…(约 1-3 分钟,完成后自动启用)</el-tag>
                  <el-tag v-else type="info" effect="plain">未检测到 Docker,当前为宿主机执行(不隔离)</el-tag>
                  <el-button size="small" :loading="sandboxLoading" @click="detectSandbox"><el-icon><Refresh /></el-icon>重新检测</el-button>
                </div>
                <el-radio-group v-model="piContainer" :disabled="!piSandbox.image">
                  <el-radio value="auto">自动(有容器镜像就用容器,否则宿主机)</el-radio>
                  <el-radio value="on">强制容器(未构建镜像则报错)</el-radio>
                  <el-radio value="off">宿主机执行(不隔离)</el-radio>
                </el-radio-group>
                <div v-if="!piSandbox.image && !piSandbox.docker" style="font-size: 12px; color: #909399; line-height: 1.7">
                  安装 Docker Engine 24+(Linux,官方源或国内镜像源均可)后无需任何手动操作,平台检测到 Docker 会自动构建沙箱镜像(hwtest-pi:local:Node.js 22 + Python 3.11 + reportlab/openpyxl/python-pptx/python-docx/matplotlib + 中文字体);暂无法安装时保持宿主机执行,注意任务间文件不隔离
                </div>
              </div>
            </el-form-item>
            <el-button type="primary" @click="saveSettings">保存</el-button>
          </el-form>
        </el-card>
        <el-card shadow="never">
          <template #header><b>裁判模型(能力评判用)</b></template>
          <el-form label-width="100px" style="max-width: 520px">
            <el-form-item label="API 地址">
              <el-input v-model="judgeForm.api_url" placeholder="如 https://token.lswz.top" />
            </el-form-item>
            <el-form-item label="API Key">
              <el-input v-model="judgeForm.api_key" type="password" show-password
                :placeholder="judgeKeySet ? '已设置,留空不修改;输入 - 清除' : '选填'" />
            </el-form-item>
            <el-form-item label="模型名称">
              <div style="display: flex; gap: 8px; width: 100%">
                <el-select v-model="judgeForm.model" filterable allow-create default-first-option style="flex: 1">
                  <el-option v-for="m in judgeModels" :key="m" :label="m" :value="m" />
                </el-select>
                <el-button :loading="judgeFetching" @click="fetchJudgeModels">查询模型</el-button>
              </div>
            </el-form-item>
            <el-button type="primary" @click="saveJudge">保存裁判配置</el-button>
            <span class="hint" style="margin-left: 10px">未配置时能力题显示「未评判」</span>
          </el-form>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<script setup>
import { onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Download, Refresh } from '@element-plus/icons-vue'
import api from '../api'
import BackBar from '../components/BackBar.vue'
import { fmtTime } from '../utils'

const router = useRouter()
const forceChange = ref(false)
const pwd = ref({ old: '', new1: '', new2: '' })
const pwdSaving = ref(false)
const parallelLimit = ref(4)
const baselineWait = ref(10)
const judgeForm = ref({ api_url: '', api_key: '', model: '' })
const judgeKeySet = ref(false)
const judgeModels = ref([])
const judgeFetching = ref(false)
const qualityEngine = ref('pi')
const piContainer = ref('auto')
const piSandbox = ref({ docker: false, image: false })
const sandboxLoading = ref(false)
// pi 沙箱实时探测(V2.23):刷新按钮随时重测
const detectSandbox = async () => {
  sandboxLoading.value = true
  try {
    const { data } = await api.get('/settings')
    piSandbox.value = data.pi_sandbox || { docker: false, image: false }
    if (piSandbox.value.image) ElMessage.success('容器沙箱已就绪')
    else ElMessage.warning(piSandbox.value.docker ? '未构建沙箱镜像' : '未检测到 Docker')
  } finally {
    sandboxLoading.value = false
  }
}
// 构建中自动轮询,完成后自动变绿(无需手动刷新)
let sandboxTimer = null
watch(() => piSandbox.value.docker && !piSandbox.value.image, (building) => {
  if (building && !sandboxTimer) {
    sandboxTimer = setInterval(async () => {
      const { data } = await api.get('/settings')
      piSandbox.value = data.pi_sandbox || { docker: false, image: false }
      if (piSandbox.value.image || !piSandbox.value.docker) {
        clearInterval(sandboxTimer)
        sandboxTimer = null
        if (piSandbox.value.image) ElMessage.success('沙箱镜像构建完成,容器沙箱已启用')
      }
    }, 10000)
  } else if (!building && sandboxTimer) {
    clearInterval(sandboxTimer)
    sandboxTimer = null
  }
})
const apiTokenSet = ref(false)
const apiTokenCreated = ref(0)
const newToken = ref('')
const versions = ref({ platform: '', pi: '', pigoal: '' })
const genToken = async () => {
  const { data } = await api.post('/settings/api-token')
  newToken.value = data.token || ''
  apiTokenSet.value = true
  ElMessage.success('Token 已生成,请立即保存')
}
const revokeToken = async () => {
  await api.delete('/settings/api-token')
  apiTokenSet.value = false
  newToken.value = ''
  ElMessage.success('Token 已吊销')
}
const dp = ref({
  gen: { temperature: 0.7, top_p: 0.9, top_k: 20, max_tokens: 1024 },
  exec: { warmup: 3, repeats: 3, ladder_tiers: ['1K', '10K', '20K', '30K'], concurrency_levels: [1, 4, 8, 16, 32],
    stability_minutes: 30, stability_tier: '10K', stability_concurrency: 1,
    niah_tiers: [], niah_depths: [10, 30, 50, 70, 90], niah_repeats: 2 },
  advanced: { request_timeout_sec: 300, ignore_eos: false },
  items: { ladder: true, concurrency: true, cache: true, stability: true, quality: true, niah: false },
})

const changePwd = async () => {
  if (pwd.value.new1.length < 8) { ElMessage.warning('新密码至少 8 位'); return }
  if (pwd.value.new1 !== pwd.value.new2) { ElMessage.warning('两次输入不一致'); return }
  pwdSaving.value = true
  try {
    await api.post('/password', { old_password: pwd.value.old, new_password: pwd.value.new1 })
    ElMessage.success('密码已修改,请重新登录')
    router.push('/login')
  } finally {
    pwdSaving.value = false
  }
}
const saveSettings = async () => {
  await api.put('/settings', {
    parallel_limit: String(parallelLimit.value),
    baseline_wait_sec: String(baselineWait.value),
    default_task_params: JSON.stringify(dp.value),
    quality_engine: qualityEngine.value,
    pi_container: piContainer.value,
  })
  ElMessage.success('设置已保存')
}
const backup = () => window.open('/api/backup', '_blank')
// 下载智能体技能包(SKILL.md + CLI)
const downloadSkill = () => window.open('/api/settings/skill.zip', '_blank')
const fetchJudgeModels = async () => {
  if (!judgeForm.value.api_url) { ElMessage.warning('先填 API 地址'); return }
  judgeFetching.value = true
  try {
    const { data } = await api.post('/tasks/fetch-models', {
      api_base_url: judgeForm.value.api_url, api_key: judgeForm.value.api_key || '',
    })
    judgeModels.value = data.models || []
    ElMessage.success(`查询到 ${judgeModels.value.length} 个模型`)
  } finally {
    judgeFetching.value = false
  }
}
const saveJudge = async () => {
  await api.put('/settings', {
    judge_api_url: judgeForm.value.api_url,
    judge_model: judgeForm.value.model,
    judge_api_key: judgeForm.value.api_key,
  })
  judgeKeySet.value = judgeForm.value.api_key !== '' ? true : judgeKeySet.value
  ElMessage.success('裁判配置已保存')
}

onMounted(async () => {
  const [me, st] = await Promise.all([api.get('/me'), api.get('/settings')])
  forceChange.value = !!me.data.force_password_change
  parallelLimit.value = parseInt(st.data.parallel_limit || '4', 10)
  baselineWait.value = parseInt(st.data.baseline_wait_sec || '10', 10)
  try { dp.value = JSON.parse(st.data.default_task_params) } catch { /* 保留默认 */ }
  judgeForm.value.api_url = st.data.judge_api_url || ''
  judgeForm.value.model = st.data.judge_model || ''
  judgeKeySet.value = !!st.data.judge_key_set
  qualityEngine.value = st.data.quality_engine || 'pi'
  piContainer.value = st.data.pi_container || 'auto'
  piSandbox.value = st.data.pi_sandbox || { docker: false, image: false }
  apiTokenSet.value = !!st.data.api_token_set
  apiTokenCreated.value = st.data.api_token_created || 0
  versions.value = { platform: st.data.platform_version, pi: st.data.pi_version, pigoal: st.data.pi_goal_version }
})
</script>

<style scoped>
.hint { font-size: 12px; color: #909399; }
</style>
