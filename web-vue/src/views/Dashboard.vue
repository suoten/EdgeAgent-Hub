<template>
  <div class="eh-page">
    <!-- 新手引导（仅首次显示） -->
    <el-alert
      v-if="showGuide"
      title="欢迎使用 EdgeAgent Hub 边缘智能中枢"
      type="success"
      :closable="true"
      show-icon
      @close="dismissGuide"
      style="margin-bottom: 16px"
    >
      <template #default>
        <div style="line-height: 1.8">
          系统已预置示例模型、智能体和工作流，可开箱即用。建议按以下步骤快速上手：
          <el-button text type="primary" @click="$router.push('/models')">① 查看模型</el-button>
          →
          <el-button text type="primary" @click="$router.push('/agents')">② 检查智能体</el-button>
          →
          <el-button text type="primary" @click="$router.push('/workflows')">③ 触发工作流</el-button>
          →
          <el-button text type="primary" @click="$router.push('/chat')">④ 智能对话</el-button>
        </div>
      </template>
    </el-alert>

    <!-- 统计卡片 -->
    <el-row :gutter="20" class="stat-row">
      <el-col :xs="12" :sm="6" v-for="card in statCards" :key="card.label">
        <div class="stat-card" :style="{ '--card-color': card.color }" @click="$router.push(card.link)">
          <div class="stat-icon-wrap" :style="{ background: card.bg }">
            <el-icon :size="28" :color="card.color"><component :is="card.icon" /></el-icon>
          </div>
          <div class="stat-body">
            <div class="stat-value">{{ card.value }}</div>
            <div class="stat-label">{{ card.label }}</div>
          </div>
        </div>
      </el-col>
    </el-row>

    <!-- 核心价值 -->
    <div class="eh-card" style="margin-bottom: 20px">
      <div class="eh-card-header">
        <span class="eh-card-title"><el-icon><Aim /></el-icon> EdgeAgent Hub 能为你的网关做什么</span>
      </div>
      <div class="value-grid">
        <div class="value-item">
          <div class="value-icon" style="background: #e8f5e9"><el-icon :size="24" color="#67c23a"><Cpu /></el-icon></div>
          <div class="value-text">
            <div class="value-title">协议接入不走云</div>
            <div class="value-desc">Modbus / OPC UA / MQTT / BLE / ONVIF 数据在边缘侧直接采集处理，毫秒级响应，断网不断服务。</div>
          </div>
        </div>
        <div class="value-item">
          <div class="value-icon" style="background: #e3f2fd"><el-icon :size="24" color="#1890ff"><Box /></el-icon></div>
          <div class="value-text">
            <div class="value-title">边缘 AI 推理</div>
            <div class="value-desc">振动、温度、电流、功率、压力数据实时 ONNX 推理，异常检测秒级告警，不需上传云端。</div>
          </div>
        </div>
        <div class="value-item">
          <div class="value-icon" style="background: #fff3e0"><el-icon :size="24" color="#e6a23c"><ChatDotRound /></el-icon></div>
          <div class="value-text">
            <div class="value-title">RAG 智能对话 + 告警生成</div>
            <div class="value-desc">接入 Qwen/DeepSeek/GLM 等大模型，结合知识库生成可验证的告警描述和处置建议。</div>
          </div>
        </div>
        <div class="value-item">
          <div class="value-icon" style="background: #fce4ec"><el-icon :size="24" color="#f56c6c"><BellFilled /></el-icon></div>
          <div class="value-text">
            <div class="value-title">工作流自动编排</div>
            <div class="value-desc">传感器异常 → ONNX 推理 → RAG 分析 → 钉钉/短信/Webhook 通知，全链路自动执行。</div>
          </div>
        </div>
      </div>
    </div>

    <!-- 快速操作 -->
    <div class="quick-actions">
      <div class="quick-action-title">
        <el-icon><Operation /></el-icon> 快速操作
      </div>
      <div class="quick-action-buttons">
        <el-button @click="$router.push('/chat')" :icon="ChatDotRound">智能对话</el-button>
        <el-button @click="$router.push('/workflows')" :icon="VideoPlay">触发工作流</el-button>
        <el-button @click="$router.push('/models')" :icon="Box">管理模型</el-button>
        <el-button @click="$router.push('/agents')" :icon="Connection">注册智能体</el-button>
        <el-button @click="$router.push('/knowledge')" :icon="Collection">知识库</el-button>
        <el-button @click="$router.push('/monitor')" :icon="Monitor">系统监控</el-button>
      </div>
    </div>

    <el-row :gutter="20">
      <!-- 系统健康 -->
      <el-col :xs="24" :lg="12">
        <div class="eh-card">
          <div class="eh-card-header">
            <span class="eh-card-title">
              <el-icon><Monitor /></el-icon> 系统健康状态
            </span>
            <el-tag :type="health.status === 'healthy' ? 'success' : 'warning'" effect="dark" size="small">
              {{ health.status === 'healthy' ? '健康' : (health.status === 'degraded' ? '降级' : (health.status === 'unhealthy' ? '异常' : health.status || '...')) }}
            </el-tag>
          </div>
          <div class="health-grid">
            <div class="health-item" v-for="comp in healthComponents" :key="comp.name">
              <span class="health-name">{{ comp.name }}</span>
              <el-tag :type="comp.type" size="small" effect="plain">{{ comp.value }}</el-tag>
            </div>
          </div>
        </div>
      </el-col>

      <!-- 消息吞吐量图表 -->
      <el-col :xs="24" :lg="12">
        <div class="eh-card">
          <div class="eh-card-header">
            <span class="eh-card-title">
              <el-icon><DataLine /></el-icon> 消息吞吐量
            </span>
            <el-tooltip content="展示最近12分钟内的MQTT消息接收与发送量" placement="top">
              <el-icon style="color: #909399; cursor: help"><InfoFilled /></el-icon>
            </el-tooltip>
          </div>
          <v-chart class="chart" :option="chartOption" autoresize style="height: 220px" />
        </div>
      </el-col>
    </el-row>

    <el-row :gutter="20">
      <!-- 最近告警 -->
      <el-col :xs="24" :lg="12">
        <div class="eh-card">
          <div class="eh-card-header">
            <span class="eh-card-title">
              <el-icon><BellFilled /></el-icon> 最近告警
            </span>
            <el-button text size="small" @click="$router.push('/alerts')">查看全部</el-button>
          </div>
          <div v-if="recentAlerts.length === 0" class="eh-empty">
            <el-icon><CircleCheckFilled /></el-icon>
            <p>暂无活跃告警，系统运行正常</p>
          </div>
          <div v-else class="alert-list">
            <div v-for="alert in recentAlerts" :key="alert.id" class="alert-item">
              <div class="alert-left">
                <el-tag :type="severityType(alert.severity)" size="small" effect="dark">
                  {{ severityLabel(alert.severity) }}
                </el-tag>
                <span class="alert-device">{{ alert.device_id }}</span>
              </div>
              <div class="alert-message">{{ alert.message }}</div>
              <div class="alert-time">{{ formatTime(alert.created_at) }}</div>
            </div>
          </div>
        </div>
      </el-col>

      <!-- 智能体状态 -->
      <el-col :xs="24" :lg="12">
        <div class="eh-card">
          <div class="eh-card-header">
            <span class="eh-card-title">
              <el-icon><Connection /></el-icon> 智能体状态
            </span>
            <el-button text size="small" @click="$router.push('/agents')">管理</el-button>
          </div>
          <div v-if="agents.length === 0" class="eh-empty">
            <el-icon><Connection /></el-icon>
            <p>暂无智能体</p>
          </div>
          <div v-else class="agent-list">
            <div v-for="agent in agents" :key="agent.id" class="agent-item">
              <div class="agent-left">
                <span class="eh-status-dot" :class="agent.status === 'online' ? 'online' : (agent.status === 'degraded' ? 'degraded' : 'offline')"></span>
                <span class="agent-id">{{ agent.id }}</span>
                <el-tag size="small" :type="agent.status === 'online' ? 'success' : (agent.status === 'degraded' ? 'warning' : 'danger')" effect="plain">{{ { online: '在线', offline: '离线', degraded: '降级' }[agent.status] || agent.status }}</el-tag>
              </div>
              <div class="agent-tags">
                <el-tag v-for="cap in (agent.capabilities || []).slice(0, 3)" :key="cap" size="small" type="info" effect="plain">
                  {{ cap }}
                </el-tag>
              </div>
            </div>
          </div>
        </div>
      </el-col>
    </el-row>
  </div>
