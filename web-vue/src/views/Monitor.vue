<template>
  <div class="eh-page">
    <el-alert type="info" :closable="false" show-icon style="margin-bottom: 16px">
      <template #title>系统监控</template>
      <template #default>
        实时查看系统健康状态、关键运行指标和审计日志。支持导出系统日志和创建数据备份。
      </template>
    </el-alert>

    <el-row :gutter="20">
      <el-col :xs="24" :lg="12">
        <div class="eh-card">
          <div class="eh-card-header"><span class="eh-card-title"><el-icon><Monitor /></el-icon> 系统健康</span></div>
          <div class="health-grid">
            <div class="health-item" v-for="comp in healthComponents" :key="comp.name">
              <span class="health-name">{{ comp.name }}</span>
              <el-tag :type="comp.type" size="small" effect="plain">{{ comp.value }}</el-tag>
            </div>
          </div>
        </div>
      </el-col>
      <el-col :xs="24" :lg="12">
        <div class="eh-card">
          <div class="eh-card-header">
            <span class="eh-card-title"><el-icon><DataAnalysis /></el-icon> 关键指标</span>
            <div style="display: flex; align-items: center; gap: 12px">
              <span style="font-size: 13px; color: #909399">离线模式</span>
              <el-switch v-model="offlineMode" @change="toggleOfflineMode" :loading="offlineLoading" />
            </div>
          </div>
          <div class="metrics-grid">
            <div class="metric-item">
              <div class="metric-label">消息接收总量</div>
              <div class="metric-value">{{ metrics.messages_in_total || 0 }}</div>
            </div>
            <div class="metric-item">
              <div class="metric-label">消息发送总量</div>
              <div class="metric-value">{{ metrics.messages_out_total || 0 }}</div>
            </div>
            <div class="metric-item">
              <div class="metric-label">在线智能体</div>
              <div class="metric-value">{{ metrics.agents_online || 0 }}</div>
            </div>
            <div class="metric-item">
              <div class="metric-label">活跃工作流</div>
              <div class="metric-value">{{ metrics.active_workflows || 0 }}</div>
            </div>
            <div class="metric-item">
              <div class="metric-label">离线队列</div>
              <div class="metric-value">{{ metrics.offline_queue || 0 }}</div>
            </div>
            <div class="metric-item">
              <div class="metric-label">离线模式</div>
              <div class="metric-value">{{ metrics.offline_mode ? '是' : '否' }}</div>
            </div>
          </div>
        </div>
      </el-col>
    </el-row>

    <div class="eh-card">
      <div class="eh-card-header">
        <span class="eh-card-title"><el-icon><Document /></el-icon> 审计日志</span>
        <div style="display: flex; gap: 8px">
          <el-button :icon="Download" @click="exportLogs">导出日志</el-button>
          <el-button :icon="FolderOpened" type="primary" @click="backupSystem" :loading="backingUp">系统备份</el-button>
          <el-button :icon="Refresh" @click="loadAuditLogs" :loading="auditLoading">刷新</el-button>
        </div>
      </div>
      <el-table :data="auditLogs" v-loading="auditLoading" stripe size="small">
        <el-table-column label="时间" width="180">
          <template #default="{ row }">{{ formatTime(row.timestamp) }}</template>
        </el-table-column>
        <el-table-column prop="actor" label="操作者" width="120" />
        <el-table-column label="操作" width="140">
          <template #default="{ row }"><el-tag size="small" type="info">{{ auditActionLabel(row.action) }}</el-tag></template>
        </el-table-column>
        <el-table-column label="资源" min-width="160" show-overflow-tooltip>
          <template #default="{ row }">{{ row.resource && row.resource !== '<nil>' ? row.resource : '-' }}</template>
        </el-table-column>
        <el-table-column prop="ip" label="IP地址" width="140" />
      </el-table>
      <div v-if="!auditLoading && auditLogs.length === 0" class="eh-empty">
        <el-icon><Document /></el-icon>
        <p>暂无审计日志</p>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, computed, onMounted, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Monitor, DataAnalysis, Document, Refresh, Download, FolderOpened } from '@element-plus/icons-vue'
