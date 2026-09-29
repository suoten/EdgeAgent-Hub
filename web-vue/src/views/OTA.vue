<template>
  <div class="eh-page">
    <el-alert type="info" :closable="false" show-icon style="margin-bottom: 16px">
      <template #title>模型 OTA 更新</template>
      <template #default>
        管理 AI 模型（ONNX/GGUF）的版本更新，支持 A/B 双分区无缝切换、灰度发布、签名验证和自动回滚。
        <br>注意：此功能用于更新边缘推理模型，不是更新 EdgeAgent Hub 软件本身。
        操作流程：① 检查更新 → ② 下载到备用分区 → ③ 验证签名 → ④ 切换激活
      </template>
    </el-alert>

    <!-- 步骤引导 -->
    <div class="eh-card" style="margin-bottom: 16px">
      <div class="steps-guide">
        <div class="step-item" :class="{ active: tasks.length === 0 }">
          <div class="step-num">1</div>
          <div class="step-text">
            <div class="step-title">检查更新</div>
            <div class="step-desc">输入模型名称检查是否有可用更新</div>
          </div>
        </div>
        <div class="step-arrow">→</div>
        <div class="step-item" :class="{ active: tasks.length > 0 }">
          <div class="step-num">2</div>
          <div class="step-text">
            <div class="step-title">下载验证</div>
            <div class="step-desc">系统自动下载并验证签名</div>
          </div>
        </div>
        <div class="step-arrow">→</div>
        <div class="step-item" :class="{ active: tasks.some(t => t.status === 'pending' || t.status === 'verifying') }">
          <div class="step-num">3</div>
          <div class="step-text">
            <div class="step-title">应用更新</div>
            <div class="step-desc">点击「应用」将更新写入备用分区</div>
          </div>
        </div>
        <div class="step-arrow">→</div>
        <div class="step-item" :class="{ active: tasks.some(t => t.status === 'complete') }">
          <div class="step-num">4</div>
          <div class="step-text">
            <div class="step-title">切换激活</div>
            <div class="step-desc">重启后自动切换到新版本</div>
          </div>
        </div>
      </div>
    </div>

    <div class="eh-card">
      <div class="eh-card-header">
        <span class="eh-card-title"><el-icon><Promotion /></el-icon> OTA 更新任务</span>
        <div>
          <el-button type="primary" :icon="Search" @click="showCheckDialog = true">检查更新</el-button>
          <el-button :icon="Refresh" @click="loadTasks" :loading="loading">刷新</el-button>
        </div>
      </div>
      <el-table :data="tasks" v-loading="loading" stripe>
        <el-table-column prop="id" label="任务ID" min-width="180" show-overflow-tooltip />
        <el-table-column prop="model_name" label="模型" width="140" />
        <el-table-column prop="version" label="版本" width="100" />
        <el-table-column label="状态" width="120" align="center">
          <template #default="{ row }">
            <el-tag :type="otaStatusType(row.status)" size="small" effect="dark">{{ otaStatusLabel(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="分区" width="80" align="center">
          <template #default="{ row }">{{ row.target_partition || '-' }}</template>
        </el-table-column>
        <el-table-column label="大小" width="100">
          <template #default="{ row }">{{ formatSize(row.size_bytes) }}</template>
        </el-table-column>
        <el-table-column label="创建时间" width="180">
          <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="120" align="center">
          <template #default="{ row }">
            <el-button v-if="row.status === 'pending' || row.status === 'verifying'" size="small" type="success" :icon="Download" @click="applyTask(row)">应用</el-button>
          </template>
        </el-table-column>
      </el-table>
      <div v-if="!loading && tasks.length === 0" class="eh-empty">
        <el-icon><Promotion /></el-icon>
        <p>暂无 OTA 任务</p>
        <el-button text type="primary" @click="showCheckDialog = true">检查更新</el-button>
      </div>
    </div>

    <el-dialog v-model="showCheckDialog" title="检查模型更新" width="480px">
      <el-form :model="checkForm" label-width="80px">
        <el-form-item label="模型名称">
          <el-input v-model="checkForm.model_name" placeholder="输入要检查的模型名称，如 power_trend_predict" />
        </el-form-item>
        <el-form-item label="版本">
          <el-input v-model="checkForm.version" placeholder="输入目标版本号，如 1.1.0（可留空检查最新）" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showCheckDialog = false">取消</el-button>
        <el-button type="primary" @click="doCheck" :loading="checkLoading">检查</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="showApplyDialog" title="应用 OTA 更新" width="480px">
      <el-alert type="warning" :closable="false" show-icon style="margin-bottom: 16px">
        应用更新将下载模型文件到备用分区，下载完成后需要重启系统切换激活。
      </el-alert>
      <el-form :model="applyForm" label-width="80px">
        <el-form-item label="任务ID"><el-input v-model="applyForm.task_id" disabled /></el-form-item>
        <el-form-item label="下载URL">
          <el-input v-model="applyForm.download_url" placeholder="https://... 模型文件下载地址" />
          <div style="font-size: 12px; color: #909399; margin-top: 4px">留空则使用任务检查时获取的地址</div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showApplyDialog = false">取消</el-button>
        <el-button type="primary" @click="doApply" :loading="applyLoading">应用</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Promotion, Search, Refresh, Download } from '@element-plus/icons-vue'
import dayjs from 'dayjs'
import api from '@/api'

const tasks = ref([])
const loading = ref(false)
const showCheckDialog = ref(false)
const showApplyDialog = ref(false)
const checkLoading = ref(false)
const applyLoading = ref(false)
const checkForm = reactive({ model_name: '', version: '' })
const applyForm = reactive({ task_id: '', download_url: '' })

const formatSize = (b) => {
  if (!b) return '-'
  if (b < 1024) return b + 'B'
  if (b < 1048576) return (b / 1024).toFixed(1) + 'KB'
  if (b < 1073741824) return (b / 1048576).toFixed(1) + 'MB'
  return (b / 1073741824).toFixed(2) + 'GB'
}
const formatTime = (t) => t ? dayjs(t).format('YYYY-MM-DD HH:mm:ss') : '-'
const otaStatusLabel = (s) => ({
  pending: '等待中', verifying: '验证中', downloading: '下载中',
  applying: '应用中', complete: '已完成', failed: '失败', rolled_back: '已回滚',
})[s] || s
const otaStatusType = (s) => {
  if (s === 'complete') return 'success'
  if (s === 'failed') return 'danger'
  if (s === 'rolled_back') return 'info'
  return 'warning'
}

const loadTasks = async () => {
  loading.value = true
  try { const data = await api.get('/ota/tasks'); tasks.value = data.tasks || [] } catch {} finally { loading.value = false }
}

const doCheck = async () => {
  if (!checkForm.model_name.trim()) { ElMessage.warning('请输入模型名称'); return }
  checkLoading.value = true
  try {
    await api.post('/ota/check', checkForm)
    ElMessage.success('更新检查已创建')
    showCheckDialog.value = false
    checkForm.model_name = ''; checkForm.version = ''
    loadTasks()
  } catch {} finally { checkLoading.value = false }
}

const applyTask = (task) => { applyForm.task_id = task.id; applyForm.download_url = ''; showApplyDialog.value = true }

const doApply = async () => {
  applyLoading.value = true
  try {
    await api.post('/ota/apply', applyForm)
    ElMessage.success('OTA 更新已开始应用')
    showApplyDialog.value = false
    setTimeout(loadTasks, 2000)
  } catch {} finally { applyLoading.value = false }
}

onMounted(loadTasks)
</script>

<style scoped>
.steps-guide {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 16px;
  flex-wrap: wrap;
}
.step-item {
  display: flex;
  gap: 10px;
  align-items: center;
  padding: 8px 12px;
  border-radius: 8px;
  transition: background 0.3s;
}
.step-item.active {
  background: #ecf5ff;
}
.step-num {
  width: 28px;
  height: 28px;
  border-radius: 50%;
  background: #dcdfe6;
  color: #fff;
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: 700;
  font-size: 14px;
  flex-shrink: 0;
}
.step-item.active .step-num {
  background: #409eff;
}
.step-title {
  font-size: 14px;
  font-weight: 600;
}
.step-desc {
  font-size: 12px;
  color: var(--eh-text-secondary);
}
.step-arrow {
  color: #c0c4cc;
  font-size: 18px;
}
</style>
