import { createRouter, createWebHistory } from 'vue-router'

const routes = [
  { path: '/login', component: () => import('./views/Login.vue'), meta: { bare: true } },
  { path: '/', redirect: '/home' },
  { path: '/home', component: () => import('./views/Home.vue') },
  { path: '/projects', component: () => import('./views/Projects.vue'), meta: { parent: '/home' } },
  { path: '/projects/create', component: () => import('./views/ProjectCreate.vue'), meta: { parent: '/projects' } },
  { path: '/projects/:id', component: () => import('./views/ProjectDetail.vue'), meta: { parent: '/projects' } },
  { path: '/devices', redirect: '/projects' },
  { path: '/corpus', component: () => import('./views/Corpus.vue'), meta: { parent: '/home' } },
  { path: '/tasks/:id', component: () => import('./views/TaskDetail.vue'), meta: { parent: '/projects' } },
  { path: '/compare', component: () => import('./views/Compare.vue'), meta: { parent: '/home' } },
  { path: '/settings', component: () => import('./views/Settings.vue'), meta: { parent: '/home' } },
]

const router = createRouter({ history: createWebHistory(), routes })

router.beforeEach(async (to) => {
  if (to.path === '/login') return true
  try {
    const resp = await fetch('/api/me')
    if (resp.status === 401) return '/login'
    const data = await resp.json()
    if (data && data.force_password_change && to.path !== '/settings') return '/settings'
  } catch {
    return '/login'
  }
  return true
})

export default router
