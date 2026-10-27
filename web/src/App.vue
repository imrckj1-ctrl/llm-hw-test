<template>
  <router-view v-if="$route.meta.bare" />
  <el-container v-else class="layout">
    <el-aside width="220px" class="aside">
      <div class="brand">
        <img src="/logo.png" alt="灵思微致" style="width: 30px; height: 30px" />
        <div>
          <div class="brand-title">灵思微致测试平台</div>
          <div class="brand-sub">LSWZ TEST BENCH</div>
        </div>
      </div>
      <el-menu :default-active="activeMenu" router class="menu">
        <el-menu-item index="/home"><el-icon><Odometer /></el-icon>首页</el-menu-item>
        <el-menu-item index="/projects"><el-icon><FolderOpened /></el-icon>项目</el-menu-item>
        <el-menu-item index="/corpus"><el-icon><Document /></el-icon>语料与题库</el-menu-item>
        <el-menu-item index="/compare"><el-icon><DataAnalysis /></el-icon>对比分析</el-menu-item>
        <el-menu-item index="/settings"><el-icon><Setting /></el-icon>系统设置</el-menu-item>
      </el-menu>
      <div class="aside-foot">
        <el-button text type="danger" @click="logout"><el-icon><SwitchButton /></el-icon>退出登录</el-button>
      </div>
    </el-aside>
    <el-main class="main">
      <router-view />
    </el-main>
  </el-container>
</template>

<script setup>
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import api from './api'

const route = useRoute()
const router = useRouter()
const activeMenu = computed(() => {
  const p = route.path
  if (p.startsWith('/home')) return '/home'
  if (p.startsWith('/corpus')) return '/corpus'
  if (p.startsWith('/compare')) return '/compare'
  if (p.startsWith('/settings')) return '/settings'
  return '/projects'
})
const logout = async () => {
  await api.post('/logout').catch(() => {})
  router.push('/login')
}
</script>

<style>
html, body, #app { height: 100%; margin: 0; }
body { font-family: 'Helvetica Neue', 'PingFang SC', 'Microsoft YaHei', sans-serif; background: #f5f7fa; }
.layout { height: 100vh; }
.aside { background: #fff; border-right: 1px solid #e4e7ed; display: flex; flex-direction: column; }
.brand { display: flex; gap: 10px; align-items: center; padding: 18px 16px; border-bottom: 1px solid #f0f2f5; }
.brand-title { font-size: 15px; font-weight: 600; color: #303133; }
.brand-sub { font-size: 11px; color: #909399; }
.menu { border-right: none; flex: 1; }
.aside-foot { padding: 12px 16px; border-top: 1px solid #f0f2f5; }
.main { padding: 16px 20px; overflow-y: auto; }
.page-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 14px; }
.page-title { font-size: 18px; font-weight: 600; color: #303133; margin: 0; }
.filter-bar { display: flex; gap: 10px; align-items: center; margin-bottom: 12px; flex-wrap: wrap; }
.metric-card { text-align: center; padding: 6px 4px; }
.metric-value { font-size: 20px; font-weight: 700; color: #303133; }
.metric-label { font-size: 12px; color: #909399; margin-top: 4px; }
</style>
