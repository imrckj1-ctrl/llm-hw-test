<template>
  <el-form label-width="120px" :label-position="dense ? 'top' : 'right'" size="default">
    <el-divider content-position="left">接口与模型</el-divider>
    <el-row :gutter="12">
      <el-col :span="12">
        <el-form-item label="API 地址" required>
          <div style="display: flex; gap: 8px; width: 100%">
            <el-input v-model="v.api_base_url" placeholder="http://192.168.x.x:8000 或 http://…/v1" style="flex: 1" />
            <el-tooltip content="按 OpenAI 标准 GET /v1/models 拉取模型列表" placement="top">
              <el-button :loading="fetchingModels" @click="fetchModels"><el-icon><Search /></el-icon>查询模型</el-button>
            </el-tooltip>
          </div>
        </el-form-item>
      </el-col>
      <el-col :span="6">
        <el-form-item label="模型名称" required>
          <el-select v-model="v.model_name" filterable allow-create default-first-option
            :placeholder="modelsList.length ? '从服务端列表选择' : '与服务端 --served-model-name 一致'" style="width: 100%">
            <el-option v-for="m in modelsList" :key="m" :label="m" :value="m" />
          </el-select>
        </el-form-item>
      </el-col>
      <el-col :span="6">
        <el-form-item label="量化精度">
          <el-select v-model="v.quant" filterable allow-create default-first-option placeholder="选择或输入">
            <el-option v-for="q in quantOptions" :key="q" :label="q" :value="q" />
          </el-select>
        </el-form-item>
      </el-col>
    </el-row>
    <el-row :gutter="12">
      <el-col :span="6">
        <el-form-item label="HF ID"><el-input v-model="v.model_hf_id" placeholder="选填" /></el-form-item>
      </el-col>
      <el-col :span="6">
        <el-form-item label="魔搭 ID"><el-input v-model="v.model_ms_id" placeholder="选填" /></el-form-item>
      </el-col>
      <el-col :span="6">
        <el-form-item label="上下文长度">
          <el-input v-model="contextText" placeholder="256K" style="width: 100%" />
          <div class="hint" :style="ctxErr ? 'color:#f56c6c' : ''">
            {{ ctxErr || '按 1024 进制:填 256K 即 262,144 tokens;百万级填 1M。默认 256K' }}
          </div>
        </el-form-item>
      </el-col>
      <el-col :span="6">
        <el-form-item label="思考等级(Reasoning Effort)">
          <el-select v-model="v.params.advanced.thinking_mode" style="width: 100%">
            <el-option label="默认(default,不加字段)" value="default" />
            <el-option label="关闭思考(off)" value="off" />
            <el-option label="极简(minimal)" value="minimal" />
            <el-option label="低(low)" value="low" />
            <el-option label="中(medium)" value="medium" />
            <el-option label="高(high)" value="high" />
            <el-option label="极高(xhigh)" value="xhigh" />
            <el-option label="最高(max)" value="max" />
          </el-select>
          <div class="hint">仅质量验证(能力测试)启用</div>
        </el-form-item>
      </el-col>
    </el-row>
    <el-row :gutter="12">
      <el-col :span="12">
        <el-form-item label="API Key">
          <el-input v-model="v.api_key" type="password" show-password
            :placeholder="v.api_key_set ? '已设置,留空不修改' : '选填;服务端需 Bearer 鉴权时填写'" />
        </el-form-item>
      </el-col>
      <el-col :span="6" v-if="v.api_key_set">
        <el-form-item label="清除密钥">
          <el-checkbox v-model="v.api_key_clear">保存时清除已设置的 Key</el-checkbox>
        </el-form-item>
      </el-col>
    </el-row>

    <el-divider content-position="left">测试项(勾选后展开各自参数)</el-divider>
    <el-row :gutter="12">
      <el-col :span="10">
        <el-form-item label="语料版本(全部压测项共用)">
          <el-select v-model="v.corpus_version_id" style="width: 100%">
            <el-option v-for="cv in corpusVersions" :key="cv.id" :label="cv.version_no" :value="cv.id" />
          </el-select>
          <div class="hint" style="margin-top: 4px">下方各测试项的档位选项 = 该版本实有的档位</div>
        </el-form-item>
      </el-col>
    </el-row>
    <div class="item-list">
      <div class="item-block">
        <el-checkbox v-model="v.params.items.quality"><b>能力题库</b></el-checkbox>
        <div v-if="v.params.items.quality" class="item-config">
          <el-row :gutter="12">
            <el-col :span="24">
              <el-form-item label="选题方式">
                <el-radio-group v-model="qualityMode">
                  <el-radio-button value="all">全部题目</el-radio-button>
                  <el-radio-button value="cats">按维度</el-radio-button>
                  <el-radio-button value="group">按题组</el-radio-button>
                  <el-radio-button value="custom">自定义题目</el-radio-button>
                </el-radio-group>
              </el-form-item>
            </el-col>
          </el-row>
          <el-row :gutter="12" v-if="qualityMode === 'cats'">
            <el-col :span="24">
              <el-form-item label="能力维度">
                <el-select v-model="v.params.items.capability_cats" multiple collapse-tags collapse-tags-tooltip
                  placeholder="选择维度,可多选" style="width: 100%">
                  <el-option label="常识问答" value="common" />
                  <el-option label="简单推理" value="reasoning" />
                  <el-option label="指令遵循" value="instruction" />
                  <el-option label="代码生成(产物落地)" value="code" />
                  <el-option label="创作写作" value="creative" />
                  <el-option label="长文摘要" value="summary" />
                  <el-option label="长文找回" value="needle" />
                  <el-option label="答案一致性" value="consistency" />
                </el-select>
              </el-form-item>
            </el-col>
          </el-row>
          <el-row :gutter="12" v-if="qualityMode === 'group'">
            <el-col :span="24">
              <el-form-item label="题组">
                <el-select v-model="v.params.items.quality_group_id" placeholder="选择题组(在「语料与题库 → 题组」维护)"
                  style="width: 100%">
                  <el-option v-for="g in questionGroups" :key="g.id"
                    :label="`${g.name}(${(g.question_ids || []).length} 题)`" :value="g.id" />
                </el-select>
                <div class="hint" style="margin-top: 4px">没有题组?去「语料与题库 → 题组」新建;停用题目不参与</div>
              </el-form-item>
            </el-col>
          </el-row>
          <el-row :gutter="12" v-if="qualityMode === 'custom'">
            <el-col :span="24">
              <el-form-item label="题目">
                <el-button @click="pickerOpen = true">
                  <el-icon><List /></el-icon>逐题选择
                </el-button>
                <el-tag style="margin-left: 8px" effect="plain">
                  已选 {{ (v.params.items.quality_question_ids || []).length }} / {{ allQuestions.length }} 题
                </el-tag>
              </el-form-item>
            </el-col>
          </el-row>
        </div>
      </div>
      <div class="item-block">
        <el-checkbox v-model="v.params.items.ladder"><b>阶梯测试</b></el-checkbox>
        <div v-if="v.params.items.ladder" class="item-config">
          <el-row :gutter="12">
            <el-col :span="14">
              <el-form-item label="阶梯档位">
                <el-select v-model="v.params.exec.ladder_tiers" multiple
                  placeholder="档位来自语料库实有档位" style="width: 100%">
                  <el-option v-for="t in tierOptions" :key="t" :label="t" :value="t" />
                </el-select>
                <div class="hint" style="margin-top: 4px">需要 50K/100K 等新档位?先去「语料与题库」上传对应语料</div>
              </el-form-item>
            </el-col>
            <el-col :span="10">
              <el-form-item label="每档重复">
                <el-input-number v-model="v.params.exec.ladder_repeats" :min="0" placeholder="默认3" style="width: 100%" />
                <div class="hint" style="margin-top: 4px">0 = 默认 3 次;阶梯不预热,每次随机前缀防缓存干扰</div>
              </el-form-item>
            </el-col>
          </el-row>
        </div>
      </div>
      <div class="item-block">
        <el-checkbox v-model="v.params.items.concurrency"><b>并发测试</b></el-checkbox>
        <div v-if="v.params.items.concurrency" class="item-config">
          <el-row :gutter="12">
            <el-col :span="6">
              <el-form-item label="输入档(可多选)">
                <el-select v-model="v.params.exec.concurrency_tiers" multiple filterable allow-create default-first-option style="width: 100%">
                  <el-option v-for="t in tierOptions" :key="t" :label="t" :value="t" />
                </el-select>
                <div class="hint" style="margin-top: 4px">多选则逐档各跑一遍(成本随档数翻倍);熔断按档独立</div>
              </el-form-item>
            </el-col>
            <el-col :span="12">
              <el-form-item label="并发档位">
                <el-select v-model="v.params.exec.concurrency_levels" multiple filterable allow-create default-first-option
                  placeholder="可选预设,或输入整数回车自定义" style="width: 100%">
                  <el-option v-for="c in [1, 4, 8, 16, 32, 64]" :key="c" :label="c" :value="c" />
                </el-select>
                <div class="hint" style="margin-top: 4px">输入任意正整数回车即可加自定义档,如 24 / 48</div>
              </el-form-item>
            </el-col>
            <el-col :span="6">
              <el-form-item label="每档重复">
                <el-input-number v-model="v.params.exec.concurrency_repeats" :min="0" style="width: 100%" />
              </el-form-item>
            </el-col>
          </el-row>
        </div>
      </div>
      <div class="item-block">
        <el-checkbox v-model="v.params.items.cache"><b>缓存测试</b></el-checkbox>
        <div v-if="v.params.items.cache" class="item-config">
          <el-row :gutter="12">
            <el-col :span="8">
              <el-form-item label="测试档位">
                <el-select v-model="v.params.exec.cache_tiers" multiple placeholder="留空 = 跟随阶梯档位" style="width: 100%">
                  <el-option v-for="t in tierOptions" :key="t" :label="t" :value="t" />
                </el-select>
              </el-form-item>
            </el-col>
            <el-col :span="6">
              <el-form-item label="叠加轮次">
                <el-input-number v-model="v.params.exec.cache_repeats" :min="0" style="width: 100%" />
              </el-form-item>
            </el-col>
            <el-col :span="10">
              <div class="hint" style="padding-top: 8px">对话式叠加:每轮把上轮回答并入上下文再发同一文档;0 = 默认 3 轮</div>
            </el-col>
          </el-row>
        </div>
      </div>
      <div class="item-block">
        <el-checkbox v-model="v.params.items.stability"><b>稳定性测试</b></el-checkbox>
        <div v-if="v.params.items.stability" class="item-config">
          <el-row :gutter="12">
            <el-col :span="6">
              <el-form-item label="输入档">
                <el-select v-model="v.params.exec.stability_tier" style="width: 100%">
                  <el-option v-for="t in tierOptions" :key="t" :label="t" :value="t" />
                </el-select>
              </el-form-item>
            </el-col>
            <el-col :span="6">
              <el-form-item label="每波并发">
                <el-input-number v-model="v.params.exec.stability_concurrency" :min="1" style="width: 100%" />
              </el-form-item>
            </el-col>
            <el-col :span="6">
              <el-form-item label="时长(分)">
                <el-input-number v-model="v.params.exec.stability_minutes" :min="1" style="width: 100%" />
              </el-form-item>
            </el-col>
          </el-row>
        </div>
      </div>
      <div class="item-block">
        <el-checkbox v-model="v.params.items.niah"><b>NIAH 检索测试</b></el-checkbox>
        <div v-if="v.params.items.niah" class="item-config">
          <el-row :gutter="12">
            <el-col :span="8">
              <el-form-item label="边界点复测次数">
                <el-input-number v-model="v.params.exec.niah_repeats" :min="1" :max="9" placeholder="默认3" style="width: 100%" />
              </el-form-item>
            </el-col>
          </el-row>
          <div class="hint">勾选后自动扫描,无需其他配置。填了上下文长度:验证宣称值——硬上限预检 + 长度{50%,75%,100%} × 埋针深度{25%,50%,75%} × 2 次,判定是否与宣称一致;
            未填上下文长度:探测真实上限——8K 起逐档翻倍(默认最高 1M),首个失败点二分细化,边界点 3 次 × 3 深度复测确认。
            请求被拒/截断记「错误」不计入丢针,两种结局分别给出硬上限与有效上下文(多少 K)。</div>
        </div>
      </div>
    </div>

    <el-collapse class="adv-collapse">
      <el-collapse-item title="生成参数与高级设置(默认不展开,使用默认值)" name="adv">
        <el-divider content-position="left">生成参数</el-divider>
        <el-row :gutter="12">
          <el-col :span="6"><el-form-item label="temperature"><el-input-number v-model="v.params.gen.temperature" :min="0" :max="2" :step="0.1" style="width: 100%" /></el-form-item></el-col>
          <el-col :span="6"><el-form-item label="top_p"><el-input-number v-model="v.params.gen.top_p" :min="0" :max="1" :step="0.05" style="width: 100%" /></el-form-item></el-col>
          <el-col :span="6"><el-form-item label="top_k"><el-input-number v-model="v.params.gen.top_k" :min="0" style="width: 100%" /></el-form-item></el-col>
          <el-col :span="6">
            <el-form-item label="max_tokens(输出上限)"><el-input-number v-model="v.params.gen.max_tokens" :min="1" :step="128" style="width: 100%" /></el-form-item>
            <div class="hint" style="margin-top: 2px">只管性能测试项;质量抽查不限制输出长度(模型自由作答)</div>
          </el-col>
        </el-row>
        <el-divider content-position="left">缓存信息(向厂商确认后填)</el-divider>
        <el-row :gutter="12">
          <el-col :span="6">
            <el-form-item label="缓存开关">
              <el-select v-model="v.cache_enabled" style="width: 100%">
                <el-option label="未知" value="unknown" />
                <el-option label="开" value="on" />
                <el-option label="关" value="off" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="缓存说明"><el-input v-model="v.cache_note" placeholder="选填" /></el-form-item>
          </el-col>
        </el-row>
        <el-divider content-position="left">高级</el-divider>
        <el-row :gutter="12">
          <el-col :span="6"><el-form-item label="单请求超时(s)"><el-input-number v-model="v.params.advanced.request_timeout_sec" :min="10" style="width: 100%" /></el-form-item></el-col>
          <el-col :span="6">
            <el-form-item label="预热次数(并发/稳定性)">
              <el-input-number v-model="v.params.exec.warmup" :min="0" style="width: 100%" />
            </el-form-item>
          </el-col>
          <el-col :span="6">
            <el-form-item label="ignore_eos">
              <el-switch v-model="v.params.advanced.ignore_eos" />
            </el-form-item>
          </el-col>
        </el-row>
        <el-row :gutter="12">
          <el-col :span="24">
            <el-form-item label="自定义请求参数">
              <el-input v-model="v.params.advanced.extra_params" placeholder='选填 JSON,如 {"chat_template_kwargs":{"enable_thinking":false}}' />
            </el-form-item>
          </el-col>
        </el-row>
    </el-collapse-item>
    </el-collapse>

    <el-dialog v-model="pickerOpen" title="自定义选题(逐题勾选)" width="760px" top="5vh" append-to-body>
      <div style="display: flex; gap: 8px; margin-bottom: 10px; align-items: center">
        <el-button size="small" @click="pickAll">全选</el-button>
        <el-button size="small" @click="pickNone">清空</el-button>
        <span class="hint">已选 {{ (v.params.items.quality_question_ids || []).length }} / {{ allQuestions.length }} 题;停用题目不参与执行</span>
      </div>
      <div style="max-height: 55vh; overflow-y: auto">
        <div v-for="grp in pickerGroups" :key="grp.cat" style="margin-bottom: 10px">
          <div style="font-weight: 600; font-size: 13px; margin-bottom: 4px">{{ grp.name }}({{ grp.list.length }})</div>
          <el-checkbox v-for="q in grp.list" :key="q.id" :model-value="picked.has(q.id)"
            @change="(val) => togglePick(q.id, val)" style="display: flex; margin-right: 0">
            <span :style="{ color: q.enabled ? '#303133' : '#c0c4cc' }">
              {{ q.id }} {{ q.title || q.prompt.slice(0, 30) }}{{ q.enabled ? '' : '(停用)' }}
            </span>
          </el-checkbox>
        </div>
        <el-empty v-if="!allQuestions.length" description="题库为空,请先到「语料与题库」添加题目" />
      </div>
      <template #footer>
        <el-button type="primary" @click="pickerOpen = false">完成</el-button>
      </template>
    </el-dialog>
  </el-form>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import api from '../api'