</template>

<script setup>
import { ref, reactive, computed, onMounted, onUnmounted } from 'vue'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, LegendComponent } from 'echarts/components'
import VChart from 'vue-echarts'
import dayjs from 'dayjs'
import api from '@/api'
import {
  Monitor, DataLine, BellFilled, Connection, CircleCheckFilled, InfoFilled,
  Operation, ChatDotRound, VideoPlay, Box, Cpu, Collection, Aim,
} from '@element-plus/icons-vue'

use([CanvasRenderer, LineChart, GridComponent, TooltipComponent, LegendComponent])

const health = reactive({})
const recentAlerts = ref([])
const agents = ref([])
const models = ref([])
const metrics = reactive({})
const showGuide = ref(!localStorage.getItem('guide_dismissed'))
let refreshTimer = null

const dismissGuide = () => {
  localStorage.setItem('guide_dismissed', '1')
  showGuide.value = false
}

const statCards = computed(() => [
  { label: '智能体总数', value: agents.value.length, icon: Cpu, color: '#1890ff', bg: '#e6f7ff', link: '/agents' },
  { label: '活跃告警', value: recentAlerts.value.length, icon: BellFilled, color: '#f56c6c', bg: '#fef0f0', link: '/alerts' },
  { label: '已加载模型', value: models.value.filter(m => m.active).length, icon: Box, color: '#67c23a', bg: '#f0f9eb', link: '/models' },
  { label: '在线智能体', value: agents.value.filter(a => a.status === 'online').length, icon: Connection, color: '#722ed1', bg: '#f9f0ff', link: '/agents' },
])

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

