<template>
  <div class="eh-page">
    <el-alert type="info" :closable="false" show-icon style="margin-bottom: 16px">
      <template #title>智能体管理</template>
      <template #default>
        智能体是执行边缘 AI 任务的独立服务实例。系统已预置 ONNX 推理、模板告警、多渠道通知等内置智能体。
        点击「注册智能体」可添加新的外部智能体服务。
      </template>
    </el-alert>

    <div class="eh-card">
      <div class="eh-card-header">
        <span class="eh-card-title"><el-icon><Connection /></el-icon> 智能体列表</span>
        <div>
          <el-tooltip content="刷新智能体列表" placement="top">
            <el-button :icon="Refresh" @click="loadAgents" :loading="loading">刷新</el-button>
          </el-tooltip>
          <el-button type="primary" :icon="Plus" @click="openRegisterDialog">注册智能体</el-button>
        </div>
      </div>
      <el-table :data="agents" v-loading="loading" stripe>
        <el-table-column prop="id" label="ID" min-width="160" show-overflow-tooltip />
        <el-table-column label="类型" width="120">
          <template #default="{ row }">
            <el-tag size="small" :type="agentTypeTag(row.type)">{{ agentTypeLabel(row.type) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="endpoint" label="端点" min-width="180" show-overflow-tooltip />
        <el-table-column label="能力" min-width="240">
          <template #default="{ row }">
            <el-tag v-for="cap in (row.capabilities || [])" :key="cap" size="small" type="info" effect="plain" style="margin-right: 4px">{{ capLabel(cap) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="100" align="center">
          <template #default="{ row }">
            <span class="eh-status-dot" :class="statusClass(row.status)"></span>
            {{ statusLabel(row.status) }}
          </template>
        </el-table-column>
        <el-table-column prop="weight" label="权重" width="80" align="center" />
      </el-table>
      <div v-if="!loading && agents.length === 0" class="eh-empty">
        <el-icon><Connection /></el-icon>
        <p>暂无智能体，点击「注册智能体」添加</p>
      </div>
    </div>

    <!-- 类型说明卡片 -->
    <el-row :gutter="12" style="margin-top: 16px">
      <el-col :xs="24" :sm="8" v-for="info in agentTypeInfo" :key="info.type">
        <div class="type-info-card">
          <el-icon :size="24" :color="info.color"><component :is="info.icon" /></el-icon>
          <div>
            <div class="type-info-title">{{ info.label }}</div>
            <div class="type-info-desc">{{ info.desc }}</div>
          </div>
        </div>
      </el-col>
    </el-row>

    <el-dialog v-model="showRegisterDialog" title="注册智能体" width="520px">
      <el-form :model="regForm" label-width="80px">
        <el-form-item label="ID">
          <el-input v-model="regForm.id" placeholder="输入智能体唯一标识，如 onnx.inference" />
        </el-form-item>
        <el-form-item label="类型">
          <el-select v-model="regForm.type" style="width: 100%">
            <el-option label="ONNX 推理（边缘模型推理）" value="onnx" />
            <el-option label="LLM 大模型（对话生成）" value="llm" />
            <el-option label="RAG 检索（知识库检索）" value="rag" />
            <el-option label="模板告警（规则告警）" value="template" />
            <el-option label="多渠道通知（钉钉/短信等）" value="notify" />
          </el-select>
        </el-form-item>
        <el-form-item label="端点">
          <el-input v-model="regForm.endpoint" placeholder="如 http://127.0.0.1:50052（内置智能体填 internal）" />
        </el-form-item>
        <el-form-item label="能力">
          <el-input v-model="regForm.capabilities" placeholder="逗号分隔，如: onnx,inference,anomaly" />
          <div style="font-size: 12px; color: #909399; margin-top: 4px">输入智能体具备的能力标签，用于工作流调度匹配</div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showRegisterDialog = false">取消</el-button>
        <el-button type="primary" @click="doRegister" :loading="regLoading">注册</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Connection, Plus, Refresh, Box, ChatDotRound, Collection, BellFilled, Promotion } from '@element-plus/icons-vue'
import api from '@/api'

const agents = ref([])
const loading = ref(false)
const showRegisterDialog = ref(false)
const regLoading = ref(false)
const regForm = reactive({ id: '', type: 'onnx', endpoint: '', capabilities: '' })

const agentTypeInfo = [
  { type: 'onnx', label: 'ONNX 推理', desc: '执行 ONNX 格式的机器学习模型推理，如振动检测、温度预测', icon: Box, color: '#409eff' },
  { type: 'llm', label: 'LLM 大模型', desc: '运行 GGUF 格式的大语言模型，提供对话和文本生成能力', icon: ChatDotRound, color: '#67c23a' },
  { type: 'rag', label: 'RAG 检索', desc: '从知识库中检索相关文档，为对话和告警提供锚定验证', icon: Collection, color: '#e6a23c' },
  { type: 'template', label: '模板告警', desc: '基于规则模板生成告警消息，支持多级别严重度', icon: BellFilled, color: '#909399' },
  { type: 'notify', label: '多渠道通知', desc: '通过钉钉、短信、Webhook 等渠道发送告警通知', icon: Promotion, color: '#f56c6c' },
]

const agentTypeLabel = (t) => ({
  onnx: 'ONNX 推理', llm: 'LLM 大模型', rag: 'RAG 检索', template: '模板告警', notify: '多渠道通知',
})[t] || t
const agentTypeTag = (t) => ({
  onnx: 'primary', llm: 'success', rag: 'warning', template: 'info', notify: 'danger',
})[t] || 'info'
const statusLabel = (s) => ({ online: '在线', offline: '离线', degraded: '降级' })[s] || s
const statusClass = (s) => ({ online: 'online', offline: 'offline', degraded: 'degraded' })[s] || 'offline'
const capLabel = (cap) => ({
  onnx: 'ONNX推理', inference: '推理', vibration: '振动检测', temperature: '温度检测', anomaly: '异常检测',
  llm: '大模型', rag: 'RAG', chat: '对话', alert: '告警', search: '检索', index: '索引',
  template: '模板', fallback: '回退', notify: '通知', dingtalk: '钉钉', sms: '短信', webhook: 'Webhook',
  vision: '视觉', camera: '摄像头', quality: '质量检测', inspection: '检查',
  power: '功率', energy: '能源', camera_check: '摄像头检查',
})[cap] || cap

const loadAgents = async () => {
  loading.value = true
  try { const data = await api.get('/agents'); agents.value = data.agents || [] } catch {} finally { loading.value = false }
}

const openRegisterDialog = () => {
  regForm.id = ''
  regForm.type = 'onnx'
  regForm.endpoint = ''
  regForm.capabilities = ''
  showRegisterDialog.value = true
}

const doRegister = async () => {
  if (!regForm.id.trim()) { ElMessage.warning('请输入智能体ID'); return }
  regLoading.value = true
  try {
    await api.post('/agents/register', {
      id: regForm.id,
      type: regForm.type,
      endpoint: regForm.endpoint,
      capabilities: regForm.capabilities.split(',').map(s => s.trim()).filter(Boolean),
      status: 'online',
      weight: 1,
    })
    ElMessage.success('智能体注册成功')
    showRegisterDialog.value = false
    loadAgents()
  } catch {} finally { regLoading.value = false }
}

onMounted(loadAgents)
</script>

<style scoped>
.eh-status-dot.degraded { background: #e6a23c; }

.type-info-card {
  display: flex;
  gap: 12px;
  align-items: flex-start;
  background: #fff;
  border-radius: var(--eh-radius);
  box-shadow: var(--eh-shadow);
  padding: 14px 16px;
  margin-bottom: 12px;
}
.type-info-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--eh-text);
}
.type-info-desc {
  font-size: 12px;
  color: var(--eh-text-secondary);
  line-height: 1.5;
  margin-top: 4px;
}
</style>