import { quantOptions, parseContextLen, formatContextLen } from '../utils'

const props = defineProps({ modelValue: Object, corpusVersions: Array, dense: Boolean })
const emit = defineEmits(['update:modelValue'])
const v = computed({
  get: () => props.modelValue,
  set: (val) => emit('update:modelValue', val),
})

// V2.12:上下文长度 K/M 文本输入(1024 进制),落库仍为 tokens 数字
const ctxErr = ref('')
const contextText = computed({
  get: () => formatContextLen(v.value.context_len),
  set: (val) => {
    const n = parseContextLen(val)
    if (n === null) {
      ctxErr.value = '格式不正确:请填数字 + 单位,如 256K / 1M'
      return
    }
    ctxErr.value = ''
    v.value.context_len = n
  },
})

// 档位选项:完全由「所选语料版本」实有档位驱动(V2.6 起按版本过滤,不再是全库并集)
const tierOptions = computed(() => {
  const cvs = props.corpusVersions || []
  if (!cvs.length) return ['1K', '10K', '20K', '30K']
  const sel = cvs.find((x) => x.id === v.value.corpus_version_id) || cvs[cvs.length - 1]
  const set = new Set()
  for (const t of (sel && sel.texts) || []) set.add(t.tier)
  if (set.size === 0) ['1K', '10K', '20K', '30K'].forEach((t) => set.add(t))
  return [...set].sort((a, b) => parseFloat(a) - parseFloat(b))
})

