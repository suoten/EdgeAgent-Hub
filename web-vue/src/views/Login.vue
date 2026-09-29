<template>
  <div class="login-container">
    <div class="login-bg">
      <div class="login-bg-circle c1"></div>
      <div class="login-bg-circle c2"></div>
      <div class="login-bg-circle c3"></div>
    </div>
    <div class="login-box">
      <div class="login-header">
        <el-icon class="login-logo"><Lightning /></el-icon>
        <h1>EdgeAgent Hub</h1>
        <p>边缘智能体管理平台</p>
      </div>
      <el-form ref="formRef" :model="form" :rules="rules" @submit.prevent="handleLogin">
        <el-form-item prop="username">
          <el-input
            v-model="form.username"
            size="large"
            placeholder="用户名"
            :prefix-icon="User"
            clearable
          />
        </el-form-item>
        <el-form-item prop="password">
          <el-input
            v-model="form.password"
            size="large"
            type="password"
            placeholder="密码"
            :prefix-icon="Lock"
            show-password
            @keyup.enter="handleLogin"
          />
        </el-form-item>
        <el-form-item>
          <el-button
            type="primary"
            size="large"
            :loading="loading"
            style="width: 100%"
            @click="handleLogin"
          >
            <el-icon v-if="!loading"><Right /></el-icon>
            登 录
          </el-button>
        </el-form-item>
      </el-form>
      <div class="login-footer">
        <span>首次使用请向系统管理员获取账号</span>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { User, Lock, Right, Lightning } from '@element-plus/icons-vue'
import api from '@/api'

const router = useRouter()
const formRef = ref()
const loading = ref(false)

const form = reactive({
  username: '',
  password: '',
})

const rules = {
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }],
}

const handleLogin = async () => {
  if (!formRef.value) return
  await formRef.value.validate(async (valid) => {
    if (!valid) return
    loading.value = true
    try {
      const data = await api.post('/auth/login', {
        username: form.username,
        password: form.password,
      })
      localStorage.setItem('edgeagent_token', data.access_token)
      localStorage.setItem('edgeagent_refresh_token', data.refresh_token)
      localStorage.setItem('edgeagent_user', form.username)
      localStorage.setItem('edgeagent_role', data.role || 'admin')
      ElMessage.success('登录成功')
      router.push('/dashboard')
    } catch (e) {
      // 错误已在拦截器处理
    } finally {
      loading.value = false
    }
  })
}
</script>

<style scoped>
.login-container {
  height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #0c1929 0%, #1a2a4a 50%, #0d1b2a 100%);
  position: relative;
  overflow: hidden;
}

.login-bg {
  position: absolute;
  inset: 0;
  pointer-events: none;
}

.login-bg-circle {
  position: absolute;
  border-radius: 50%;
  filter: blur(80px);
  opacity: 0.15;
}
.c1 { width: 400px; height: 400px; background: #1890ff; top: -100px; left: -100px; }
.c2 { width: 300px; height: 300px; background: #52c41a; bottom: -50px; right: 10%; }
.c3 { width: 250px; height: 250px; background: #722ed1; top: 40%; right: -50px; }

.login-box {
  width: 420px;
  background: rgba(255, 255, 255, 0.98);
  border-radius: 16px;
  padding: 48px 40px 32px;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.3);
  backdrop-filter: blur(20px);
  z-index: 1;
}

.login-header {
  text-align: center;
  margin-bottom: 32px;
}

.login-logo {
  font-size: 48px;
  color: #1890ff;
  margin-bottom: 12px;
}

.login-header h1 {
  font-size: 26px;
  font-weight: 700;
  color: #1a1a2e;
  margin-bottom: 6px;
}

.login-header p {
  font-size: 14px;
  color: #909399;
}

.login-footer {
  text-align: center;
  margin-top: 24px;
  font-size: 12px;
  color: #c0c4cc;
}
</style>
