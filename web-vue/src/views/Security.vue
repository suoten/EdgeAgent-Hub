<template>
  <div class="eh-page">
    <el-alert type="info" :closable="false" show-icon style="margin-bottom: 16px">
      <template #title>安全管理</template>
      <template #default>
        基于角色的权限管理（RBAC）。系统内置三种角色：管理员（全部权限）、操作员（操作类权限）、观察者（只读权限）。
      </template>
    </el-alert>

    <!-- 权限矩阵 -->
    <div class="eh-card" style="margin-bottom: 16px">
      <div class="eh-card-header">
        <span class="eh-card-title"><el-icon><Key /></el-icon> 角色权限矩阵</span>
      </div>
      <el-table :data="permissionMatrix" stripe size="small">
        <el-table-column prop="module" label="功能模块" width="140" />
        <el-table-column label="管理员" width="100" align="center">
          <template #default="{ row }">
            <el-icon v-if="row.admin" color="#67c23a"><CircleCheckFilled /></el-icon>
            <el-icon v-else color="#f56c6c"><CircleCloseFilled /></el-icon>
          </template>
        </el-table-column>
        <el-table-column label="操作员" width="100" align="center">
          <template #default="{ row }">
            <el-icon v-if="row.operator" color="#67c23a"><CircleCheckFilled /></el-icon>
            <el-icon v-else color="#f56c6c"><CircleCloseFilled /></el-icon>
          </template>
        </el-table-column>
        <el-table-column label="观察者" width="100" align="center">
          <template #default="{ row }">
            <el-icon v-if="row.viewer" color="#67c23a"><CircleCheckFilled /></el-icon>
            <el-icon v-else color="#f56c6c"><CircleCloseFilled /></el-icon>
          </template>
        </el-table-column>
        <el-table-column prop="desc" label="说明" />
      </el-table>
    </div>

    <div class="eh-card">
      <div class="eh-card-header">
        <span class="eh-card-title"><el-icon><Lock /></el-icon> 用户管理</span>
        <el-button type="primary" :icon="Plus" @click="openCreateDialog">创建用户</el-button>
      </div>
      <el-table :data="users" v-loading="loading" stripe>
        <el-table-column prop="username" label="用户名" min-width="140" />
        <el-table-column label="角色" width="120">
          <template #default="{ row }">
            <el-tag :type="roleType(row.role)" size="small" effect="dark">{{ roleLabel(row.role) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="创建时间" width="180">
          <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
        </el-table-column>
        <el-table-column label="最后登录" width="180">
          <template #default="{ row }">{{ row.last_login ? formatTime(row.last_login) : '从未登录' }}</template>
        </el-table-column>
        <el-table-column label="操作" width="200" align="center">
          <template #default="{ row }">
            <el-button size="small" type="primary" :icon="Key" @click="openResetDialog(row.username)">重置密码</el-button>
            <el-button v-if="row.username !== 'admin'" size="small" type="danger" :icon="Delete" @click="deleteUser(row.username)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
      <div v-if="!loading && users.length === 0" class="eh-empty">
        <el-icon><Lock /></el-icon>
        <p>暂无用户</p>
      </div>
    </div>

    <el-dialog v-model="showCreateDialog" title="创建用户" width="460px">
      <el-form :model="createForm" :rules="createRules" ref="createFormRef" label-width="80px">
        <el-form-item label="用户名" prop="username">
          <el-input v-model="createForm.username" placeholder="输入用户名（3-32个字符）" />
        </el-form-item>
        <el-form-item label="密码" prop="password">
          <el-input v-model="createForm.password" type="password" placeholder="输入密码（至少6位）" show-password />
        </el-form-item>
        <el-form-item label="角色" prop="role">
          <el-select v-model="createForm.role" style="width: 100%">
            <el-option label="管理员（全部权限）" value="admin" />
            <el-option label="操作员（操作类权限）" value="operator" />
            <el-option label="观察者（只读权限）" value="viewer" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showCreateDialog = false">取消</el-button>
        <el-button type="primary" @click="doCreate" :loading="createLoading">创建</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="showResetDialog" title="重置用户密码" width="460px">
      <el-form :model="resetForm" :rules="resetRules" ref="resetFormRef" label-width="80px">
        <el-form-item label="用户名">
          <el-input v-model="resetForm.username" disabled />
        </el-form-item>
        <el-form-item label="新密码" prop="password">
          <el-input v-model="resetForm.password" type="password" placeholder="输入新密码（至少6位）" show-password />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showResetDialog = false">取消</el-button>
        <el-button type="primary" @click="doReset" :loading="resetLoading">重置</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Lock, Plus, Key, Delete, CircleCheckFilled, CircleCloseFilled } from '@element-plus/icons-vue'
import dayjs from 'dayjs'
import api from '@/api'

const users = ref([])
const loading = ref(false)
const showCreateDialog = ref(false)
const createLoading = ref(false)
const createFormRef = ref()
const createForm = reactive({ username: '', password: '', role: 'viewer' })
const showResetDialog = ref(false)
const resetLoading = ref(false)
const resetFormRef = ref()
const resetForm = reactive({ username: '', password: '' })
const resetRules = {
  password: [
    { required: true, message: '请输入新密码', trigger: 'blur' },
    { min: 6, message: '密码至少 6 位', trigger: 'blur' },
  ],
}
const createRules = {
  username: [
    { required: true, message: '请输入用户名', trigger: 'blur' },
    { min: 3, max: 32, message: '用户名长度 3-32 个字符', trigger: 'blur' },
  ],
  password: [
    { required: true, message: '请输入密码', trigger: 'blur' },
    { min: 6, message: '密码至少 6 位', trigger: 'blur' },
  ],
  role: [{ required: true, message: '请选择角色', trigger: 'change' }],
}

const permissionMatrix = [
  { module: '模型管理', admin: true, operator: true, viewer: false, desc: '加载、切换、卸载、回滚模型' },
  { module: '工作流触发', admin: true, operator: true, viewer: false, desc: '手动触发工作流执行' },
  { module: 'OTA更新', admin: true, operator: true, viewer: false, desc: '检查更新、应用更新' },
  { module: '知识库', admin: true, operator: true, viewer: false, desc: '上传文档、检索知识' },
  { module: '告警确认', admin: true, operator: true, viewer: false, desc: '确认和处理告警' },
  { module: '用户管理', admin: true, operator: false, viewer: false, desc: '创建、删除用户' },
  { module: '系统监控', admin: true, operator: true, viewer: true, desc: '查看系统健康和审计日志' },
  { module: '数据查看', admin: true, operator: true, viewer: true, desc: '查看设备、告警、智能体等' },
]

const formatTime = (t) => t ? dayjs(t).format('YYYY-MM-DD HH:mm:ss') : '-'
const roleLabel = (r) => ({ admin: '管理员', operator: '操作员', viewer: '观察者' })[r] || r
const roleType = (r) => ({ admin: 'danger', operator: 'warning', viewer: 'info' })[r] || 'info'

const loadUsers = async () => {
  loading.value = true
  try { const data = await api.get('/security/users'); users.value = data.users || [] } catch {} finally { loading.value = false }
}

const openCreateDialog = () => {
  createForm.username = ''
  createForm.password = ''
  createForm.role = 'viewer'
  showCreateDialog.value = true
}

const doCreate = async () => {
  if (!createFormRef.value) return
  await createFormRef.value.validate(async (valid) => {
    if (!valid) return
    createLoading.value = true
    try {
      await api.post('/security/users', {
        username: createForm.username,
        password: createForm.password,
        role: createForm.role,
      })
      ElMessage.success('用户创建成功')
      showCreateDialog.value = false
      loadUsers()
    } catch {} finally { createLoading.value = false }
  })
}

const deleteUser = async (username) => {
  try {
    await ElMessageBox.confirm(`确定删除用户 ${username}？此操作不可恢复。`, '删除确认', { type: 'warning' })
    await api.delete(`/security/users/${username}`)
    ElMessage.success('用户已删除')
    loadUsers()
  } catch {}
}

const openResetDialog = (username) => {
  resetForm.username = username
  resetForm.password = ''
  showResetDialog.value = true
}

const doReset = async () => {
  if (!resetFormRef.value) return
  await resetFormRef.value.validate(async (valid) => {
    if (!valid) return
    resetLoading.value = true
    try {
      await api.post('/security/users/' + resetForm.username + '/reset', { password: resetForm.password })
      ElMessage.success('密码已重置')
      showResetDialog.value = false
    } catch {} finally { resetLoading.value = false }
  })
}

onMounted(loadUsers)
</script>
