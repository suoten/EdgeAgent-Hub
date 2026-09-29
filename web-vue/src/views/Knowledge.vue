<template>
  <div class="eh-page">
    <el-alert type="info" :closable="false" show-icon style="margin-bottom: 16px">
      <template #title>知识库管理</template>
      <template #default>
        知识库提供 RAG 检索增强生成能力。上传文档后系统自动向量化索引，
        对话和告警生成时会从知识库检索相关内容进行锚定验证，确保输出准确可靠。
      </template>
    </el-alert>

    <div class="eh-card">
      <div class="eh-card-header">
        <span class="eh-card-title"><el-icon><Collection /></el-icon> 知识库管理</span>
      </div>
      <el-tabs v-model="activeTab">
        <el-tab-pane label="知识检索" name="search">
          <div class="search-bar">
            <el-input v-model="query" placeholder="输入查询内容，如：振动异常如何处理？" size="large" :prefix-icon="Search" @keyup.enter="doSearch" clearable>
              <template #append>
                <el-button :icon="Search" @click="doSearch" :loading="searching">搜索</el-button>
              </template>
            </el-input>
          </div>

          <!-- 示例查询 -->
          <div class="example-queries" v-if="!hasSearched">
            <div class="example-title">试试这些查询：</div>
            <div class="example-buttons">
              <el-button size="small" round @click="useExample('设备振动异常如何处理？')">设备振动异常如何处理？</el-button>
              <el-button size="small" round @click="useExample('功率预测模型准确率如何？')">功率预测模型准确率如何？</el-button>
              <el-button size="small" round @click="useExample('温度异常告警规则是什么？')">温度异常告警规则是什么？</el-button>
              <el-button size="small" round @click="useExample('电流过载怎么检测？')">电流过载怎么检测？</el-button>
              <el-button size="small" round @click="useExample('管道压力监测怎么做？')">管道压力监测怎么做？</el-button>
              <el-button size="small" round @click="useExample('如何配置多渠道通知？')">如何配置多渠道通知？</el-button>
            </div>
          </div>

          <div v-if="searching" style="text-align: center; padding: 40px"><el-icon class="is-loading" :size="32"><Loading /></el-icon></div>
          <div v-else-if="results.length === 0 && hasSearched" class="eh-empty">
            <el-icon><Collection /></el-icon>
            <p>未找到相关内容</p>
            <p style="font-size: 13px; color: #909399">请尝试其他关键词，或先上传知识文档</p>
          </div>
          <div v-else>
            <div v-for="(r, i) in results" :key="i" class="result-item">
              <div class="result-header">
                <span class="result-source"><el-icon><Document /></el-icon> {{ r.title || r.source || '未知来源' }}</span>
                <el-tag size="small" type="info">相似度: {{ formatScore(r.score) }}</el-tag>
              </div>
              <div class="result-content">{{ r.content }}</div>
            </div>
          </div>
        </el-tab-pane>

        <el-tab-pane label="知识上传" name="upload">
          <el-upload
            class="upload-area"
            drag
            action="/api/v1/knowledge/upload"
            :headers="uploadHeaders"
            :on-success="handleUploadSuccess"
            :on-error="handleUploadError"
            multiple
          >
            <el-icon class="el-icon--upload"><UploadFilled /></el-icon>
            <div class="el-upload__text">拖拽文件到此处或<em>点击上传</em></div>
            <template #tip>
              <div class="el-upload__tip">
                支持 PDF / TXT / MD / CSV 等文档格式，上传后自动向量化索引。
                <br>知识库内容将用于智能对话的 RAG 锚定验证和告警消息生成。
              </div>
            </template>
          </el-upload>
        </el-tab-pane>
      </el-tabs>
    </div>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import { ElMessage } from 'element-plus'
import { Collection, Search, Loading, Document, UploadFilled } from '@element-plus/icons-vue'
import api from '@/api'

const activeTab = ref('search')
const query = ref('')
const results = ref([])
const searching = ref(false)
const hasSearched = ref(false)

const uploadHeaders = computed(() => ({
  Authorization: `Bearer ${localStorage.getItem('edgeagent_token')}`,
}))

const formatScore = (s) => {
  if (!s) return '-'
  if (s <= 1) return (s * 100).toFixed(1) + '%'
  return s.toFixed(1) + '%'
}

const useExample = (text) => {
  query.value = text
  doSearch()
}

const doSearch = async () => {
  if (!query.value.trim()) return
  searching.value = true
  hasSearched.value = true
  try {
    const data = await api.get('/knowledge/search', { params: { q: query.value } })
    results.value = data.results || []
    if (data.message) {
      ElMessage.info(data.message)
    }
  } catch { results.value = [] } finally { searching.value = false }
}

const handleUploadSuccess = () => { ElMessage.success('文件上传成功，正在索引') }
const handleUploadError = () => { ElMessage.error('上传失败') }
</script>

<style scoped>
.search-bar { margin-bottom: 20px; }
.example-queries { background: #f9fafb; border-radius: 8px; padding: 16px; margin-bottom: 20px; }
.example-title { font-size: 13px; color: var(--eh-text-secondary); margin-bottom: 8px; }
.example-buttons { display: flex; flex-wrap: wrap; gap: 8px; }
.result-item { padding: 14px 0; border-bottom: 1px solid #f0f0f0; }
.result-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 6px; }
.result-source { display: flex; align-items: center; gap: 4px; font-weight: 600; font-size: 14px; }
.result-content { font-size: 13px; color: var(--eh-text-secondary); line-height: 1.6; }
.upload-area { text-align: center; }
</style>