import dayjs from 'dayjs'
import api from '@/api'

const health = reactive({})
const metrics = reactive({})
const auditLogs = ref([])
const auditLoading = ref(false)
const backingUp = ref(false)

const formatTime = (t) => t ? dayjs(t).format('YYYY-MM-DD HH:mm:ss') : '-'
const compLabel = (v) => ({ available: '可用', unavailable: '不可用', connected: '已连接', degraded: '降级可用' })[v] || '未知'
const compType = (v) => (v === 'available' || v === 'connected') ? 'success' : (v === 'degraded' ? 'warning' : (v === 'unavailable' ? 'danger' : 'info'))
const auditActionLabel = (a) => ({
  login: '登录', logout: '登出', 'user:create': '创建用户', 'model:load': '模型加载',
  'model:unload': '模型卸载', 'model:switch': '模型切换', 'workflow:trigger': '工作流触发',
  'ota:check': 'OTA检查', 'ota:apply': 'OTA应用', 'alert:ack': '告警确认',
  'agent:register': '智能体注册', 'knowledge:upload': '知识上传',
  'model:rollback': '模型回滚', backup: '系统备份', restore: '系统恢复',
})[a] || a

const healthComponents = computed(() => {
  const comps = health.components || {}
  return [
    { name: 'NATS', value: health.nats_connected ? '已连接' : '未连接', type: health.nats_connected ? 'success' : 'danger' },
    { name: 'MQTT', value: health.mqtt_connected ? '已连接' : '未连接', type: health.mqtt_connected ? 'success' : 'danger' },
    { name: 'ONNX 推理引擎', value: compLabel(comps.onnx), type: compType(comps.onnx) },
    { name: 'LLM 大模型', value: compLabel(comps.llm), type: compType(comps.llm) },
    { name: 'RAG 检索', value: compLabel(comps.rag), type: compType(comps.rag) },
    { name: '离线模式', value: health.offline_mode ? '是' : '否', type: health.offline_mode ? 'warning' : 'info' },
  ]
})

const offlineMode = ref(false)
const offlineLoading = ref(false)

const toggleOfflineMode = async (val) => {
  offlineLoading.value = true
  try {
    const data = await api.post('/system/offline-mode', { offline: val })
    ElMessage.success(data.message)
    loadHealth()
    loadMetrics()
  } catch {
    offlineMode.value = !val // revert
  } finally { offlineLoading.value = false }
}

const loadHealth = async () => { try { Object.assign(health, await api.get('/system/health')) } catch {} }
const loadMetrics = async () => { try { Object.assign(metrics, await api.get('/system/metrics')) } catch {} }

const loadAuditLogs = async () => {
  auditLoading.value = true
  try { const data = await api.get('/audit/logs'); auditLogs.value = data.logs || data || [] } catch {} finally { auditLoading.value = false }
}

const exportLogs = () => {
  const link = document.createElement('a')
  link.href = '/api/v1/system/logs/export'
  link.download = 'edgeagent-logs.txt'
  link.click()
  ElMessage.success('日志导出已开始')
}

const backupSystem = async () => {
  backingUp.value = true
  try {
    const data = await api.post('/system/backup')
    ElMessage.success(`备份已创建: ${data.path}`)
  } catch {} finally { backingUp.value = false }
}

onMounted(() => { loadHealth(); loadMetrics(); loadAuditLogs() })

// 同步离线模式状态
watch(() => health.offline_mode, (val) => { offlineMode.value = val })
</script>

<style scoped>
.health-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(160px, 1fr)); gap: 12px; }
.health-item { display: flex; align-items: center; justify-content: space-between; padding: 10px 14px; background: #f9fafb; border-radius: 8px; }
.health-name { font-size: 13px; font-weight: 500; }
.metrics-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(140px, 1fr)); gap: 12px; }
.metric-item { background: #f9fafb; border-radius: 8px; padding: 14px; text-align: center; }
.metric-label { font-size: 12px; color: var(--eh-text-secondary); margin-bottom: 6px; }
.metric-value { font-size: 24px; font-weight: 700; color: var(--eh-primary); }
</style>
