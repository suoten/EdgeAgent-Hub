import { createRouter, createWebHashHistory } from 'vue-router'

const routes = [
  {
    path: '/login',
    name: 'Login',
    component: () => import('@/views/Login.vue'),
    meta: { public: true },
  },
  {
    path: '/',
    component: () => import('@/layout/MainLayout.vue'),
    redirect: '/dashboard',
    children: [
      { path: 'dashboard', name: 'Dashboard', component: () => import('@/views/Dashboard.vue'), meta: { title: '仪表盘', icon: 'Odometer' } },
      { path: 'devices', name: 'Devices', component: () => import('@/views/Devices.vue'), meta: { title: '设备管理', icon: 'Cpu' } },
      { path: 'alerts', name: 'Alerts', component: () => import('@/views/Alerts.vue'), meta: { title: '告警中心', icon: 'BellFilled' } },
      { path: 'workflows', name: 'Workflows', component: () => import('@/views/Workflows.vue'), meta: { title: '编排管理', icon: 'SetUp' } },
      { path: 'models', name: 'Models', component: () => import('@/views/Models.vue'), meta: { title: '模型管理', icon: 'Box' } },
      { path: 'knowledge', name: 'Knowledge', component: () => import('@/views/Knowledge.vue'), meta: { title: '知识库', icon: 'Collection' } },
      { path: 'ota', name: 'OTA', component: () => import('@/views/OTA.vue'), meta: { title: 'OTA更新', icon: 'Promotion' } },
      { path: 'monitor', name: 'Monitor', component: () => import('@/views/Monitor.vue'), meta: { title: '系统监控', icon: 'Monitor' } },
      { path: 'agents', name: 'Agents', component: () => import('@/views/Agents.vue'), meta: { title: '智能体', icon: 'Connection' } },
      { path: 'chat', name: 'Chat', component: () => import('@/views/Chat.vue'), meta: { title: '智能对话', icon: 'ChatDotRound' } },
      { path: 'security', name: 'Security', component: () => import('@/views/Security.vue'), meta: { title: '安全管理', icon: 'Lock' } },
    ],
  },
]

const router = createRouter({
  history: createWebHashHistory(),
  routes,
})

router.beforeEach((to, from, next) => {
  const token = localStorage.getItem('edgeagent_token')
  if (!to.meta.public && !token) {
    next('/login')
  } else if (to.path === '/login' && token) {
    next('/dashboard')
  } else {
    next()
  }
})

export default router