// 默认选中最新语料版本;切换版本时剔除该版本没有的档位
watch(() => [props.corpusVersions, v.value.corpus_version_id], () => {
  const cvs = props.corpusVersions || []
  if (!cvs.length) return
  if (!v.value.corpus_version_id || !cvs.some((x) => x.id === v.value.corpus_version_id)) {
    v.value.corpus_version_id = cvs[cvs.length - 1].id
    return // 下次触发再做剔除
  }
  const avail = new Set(tierOptions.value)
  const ex = v.value.params.exec
  if (!ex.concurrency_tier) ex.concurrency_tier = '1K' // 并发输入档默认 1K
  // V2.15:并发输入档多选;老任务单档字段自动迁入
  if (!Array.isArray(ex.concurrency_tiers) || ex.concurrency_tiers.length === 0) {
    ex.concurrency_tiers = [ex.concurrency_tier || '1K']
  }
  ex.concurrency_tiers = ex.concurrency_tiers.filter((t) => avail.has(t))
  if (ex.concurrency_tiers.length === 0) ex.concurrency_tiers = [tierOptions.value[0] || '1K']
  ex.ladder_tiers = (ex.ladder_tiers || []).filter((t) => avail.has(t))
  ex.cache_tiers = (ex.cache_tiers || []).filter((t) => avail.has(t))
  ex.niah_tiers = (ex.niah_tiers || []).filter((t) => avail.has(t))
  if (ex.stability_tier && !avail.has(ex.stability_tier)) ex.stability_tier = tierOptions.value[0] || '1K'
  if (ex.concurrency_tier && !avail.has(ex.concurrency_tier)) ex.concurrency_tier = tierOptions.value[0] || '1K'
}, { immediate: true, deep: true })