const chartOption = computed(() => {
  const inData = []
  const outData = []
  const labels = []
  const histIn = metrics.message_history_in || []
  const histOut = metrics.message_history_out || []
  for (let i = 11; i >= 0; i--) {
    labels.push(dayjs().subtract(i, 'minute').format('HH:mm'))
    inData.push(histIn[11 - i] !== undefined ? histIn[11 - i] : 0)
    outData.push(histOut[11 - i] !== undefined ? histOut[11 - i] : 0)
  }
  return {
    tooltip: { trigger: 'axis' },
    legend: { data: ['接收', '发送'], bottom: 0 },
    grid: { left: '3%', right: '4%', bottom: '10%', top: '5%', containLabel: true },
    xAxis: { type: 'category', data: labels, boundaryGap: false },
    yAxis: { type: 'value' },
    series: [
      { name: '接收', type: 'line', smooth: true, data: inData, areaStyle: { opacity: 0.15 }, itemStyle: { color: '#1890ff' } },
      { name: '发送', type: 'line', smooth: true, data: outData, areaStyle: { opacity: 0.15 }, itemStyle: { color: '#67c23a' } },
    ],
  }
})

const severityType = (sev) => {
  if (sev === 'CRITICAL' || sev === 'HIGH') return 'danger'
  if (sev === 'MEDIUM') return 'warning'
  return 'info'
}

