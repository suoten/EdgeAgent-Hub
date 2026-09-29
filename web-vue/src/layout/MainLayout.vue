<template>
  <el-container class="main-layout">
    <!-- 侧边栏 -->
    <el-aside :width="isCollapse ? '64px' : '240px'" class="sidebar">
      <div class="sidebar-logo">
        <el-icon class="logo-icon"><Lightning /></el-icon>
        <span v-show="!isCollapse" class="logo-text">EdgeAgent Hub</span>
      </div>
      <el-menu
        :default-active="activeMenu"
        :collapse="isCollapse"
        :collapse-transition="false"
        background-color="#001529"
        text-color="#a6adb4"
        active-text-color="#ffffff"
        router
      >
        <el-menu-item
          v-for="item in menuItems"
          :key="item.path"
          :index="item.path"
        >
          <el-icon><component :is="item.icon" /></el-icon>
          <template #title>{{ item.title }}</template>
        </el-menu-item>
      </el-menu>
      <div class="sidebar-footer" v-show="!isCollapse">
        <div class="connection-status">
          <span class="eh-status-dot" :class="connected ? 'online' : 'offline'"></span>
          <span>{{ connected ? '系统已连接' : '连接断开' }}</span>
        </div>
      </div>
    </el-aside>

    <!-- 主区域 -->
    <el-container>
      <!-- 顶栏 -->
      <el-header class="topbar" height="56px">
        <div class="topbar-left">
          <el-icon class="collapse-btn" @click="isCollapse = !isCollapse">
            <Fold v-if="!isCollapse" />
            <Expand v-else />
          </el-icon>
          <el-breadcrumb separator="/">
            <el-breadcrumb-item :to="{ path: '/dashboard' }">首页</el-breadcrumb-item>
            <el-breadcrumb-item>{{ currentTitle }}</el-breadcrumb-item>
          </el-breadcrumb>
        </div>
        <div class="topbar-right">
          <el-dropdown @command="handleCommand">
            <span class="user-info">
              <el-avatar :size="32" class="user-avatar">
                <el-icon><UserFilled /></el-icon>
              </el-avatar>
              <span class="username">{{ username }}</span>
              <el-tag size="small" :type="roleTagType" effect="plain">{{ roleLabel }}</el-tag>
              <el-icon><ArrowDown /></el-icon>
            </span>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="profile" :icon="User">个人信息</el-dropdown-item>
                <el-dropdown-item command="logout" :icon="SwitchButton" divided>退出登录</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </el-header>

      <!-- 内容区 -->
      <el-main class="content-area">
        <router-view v-slot="{ Component }">
          <transition name="fade" mode="out-in">
            <component :is="Component" />
          </transition>
        </router-view>
      </el-main>
    </el-container>

    <!-- 个人信息弹窗 -->
    <el-dialog v-model="showProfileDialog" title="个人信息" width="480px">
      <div style="margin-bottom: 20px">
        <el-descriptions :column="1" border>
          <el-descriptions-item label="用户名">{{ username }}</el-descriptions-item>
          <el-descriptions-item label="角色">{{ roleLabel }}</el-descriptions-item>
        </el-descriptions>
      </div>
      <el-divider content-position="left">修改密码</el-divider>
      <el-form :model="pwdForm" :rules="pwdRules" ref="pwdFormRef" label-width="90px">
        <el-form-item label="当前密码" prop="old_password">
          <el-input v-model="pwdForm.old_password" type="password" placeholder="输入当前密码" show-password />
        </el-form-item>
        <el-form-item label="新密码" prop="new_password">
          <el-input v-model="pwdForm.new_password" type="password" placeholder="输入新密码（至少6位）" show-password />
        </el-form-item>
        <el-form-item label="确认密码" prop="confirm_password">
          <el-input v-model="pwdForm.confirm_password" type="password" placeholder="再次输入新密码" show-password />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showProfileDialog = false">关闭</el-button>
        <el-button type="primary" @click="doChangePassword" :loading="pwdLoading">修改密码</el-button>
      </template>
    </el-dialog>
  </el-container>
</template>

<script setup>
import { ref, computed, reactive, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  Lightning, Fold, Expand, ArrowDown, UserFilled, User, SwitchButton,
  Odometer, Cpu, BellFilled, SetUp, Box, Collection, Promotion, Monitor, Connection, ChatDotRound, Lock,
} from '@element-plus/icons-vue'
import api from '@/api'

const route = useRoute()
const router = useRouter()
const isCollapse = ref(false)
const connected = ref(true)
let healthTimer = null

