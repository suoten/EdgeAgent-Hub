import axios from 'axios'
import { ElMessage } from 'element-plus'
import router from '@/router'

const api = axios.create({
  baseURL: '/api/v1',
  timeout: 30000,
  headers: { 'Content-Type': 'application/json' },
})

// 请求拦截器
api.interceptors.request.use((config) => {
  const token = localStorage.getItem('edgeagent_token')
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

// 响应拦截器
api.interceptors.response.use(
  (response) => response.data,
  (error) => {
    if (error.response) {
      const { status, data } = error.response
      if (status === 401) {
        localStorage.removeItem('edgeagent_token')
        localStorage.removeItem('edgeagent_user')
        localStorage.removeItem('edgeagent_role')
        router.push('/login')
        ElMessage.error('登录已过期，请重新登录')
      } else if (data && data.error) {
        ElMessage.error(data.error)
      } else {
        ElMessage.error(`请求失败 (${status})`)
      }
    } else if (error.code === 'ECONNABORTED') {
      ElMessage.error('请求超时，请检查网络')
    } else {
      ElMessage.error('网络异常')
    }
    return Promise.reject(error)
  }
)

export default api
