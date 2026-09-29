<template>
  <div class="eh-page chat-page">
    <div class="chat-container">
      <div class="chat-header">
        <span class="eh-card-title"><el-icon><ChatDotRound /></el-icon> 智能对话（RAG 锚定）</span>
        <div style="display: flex; align-items: center; gap: 8px">
          <el-tag size="small" :type="llmOnline ? 'success' : (llmDegraded ? 'warning' : 'danger')" effect="plain">
            {{ llmOnline ? 'LLM AI对话' : (llmDegraded ? 'LLM 降级模式' : 'LLM 离线') }}
          </el-tag>
          <el-tooltip content="配置远程 LLM API 后启用 AI 对话；未配置时使用内置规则引擎 + 知识库检索" placement="bottom">
            <el-icon style="color: #909399; cursor: help"><InfoFilled /></el-icon>
          </el-tooltip>
        </div>
      </div>
      <div class="chat-messages" ref="msgContainer">
        <div v-if="messages.length === 0" class="chat-welcome">
          <el-icon :size="48" color="#1890ff"><ChatDotRound /></el-icon>
          <h3>欢迎使用智能对话</h3>
          <p>基于 RAG 锚定的边缘 AI 对话，所有回答均经过知识库验证</p>
          <div class="chat-suggestions">
            <el-button size="small" round @click="useSuggestion('设备振动异常如何处理？')">设备振动异常如何处理？</el-button>
            <el-button size="small" round @click="useSuggestion('功率预测模型准确率如何？')">功率预测模型准确率如何？</el-button>
            <el-button size="small" round @click="useSuggestion('系统当前运行状态如何？')">系统当前运行状态如何？</el-button>
            <el-button size="small" round @click="useSuggestion('温度异常告警规则是什么？')">温度异常告警规则是什么？</el-button>
            <el-button size="small" round @click="useSuggestion('电流过载怎么检测？')">电流过载怎么检测？</el-button>
            <el-button size="small" round @click="useSuggestion('管道压力监测怎么做？')">管道压力监测怎么做？</el-button>
          </div>
        </div>
        <div v-for="(msg, i) in messages" :key="i" class="chat-message" :class="msg.role">
          <div class="message-avatar">
            <el-avatar :size="36" :style="{ background: msg.role === 'user' ? '#1890ff' : '#67c23a' }">
              <el-icon><component :is="msg.role === 'user' ? 'User' : 'ChatDotRound'" /></el-icon>
            </el-avatar>
          </div>
          <div class="message-body">
            <div class="message-bubble" :class="{ degraded: !msg.anchored && msg.role === 'assistant' }" v-html="formatMessage(msg.content)"></div>
            <div v-if="msg.anchored" class="anchor-badge">
              <el-icon><CircleCheckFilled /></el-icon> RAG 锚定验证通过
            </div>
            <div v-else-if="msg.role === 'assistant'" class="anchor-badge" style="color: #e6a23c">
              <el-icon><WarningFilled /></el-icon> 未经过 RAG 锚定验证
            </div>
          </div>
        </div>
        <div v-if="sending" class="chat-message assistant">
          <div class="message-avatar"><el-avatar :size="36" :style="{ background: '#67c23a' }"><el-icon class="is-loading"><Loading /></el-icon></el-avatar></div>
          <div class="message-body"><div class="message-bubble typing">正在思考...</div></div>
        </div>
      </div>
      <div class="chat-input-bar">
        <el-input
          v-model="input"
          type="textarea"
          :rows="2"
          placeholder="输入消息，按 Enter 发送..."
          resize="none"
          @keydown.enter.exact.prevent="send"
        />
        <el-button type="primary" :icon="Promotion" @click="send" :loading="sending" :disabled="!input.trim()">发送</el-button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, nextTick, onMounted } from 'vue'
import { ChatDotRound, CircleCheckFilled, Loading, Promotion, InfoFilled, WarningFilled, User } from '@element-plus/icons-vue'
import api from '@/api'

const messages = ref([])
const input = ref('')
const sending = ref(false)
const msgContainer = ref()
const llmOnline = ref(false)
const llmDegraded = ref(false)

const checkLLMStatus = async () => {
  try {
    const data = await api.get('/system/health')
    const llmStatus = data.components?.llm
    llmOnline.value = llmStatus === 'available'
    llmDegraded.value = llmStatus === 'degraded'
  } catch {}
}

const send = async () => {
  const text = input.value.trim()
  if (!text || sending.value) return
  messages.value.push({ role: 'user', content: text })
  input.value = ''
  sending.value = true
  await scrollToBottom()
  try {
    const data = await api.post('/chat', { message: text })
    messages.value.push({
      role: 'assistant',
      content: data.response || data.message || '无响应',
      anchored: data.anchored || false,
    })
  } catch {
    messages.value.push({ role: 'assistant', content: '请求失败，请稍后重试' })
  } finally {
    sending.value = false
    await scrollToBottom()
  }
}

const useSuggestion = (text) => {
  input.value = text
  send()
}

const scrollToBottom = async () => {
  await nextTick()
  if (msgContainer.value) msgContainer.value.scrollTop = msgContainer.value.scrollHeight
}

const formatMessage = (text) => {
  return text.replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/\n/g, '<br>')
}

onMounted(() => {
  checkLLMStatus()
})
</script>

<style scoped>
.chat-page { height: calc(100vh - 56px); display: flex; padding: 0; }
.chat-container { flex: 1; display: flex; flex-direction: column; background: #fff; margin: 16px; border-radius: var(--eh-radius); box-shadow: var(--eh-shadow); overflow: hidden; }
.chat-header { display: flex; align-items: center; justify-content: space-between; padding: 16px 20px; border-bottom: 1px solid var(--eh-border); }
.chat-messages { flex: 1; overflow-y: auto; padding: 20px; }
.chat-welcome { text-align: center; padding: 60px 20px; color: var(--eh-text-secondary); }
.chat-welcome h3 { margin: 16px 0 8px; color: var(--eh-text); }
.chat-welcome p { margin-bottom: 20px; }
.chat-suggestions { display: flex; flex-wrap: wrap; gap: 8px; justify-content: center; }
.chat-message { display: flex; gap: 12px; margin-bottom: 20px; }
.chat-message.user { flex-direction: row-reverse; }
.message-body { max-width: 70%; }
.message-bubble { padding: 12px 16px; border-radius: 12px; font-size: 14px; line-height: 1.6; }
.message-bubble.degraded { border-left: 3px solid #e6a23c; }
.chat-message.user .message-bubble { background: #1890ff; color: #fff; border-top-right-radius: 4px; }
.chat-message.assistant .message-bubble { background: #f4f5f7; color: var(--eh-text); border-top-left-radius: 4px; }
.typing { color: var(--eh-text-secondary); font-style: italic; }
.anchor-badge { display: flex; align-items: center; gap: 4px; margin-top: 6px; font-size: 12px; color: var(--eh-success); }
.chat-input-bar { display: flex; gap: 12px; padding: 16px 20px; border-top: 1px solid var(--eh-border); align-items: flex-end; }
.chat-input-bar .el-input { flex: 1; }
</style>
