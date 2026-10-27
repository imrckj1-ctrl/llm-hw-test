<template>
  <div ref="el" :style="{ height: height + 'px', width: '100%' }"></div>
</template>

<script setup>
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import * as echarts from 'echarts'

const props = defineProps({ option: Object, height: { type: Number, default: 280 } })
const el = ref(null)
let chart = null
let ro = null

const renderChart = () => {
  if (!chart || !props.option) return
  chart.setOption(props.option, true)
}
onMounted(() => {
  chart = echarts.init(el.value)
  renderChart()
  ro = new ResizeObserver(() => chart && chart.resize())
  ro.observe(el.value)
})
watch(() => props.option, renderChart, { deep: true })
onBeforeUnmount(() => {
  ro && ro.disconnect()
  chart && chart.dispose()
})
</script>
