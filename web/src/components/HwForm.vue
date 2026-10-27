<template>
  <div>
    <el-divider content-position="left">算力设备信息(随任务保存快照)</el-divider>
    <el-row :gutter="12">
      <el-col :span="8"><el-form-item label="硬件名称" required><el-input v-model="v.name" placeholder="如 智凯100 双卡整机" /></el-form-item></el-col>
      <el-col :span="8"><slot name="after-name" /></el-col>
      <el-col :span="8">
        <el-form-item label="设备形态">
          <el-select v-model="v.form_factor" style="width: 100%">
            <el-option v-for="f in formFactors" :key="f" :label="f" :value="f" />
          </el-select>
        </el-form-item>
      </el-col>
    </el-row>
    <el-row :gutter="12">
      <el-col :span="8"><el-form-item label="AI 芯片"><el-input v-model="v.chip_model" placeholder="型号,如 RK3588" /></el-form-item></el-col>
      <el-col :span="8"><el-form-item label="TOPS@INT8"><el-input-number v-model="v.tops_int8" :min="0" controls-position="right" style="width: 100%" /></el-form-item></el-col>
      <el-col :span="8"><el-form-item label="TFLOPS@FP16"><el-input-number v-model="v.tflops_fp16" :min="0" controls-position="right" style="width: 100%" /></el-form-item></el-col>
    </el-row>
    <el-row :gutter="12">
      <el-col :span="6"><el-form-item label="显存 (GB)"><el-input-number v-model="v.vram_gb" :min="0" controls-position="right" style="width: 100%" /></el-form-item></el-col>
      <el-col :span="6"><el-form-item label="显存类型"><el-input v-model="v.vram_type" /></el-form-item></el-col>
      <el-col :span="6"><el-form-item label="主控 CPU/SoC"><el-input v-model="v.cpu_soc" /></el-form-item></el-col>
      <el-col :span="6"><el-form-item label="标称 TDP (W)"><el-input-number v-model="v.tdp_w" :min="0" controls-position="right" style="width: 100%" /></el-form-item></el-col>
    </el-row>
    <el-row :gutter="12">
      <el-col :span="6"><el-form-item label="散热方式"><el-input v-model="v.cooling" /></el-form-item></el-col>
      <el-col :span="6"><el-form-item label="固件版本"><el-input v-model="v.firmware" placeholder="算力卡 / AI盒子固件版本" /></el-form-item></el-col>
      <el-col :span="12"><el-form-item label="硬件备注"><el-input v-model="v.notes" placeholder="驱动/固件等" /></el-form-item></el-col>
    </el-row>
    <el-divider content-position="left">测试整机信息(算力设备所安装的整机)</el-divider>
    <el-row :gutter="12">
      <el-col :span="8">
        <el-form-item label="整机形态">
          <el-select v-model="v.machine_form" style="width: 100%">
            <el-option v-for="f in machineFormFactors" :key="f" :label="f" :value="f" />
          </el-select>
        </el-form-item>
      </el-col>
      <el-col :span="8"><el-form-item label="整机型号 / 平台"><el-input v-model="v.machine_model" placeholder="如 联想 ThinkStation P3" /></el-form-item></el-col>
      <el-col :span="8"><el-form-item label="整机 CPU"><el-input v-model="v.machine_cpu" placeholder="如 i7-13700" /></el-form-item></el-col>
    </el-row>
    <el-row :gutter="12">
      <el-col :span="8"><el-form-item label="整机内存"><el-input v-model="v.memory" placeholder="如 32GB DDR5 5600" /></el-form-item></el-col>
      <el-col :span="8"><el-form-item label="整机硬盘"><el-input v-model="v.storage" placeholder="如 1TB NVMe" /></el-form-item></el-col>
      <el-col :span="8"><el-form-item label="操作系统及版本"><el-input v-model="v.os_version" placeholder="如 Ubuntu 22.04 / Win11 23H2" /></el-form-item></el-col>
    </el-row>
    <el-divider content-position="left">推理软件环境(模型推理链路软件栈)</el-divider>
    <el-row :gutter="12">
      <el-col :span="8"><el-form-item label="推理框架及版本"><el-input v-model="v.framework" placeholder="如 vLLM 0.27.1 / SGLang 0.4.x" /></el-form-item></el-col>
      <el-col :span="8"><el-form-item label="GPU / NPU 驱动版本"><el-input v-model="v.driver_ver" placeholder="如 550.90.07" /></el-form-item></el-col>
      <el-col :span="8"><el-form-item label="加速 SDK / 算子库版本"><el-input v-model="v.sdk_ver" placeholder="如 CANN 8.0 / RKNN 2.x / TensorRT 10.x" /></el-form-item></el-col>
    </el-row>
    <el-form-item label="模型启动命令(完整)">
      <el-input v-model="v.startup_cmd" type="textarea" :rows="3"
        placeholder="python3 -m vllm.entrypoints.openai.api_server --model /path/to/model --tensor-parallel-size 2 --enable-prefix-caching ..." />
      <div class="hint">原样粘贴完整启动命令,框架参数(kv-cache-dtype、prefix-caching、TP 数等)以此为准</div>
    </el-form-item>
    <el-form-item label="备注说明">
      <el-input v-model="v.env_note" type="textarea" :rows="2"
        placeholder="可添加必要的说明:启动环境、依赖安装、整机/算力设备的特殊情况等——面向整套硬件环境,不限于推理链路" />
    </el-form-item>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { formFactors, machineFormFactors } from '../utils'

const props = defineProps({ modelValue: Object })
const emit = defineEmits(['update:modelValue'])
const v = computed({
  get: () => props.modelValue,
  set: (val) => emit('update:modelValue', val),
})
</script>

<style scoped>
.hint { font-size: 12px; color: #909399; }
</style>