// ---- 能力题库选题(V2.4):全部/按维度/按题组/自定义题目 ----
const catNames = { common: '常识问答', reasoning: '简单推理', instruction: '指令遵循', code: '代码生成',
  creative: '创作写作', summary: '长文摘要', needle: '长文找回', consistency: '答案一致性', doc: '文档生成' }
const questionGroups = ref([])
const allQuestions = ref([])
const pickerOpen = ref(false)

const qualityMode = computed({
  get: () => {
    const it = v.value.params.items
    if (it.quality_mode) return it.quality_mode
    // 兼容旧任务:按字段推断
    if ((it.quality_question_ids || []).length) return 'custom'
    if (it.quality_group_id) return 'group'
    if ((it.capability_cats || []).length) return 'cats'
    return 'all'
  },
  set: (mode) => { v.value.params.items.quality_mode = mode },
})

const picked = computed(() => new Set(v.value.params.items.quality_question_ids || []))
const togglePick = (id, val) => {
  const s = new Set(v.value.params.items.quality_question_ids || [])
  if (val) s.add(id); else s.delete(id)
  v.value.params.items.quality_question_ids = [...s].sort((a, b) => a - b)
}
const pickAll = () => { v.value.params.items.quality_question_ids = allQuestions.value.map((q) => q.id) }
const pickNone = () => { v.value.params.items.quality_question_ids = [] }
const pickerGroups = computed(() => {
  const byCat = {}
  for (const q of allQuestions.value) {
    const c = q.category || q.qtype || 'common'
    ;(byCat[c] = byCat[c] || []).push(q)
  }
  return Object.entries(byCat).map(([cat, list]) => ({ cat, name: catNames[cat] || cat, list }))
})

