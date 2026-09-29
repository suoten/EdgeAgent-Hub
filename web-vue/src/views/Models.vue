<template>
  <div class="eh-page">
    <el-alert type="info" :closable="false" show-icon style="margin-bottom: 16px">
      <template #title>模型管理</template>
      <template #default>
        管理 ONNX 推理模型和 GGUF 大语言模型的上传、加载、A/B 分区切换和版本回滚。
        支持 .onnx 和 .gguf 格式文件上传。配置远程 LLM API 可启用 AI 对话能力。
        <br>模型激活规则：多个模型可同时激活，点击「激活/已激活」按钮切换状态。
      </template>
    </el-alert>

    <!-- ONNX 模型说明区 -->
    <div class="eh-card" style="margin-bottom: 16px">
      <div class="eh-card-header">
        <span class="eh-card-title"><el-icon><InfoFilled /></el-icon> ONNX 推理模型说明</span>
      </div>
      <div style="padding: 12px 0">
        <p style="margin-bottom: 8px; color: #606266">EdgeAgent Hub 支持标准 ONNX 格式推理模型。获取模型的途径：</p>
        <el-collapse>
          <el-collapse-item title="方式一：上传自有模型（推荐）" name="1">
            <p>将训练好的模型导出为 ONNX 格式（PyTorch/TensorFlow/sklearn 均可），通过上方「上传模型」按钮上传。</p>
            <p style="font-size: 13px; color: #909399">示例：PyTorch → torch.onnx.export(model, dummy_input, 'model.onnx') → 上传</p>
          </el-collapse-item>
          <el-collapse-item title="方式二：从开源模型库获取" name="2">
            <p>ONNX Model Zoo: https://github.com/onnx/models</p>
            <p>Hugging Face: https://huggingface.co/models?library=onnx</p>
            <p>下载 .onnx 文件后上传即可。常见的工业场景模型包括：异常检测、时序预测、分类识别。</p>
          </el-collapse-item>
          <el-collapse-item title="方式三：用数据自动训练（高级）" name="3">
            <p>使用 Python 训练脚本，基于设备历史数据自动生成 ONNX 推理模型：</p>
            <pre style="background: #f5f7fa; padding: 12px; border-radius: 4px; font-size: 12px">pip install scikit-learn onnx skl2onnx

# 示例：训练振动异常检测模型
from sklearn.ensemble import IsolationForest
from skl2onnx import to_onnx

model = IsolationForest(contamination=0.05)
model.fit(X_train)  # X_train: 历史振动数据
onnx_model = to_onnx(model, X_train[:1].astype('float32'),
                     target_opset=15, name='vibration_anomaly')
