import dayjs from 'dayjs'

export const fmtTime = (ms) => (ms ? dayjs(ms).format('YYYY-MM-DD HH:mm:ss') : '-')
export const fmtTimeShort = (ms) => (ms ? dayjs(ms).format('HH:mm:ss') : '-')
export const fmtNum = (v, d = 2) => (v > 0 ? Number(v.toFixed(d)) : '-')

// token 数 → 人类可读(128000 → 128K,1000000 → 1M)
export const fmtTokens = (n) => {
  if (!n || n <= 0) return '-'
  if (n >= 1000000) return `${Number((n / 1000000).toFixed(2))}M`
  if (n >= 1000) return `${Number((n / 1000).toFixed(1))}K`
  return String(n)
}

export const taskStatus = {
  pending: { text: '待执行', type: 'info' },
  queued: { text: '排队中', type: 'warning' },
  running: { text: '执行中', type: 'primary' },
  done: { text: '已完成', type: 'success' },
  failed: { text: '失败', type: 'danger' },
  cancelled: { text: '已取消', type: 'info' },
  interrupted: { text: '中断', type: 'danger' },
  paused: { text: '已暂停', type: 'warning' },
}

export const projectStatus = {
  pending: { text: '待启动', type: 'info' },
  queued: { text: '排队中', type: 'warning' },
  running: { text: '进行中', type: 'primary' },
  done: { text: '已完成', type: 'success' },
  partial_failed: { text: '部分失败', type: 'warning' },
  paused: { text: '已暂停', type: 'warning' },
}

export const itemStatus = {
  pending: { text: '待执行', type: 'info' },
  running: { text: '执行中', type: 'primary' },
  done: { text: '完成', type: 'success' },
  failed: { text: '失败', type: 'danger' },
  skipped: { text: '跳过', type: 'warning' },
  cancelled: { text: '已取消', type: 'info' },
  interrupted: { text: '中断', type: 'warning' },
}

export const itemTypeName = {
  ladder: '阶梯测试',
  concurrency: '并发测试',
  cache: '缓存测试',
  stability: '稳定性测试',
  quality: '质量抽查',
  niah: 'NIAH 检索',
}

export const phaseName = {
  warmup: '预热',
  calibration: '校准',
  formal: '正式',
  cold: '冷',
  hot: '热',
  stability: '稳定性',
  quality: '质量',
  niah: 'NIAH',
}

export const formFactors = ['AI盒子', 'PCIe加速卡', 'M.2算力卡', '涡轮风扇卡', '服务器整机', '其他']

// V2.12:测试整机形态(算力设备所安装的整机)
export const machineFormFactors = ['台式机', '工作站', '服务器', 'miniPC', '笔记本', '其他']

// V2.12:上下文长度 K/M 文本解析(1024 进制):256K→262144,1M→1048576;纯数字按 tokens 直取
export function parseContextLen(text) {
  const s = String(text ?? '').trim().toUpperCase()
  if (!s) return 0
  const m = s.match(/^(\d+(?:\.\d+)?)\s*([KM])?$/)
  if (!m) return null
  const base = parseFloat(m[1])
  if (!(base > 0)) return null
  const mult = m[2] === 'K' ? 1024 : m[2] === 'M' ? 1024 * 1024 : 1
  return Math.round(base * mult)
}

// 数字 → K/M 展示(整除才缩写,否则原样)
export function formatContextLen(n) {
  n = Number(n) || 0
  if (n <= 0) return ''
  if (n % (1024 * 1024) === 0) return `${n / (1024 * 1024)}M`
  if (n % 1024 === 0) return `${n / 1024}K`
  return String(n)
}

// V2.16:ITL(ms/token) = 1000 ÷ TPS(decode 口径,不含 TTFT)
export function fmtITL(tps) {
  const v = Number(tps) || 0
  return v > 0 ? (1000 / v).toFixed(1) : '-'
}

export const quantOptions = ['FP16', 'BF16', 'FP8', 'INT8', 'INT4', 'W4A16', 'W8A8', '其他']

// V3.0.1:硬件信息空白模板(新建/编辑共用,字段与 HwForm 对齐)
export const blankHw = () => ({
  name: '', form_factor: '', cpu_soc: '', chip_model: '', chip_count: 1,
  tops_int8: 0, tflops_fp16: 0, vram_gb: 0, vram_type: '', vram_bw_gbps: 0, vram_shared: false,
  host_platform: '', pcie_gen: '', pcie_lanes: '', memory: '', storage: '', cooling: '',
  tdp_w: 0, network: '', notes: '',
  // V2.12:整机信息 + 推理软件环境
  firmware: '', machine_form: '', machine_model: '', machine_cpu: '',
  os_version: '', framework: '', driver_ver: '', sdk_ver: '', startup_cmd: '', env_note: '',
})

// B2:提交前规范化任务参数(并发档转数字/去重/升序,阶梯档 NK 校验)
export function normalizeTaskParams(params) {
  const p = JSON.parse(JSON.stringify(params))
  const lv = (p.exec.concurrency_levels || [])
    .map((x) => parseInt(x, 10))
    .filter((x) => Number.isInteger(x) && x > 0)
  p.exec.concurrency_levels = [...new Set(lv)].sort((a, b) => a - b)
  const tierRe = /^\d+(\.\d+)?K$/i
  p.exec.ladder_tiers = [...new Set((p.exec.ladder_tiers || [])
    .map((x) => String(x).trim().toUpperCase())
    .filter((x) => tierRe.test(x)))]
  // V2.15:并发输入档多选同规则校验
  p.exec.concurrency_tiers = [...new Set((p.exec.concurrency_tiers || [])
    .map((x) => String(x).trim().toUpperCase())
    .filter((x) => tierRe.test(x)))]
  return p
}