let bankLoaded = false
const loadBank = async () => {
  if (bankLoaded) return
  bankLoaded = true
  try {
    const [{ data: gs }, { data: qs }] = await Promise.all([
      api.get('/question-groups'), api.get('/questions'),
    ])
    questionGroups.value = gs || []
    allQuestions.value = qs || []
  } catch { /* 列表为空时忽略 */ }
}
watch(() => v.value.params.items.quality, (on) => { if (on) loadBank() }, { immediate: true })

const fetchingModels = ref(false)
const modelsList = ref([])
const fetchModels = async () => {
  if (!v.value.api_base_url) { ElMessage.warning('请先填写 API 地址'); return }
  fetchingModels.value = true
  try {
    const { data } = await api.post('/tasks/fetch-models', {
      api_base_url: v.value.api_base_url,
      api_key: v.value.api_key || '',
      task_id: v.value.id || 0,
    })
    modelsList.value = data.models || []
    ElMessage.success(`查询到 ${modelsList.value.length} 个模型`)
    if (modelsList.value.length === 1) v.value.model_name = modelsList.value[0]
  } finally {
    fetchingModels.value = false
  }
}
</script>

<style scoped>
.hint { font-size: 12px; color: #909399; }
.item-list { display: flex; flex-direction: column; gap: 4px; margin-bottom: 14px; }
.item-block { border: 1px solid #ebeef5; border-radius: 8px; padding: 10px 14px; }
.item-config { margin-top: 8px; padding-top: 10px; border-top: 1px dashed #e4e7ed; }
.adv-collapse { border-top: none; }
</style>