open('vibration_anomaly.onnx', 'wb').write(onnx_model.SerializeToString())</pre>
            <p style="font-size: 13px; color: #909399">训练完成后上传 .onnx 文件，系统自动加载并可通过工作流调用推理。</p>
            <p style="margin-top: 8px">常见工业场景对应的模型类型：</p>
            <ul style="font-size: 13px; color: #606266; padding-left: 20px">
              <li>振动异常检测 → IsolationForest / OneClassSVM</li>
              <li>温度趋势预测 → LSTM / RandomForest 回归</li>
              <li>功率负荷预测 → GradientBoosting 回归</li>
              <li>电流过载检测 → IsolationForest</li>
              <li>管道压力监测 → IsolationForest / AutoEncoder</li>
              <li>设备故障分类 → RandomForest / XGBoost 分类</li>
            </ul>
          </el-collapse-item>
          <el-collapse-item title="如何保证模型真正有用？" name="4">
            <p>1. 使用真实设备数据训练，覆盖正常和异常工况</p>
            <p>2. 训练集和测试集分离，确保泛化能力</p>
            <p>3. 上传后在「编排管理」中手动触发工作流验证推理结果</p>
            <p>4. 在「智能对话」中询问模型相关信息，检查 RAG 锚定是否通过</p>
            <p>5. 通过 sidecar 的 /infer 接口直接测试推理输出</p>
          </el-collapse-item>
        </el-collapse>
      </div>
    </div>

    <!-- LLM API 配置卡片 -->
    <div class="eh-card" style="margin-bottom: 16px">
      <div class="eh-card-header">
        <span class="eh-card-title"><el-icon><Setting /></el-icon> LLM API 配置</span>
        <el-tag :type="llmConfig.api_enabled && llmConfig.api_base_url ? 'success' : 'warning'" size="small" effect="plain">
          {{ llmConfig.api_enabled && llmConfig.api_base_url ? '已启用' : '未启用（降级模式）' }}
        </el-tag>
      </div>
      <el-form :model="llmConfig" label-width="120px" style="max-width: 700px; margin-top: 12px">
        <el-form-item label="启用远程 API">
          <el-switch v-model="llmConfig.api_enabled" />
          <span style="margin-left: 8px; font-size: 12px; color: #909399">
            启用后智能对话使用远程 LLM，未启用时使用内置规则引擎
          </span>
        </el-form-item>
        <el-form-item label="API 地址">
          <el-input v-model="llmConfig.api_base_url" placeholder="如 https://dashscope.aliyuncs.com/compatible-mode" />
        </el-form-item>
        <el-form-item label="API Key">
          <el-input v-model="llmConfig.api_key" type="password" show-password placeholder="API Key (Ollama 不需要)" />
        </el-form-item>
        <el-form-item label="模型名称">
          <el-input v-model="llmConfig.api_model" placeholder="如 qwen-plus / deepseek-chat / gpt-4o-mini" />
        </el-form-item>
        <el-form-item label="温度">
          <el-slider v-model="llmConfig.temperature" :min="0" :max="2" :step="0.1" show-input style="max-width: 400px" />
        </el-form-item>
        <el-form-item label="快速预设">
          <div style="display: flex; gap: 8px; flex-wrap: wrap">
            <el-button size="small" @click="presetLLM('qwen')">Qwen</el-button>
            <el-button size="small" @click="presetLLM('deepseek')">DeepSeek</el-button>
            <el-button size="small" @click="presetLLM('glm')">GLM</el-button>
            <el-button size="small" @click="presetLLM('kimi')">Kimi</el-button>
            <el-button size="small" @click="presetLLM('mimo')">MiMo</el-button>
            <el-button size="small" @click="presetLLM('ollama')">Ollama 本地</el-button>
            <el-button size="small" @click="presetLLM('openai')">OpenAI</el-button>
          </div>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="saveLLMConfig" :loading="llmSaving">保存配置</el-button>
          <el-button @click="loadLLMConfig">重新加载</el-button>
        </el-form-item>
      </el-form>
    </div>

    <div class="eh-card">
      <div class="eh-card-header">
        <span class="eh-card-title"><el-icon><Box /></el-icon> 模型列表</span>
        <div>
          <el-upload :show-file-list="false" :before-upload="handleFileUpload" accept=".onnx,.gguf">
            <el-button :icon="Upload" type="primary">上传模型</el-button>
          </el-upload>
          <el-button :icon="RefreshLeft" @click="loadModels" :loading="loading" style="margin-left: 8px">刷新</el-button>
        </div>
      </div>
      <el-table :data="models" v-loading="loading" stripe>
        <el-table-column prop="id" label="ID" min-width="160" show-overflow-tooltip />
        <el-table-column prop="name" label="名称" min-width="200" />
        <el-table-column prop="version" label="版本" width="100" />
        <el-table-column label="类型" width="110">
          <template #default="{ row }">
            <el-tag size="small" :type="row.type === 'onnx' ? 'primary' : 'success'">{{ modelTypeLabel(row.type) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="分区" width="80" align="center">
          <template #default="{ row }">
            <el-tag size="small" :type="row.partition === 'A' ? 'success' : 'warning'" effect="plain">{{ row.partition }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="100" align="center">
          <template #default="{ row }">
            <el-tag :type="row.active ? 'success' : 'info'" size="small" effect="dark">
              {{ row.active ? '已激活' : '未激活' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="大小" width="100">
          <template #default="{ row }">{{ formatSize(row.size_bytes) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="200" align="center">
          <template #default="{ row }">
            <el-tooltip :content="row.active ? '点击取消激活' : '点击激活此模型，可多模型同时激活'" placement="top">
              <el-button size="small" :type="row.active ? 'success' : 'primary'" :icon="Switch" @click="switchModel(row.id)">{{ row.active ? '已激活' : '激活' }}</el-button>
            </el-tooltip>
            <el-tooltip content="卸载此模型" placement="top">
              <el-button size="small" type="danger" :icon="Delete" @click="unloadModel(row.id)">卸载</el-button>
            </el-tooltip>
          </template>
        </el-table-column>
      </el-table>
      <div v-if="!loading && models.length === 0" class="eh-empty">
        <el-icon><Box /></el-icon>
        <p>暂无模型，点击「上传模型」添加 .onnx 或 .gguf 文件</p>
      </div>
    </div>

    <!-- 回滚按钮 -->
    <div class="eh-card" style="text-align: center">
      <el-tooltip content="将活跃模型回滚到上一版本（A/B 分区切换）" placement="top">
        <el-button type="warning" :icon="Back" @click="rollbackModel">回滚到上一版本</el-button>
      </el-tooltip>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Box, Upload, RefreshLeft, Switch, Delete, Back, Setting, InfoFilled } from '@element-plus/icons-vue'
import api from '@/api'

const models = ref([])
const loading = ref(false)
const llmSaving = ref(false)
const llmConfig = reactive({
  api_enabled: false,
  api_base_url: '',
  api_key: '',
  api_model: '',
  temperature: 0.3,
  max_tokens: 512,
  system_prompt: '',
})

const modelTypeLabel = (t) => ({ onnx: 'ONNX 推理', gguf: 'GGUF 大模型' })[t] || t

const formatSize = (b) => {
  if (!b) return '-'
  if (b < 1024) return b + 'B'
  if (b < 1048576) return (b / 1024).toFixed(1) + 'KB'
  if (b < 1073741824) return (b / 1048576).toFixed(1) + 'MB'
  return (b / 1073741824).toFixed(2) + 'GB'
}

const presetLLM = (provider) => {
  const presets = {
    qwen: { api_base_url: 'https://dashscope.aliyuncs.com/compatible-mode', api_model: 'qwen-plus' },
    deepseek: { api_base_url: 'https://api.deepseek.com', api_model: 'deepseek-chat' },
    ollama: { api_base_url: 'http://localhost:11434', api_model: 'qwen2.5:3b', api_key: '' },
    openai: { api_base_url: 'https://api.openai.com', api_model: 'gpt-4o-mini' },
    glm: { api_base_url: 'https://open.bigmodel.cn/api/paas', api_model: 'glm-4-flash' },
    kimi: { api_base_url: 'https://api.moonshot.cn', api_model: 'moonshot-v1-8k' },
    mimo: { api_base_url: 'https://api.mimo.xiaomi.com', api_model: 'mimo-7b' },
  }
  const labels = { qwen: 'Qwen', deepseek: 'DeepSeek', ollama: 'Ollama', openai: 'OpenAI', glm: 'GLM', kimi: 'Kimi', mimo: 'MiMo' }
  const p = presets[provider]
  if (p) {
    llmConfig.api_base_url = p.api_base_url
    llmConfig.api_model = p.api_model
    if (p.api_key !== undefined) llmConfig.api_key = p.api_key
    llmConfig.api_enabled = true
    ElMessage.success(`已填充 ${labels[provider]} 预设配置，请补充 API Key`)
  }
}

const loadLLMConfig = async () => {
  try {
    const data = await api.get('/models/llm-config')
    Object.assign(llmConfig, data)
  } catch {}
}

const saveLLMConfig = async () => {
  llmSaving.value = true
  try {
    await api.put('/models/llm-config', llmConfig)
    ElMessage.success('LLM 配置已保存，重启服务后生效')
    loadLLMConfig()
  } catch {} finally { llmSaving.value = false }
}

const handleFileUpload = async (file) => {
  const formData = new FormData()
  formData.append('file', file)
  formData.append('model_id', file.name.replace(/\.(onnx|gguf)$/i, ''))
  try {
    ElMessage.info(`正在上传 ${file.name}...`)
    await api.post('/models/upload', formData, { headers: { 'Content-Type': 'multipart/form-data' } })
    ElMessage.success(`模型 ${file.name} 上传成功`)
    loadModels()
  } catch {}
  return false // 阻止 el-upload 默认行为
}

const loadModels = async () => {
  loading.value = true
  try {
    const data = await api.get('/models')
    models.value = data.models || []
  } catch {} finally { loading.value = false }
}

const switchModel = async (id) => {
  try {
    const data = await api.post('/models/switch', { model_id: id })
    if (data.status === 'activated') {
      ElMessage.success('模型已激活')
    } else {
      ElMessage.info('模型已取消激活')
    }
    loadModels()
  } catch {}
}

const unloadModel = async (id) => {
  try {
    await ElMessageBox.confirm(`确定卸载模型 ${id}？`, '卸载确认', { type: 'warning', confirmButtonText: '确定卸载', cancelButtonText: '取消' })
    await api.post('/models/unload', { model_id: id })
    ElMessage.success('模型已卸载')
    loadModels()
  } catch {}
}

const rollbackModel = async () => {
  try {
    await ElMessageBox.confirm('确定回滚到上一版本？', '回滚确认', { type: 'warning' })
    await api.post('/models/rollback')
    ElMessage.success('已回滚到上一版本')
    loadModels()
  } catch {}
}

onMounted(() => {
  loadModels()
  loadLLMConfig()
})
</script>
