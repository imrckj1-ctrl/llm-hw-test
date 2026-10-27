<template>
  <div class="login-wrap">
    <div class="login-box">
      <div class="login-brand">
        <img src="/logo.png" alt="灵思微致" style="width: 52px; height: 52px" />
        <h2>灵思微致测试平台</h2>
        <p>统一口径 · 自动化测试 · 数据沉淀</p>
      </div>
      <el-form @submit.prevent="submit">
        <el-form-item>
          <el-input v-model="password" type="password" size="large" placeholder="访问密码" show-password
            autofocus @keyup.enter="submit">
            <template #prefix><el-icon><Lock /></el-icon></template>
          </el-input>
        </el-form-item>
        <el-button type="primary" size="large" style="width: 100%" :loading="loading" @click="submit">登 录</el-button>
      </el-form>
      <el-alert v-if="tip" :title="tip" type="warning" :closable="false" style="margin-top: 14px" />
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import api from '../api'

const router = useRouter()
const password = ref('')
const loading = ref(false)
const tip = ref('')

const submit = async () => {
  if (!password.value) return
  loading.value = true
  tip.value = ''
  try {
    const { data } = await api.post('/login', { password: password.value })
    router.push(data.force_password_change ? '/settings' : '/home')
  } catch (e) {
    const d = e.response && e.response.data
    if (d && d.locked) tip.value = `已锁定,请 ${Math.ceil(d.remain_sec / 60)} 分钟后再试`
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.login-wrap { height: 100vh; display: flex; align-items: center; justify-content: center;
  background: linear-gradient(135deg, #1f2d3d 0%, #2b4c7e 100%); }
.login-box { width: 380px; background: #fff; border-radius: 8px; padding: 36px 32px; box-shadow: 0 8px 30px rgba(0,0,0,.25); }
.login-brand { text-align: center; margin-bottom: 26px; }
.login-brand h2 { font-size: 19px; margin: 12px 0 6px; color: #303133; }
.login-brand p { color: #909399; font-size: 12px; margin: 0; }
</style>
