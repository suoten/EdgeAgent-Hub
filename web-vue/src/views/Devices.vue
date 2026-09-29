<template>
  <div class="eh-page">
    <el-alert type="info" :closable="false" show-icon style="margin-bottom: 16px">
      <template #title>设备管理</template>
      <template #default>
        展示所有已注册的南向设备，含 Modbus、OPC UA、BLE、ONVIF、Webhook 等工业协议接入的传感器和设备。
        设备状态由心跳自动维护，新设备需在配置文件中添加南向设备配置。
      </template>
    </el-alert>

    <!-- 工业场景说明 -->
    <div class="eh-card" style="margin-bottom: 16px">
      <div class="eh-card-header">
        <span class="eh-card-title"><el-icon><InfoFilled /></el-icon> 网关对接方式</span>
      </div>
      <div style="padding: 12px 0">
        <el-alert type="success" :closable="false" show-icon style="margin-bottom: 12px">
          <template #default>
            EdgeAgent Hub 与物联网关配合使用：网关负责采集传感器数据，Hub 负责边缘 AI 推理、告警生成和通知推送。
          </template>
        </el-alert>
        <el-collapse>
          <el-collapse-item title="方式一：MQTT 对接（推荐，适用于所有支持 MQTT 的网关）" name="0">
            <p>在网关的 MQTT 配置中，将 Broker 地址指向 EdgeAgent Hub 的 IP 和端口（默认 1883）。</p>
            <p>网关需要发布到以下主题：</p>
            <ul style="font-size: 13px; color: #606266; padding-left: 20px; line-height: 1.8">
              <li><code>sensors/vibration/{设备ID}</code> — 振动数据</li>
              <li><code>sensors/temperature/{设备ID}</code> — 温度数据</li>
              <li><code>sensors/power/{设备ID}</code> — 功率数据</li>
              <li><code>sensors/current/{设备ID}</code> — 电流数据</li>
              <li><code>devices/{设备ID}/telemetry</code> — 通用遥测数据</li>
            </ul>
            <p style="font-size: 13px; color: #909399">数据格式：{"value": 5.2, "unit": "mm/s", "timestamp": 1696000000}</p>
          </el-collapse-item>
          <el-collapse-item title="方式二：Modbus TCP 直连" name="1">
            <p>适用于 PLC、变频器、电表、温控器、传感器变送器等。EdgeAgent Hub 作为 Modbus 主站，轮询读取保持寄存器和输入寄存器数据。</p>
            <p style="font-size: 13px; color: #909399">配置路径：configs/ 下添加设备配置，指定协议 modbus、从站地址、寄存器映射表。</p>
          </el-collapse-item>
          <el-collapse-item title="方式三：OPC UA 对接" name="2">
            <p>适用于工业 SCADA、DCS、MES 系统、西门子/施耐德/ABB PLC。通过 OPC UA 订阅节点数据变化，实现高效实时数据采集。</p>
          </el-collapse-item>
          <el-collapse-item title="方式四：HTTP Webhook 推送" name="3">
            <p>如果网关支持 HTTP 推送，可直接调用 API 上报数据：</p>
            <pre style="background: #f5f7fa; padding: 8px; border-radius: 4px; font-size: 12px">POST /api/v1/data/ingest
Content-Type: application/json
Authorization: Bearer &lt;token&gt;

{"device_id": "sensor_01", "type": "vibration", "value": 5.8}</pre>
          </el-collapse-item>
          <el-collapse-item title="方式五：ONVIF 摄像头" name="4">
            <p>适用于工业现场监控摄像头。通过 ONVIF 协议获取摄像头状态，结合振动/温度异常触发视觉确认工作流。</p>
          </el-collapse-item>
          <el-collapse-item title="方式六：BLE 蓝牙设备" name="5">
            <p>适用于蓝牙温度贴片、振动传感器、资产定位标签等低功耗无线设备。</p>
          </el-collapse-item>
        </el-collapse>
      </div>
    </div>

    <div class="eh-card">
      <div class="eh-card-header">
        <span class="eh-card-title"><el-icon><Cpu /></el-icon> 设备列表</span>
        <el-button :icon="Refresh" @click="loadDevices" :loading="loading">刷新</el-button>
      </div>
      <el-table :data="devices" v-loading="loading" stripe style="width: 100%">
        <el-table-column prop="id" label="设备ID" min-width="160" show-overflow-tooltip />
        <el-table-column label="类型" width="140">
          <template #default="{ row }">
            <el-tag size="small" type="info" effect="plain">{{ deviceTypeLabel(row.type) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="endpoint" label="端点" min-width="180" show-overflow-tooltip />
        <el-table-column label="能力" min-width="200">
          <template #default="{ row }">
            <el-tag v-for="cap in (row.capabilities || [])" :key="cap" size="small" style="margin-right: 4px" type="info" effect="plain">{{ cap }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="100" align="center">
          <template #default="{ row }">
            <span class="eh-status-dot" :class="statusClass(row.status)"></span>
            {{ statusLabel(row.status) }}
          </template>
        </el-table-column>
      </el-table>
      <div v-if="!loading && devices.length === 0" class="eh-empty">
        <el-icon><Cpu /></el-icon>
        <p>暂无设备</p>
        <p style="font-size: 13px; color: #909399; max-width: 400px; margin: 0 auto">
          设备需通过配置文件注册。请在 configs/ 目录中添加南向设备配置，
          配置 MQTT 或 Modbus 等协议接入的传感器和工业设备。
        </p>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { Cpu, Refresh, InfoFilled } from '@element-plus/icons-vue'
import api from '@/api'

const devices = ref([])
const loading = ref(false)

const deviceTypeLabel = (t) => ({
  modbus: 'Modbus 设备', opcua: 'OPC UA 设备', ble: '蓝牙设备',
  onvif: 'ONVIF 摄像头', webhook: 'Webhook 设备', mqtt: 'MQTT 设备',
})[t] || t || '-'
const statusLabel = (s) => ({ online: '在线', offline: '离线', degraded: '降级' })[s] || '未知'
const statusClass = (s) => ({ online: 'online', offline: 'offline', degraded: 'degraded' })[s] || 'offline'

const loadDevices = async () => {
  loading.value = true
  try {
    const data = await api.get('/devices')
    devices.value = data.agents || data.devices || []
  } catch {} finally {
    loading.value = false
  }
}

onMounted(loadDevices)
</script>
