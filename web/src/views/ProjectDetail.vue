<template>
  <div v-loading="true" style="padding: 40px; text-align: center; color: #909399">正在打开任务详情…</div>
</template>

<script setup>
// V2.4:项目即任务。/projects/:id 只作兼容入口,重定向到唯一任务的详情页
import { onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import api from '../api'

const route = useRoute()
const router = useRouter()

onMounted(async () => {
  try {
    const { data } = await api.get(`/projects/${route.params.id}`)
    const tid = (data.tasks || [])[0]?.id
    if (tid) router.replace(`/tasks/${tid}`)
    else {
      ElMessage.warning('该项目还没有任务')
      router.replace('/projects')
    }
  } catch {
    router.replace('/projects')
  }
})
</script>
