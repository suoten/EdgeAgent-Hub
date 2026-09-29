<div align="center">

# EdgeAgent Hub

### 边缘智能体管理平台 — 让工业边缘 AI 触手可及

[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![Vue Version](https://img.shields.io/badge/Vue-3.5+-42b883?logo=vuedotjs&logoColor=white)](https://vuejs.org)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20Windows%20%7C%20ARM64-lightgrey)]()

**断网不断服务 · 边缘推理 · 多智能体协同 · RAG 锚定 · A/B 分区 OTA · 端到端安全**

</div>

---

## 为什么选择 EdgeAgent Hub？

在工业物联网场景中，云端 AI 方案面临着**网络延迟高、断网即瘫痪、数据隐私合规、推理成本贵**四大痛点。EdgeAgent Hub 将 AI 推理能力下沉到边缘侧，让每一台工业网关都拥有自主决策的"大脑"。

| 能力 | 传统云端方案 | EdgeAgent Hub |
|------|------------|---------------|
| 网络中断 | 服务瘫痪 | **断网自治，数据缓存，恢复自动重传** |
| 推理延迟 | 200-2000ms | **< 50ms 本地推理** |
| 数据隐私 | 数据上云 | **数据不出厂，本地闭环** |
| 推理成本 | 按次计费 | **一次部署，零边际成本** |
| 多协议接入 | 需额外开发 | **Modbus / OPC UA / MQTT / BLE / ONVIF / Webhook 开箱即用** |

---

## 谁需要 EdgeAgent Hub？

如果你正在做以下任何一件事，EdgeAgent Hub 就是你需要的：

| 角色 | 痛点 | EdgeAgent Hub 怎么帮 |
|------|------|---------------------|
| **物联网关 / 边缘计算网关厂商** | 网关只负责采集和转发，没有 AI 推理能力，附加值低 | 部署 EdgeAgent Hub 到网关上，立刻拥有振动检测、温度预警、功率预测等工业 AI 能力，网关从"管道"变成"智能终端" |
| **工业自动化集成商** | 每个项目都要从零搭建数据采集 + 告警 + 通知系统 | 一次部署，Modbus/OPC UA/MQTT 全协议接入，4 个工业工作流开箱即用，项目交付周期缩短 60% |
| **工厂运维团队** | 设备故障靠人工巡检，发现问题不及时，定位原因慢 | 传感器数据自动触发 AI 推理，异常秒级告警推送到钉钉/短信，RAG 知识库辅助生成处置建议 |
| **智能制造方案商** | 云端 AI 方案延迟高、断网即瘫痪、数据合规不允许上云 | 边缘本地推理 < 50ms，断网自治不中断，数据不出厂，一次部署零边际成本 |
| **PLC / SCADA 开发者** | 想给现有系统加 AI 能力，但不熟悉机器学习框架 | ONNX 模型一键加载，YAML 工作流定义推理链路，不需要写 Python 代码 |
| **工业设备制造商** | 卖出设备后无法远程升级模型和固件 | A/B 分区 OTA 热切换，灰度发布 + 自动回滚，模型签名防篡改 |
| **能源 / 化工 / 冶金企业** | 功率超限、电流过载、管道压力异常等隐患发现太晚 | 预置功率趋势预测、电流过载检测、管道压力监测模型，实时推理 + 告警 |

> **一句话**：如果你的设备在产生数据（振动、温度、电流、功率、压力……），而你还没用 AI 来做实时异常检测和智能告警 — 那你需要 EdgeAgent Hub。

---

## 核心功能一览

### 智能推理引擎
- **ONNX Runtime** — 振动分析、温度预测、异常检测等工业 AI 模型本地推理
- **llama.cpp** — 边缘侧大语言模型 (LLM) 推理，支持 GGUF 格式，CPU 即可运行
- **RAG 锚定** — 基于知识库检索增强生成，防止 LLM 幻觉，确保输出可追溯
- **资源自适应** — 自动检测硬件规格，智能选择模型精度 (FP32/FP16/Q4) 和推理后端

### 多智能体编排
- **YAML DSL 声明式工作流** — 无需写代码，用配置文件定义推理流水线
- **并行执行 + 条件分支** — 支持多步骤并行、条件判断、超时回滚
- **断点续行** — JetStream 持久化执行状态，重启后自动恢复未完成任务
- **内置智能体** — ONNX 推理、LLM 对话、RAG 检索、模板告警、多通道通知

### 全协议南向接入
| 协议 | 场景 | 状态 |
|------|------|------|
| **Modbus TCP** | PLC、传感器、变频器 | 生产就绪 |
| **OPC UA** | 工业控制系统 | 生产就绪 |
| **MQTT** | IoT 设备遥测 | 生产就绪 |
| **BLE** | 蓝牙传感器 | 框架就绪 |
| **ONVIF** | IP 摄像头 | 框架就绪 |
| **Webhook** | 第三方系统推送 | 生产就绪 |

### 生产级安全
- **mTLS 双向认证** — 证书自动轮换，零信任通信
- **RBAC 权限模型** — 管理员/操作员/观察者三级角色
- **JWT + Token 撤销** — 支持主动注销，离线 Token 即时失效
- **推理投毒防护** — 输入范围校验，异常数据自动拦截
- **LLM 输出过滤** — Prompt 注入检测，敏感信息脱敏
- **登录保护** — 暴力破解锁定，IP 限流

### A/B 分区 OTA
- **双分区冗余** — A/B 分区热切换，升级失败秒级回滚
- **灰度发布** — 按比例灰度，先小流量验证再全量
- **模型签名验证** — 下载完整性校验，防篡改
- **USB 离线导入** — 适用于无网络环境

### 可观测性
- **Prometheus 指标** — 标准化指标导出，Grafana 开箱即用
- **分布式追踪** — 全链路 Span 追踪
- **审计日志** — 所有操作留痕，满足合规要求
- **EWMA 自学习** — 动态阈值自适应，减少误报

### 断网自治
- **离线缓存** — 断网期间数据本地持久化，最大 10000 条队列
- **自动重传** — 网络恢复后按序重传，指数退避重试
- **本地决策** — 断网不中断推理和告警

---

## 架构图

```
                          ┌─────────────────────────────────────────────┐
                          │            EdgeAgent Hub (Go)               │
                          │                                             │
    ┌─────────┐  MQTT     │  ┌──────────┐  ┌──────────┐  ┌──────────┐  │
    │ Sensors ├──────────►│  │ Protocol │  │ Data     │  │ Learning │  │
    │ (PLC,   │  Modbus   │  │ Manager  ├──► Quality  ├──► EWMA     │  │
    │  Temp,  ├──────────►│  │          │  │ Monitor  │  │ Self-Adapt│  │
    │  Vib)   │  OPC UA   │  └────┬─────┘  └──────────┘  └────┬─────┘  │
    └─────────┘           │       │                          │        │
                          │  ┌────▼──────────────────────────▼─────┐ │
    ┌─────────┐  NATS     │  │         Orchestration Engine         │ │
    │  ONNX   │◄─────────►│  │  (YAML DSL Workflows + JetStream)    │ │
    │ Inference│          │  └────┬──────────────┬──────────────┘  │ │
    └─────────┘           │       │              │                  │
    ┌─────────┐  NATS     │  ┌────▼────┐   ┌────▼─────┐             │
    │   LLM   │◄─────────►│  │  ONNX   │   │   LLM    │             │
    │ + RAG   │           │  │ Agent   │   │  Agent   │             │
    └─────────┘           │  └─────────┘   └──────────┘             │ │
                          │                                             │
    ┌─────────┐  REST API │  ┌──────────────────────────────────┐    │
    │ Web UI  │◄─────────►│  │  Vue 3 + Element Plus Management │    │
    │ (Vue 3) │           │  │  Dashboard / Devices / OTA / ... │    │
    └─────────┘           │  └──────────────────────────────────┘    │
                          └─────────────────────┬───────────────────┘
                                                │
                          ┌─────────────────────▼───────────────────┐
                          │        Cloud / Northbound                │
                          │  Webhook · MQTT Bridge · Notifications  │
                          └─────────────────────────────────────────┘
```

---

## 快速开始

### 方式一：一键安装（推荐，小白友好）

> 不需要安装 Go、Node.js，下载预编译包即可运行

#### Linux (amd64 / arm64)

```bash
# 1. 下载发布包 (从 Release 页面获取)
wget https://github.com/edgelite/edgeagent-hub/releases/download/v1.0.0/edgeagent-hub-1.0.0-linux-amd64.zip

# 2. 解压
unzip edgeagent-hub-1.0.0-linux-amd64.zip
cd edgeagent-hub-1.0.0-linux-amd64-pkg

# 3. 一键安装 (自动创建用户、目录、systemd 服务、下载 NATS)
sudo bash install.sh

# 4. 打开浏览器访问
#    http://<服务器IP>:8080
#    初始账号: admin  密码: admin123
#    ⚠ 首次登录后请立即修改密码（右上角个人信息 → 修改密码）
```

#### Windows

```powershell
# 1. 下载 edgeagent-hub-1.0.0-windows-amd64.zip 并解压
# 2. 打开 PowerShell，进入解压目录
# 3. 运行
.\edgeagent-hub.exe -config config.yaml

# 4. 打开浏览器访问 http://localhost:8080
#    初始账号: admin  密码: admin123
#    ⚠ 首次登录后请立即修改密码（右上角个人信息 → 修改密码）
```

> **注意**: EdgeAgent Hub 依赖 NATS Server 作为消息骨干。Linux 安装脚本会自动下载安装 NATS；Windows 用户需手动安装 NATS ([下载地址](https://github.com/nats-io/nats-server/releases))。

### 方式二：Docker 部署

```bash
# 克隆仓库
git clone https://github.com/edgelite/edgeagent-hub.git
cd edgeagent-hub

# 一键启动全部服务 (EdgeAgent Hub + NATS + ONNX + LLM)
docker compose up -d

# 仅启动核心服务 (不含 LLM)
docker compose up -d nats edgeagent-hub inference-onnx

# 含监控套件 (Prometheus + Grafana)
docker compose --profile monitoring up -d
```

### 方式三：从源码构建

> 适合开发者，需要 Go 1.24+ 和 Node.js 18+

```bash
# 1. 克隆
git clone https://github.com/edgelite/edgeagent-hub.git
cd edgeagent-hub

# 2. 一键构建 (前端 + 后端)
make build

# 3. 运行
./dist/edgehub -config configs/config.yaml

# 4. 开发模式 (热更新前端)
cd web-vue && npm install && npm run dev
```

### 方式四：发布打包

```powershell
# Windows 上执行，生成多平台发布包
powershell -ExecutionPolicy Bypass -File make-release.ps1 -Version 1.0.0

# 产出:
#   release/edgeagent-hub-1.0.0-linux-amd64.zip
#   release/edgeagent-hub-1.0.0-linux-arm64.zip
#   release/edgeagent-hub-1.0.0-windows-amd64.zip
#   release/checksums.txt
```

---

## 安装后配置

### 0. 首次登录

安装完成后，使用浏览器访问 `http://<服务器IP>:9090`（默认端口 9090）。

| 项目 | 值 |
|------|------|
| **初始账号** | `admin` |
| **初始密码** | `admin123` |

> ⚠ **安全提示**：首次登录后，请立即点击右上角用户名 → 「个人信息」→ 修改密码。
> 也可以在「安全管理」页面由管理员重置任意用户密码。

### 1. 配置 MQTT / 设备接入

编辑配置文件 (Linux: `/etc/edgeagent-hub/config.yaml`，Docker: `configs/config.yaml`)：

```yaml
mqtt:
  broker: "tcp://192.168.1.100:1883"  # 你的 MQTT Broker 地址
  subscribe_topics:
    - "sensors/+/+"
    - "devices/+/telemetry"

protocols:
  modbus:
    enabled: true
    devices:
      - device_id: "line1.plc1"
        host: "192.168.1.100"
        port: 502
        unit_id: 1
        poll_interval: 1s
        registers:
          - name: "vibration_rms"
            address: 0
            quantity: 1
            func_code: 3
            data_type: "uint16"
            scale: 0.1
            unit: "mm/s"
```

### 2. 放入 AI 模型

```
/var/lib/edgeagent-hub/models/
├── onnx/                         # ONNX 推理模型
│   └── vibration_anomaly.onnx
├── llm/                          # GGUF 大语言模型
│   └── qwen3-0.6b-q4_k_m.gguf
├── partition_a/                  # OTA A 分区
└── partition_b/                  # OTA B 分区
```

### 3. 配置工作流

在 `workflows/` 目录中创建 YAML 工作流：

```yaml
name: energy_peak_warning
trigger:
  type: threshold
  metric: power
  operator: ">"
  value: 100
steps:
  - id: detect
    agent: onnx.inference
    input:
      model: anomaly_detect
      data: "{{ trigger.data }}"
  - id: llm_analysis
    agent: llm.rag_anchored
    depends_on: [detect]
    input:
      prompt: "分析设备功率异常原因: {{ detect.output }}"
    rag_anchored: true
  - id: notify
    agent: notify.multi_channel
    depends_on: [llm_analysis]
    input:
      message: "{{ llm_analysis.output }}"
```

### 4. 运行自检

```bash
# Linux
sudo -u edgeagent /opt/edgeagent-hub/edgeagent-hub selftest -config /etc/edgeagent-hub/config.yaml

# Docker
docker compose exec edgeagent-hub /app/edgehub selftest
```

---

## 服务管理 (Linux)

```bash
# 启动 / 停止 / 重启
sudo systemctl start edgeagent-hub
sudo systemctl stop edgeagent-hub
sudo systemctl restart edgeagent-hub

# 查看状态
sudo systemctl status edgeagent-hub

# 查看实时日志
sudo journalctl -u edgeagent-hub -f

# 卸载
sudo bash uninstall.sh
```

---

## CLI 命令

```bash
# 系统自检
edgeagent-hub selftest

# 初始化
edgeagent-hub init

# 模型管理
edgeagent-hub model list
edgeagent-hub model load <model_id> <file_path>
edgeagent-hub model unload <model_id>
edgeagent-hub model switch <model_id>
edgeagent-hub model rollback

# 工作流管理
edgeagent-hub workflow list
edgeagent-hub workflow trigger <name>
edgeagent-hub workflow show <name>

# 版本
edgeagent-hub version
```

---

## Web 管理界面

| 页面 | 功能 |
|------|------|
| **Dashboard** | 实时指标卡片、消息吞吐趋势图、智能体在线状态 |
| **设备管理** | 设备列表、协议类型、在线状态、最新遥测值 |
| **告警中心** | 告警列表、级别筛选、确认/处置 |
| **编排管理** | 工作流列表、触发执行、执行详情查看 |
| **模型管理** | 模型加载/卸载、活跃切换、版本回滚 |
| **知识库** | 文档上传、语义搜索、RAG 索引 |
| **OTA 更新** | 检查更新、灰度发布、任务列表 |
| **系统监控** | 健康状态、Prometheus 指标、审计日志 |
| **智能体** | 智能体列表、在线状态、注册新智能体 |
| **智能对话** | LLM + RAG 对话交互 |
| **安全管理** | 用户管理、角色分配 |

---

## 技术栈

| 层 | 技术 |
|----|------|
| **后端** | Go 1.24 · Echo · NATS JetStream · modernc SQLite |
| **前端** | Vue 3 · Vite · Element Plus · ECharts |
| **推理** | ONNX Runtime · llama.cpp (GGUF) |
| **消息** | NATS JetStream (内部) · MQTT (南向) |
| **安全** | mTLS · JWT · bcrypt · RBAC |
| **监控** | Prometheus · Grafana |
| **部署** | Docker · systemd · 交叉编译 |

---

## 项目结构

```
EdgeAgent Hub/
├── cmd/edgehub/              # 主程序入口
├── internal/
│   ├── agent/                # 智能体注册中心 + OOM 管理
│   ├── api/                  # REST API 服务器
│   ├── cli/                  # CLI 命令行工具
│   ├── config/               # 配置加载
│   ├── dataprocess/          # 数据质量 + 预处理
│   ├── learning/             # EWMA 自学习
│   ├── messaging/            # MQTT-NATS 桥接 + JetStream
│   ├── models/               # 数据模型
│   ├── northbound/           # 北向输出
│   ├── observability/        # 指标 + 追踪 + 日志
│   ├── offline/              # 断网自治
│   ├── orchestrator/         # 工作流编排引擎
│   ├── ota/                  # A/B 分区 OTA
│   ├── protocol/             # 南向协议适配器
│   ├── resource/             # 硬件检测 + 精度选择
│   ├── security/             # mTLS/RBAC/防护/证书轮换
│   └── storage/              # SQLite 存储 + 备份 + 加密
├── ai_sidecar/               # Python 推理服务 (ONNX + LLM)
├── web-vue/                  # Vue 3 前端源码
├── configs/                  # 默认配置
├── workflows/                # 示例工作流
├── deploy/                   # 部署脚本
├── Dockerfile                # 主服务 Docker 镜像
├── Dockerfile.sidecar        # AI 推理服务 Docker 镜像
├── docker-compose.yml        # 全套服务编排
├── Makefile                  # 构建自动化
└── make-release.ps1          # 发布打包脚本
```

---

## 物联网关 / 边缘计算网关对接指南

EdgeAgent Hub 可以与各种采集物联网关、边缘计算网关配合使用。以下是常见的对接方式：

### 对接架构

```
┌─────────────┐      MQTT       ┌───────────────────┐
│  物联网关    │  sensors/temp/x  │  EdgeAgent Hub    │
│  (网关品牌)  │ ──────────────► │  边缘推理 + 告警   │
└─────────────┘                 └───────────────────┘
       │                               │
   Modbus/OPC UA                  Webhook/钉钉
   采集传感器数据                 告警通知运维人员
```

### 方式一：MQTT 对接（最常用）

绝大多数物联网关都支持 MQTT 协议上报数据。只需将网关的 MQTT 发布地址指向 EdgeAgent Hub 即可。

**步骤：**

1. 在网关的「MQTT 配置」页面，填写 EdgeAgent Hub 的地址：
   - Broker: `192.168.1.100`（EdgeAgent Hub 所在机器 IP）
   - Port: `1883`（或自定义端口）

2. 配置网关的数据上报主题，EdgeAgent Hub 默认订阅以下主题：
   - `sensors/{传感器类型}/{设备ID}` — 传感器数据
   - `devices/{设备ID}/telemetry` — 设备遥测数据

3. 数据格式（JSON）：
   ```json
   {
     "value": 5.2,
     "unit": "mm/s",
     "timestamp": 1696000000
   }
   ```

4. EdgeAgent Hub 收到数据后自动触发工作流：
   - 振动 > 4.5 → 触发振动异常检测工作流
   - 温度 > 60 → 触发温度过热告警工作流
   - 功率 > 100 → 触发功率预测调度工作流
   - 电流 > 50 → 触发电流过载检测工作流

**适配的网关品牌：** 硕物、有人、研华、古瑞、锐谷、峰峦、各品牌工业网关

### 方式二：Modbus 直连

如果网关本身不支持 MQTT，EdgeAgent Hub 可以直接作为 Modbus 主站读取网关采集的数据。

**步骤：**

1. 在 `config.yaml` 中配置 Modbus 设备：
   ```yaml
   protocols:
     modbus:
       enabled: true
       devices:
         - device_id: "gateway_01"
           host: "192.168.1.200"  # 网关 IP
           port: 502
           unit_id: 1
           poll_interval: 1s
           registers:
             - name: "vibration"
               address: 0
               quantity: 1
               func_code: 3
               data_type: "float32"
               scale: 0.01
               unit: "mm/s"
             - name: "temperature"
               address: 2
               quantity: 1
               func_code: 3
               data_type: "float32"
               scale: 0.1
               unit: "°C"
   ```

2. 重启服务，EdgeAgent Hub 会自动轮询寄存器并触发对应工作流。

### 方式三：OPC UA 对接

适用于西门子、施耐德、ABB 等 PLC 直接对接，或通过 OPC UA 服务器中转。

**步骤：**

1. 在 `config.yaml` 中配置 OPC UA：
   ```yaml
   protocols:
     opcua:
       enabled: true
       devices:
         - device_id: "plc_siemens_01"
           endpoint: "opc.tcp://192.168.1.50:4840"
           security: "None"
           nodes:
             - node_id: "ns=2;s=Temperature"
               name: "temperature"
             - node_id: "ns=2;s=Vibration"
               name: "vibration"
   ```

### 方式四：HTTP Webhook 对接

如果网关支持 HTTP 推送，可以直接调用 EdgeAgent Hub 的 API：

```bash
curl -X POST http://192.168.1.100:9090/api/v1/data/ingest \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <token>" \
  -d '{
    "device_id": "sensor_01",
    "type": "vibration",
    "value": 5.8,
    "unit": "mm/s"
  }'
```

### 对接后能做什么？

| 场景 | 触发条件 | 自动执行链路 |
|------|---------|-------------|
| 振动异常告警 | 振动 > 4.5 mm/s | ONNX 推理 → RAG 分析 → 钉钉/短信通知 |
| 温度过热告警 | 温度 > 60°C | ONNX 推理 → RAG 生成处置建议 → Webhook 通知 |
| 功率超限调度 | 功率 > 100 kW | ONNX 趋势预测 → RAG 生成调度建议 → Webhook 通知 |
| 电流过载检测 | 电流 > 50 A | ONNX 异常检测 → RAG 告警 → 短信通知 |
| 断网自治 | 网络中断 | 本地缓存数据 → 恢复后自动重传 |
| 智能问答 | 运维人员提问 | 知识库检索 → LLM 生成回答（RAG 锚定验证）|

---

---

## 常见问题

### Q: 启动报错 "Failed to connect to NATS"
NATS Server 未运行。Linux 用户可运行 `install.sh` 自动安装；或手动安装：
```bash
# Docker 方式
docker run -d --name nats -p 4222:4222 -p 8222:8222 nats:2.10-alpine -js -sd /data
```

### Q: LLM 推理很慢
- 确保使用 Q4 量化模型 (如 `qwen3-0.6b-q4_k_m.gguf`)
- 检查内存是否足够 (最低 1GB 可用)
- 资源适配器会自动检测并选择最优精度

### Q: 忘记密码怎么办？
如果遗忘 admin 密码，可以通过 CLI 重置：
```bash
edgeagent-hub -cli reset-password
```
或者在数据库中手动重置（需停止服务后操作）。

### Q: 如何接入 Modbus 设备？
在 `config.yaml` 的 `protocols.modbus` 中配置设备 IP、端口、寄存器地址，重启服务即可。

### Q: 断网后数据会丢失吗？
不会。EdgeAgent Hub 会将数据缓存在本地 SQLite + JetStream 中，网络恢复后自动重传。

### Q: 支持哪些 AI 模型格式？
- ONNX: `.onnx` 格式，支持 CPU/CUDA/TensorRT/OpenVINO/RKNN
- LLM: `.gguf` 格式 (llama.cpp)，支持 Q4_K_M / Q5_K_M / Q8 等量化

---

## License

MIT License

---

## 致谢

EdgeAgent Hub 站在巨人的肩膀上，感谢以下开源项目：

- [Go](https://go.dev) · [Echo](https://echo.labstack.com) · [NATS](https://nats.io)
- [Vue](https://vuejs.org) · [Element Plus](https://element-plus.org) · [ECharts](https://echarts.apache.org)
- [ONNX Runtime](https://onnxruntime.ai) · [llama.cpp](https://github.com/ggerganov/llama.cpp)
- [Prometheus](https://prometheus.io) · [Grafana](https://grafana.com)

---

## 生态项目

EdgeAgent Hub 是物网联边缘计算产品矩阵的一部分，以下项目共同构成了完整的工业物联网解决方案：

| 项目 | 简介 | 仓库地址 |
|------|------|---------|
| **ProtoForge** | 工业协议解析引擎，支持 Modbus/OPC UA/MQTT/ONVIF 等多协议统一解析与转换 | [Gitee](https://gitee.com/suoten/ProtoForge) · [GitHub](https://github.com/suoten/ProtoForge) |
| **EdgeLiteGateway** | 轻量级边缘网关（Python），适配 ARM 嵌入式设备，支持多协议采集与上云 | [Gitee](https://gitee.com/suoten/EdgeLiteGateway) · [GitHub](https://github.com/suoten/EdgeLiteGateway) |
| **EdgeLiteGateway-Go** | 高性能边缘网关（Go），吞吐量更高，适合 x86/ARM64 工业主机 | [Gitee](https://gitee.com/suoten/EdgeLiteGateway-Go) · [GitHub](https://github.com/suoten/EdgeLiteGateway-Go) |
| **EdgeAgent Hub** | 边缘智能体管理平台 — 边缘 AI 推理 + RAG + 多智能体编排（本项目） | [Gitee](https://gitee.com/suoten/edgeagent-hub) · [GitHub](https://github.com/suoten/EdgeAgent-Hub) |
| **GBDoctor** | 国标 GB/T 28181 协议诊断工具，摄像头/ NVR 接入测试与故障排查 | [Gitee](https://gitee.com/suoten/GBDoctor) · [GitHub](https://github.com/suoten/GBDoctor) |
| **PyGBSentry** | 开箱即用的国标 GB/T 28181-2022 视频管理平台 · 纯 Python 自研 SIP 栈 · FastAPI + Vue 3 + ZLMediaKit | [Gitee](https://gitee.com/suoten/PyGBSentry) · [GitHub](https://github.com/suoten/PyGBSentry) |

> **典型组合**：EdgeLiteGateway 采集传感器数据 → EdgeAgent Hub 边缘 AI 推理与告警 → PyGBSentry 接入国标摄像头视频确认 → GBDoctor 诊断排查 GB/T 28181 协议故障

---

<div align="center">

**EdgeAgent Hub — 工业边缘 AI，从此触手可及**

</div>