const formatTime = (t) => t ? dayjs(t).format('MM-DD HH:mm:ss') : ''
const compLabel = (v) => ({ available: '可用', unavailable: '不可用', connected: '已连接', degraded: '降级可用' })[v] || '未知'
const compType = (v) => (v === 'available' || v === 'connected') ? 'success' : (v === 'degraded' ? 'warning' : (v === 'unavailable' ? 'danger' : 'info'))
const severityLabel = (sev) => ({ CRITICAL: '严重', HIGH: '高', MEDIUM: '中', LOW: '低', INFO: '信息' })[sev] || sev

const loadAll = async () => {
  try { Object.assign(health, await api.get('/system/health')) } catch {}
  try {
    const data = await api.get('/alerts', { params: { status: 'active', limit: 5 } })
    recentAlerts.value = data.alerts || []
  } catch {}
  try {
    const data = await api.get('/agents')
    agents.value = data.agents || []
  } catch {}
  try {
    const data = await api.get('/models')
    models.value = data.models || []
  } catch {}
  try {
    Object.assign(metrics, await api.get('/system/metrics'))
  } catch {}
}

onMounted(() => {
  loadAll()
  refreshTimer = setInterval(loadAll, 15000)
})

onUnmounted(() => {
  if (refreshTimer) clearInterval(refreshTimer)
})
</script>

<style scoped>
.stat-row { margin-bottom: 4px; }

.stat-card {
  background: #fff;
  border-radius: var(--eh-radius);
  box-shadow: var(--eh-shadow);
  padding: 20px;
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 20px;
  transition: transform 0.2s, box-shadow 0.2s;
  cursor: pointer;
}
.stat-card:hover {
  transform: translateY(-2px);
  box-shadow: var(--eh-shadow-lg);
}

.stat-icon-wrap {
  width: 56px;
  height: 56px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.stat-value {
  font-size: 28px;
  font-weight: 700;
  color: var(--card-color);
  line-height: 1.2;
}

.stat-label {
  font-size: 13px;
  color: var(--eh-text-secondary);
  margin-top: 4px;
}

.quick-actions {
  background: #fff;
  border-radius: var(--eh-radius);
  box-shadow: var(--eh-shadow);
  padding: 16px 20px;
  margin-bottom: 20px;
}
.quick-action-title {
  font-size: 14px;
  font-weight: 600;
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 12px;
  color: var(--eh-text);
}
.quick-action-buttons {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.health-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
  gap: 12px;
}

.health-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 14px;
  background: #f9fafb;
  border-radius: 8px;
}

.health-name {
  font-size: 13px;
  color: var(--eh-text);
  font-weight: 500;
}

.alert-list, .agent-list {
  max-height: 300px;
  overflow-y: auto;
}

.alert-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 0;
  border-bottom: 1px solid #f0f0f0;
}
.alert-item:last-child { border-bottom: none; }

.alert-left {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 140px;
}

.alert-device {
  font-size: 13px;
  font-weight: 500;
  color: var(--eh-text);
}

.alert-message {
  flex: 1;
  font-size: 13px;
  color: var(--eh-text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.alert-time {
  font-size: 12px;
  color: #c0c4cc;
  white-space: nowrap;
}

.agent-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 0;
  border-bottom: 1px solid #f0f0f0;
}
.agent-item:last-child { border-bottom: none; }

.agent-left {
  display: flex;
  align-items: center;
  gap: 8px;
}

.agent-id {
  font-size: 13px;
  font-weight: 500;
}

.agent-tags {
  display: flex;
  gap: 4px;
}

.chart { width: 100%; }

.value-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 16px;
  padding: 16px 0;
}
.value-item {
  display: flex;
  gap: 12px;
  align-items: flex-start;
}
.value-icon {
  width: 44px;
  height: 44px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}
.value-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--eh-text);
  margin-bottom: 4px;
}
.value-desc {
  font-size: 13px;
  color: var(--eh-text-secondary);
  line-height: 1.5;
}
</style>
