<template>
  <div class="back-bar">
      <el-tooltip content="返回上一级" placement="right">
      <el-button circle class="back-btn" @click="goBack">
        <el-icon :size="16"><ArrowLeft /></el-icon>
      </el-button>
    </el-tooltip>
    <h2 class="page-title">{{ title }}</h2>
    <div class="back-bar-actions"><slot /></div>
  </div>
</template>

<script setup>
import { useRoute, useRouter } from 'vue-router'

defineProps({ title: String })
const router = useRouter()
const route = useRoute()
// 按流程层级返回(meta.parent),不用浏览器历史——历史会跳回无关页面
const goBack = () => {
  const parent = route.meta && route.meta.parent
  router.push(parent || '/home')
}
</script>

<style scoped>
.back-bar { display: flex; align-items: center; gap: 10px; margin-bottom: 14px; }
.back-btn { flex: none; }
.page-title { font-size: 18px; font-weight: 600; color: #303133; margin: 0; flex: none; }
.back-bar-actions { margin-left: auto; display: flex; gap: 8px; align-items: center; }
</style>
