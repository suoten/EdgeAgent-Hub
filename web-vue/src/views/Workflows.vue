<template>
  <div class="eh-page">
    <el-alert type="info" :closable="false" show-icon style="margin-bottom: 16px">
      <template #title>编排管理</template>
      <template #default>
        工作流编排引擎支持 MQTT 事件自动触发和手动触发。点击「触发」可手动执行工作流，
        执行结果可在下方执行历史中查看。工作流定义文件位于 workflows/ 目录。
      </template>
    </el-alert>

    <div class="eh-card">
      <div class="eh-card-header">
        <span class="eh-card-title"><el-icon><SetUp /></el-icon> 工作流列表</span>
        <el-button :icon="Refresh" @click="loadWorkflows" :loading="loading">刷新</el-button>
      </div>
      <el-table :data="workflows" v-loading="loading" stripe>
        <el-table-column type="expand">
          <template #default="{ row }">
            <div style="padding: 12px 24px">
              <div style="font-weight: 600; margin-bottom: 8px">执行步骤：</div>
              <div v-for="(step, i) in (row.steps || [])" :key="i" style="display: flex; align-items: center; gap: 8px; padding: 4px 0">
                <el-tag size="small" type="info">{{ i + 1 }}</el-tag>
                <span style="font-weight: 500">{{ step.name || step.agent || step.id || '未命名' }}</span>
                <el-tag v-if="step.agent" size="small" effect="plain">智能体: {{ step.agent }}</el-tag>
                <el-tag v-if="step.timeout" size="small" type="warning" effect="plain">超时: {{ step.timeout }}</el-tag>
                <span v-if="step.condition" style="font-size: 12px; color: #909399">条件: {{ step.condition }}</span>
              </div>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="名称" min-width="200">
          <template #default="{ row }">
            <span style="font-weight: 600">{{ workflowLabel(row.name) }}</span>
            <div style="font-size: 12px; color: #909399">{{ row.name }}</div>
          </template>
        </el-table-column>
        <el-table-column label="触发方式" width="240">
          <template #default="{ row }">
            <el-tag size="small" :type="triggerTag(row.trigger?.source)">{{ triggerLabel(row.trigger?.source) }}</el-tag>
            <span v-if="row.trigger?.topic" style="margin-left: 8px; font-size: 12px; color: #909399">{{ row.trigger.topic }}</span>
          </template>
        </el-table-column>
        <el-table-column label="步骤数" width="80" align="center">
          <template #default="{ row }">{{ (row.steps || []).length }}</template>
        </el-table-column>
        <el-table-column label="操作" width="160" align="center">
          <template #default="{ row }">
            <el-tooltip :content="`手动触发工作流「${workflowLabel(row.name)}」`" placement="top">
              <el-button size="small" type="primary" :icon="VideoPlay" @click="triggerWorkflow(row.name)">触发</el-button>
            </el-tooltip>
          </template>
        </el-table-column>
      </el-table>
      <div v-if="!loading && workflows.length === 0" class="eh-empty">
        <el-icon><SetUp /></el-icon>
        <p>暂无工作流，请在 workflows/ 目录中添加 YAML 编排文件</p>
      </div>
    </div>

    <div class="eh-card">
      <div class="eh-card-header">
        <span class="eh-card-title"><el-icon><List /></el-icon> 执行历史</span>
        <el-button :icon="Refresh" @click="loadExecutions" :loading="execLoading">刷新</el-button>
      </div>
      <el-table :data="executions" v-loading="execLoading" stripe size="small">
        <el-table-column prop="workflow_id" label="执行ID" min-width="200" show-overflow-tooltip />
        <el-table-column label="工作流" width="200">
          <template #default="{ row }">{{ workflowLabel(row.workflow_name) }}</template>
        </el-table-column>
        <el-table-column label="状态" width="100" align="center">
          <template #default="{ row }">
            <el-tag :type="execStatusType(row.status)" size="small">{{ execStatusLabel(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="开始时间" width="180">
          <template #default="{ row }">{{ formatTime(row.start_time) }}</template>
        </el-table-column>
        <el-table-column label="耗时" width="100">
          <template #default="{ row }">
            <span v-if="row.end_time">{{ calcDuration(row) }}</span>
            <span v-else style="color: #e6a23c">运行中...</span>
          </template>
        </el-table-column>
      </el-table>
      <div v-if="!execLoading && executions.length === 0" class="eh-empty">
        <el-icon><List /></el-icon>
        <p>暂无执行记录，点击上方「触发」按钮执行工作流</p>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { SetUp, Refresh, VideoPlay, List } from '@element-plus/icons-vue'
import dayjs from 'dayjs'
import api from '@/api'

const workflows = ref([])
const executions = ref([])
const loading = ref(false)
const execLoading = ref(false)

const formatTime = (t) => t ? dayjs(t).format('YYYY-MM-DD HH:mm:ss') : '-'

const calcDuration = (row) => {
  if (!row.start_time || !row.end_time) return '-'
  const ms = new Date(row.end_time).getTime() - new Date(row.start_time).getTime()
  if (ms < 0) return '-'
  if (ms < 1000) return ms + 'ms'
  if (ms < 60000) return (ms / 1000).toFixed(1) + 's'
  if (ms > 300000) return '>5min (已恢复)'
  return (ms / 60000).toFixed(1) + 'min'
}

const workflowLabel = (name) => ({
  'energy_peak_warning': '能源园区功率预测与调度',
  'vibration_visual_confirm': '振动异常检测 → 视觉确认 → RAG告警',
  'temperature_anomaly': '温度异常检测与告警',
})[name] || name

const triggerLabel = (s) => ({
  mqtt: 'MQTT 事件触发', manual: '手动触发', schedule: '定时触发', webhook: 'Webhook 触发',
})[s] || '手动触发'
const triggerTag = (s) => ({
  mqtt: 'primary', manual: 'info', schedule: 'warning', webhook: 'success',
})[s] || 'info'

const execStatusLabel = (s) => ({
  running: '运行中', completed: '已完成', failed: '失败', timeout: '超时', pending: '等待中',
})[s] || s
const execStatusType = (s) => {
  if (s === 'completed') return 'success'
  if (s === 'failed') return 'danger'
  if (s === 'running') return 'warning'
  if (s === 'timeout') return 'danger'
  return 'info'
}

const loadWorkflows = async () => {
  loading.value = true
  try {
    const data = await api.get('/workflows')
    workflows.value = data.workflows || data || []
  } catch {} finally { loading.value = false }
}

const loadExecutions = async () => {
  execLoading.value = true
  try {
    const data = await api.get('/workflows/executions')
    executions.value = data.executions || data || []
  } catch {} finally { execLoading.value = false }
}

const triggerWorkflow = async (name) => {
  try {
    await api.post(`/workflows/${name}/trigger`, {})
    ElMessage.success(`工作流「${workflowLabel(name)}」已触发`)
    setTimeout(loadExecutions, 1000)
  } catch {}
}

onMounted(() => { loadWorkflows(); loadExecutions() })
</script>
