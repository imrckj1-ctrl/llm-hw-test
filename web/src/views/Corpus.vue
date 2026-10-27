<template>
  <div>
    <BackBar title="语料与题库" />
    <el-tabs v-model="tab">
      <el-tab-pane label="语料版本" name="corpus">
        <el-alert type="info" :closable="false" style="margin-bottom: 12px">
          <p style="margin: 0">长度校准逻辑:平台<b>禁止按字符数估算 token</b>;每个任务×档位先发送校准请求(max_tokens=5),
          按 usage.prompt_tokens 迭代截断/循环拼接语料,直至与目标偏差 ≤1% 后冻结复用(最多 3 次迭代);
          30K 档正式样本偏差 ≤1% 为验收口径(§6.2)。任意档位替换即生成新版本;建任务时选择要用的版本。</p>
        </el-alert>
        <div class="filter-bar">
          <el-button type="primary" @click="openUpload()"><el-icon><Upload /></el-icon>上传新版本</el-button>
        </div>
        <el-table :data="versions" border stripe v-loading="loading">
          <el-table-column prop="version_no" label="版本号" width="120" />
          <el-table-column label="各档字数" min-width="240">
            <template #default="{ row }">
              <el-tag v-for="t in row.texts" :key="t.tier" size="small" effect="plain" style="margin-right: 6px">
                {{ t.tier }}: {{ t.char_count }}字
              </el-tag>
              <span v-if="!row.texts || !row.texts.length" style="color: #f56c6c">无文本</span>
            </template>
          </el-table-column>
          <el-table-column prop="uploader" label="上传人" width="100" />
          <el-table-column label="上传时间" width="155">
            <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
          </el-table-column>
          <el-table-column prop="note" label="备注(来源说明)" min-width="220" show-overflow-tooltip />
          <el-table-column label="操作" width="300" fixed="right">
            <template #default="{ row }">
              <el-button size="small" @click="viewTexts(row)">查看/替换档位</el-button>
              <el-button size="small" plain @click="openEditVersion(row)">修改备注</el-button>
              <el-popconfirm title="删除该语料版本?被任务引用的版本不可删" width="240" @confirm="removeVersion(row)">
                <template #reference><el-button size="small" type="danger" text>删除</el-button></template>
              </el-popconfirm>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <el-tab-pane label="质量抽查题库" name="questions">
        <div class="filter-bar">
          <el-button type="primary" @click="openQuestion(null)"><el-icon><Plus /></el-icon>新增题目</el-button>
        </div>
        <el-table :data="questions" border stripe>
          <el-table-column prop="id" label="ID" width="60" />
          <el-table-column label="类型" width="110">
            <template #default="{ row }">
              <el-tag size="small" :type="catTagType(row.category || row.qtype)">
                {{ catName(row.category || row.qtype) }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="title" label="标题" min-width="140" />
          <el-table-column prop="prompt" label="题干" min-width="240" show-overflow-tooltip />
          <el-table-column label="附长文本" width="90">
            <template #default="{ row }">{{ row.long_text ? row.long_text.length + ' 字' : '-' }}</template>
          </el-table-column>
          <el-table-column label="启用" width="80">
            <template #default="{ row }">
              <el-switch :model-value="row.enabled" @change="(val) => toggleQuestion(row, val)" />
            </template>
          </el-table-column>
          <el-table-column label="操作" width="150">
            <template #default="{ row }">
              <el-button size="small" @click="openQuestion(row)">编辑</el-button>
              <el-popconfirm title="删除该题目?" @confirm="removeQuestion(row)">
                <template #reference><el-button size="small" type="danger" text>删除</el-button></template>
              </el-popconfirm>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <el-tab-pane label="题组" name="groups">
        <el-alert type="info" :closable="false" style="margin-bottom: 12px">
          题组是全局共享的题目编组(如「代码能力组」「推理组」)。建任务选「按题组」时整组执行;组内停用题目不参与。
        </el-alert>
        <div class="filter-bar">
          <el-button type="primary" @click="openGroup(null)"><el-icon><Plus /></el-icon>新建题组</el-button>
        </div>
        <el-table :data="groups" border stripe>
          <el-table-column prop="id" label="ID" width="60" />
          <el-table-column prop="name" label="题组名" min-width="140" />
          <el-table-column label="题目" min-width="260">
            <template #default="{ row }">
              <el-tag v-for="qid in (row.question_ids || []).slice(0, 12)" :key="qid" size="small" effect="plain" style="margin: 1px 2px">
                {{ qid }} {{ questionTitle(qid) }}
              </el-tag>
              <span v-if="(row.question_ids || []).length > 12" style="font-size: 12px; color: #909399">
                等 {{ row.question_ids.length }} 题
              </span>
            </template>
          </el-table-column>
          <el-table-column prop="note" label="备注" min-width="140" show-overflow-tooltip />
          <el-table-column label="创建时间" width="155">
            <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
          </el-table-column>
          <el-table-column label="操作" width="150">
            <template #default="{ row }">
              <el-button size="small" @click="openGroup(row)">编辑</el-button>
              <el-popconfirm title="删除该题组?(不删题目本身)" width="220" @confirm="removeGroup(row)">
                <template #reference><el-button size="small" type="danger" text>删除</el-button></template>
              </el-popconfirm>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>
    </el-tabs>

    <el-dialog v-model="uploadDialog" title="上传语料新版本" width="720px">
      <el-alert type="warning" :closable="false" style="margin-bottom: 12px">
        每档一段连续中文自然文本(禁止重复字符/无意义填充);未提交的档位自动从最新版本复制。
        参考备料:1K≈1500 字、10K≈15000 字、20K≈30000 字、30K≈45000 字(平台以 token 校准为准)。
      </el-alert>
      <el-form label-width="90px">
        <el-form-item label="上传人"><el-input v-model="uploadForm.uploader" style="width: 220px" /></el-form-item>
        <el-form-item label="备注" required>
          <el-input v-model="uploadForm.note" type="textarea" :rows="3"
            placeholder="必填:约 200 字来源说明(来源、生成方式、版本号)" />
        </el-form-item>
        <el-form-item v-for="tier in defaultTiers" :key="tier" :label="tier + ' 档'">
          <div style="width: 100%">
            <div style="display: flex; gap: 8px; margin-bottom: 6px">
              <el-input v-model="uploadForm.texts[tier]" type="textarea" :rows="tier === '1K' ? 3 : 2"
                :placeholder="`粘贴 ${tier} 档文本(留空则从最新版复制)`" style="flex: 1" />
              <el-upload :auto-upload="false" :show-file-list="false" accept=".txt"
                :on-change="(f) => readFile(f, tier)">
                <el-button style="height: 100%">上传 .txt</el-button>
              </el-upload>
            </div>
            <span style="font-size: 12px; color: #909399">{{ charCount(uploadForm.texts[tier]) }}</span>
          </div>
        </el-form-item>
        <el-form-item label="自定义档位(不限个数)">
          <div style="width: 100%">
            <div v-for="(row, i) in customRows" :key="i" class="custom-tier-row">
              <div style="display: flex; gap: 8px; align-items: center; margin-bottom: 6px">
                <el-input v-model="row.name" placeholder="档位名,如 256K / 2.5K" style="width: 180px" />
                <el-upload :auto-upload="false" :show-file-list="false" accept=".txt"
                  :on-change="(f) => readCustomFile(f, row)">
                  <el-button size="small">上传 .txt</el-button>
                </el-upload>
                <span style="flex: 1; font-size: 12px; color: #909399">{{ charCount(row.text) }}</span>
                <el-button size="small" text type="danger" @click="customRows.splice(i, 1)">删除</el-button>
              </div>
              <el-input v-model="row.text" type="textarea" :rows="2" placeholder="粘贴该档语料文本" />
            </div>
            <el-button size="small" plain type="primary" @click="customRows.push({ name: '', text: '' })">
              <el-icon><Plus /></el-icon>再添加一档
            </el-button>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="uploadDialog = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="submitUpload">提交生成新版本</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="textsDialog" :title="`版本 ${currentVersion?.version_no} 档位文本`" width="760px">
      <div style="display: flex; gap: 8px; margin-bottom: 12px; align-items: center">
        <el-input v-model="newTierName" placeholder="新增档位,如 50K" style="width: 160px" />
        <el-upload :auto-upload="false" :show-file-list="false" accept=".txt" :on-change="readNewTierFile">
          <el-button size="small">上传 .txt</el-button>
        </el-upload>
        <el-button size="small" type="primary" :disabled="!newTierText" @click="addTier">添加该档(生成新版本)</el-button>
      </div>
      <el-input v-model="newTierText" type="textarea" :rows="3" placeholder="粘贴新档位语料文本(或点上方上传 .txt)" style="margin-bottom: 12px" />
      <el-tabs v-if="versionTexts.length">
        <el-tab-pane v-for="t in versionTexts" :key="t.tier" :label="`${t.tier}(${t.char_count}字)`">
          <el-input :model-value="t.content" type="textarea" :rows="12" readonly />
          <div style="margin-top: 10px; display: flex; gap: 8px">
            <el-input v-model="replaceText" type="textarea" :rows="3" placeholder="粘贴新文本替换该档(将生成新版本)" style="flex: 1" />
            <div style="display: flex; flex-direction: column; gap: 8px">
              <el-upload :auto-upload="false" :show-file-list="false" accept=".txt" :on-change="(f) => readReplace(f)">
                <el-button>上传 .txt</el-button>
              </el-upload>
              <el-button type="primary" :disabled="!replaceText" @click="replaceTier(t.tier)">替换该档</el-button>
            </div>
          </div>
        </el-tab-pane>
      </el-tabs>
      <el-empty v-else description="该版本无文本" />
    </el-dialog>

    <el-dialog v-model="questionDialog" :title="questionForm.id ? '编辑题目' : '新增题目'" width="640px">
      <el-form label-width="90px">
        <el-form-item label="分类">
          <el-select v-model="questionForm.category" style="width: 100%">
            <el-option v-for="(n, c) in catMap" :key="c" :label="n" :value="c" />
          </el-select>
        </el-form-item>
        <el-row :gutter="10">
          <el-col :span="12">
            <el-form-item label="评分方式">
              <el-select v-model="questionForm.judge_mode" style="width: 100%">
                <el-option label="LLM 评判(裁判模型)" value="llm" />
                <el-option label="规则校验" value="rule" />
                <el-option label="仅人工" value="manual" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="产物类型">
              <el-select v-model="questionForm.artifact_type" style="width: 100%">
                <el-option label="无产物" value="none" />
                <el-option label="网页(可打开)" value="html" />
                <el-option label="代码文件" value="code" />
                <el-option label="Excel 表格" value="xlsx" />
                <el-option label="PPT 演示" value="pptx" />
                <el-option label="Word 文档" value="docx" />
                <el-option label="PDF 文档" value="pdf" />
              </el-select>
            </el-form-item>
          </el-col>
        </el-row>
        <el-form-item label="目标驱动(goal 契约)">
          <el-switch v-model="questionForm.use_goal" />
          <span class="hint" style="margin-left: 10px">开启后模型必须干到主动「盖章确认」才算完成(产物题建议开,执行时限放宽到 1 小时);普通问答题关闭,答完即收(10 分钟)</span>
        </el-form-item>
        <el-form-item label="标题"><el-input v-model="questionForm.title" /></el-form-item>
        <el-form-item label="题干" required><el-input v-model="questionForm.prompt" type="textarea" :rows="3" /></el-form-item>
        <el-form-item label="参考答案">
          <el-input v-model="questionForm.expected_answer" type="textarea" :rows="2"
            :placeholder="expectedPlaceholder" />
        </el-form-item>
        <el-form-item v-if="questionForm.category === 'summary'" label="长文本">
          <el-input v-model="questionForm.long_text" type="textarea" :rows="6" placeholder="长文摘要题附带的长文本" />
        </el-form-item>
        <el-form-item label="排序"><el-input-number v-model="questionForm.sort" :min="0" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="questionDialog = false">取消</el-button>
        <el-button type="primary" @click="saveQuestion">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="editVersionDialog" :title="`修改 ${editVersionForm.version_no} 备注`" width="520px">
      <el-form label-width="90px">
        <el-form-item label="上传人"><el-input v-model="editVersionForm.uploader" /></el-form-item>
        <el-form-item label="备注" required>
          <el-input v-model="editVersionForm.note" type="textarea" :rows="4"
            placeholder="来源、生成方式、版本号(约 200 字)" />
        </el-form-item>
        <el-alert type="info" :closable="false" title="档位文本的改动请走「查看/替换档位」——替换即生成新版本(F-3.2),历史版本不可变" />
      </el-form>
      <template #footer>
        <el-button @click="editVersionDialog = false">取消</el-button>
        <el-button type="primary" @click="saveVersionEdit">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="groupDialog" :title="groupForm.id ? '编辑题组' : '新建题组'" width="720px">
      <el-form label-width="90px">
        <el-form-item label="题组名" required><el-input v-model="groupForm.name" placeholder="如「代码能力组」" /></el-form-item>
        <el-form-item label="备注"><el-input v-model="groupForm.note" placeholder="选填" /></el-form-item>
        <el-form-item label="组内题目" required>
          <div style="width: 100%">
            <div style="display: flex; gap: 8px; margin-bottom: 8px; align-items: center">
              <el-button size="small" @click="groupPickAll">全选</el-button>
              <el-button size="small" @click="groupForm.question_ids = []">清空</el-button>
              <span style="font-size: 12px; color: #909399">已选 {{ groupForm.question_ids.length }} / {{ questions.length }} 题</span>
            </div>
            <div style="max-height: 40vh; overflow-y: auto; border: 1px solid #e4e7ed; border-radius: 6px; padding: 8px 12px">
              <div v-for="grp in groupPickerGroups" :key="grp.cat" style="margin-bottom: 8px">
                <div style="font-weight: 600; font-size: 13px; margin-bottom: 2px">{{ grp.name }}({{ grp.list.length }})</div>
                <el-checkbox v-for="q in grp.list" :key="q.id" :model-value="groupPicked.has(q.id)"
                  @change="(val) => toggleGroupPick(q.id, val)" style="display: flex; margin-right: 0">
                  {{ q.id }} {{ q.title || q.prompt.slice(0, 30) }}{{ q.enabled ? '' : '(停用)' }}
                </el-checkbox>
              </div>
              <el-empty v-if="!questions.length" description="先在「质量抽查题库」新增题目" />
            </div>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="groupDialog = false">取消</el-button>
        <el-button type="primary" @click="saveGroup">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import api from '../api'
import BackBar from '../components/BackBar.vue'
import { fmtTime } from '../utils'

const tab = ref('corpus')
const versions = ref([])
const questions = ref([])
const groups = ref([])
const loading = ref(false)
const uploadDialog = ref(false)
const textsDialog = ref(false)
const questionDialog = ref(false)
const saving = ref(false)
const editVersionDialog = ref(false)
const editVersionForm = ref({ id: 0, version_no: '', uploader: '', note: '' })
const currentVersion = ref(null)
const versionTexts = ref([])
const replaceText = ref('')
const uploadForm = ref({ uploader: '', note: '', texts: { '1K': '', '10K': '', '20K': '', '30K': '' } })
const defaultTiers = ['1K', '10K', '20K', '30K']
// 自定义档位:一体行(档位名+粘贴+上传),不限个数(V2.5)
const customRows = ref([])
const newTierName = ref('')
const newTierText = ref('')
const groupDialog = ref(false)
const groupForm = ref({ id: 0, name: '', note: '', question_ids: [] })
const questionForm = ref({ id: 0, qtype: 'common', title: '', prompt: '', long_text: '', enabled: true, sort: 0 })

const charCount = (s) => (s ? `已粘贴 ${s.length} 字符` : '')

const load = async () => {
  loading.value = true
  try {
    const [{ data: v }, { data: q }, { data: g }] = await Promise.all([
      api.get('/corpus-versions'), api.get('/questions'), api.get('/question-groups'),
    ])
    versions.value = v || []
    questions.value = q || []
    groups.value = g || []
  } finally {
    loading.value = false
  }
}
const openUpload = () => {
  uploadForm.value = { uploader: '', note: '', texts: { '1K': '', '10K': '', '20K': '', '30K': '' } }
  customRows.value = [] 
  uploadDialog.value = true
}
const readFile = (file, tier) => {
  const reader = new FileReader()
  reader.onload = () => { uploadForm.value.texts[tier] = reader.result }
  reader.readAsText(file.raw, 'utf-8')
}
const readCustomFile = (file, row) => { 
  const reader = new FileReader() 
  reader.onload = () => { row.text = reader.result } 
  reader.readAsText(file.raw, 'utf-8') 
} 
const readReplace = (file) => { 
  const reader = new FileReader() 
  reader.onload = () => { replaceText.value = reader.result } 
  reader.readAsText(file.raw, 'utf-8') 
} 
const submitUpload = async () => { 
  if (!uploadForm.value.note) { ElMessage.warning('备注必填(来源说明)'); return } 
  // 自定义档位行校验(格式/与默认档重复/漏贴语料) 
  const texts = {} 
  for (const [k, val] of Object.entries(uploadForm.value.texts)) if (val) texts[k] = val 
  for (const row of customRows.value) { 
    const t = row.name.trim().toUpperCase() 
    if (!t && !row.text.trim()) continue // 全空行跳过 
    if (!/^\d+(\.\d+)?K$/.test(t)) { ElMessage.warning(`自定义档位「${row.name}」格式须为 数字+K,如 256K / 2.5K`); return } 
    if (defaultTiers.includes(t)) { ElMessage.warning(`档位 ${t} 是上方默认档,请直接用对应文本框`); return } 
    if (texts[t]) { ElMessage.warning(`档位 ${t} 重复添加`); return } 
    if (!row.text.trim()) { ElMessage.warning(`档位 ${t} 还没有粘贴语料`); return } 
    texts[t] = row.text 
  } 
  saving.value = true 
  try { 
    const { data } = await api.post('/corpus-versions', { 
      uploader: uploadForm.value.uploader, note: uploadForm.value.note, 
      texts, 
    }) 
    ElMessage.success(`新版本 ${data.version_no} 已生成`)
    uploadDialog.value = false
    load()
  } finally {
    saving.value = false
  }
}
const openEditVersion = (row) => {
  editVersionForm.value = { id: row.id, version_no: row.version_no, uploader: row.uploader, note: row.note }
  editVersionDialog.value = true
}
const saveVersionEdit = async () => {
  if (!editVersionForm.value.note) { ElMessage.warning('备注不能为空'); return }
  await api.put(`/corpus-versions/${editVersionForm.value.id}`, {
    uploader: editVersionForm.value.uploader, note: editVersionForm.value.note,
  })
  ElMessage.success('备注已更新')
  editVersionDialog.value = false
  load()
}
const removeVersion = async (row) => {
  await api.delete(`/corpus-versions/${row.id}`)
  ElMessage.success(`版本 ${row.version_no} 已删除`)
  load()
}
const viewTexts = async (row) => {
  currentVersion.value = row
  replaceText.value = ''
  newTierName.value = ''
  newTierText.value = ''
  const { data } = await api.get(`/corpus-versions/${row.id}/texts`, { params: { content: 1 } })
  versionTexts.value = data || []
  textsDialog.value = true
}
const replaceTier = async (tier) => {
  const { data } = await api.put(`/corpus-versions/${currentVersion.value.id}/texts/${tier}`, {
    content: replaceText.value, uploader: '', note: '',
  })
  ElMessage.success(`已生成新版本 ${data.version_no}(替换 ${tier} 档)`)
  textsDialog.value = false
  load()
}

// 新增档位(V2.4):走同一替换接口——复制现有档位 + 追加新档,生成新版本
const readNewTierFile = (file) => {
  const reader = new FileReader()
  reader.onload = () => { newTierText.value = reader.result }
  reader.readAsText(file.raw, 'utf-8')
}
const addTier = async () => {
  const t = newTierName.value.trim().toUpperCase()
  if (!/^\d+(\.\d+)?K$/.test(t)) { ElMessage.warning('档位格式须为 数字+K,如 50K / 2.5K'); return }
  if ((versionTexts.value || []).some((x) => x.tier === t)) { ElMessage.warning('该档位已存在,请用「替换该档」'); return }
  if (!newTierText.value.trim()) { ElMessage.warning('请粘贴或上传新档位语料'); return }
  const { data } = await api.put(`/corpus-versions/${currentVersion.value.id}/texts/${t}`, {
    content: newTierText.value, uploader: '', note: '',
  })
  ElMessage.success(`已生成新版本 ${data.version_no}(新增 ${t} 档)`)
  textsDialog.value = false
  load()
}

// ---- 题组(V2.4) ----
const questionTitle = (qid) => {
  const q = questions.value.find((x) => x.id === qid)
  return q ? (q.title || q.prompt.slice(0, 16)) : '?'
}
const groupPicked = computed(() => new Set(groupForm.value.question_ids || []))
const toggleGroupPick = (id, val) => {
  const s = new Set(groupForm.value.question_ids || [])
  if (val) s.add(id); else s.delete(id)
  groupForm.value.question_ids = [...s].sort((a, b) => a - b)
}
const groupPickAll = () => { groupForm.value.question_ids = questions.value.map((q) => q.id) }
const groupPickerGroups = computed(() => {
  const byCat = {}
  for (const q of questions.value) {
    const c = q.category || q.qtype || 'common'
    ;(byCat[c] = byCat[c] || []).push(q)
  }
  return Object.entries(byCat).map(([cat, list]) => ({ cat, name: catMap[cat] || cat, list }))
})
const openGroup = (row) => {
  groupForm.value = row
    ? { id: row.id, name: row.name, note: row.note, question_ids: [...(row.question_ids || [])] }
    : { id: 0, name: '', note: '', question_ids: [] }
  groupDialog.value = true
}
const saveGroup = async () => {
  const f = groupForm.value
  if (!f.name.trim()) { ElMessage.warning('题组名必填'); return }
  if (!f.question_ids.length) { ElMessage.warning('至少选择 1 道题'); return }
  if (f.id) await api.put(`/question-groups/${f.id}`, f)
  else await api.post('/question-groups', f)
  ElMessage.success('题组已保存')
  groupDialog.value = false
  load()
}
const removeGroup = async (row) => {
  await api.delete(`/question-groups/${row.id}`)
  ElMessage.success('题组已删除')
  load()
}
const catMap = {
  common: '常识问答', reasoning: '简单推理', instruction: '指令遵循', code: '代码生成',
  creative: '创作写作', summary: '长文摘要', needle: '长文找回', consistency: '答案一致性', doc: '文档生成',
}
const catName = (c) => catMap[c] || c || '-'
const catTagType = (c) => ({ common: 'primary', reasoning: 'warning', instruction: 'success', code: 'danger',
  creative: 'info', summary: 'success', needle: 'warning', consistency: 'info' }[c] || 'info')
const expectedPlaceholder = computed(() => {
  const c = questionForm.value.category
  if (c === 'instruction') return '指令遵循题:填必需字段名,逗号分隔(如 name,price)'
  if (c === 'needle') return '长文找回题:留空即可,运行时自动埋针生成答案'
  return '参考答案或评分要点(rubric),LLM 评判用'
})
const openQuestion = (row) => {
  questionForm.value = row
    ? { ...row }
    : { id: 0, qtype: 'common', category: 'common', judge_mode: 'llm', artifact_type: 'none',
        expected_answer: '', title: '', prompt: '', long_text: '', enabled: true, sort: 0, use_goal: false }
  questionDialog.value = true
}
// 选产物类型时默认开启目标驱动(goal 契约)
watch(() => questionForm.value.artifact_type, (t) => {
  if (t && t !== 'none' && questionForm.value.use_goal === false) questionForm.value.use_goal = true
})
const saveQuestion = async () => {
  const f = questionForm.value
  if (!f.prompt) { ElMessage.warning('题干必填'); return }
  f.qtype = ['common', 'reasoning', 'summary'].includes(f.category) ? f.category : 'common'
  if (f.id) await api.put(`/questions/${f.id}`, f)
  else await api.post('/questions', f)
  ElMessage.success('已保存')
  questionDialog.value = false
  load()
}
const toggleQuestion = async (row, val) => { await api.put(`/questions/${row.id}`, { ...row, enabled: val }); load() }
const removeQuestion = async (row) => { await api.delete(`/questions/${row.id}`); load() }

onMounted(load) 
</script>

<style scoped>
.custom-tier-row { border: 1px solid #e4e7ed; border-radius: 6px; padding: 8px 10px; margin-bottom: 8px;
  background: #fafafa; }
</style>
