<template>
  <div class="eh-page">
    <el-alert type="info" :closable="false" show-icon style="margin-bottom: 16px">
      <template #title>告警中心</template>
      <template #default>
        查看和管理系统产生的所有告警。告警由工作流或智能体自动生成，支持按严重级别筛选。
        点击「确认」可处理活跃告警，已确认的告警将降低优先级。
      </template>
    </el-alert>

    <div class="eh-card">
      <div class="eh-card-header">
        <span class="eh-card-title"><el-icon><BellFilled /></el-icon> 告警列表</span>
        <el-select v-model="filterStatus" placeholder="状态筛选" style="width: 140px" @change="loadAlerts">
          <el-option label="全部" value="" />
          <el-option label="活跃" value="active" />
          <el-option label="已确认" value="acked" />
          <el-option label="已解决" value="resolved" />
        </el-select>
      </div>
      <div v-loading="loading">
        <div v-if="alerts.length === 0 && !loading" class="eh-empty">
          <el-icon><BellFilled /></el-icon>
          <p>暂无告警</p>
          <p style="font-size: 13px; color: #909399">系统运行正常，无活跃告警</p>
        </div>
        <div v-for="alert in alerts" :key="alert.id" class="alert-card" :class="{ acked: alert.status === 'acked' }">
          <div class="alert-card-header">
            <div class="alert-card-left">
              <el-tag :type="severityType(alert.severity)" size="small" effect="dark">{{ severityLabel(alert.severity) }}</el-tag>
              <span class="alert-device"><el-icon><Cpu /></el-icon> {{ alert.device_id }}</span>
              <el-tag v-if="alert.anchored" size="small" type="success" effect="plain">RAG锚定</el-tag>
            </div>
            <el-button v-if="alert.status === 'active'" size="small" type="primary" :icon="Check" @click="ackAlert(alert.id)">确认</el-button>
          </div>
          <div class="alert-message">{{ alert.message }}</div>
          <div class="alert-meta">
            <el-icon><Clock /></el-icon> {{ formatTime(alert.created_at) }}
            <span v-if="alert.type" style="margin-left: 12px">类型: {{ alertTypeLabel(alert.type) }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { BellFilled, Cpu, Check, Clock } from '@element-plus/icons-vue'
import dayjs from 'dayjs'
import api from '@/api'

const alerts = ref([])
const loading = ref(false)
const filterStatus = ref('active')

const severityType = (sev) => {
  if (sev === 'CRITICAL') return 'danger'
  if (sev === 'HIGH') return 'danger'
  if (sev === 'MEDIUM') return 'warning'
  return 'info'
}
const severityLabel = (sev) => ({ CRITICAL: '严重', HIGH: '高', MEDIUM: '中', LOW: '低', INFO: '信息' })[sev] || sev
const alertTypeLabel = (t) => ({
  anomaly: '异常检测', vibration: '振动告警', temperature: '温度告警',
  power: '功率告警', energy: '能源告警', fallback: '回退告警', vision: '视觉确认',
})[t] || t

const formatTime = (t) => t ? dayjs(t).format('YYYY-MM-DD HH:mm:ss') : ''

const loadAlerts = async () => {
  loading.value = true
  try {
    const params = { limit: 50 }
    if (filterStatus.value) params.status = filterStatus.value
    const data = await api.get('/alerts', { params })
    alerts.value = data.alerts || []
  } catch {} finally { loading.value = false }
}

const ackAlert = async (id) => {
  try {
    await api.put(`/alerts/${id}/ack`)
    ElMessage.success('告警已确认')
    loadAlerts()
  } catch {}
}

onMounted(loadAlerts)
</script>

<style scoped>
.alert-card {
  background: #f9fafb;
  border-radius: 8px;
  padding: 14px 16px;
  margin-bottom: 12px;
  border-left: 3px solid var(--eh-danger);
  transition: opacity 0.3s;
}
.alert-card.acked { opacity: 0.6; border-left-color: var(--eh-success); }

.alert-card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
}

.alert-card-left {
  display: flex;
  align-items: center;
  gap: 8px;
}

.alert-device {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 14px;
  font-weight: 600;
}

.alert-message {
  font-size: 14px;
  color: var(--eh-text);
  margin-bottom: 6px;
  line-height: 1.5;
}

.alert-meta {
  font-size: 12px;
  color: var(--eh-text-secondary);
  display: flex;
  align-items: center;
}
</style>