const menuItems = [
  { path: '/dashboard', title: '仪表盘', icon: Odometer },
  { path: '/devices', title: '设备管理', icon: Cpu },
  { path: '/alerts', title: '告警中心', icon: BellFilled },
  { path: '/workflows', title: '编排管理', icon: SetUp },
  { path: '/models', title: '模型管理', icon: Box },
  { path: '/knowledge', title: '知识库', icon: Collection },
  { path: '/ota', title: 'OTA更新', icon: Promotion },
  { path: '/monitor', title: '系统监控', icon: Monitor },
  { path: '/agents', title: '智能体', icon: Connection },
  { path: '/chat', title: '智能对话', icon: ChatDotRound },
  { path: '/security', title: '安全管理', icon: Lock },
]

const activeMenu = computed(() => route.path)
const currentTitle = computed(() => {
  const item = menuItems.find(i => i.path === route.path)
  return item ? item.title : ''
})

const username = computed(() => localStorage.getItem('edgeagent_user') || 'admin')
const roleLabel = computed(() => {
  const role = localStorage.getItem('edgeagent_role') || 'admin'
  const map = { admin: '管理员', operator: '操作员', viewer: '观察者' }
  return map[role] || role
})
const roleTagType = computed(() => {
  const role = localStorage.getItem('edgeagent_role') || 'admin'
  if (role === 'admin') return 'danger'
  if (role === 'operator') return 'warning'
  return 'info'
})

const handleCommand = (cmd) => {
  if (cmd === 'profile') {
    showProfileDialog.value = true
    pwdForm.old_password = ''
    pwdForm.new_password = ''
    pwdForm.confirm_password = ''
  } else if (cmd === 'logout') {
    ElMessageBox.confirm('确定退出登录吗？', '提示', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning',
    }).then(() => {
      localStorage.removeItem('edgeagent_token')
      localStorage.removeItem('edgeagent_refresh_token')
      localStorage.removeItem('edgeagent_user')
      localStorage.removeItem('edgeagent_role')
      router.push('/login')
    }).catch(() => {})
  }
}

const showProfileDialog = ref(false)
const pwdLoading = ref(false)
const pwdFormRef = ref()
const pwdForm = reactive({ old_password: '', new_password: '', confirm_password: '' })
const pwdRules = {
  old_password: [{ required: true, message: '请输入当前密码', trigger: 'blur' }],
  new_password: [
    { required: true, message: '请输入新密码', trigger: 'blur' },
    { min: 6, message: '密码至少 6 位', trigger: 'blur' },
  ],
  confirm_password: [
    { required: true, message: '请确认新密码', trigger: 'blur' },
    {
      validator: (rule, value, callback) => {
        if (value !== pwdForm.new_password) callback(new Error('两次输入的密码不一致'))
        else callback()
      },
      trigger: 'blur',
    },
  ],
}

const doChangePassword = async () => {
  if (!pwdFormRef.value) return
  await pwdFormRef.value.validate(async (valid) => {
    if (!valid) return
    pwdLoading.value = true
    try {
      const data = await api.post('/auth/change-password', {
        old_password: pwdForm.old_password,
        new_password: pwdForm.new_password,
      })
      ElMessage.success(data.message || '密码修改成功')
      showProfileDialog.value = false
    } catch {} finally { pwdLoading.value = false }
  })
}

const checkHealth = async () => {
  try {
    await api.get('/system/health')
    connected.value = true
  } catch {
    connected.value = false
  }
}

onMounted(() => {
  checkHealth()
  healthTimer = setInterval(checkHealth, 30000)
})

onUnmounted(() => {
  if (healthTimer) clearInterval(healthTimer)
})
</script>

<style scoped>
.main-layout {
  height: 100vh;
}

.sidebar {
  background: #001529;
  transition: width 0.3s;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.sidebar-logo {
  height: 56px;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
  flex-shrink: 0;
}

.logo-icon {
  font-size: 28px;
  color: #1890ff;
}

.logo-text {
  font-size: 17px;
  font-weight: 700;
  color: #fff;
  white-space: nowrap;
}

.sidebar .el-menu {
  border-right: none;
  flex: 1;
}

.sidebar-footer {
  padding: 12px 20px;
  border-top: 1px solid rgba(255, 255, 255, 0.06);
  flex-shrink: 0;
}

.connection-status {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: #a6adb4;
}

.topbar {
  background: #fff;
  border-bottom: 1px solid var(--eh-border);
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 20px;
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.04);
}

.topbar-left {
  display: flex;
  align-items: center;
  gap: 16px;
}

.collapse-btn {
  font-size: 20px;
  cursor: pointer;
  color: var(--eh-text);
  transition: color 0.2s;
}
.collapse-btn:hover { color: var(--eh-primary); }

.topbar-right {
  display: flex;
  align-items: center;
}

.user-info {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  padding: 0 8px;
  height: 56px;
}

.user-avatar {
  background: linear-gradient(135deg, #1890ff, #36cfc9);
}

.username {
  font-size: 14px;
  color: var(--eh-text);
  font-weight: 500;
}

.content-area {
  padding: 0;
  background: var(--eh-bg);
  overflow-y: auto;
}

.el-menu-item.is-active {
  background: var(--eh-sidebar-active) !important;
}
</style>
