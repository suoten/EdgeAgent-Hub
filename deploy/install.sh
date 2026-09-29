#!/bin/bash
# ============================================================
# EdgeAgent Hub - Linux 一键安装脚本
# 用法: sudo bash install.sh
# ============================================================
set -e

# ── 颜色输出 ──
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

print_ok()   { echo -e "${GREEN}[OK]${NC}   $1"; }
print_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
print_info() { echo -e "${CYAN}[INFO]${NC} $1"; }
print_err()  { echo -e "${RED}[ERROR]${NC} $1"; }

# ── 检查 root ──
if [ "$EUID" -ne 0 ]; then
    print_err "请以 root 权限运行: sudo bash install.sh"
    exit 1
fi

# ── 变量 ──
INSTALL_DIR="/opt/edgeagent-hub"
CONFIG_DIR="/etc/edgeagent-hub"
DATA_DIR="/var/lib/edgeagent-hub"
LOG_DIR="/var/log/edgeagent-hub"
SERVICE_USER="edgeagent"
SERVICE_NAME="edgeagent-hub"
BINARY_NAME="edgeagent-hub"

# 脚本所在目录
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo ""
echo "=========================================="
echo -e "${CYAN}  EdgeAgent Hub - 边缘智能体管理平台${NC}"
echo -e "${CYAN}  Linux 安装程序${NC}"
echo "=========================================="
echo ""

# ============================================================
# 1. 检查二进制文件
# ============================================================
print_info "检查安装文件..."

BINARY=""
if [ -f "$SCRIPT_DIR/$BINARY_NAME" ]; then
    BINARY="$SCRIPT_DIR/$BINARY_NAME"
elif [ -f "$SCRIPT_DIR/${BINARY_NAME}-linux-amd64" ]; then
    BINARY="$SCRIPT_DIR/${BINARY_NAME}-linux-amd64"
elif [ -f "$SCRIPT_DIR/${BINARY_NAME}-linux-arm64" ]; then
    BINARY="$SCRIPT_DIR/${BINARY_NAME}-linux-arm64"
else
    print_err "找不到 $BINARY_NAME 二进制文件"
    print_err "请确保在解压后的目录中运行此脚本"
    exit 1
fi
print_ok "找到二进制文件: $BINARY"

# 检查架构
ARCH=$(uname -m)
if [ "$ARCH" = "x86_64" ]; then
    EXPECTED_ARCH="amd64"
elif [ "$ARCH" = "aarch64" ] || [ "$ARCH" = "arm64" ]; then
    EXPECTED_ARCH="arm64"
else
    print_warn "未知的 CPU 架构: $ARCH，继续安装..."
    EXPECTED_ARCH=""
fi

if [ -n "$EXPECTED_ARCH" ] && echo "$BINARY" | grep -q "arm64\|amd64"; then
    if ! echo "$BINARY" | grep -q "$EXPECTED_ARCH"; then
        print_err "架构不匹配: 当前系统 $ARCH 需要 $EXPECTED_ARCH 二进制"
        print_err "请下载正确的版本"
        exit 1
    fi
fi

# ============================================================
# 2. 创建服务用户
# ============================================================
print_info "创建服务用户: $SERVICE_USER"
if id "$SERVICE_USER" &>/dev/null; then
    print_ok "用户 $SERVICE_USER 已存在"
else
    useradd --system --no-create-home --shell /usr/sbin/nologin "$SERVICE_USER"
    print_ok "用户 $SERVICE_USER 创建成功"
fi

# ============================================================
# 3. 创建目录
# ============================================================
print_info "创建目录结构..."
mkdir -p "$INSTALL_DIR"
mkdir -p "$CONFIG_DIR"
mkdir -p "$DATA_DIR/knowledge"
mkdir -p "$DATA_DIR/vectordb"
mkdir -p "$DATA_DIR/ota"
mkdir -p "$DATA_DIR/models/onnx"
mkdir -p "$DATA_DIR/models/llm"
mkdir -p "$DATA_DIR/models/partition_a"
mkdir -p "$DATA_DIR/models/partition_b"
mkdir -p "$LOG_DIR"
mkdir -p "$CONFIG_DIR/workflows"
print_ok "目录结构创建完成"

# ============================================================
# 4. 复制二进制文件
# ============================================================
print_info "安装二进制文件..."
cp "$BINARY" "$INSTALL_DIR/$BINARY_NAME"
chmod +x "$INSTALL_DIR/$BINARY_NAME"
print_ok "二进制已安装到 $INSTALL_DIR/$BINARY_NAME"

# ============================================================
# 5. 复制配置文件
# ============================================================
print_info "配置文件..."
if [ -f "$SCRIPT_DIR/config.yaml" ]; then
    # 如果已有配置，备份
    if [ -f "$CONFIG_DIR/config.yaml" ]; then
        cp "$CONFIG_DIR/config.yaml" "$CONFIG_DIR/config.yaml.bak.$(date +%Y%m%d%H%M%S)"
        print_warn "已有配置已备份为 config.yaml.bak.*"
    fi
    cp "$SCRIPT_DIR/config.yaml" "$CONFIG_DIR/config.yaml"
    print_ok "配置文件已复制到 $CONFIG_DIR/config.yaml"
else
    # 生成默认配置
    "$INSTALL_DIR/$BINARY_NAME" init -config "$CONFIG_DIR/config.yaml" 2>/dev/null || true
    if [ ! -f "$CONFIG_DIR/config.yaml" ]; then
        print_warn "未找到 config.yaml，将在首次启动时自动生成"
    fi
fi

# 创建环境变量文件
if [ -f "$SCRIPT_DIR/edgeagent-hub-env" ]; then
    cp "$SCRIPT_DIR/edgeagent-hub-env" "$CONFIG_DIR/env"
else
    cat > "$CONFIG_DIR/env" << 'EOF'
# EdgeAgent Hub 环境变量
# 配置文件路径
HUB_CONFIG=/etc/edgeagent-hub/config.yaml
# 时区
TZ=Asia/Shanghai
EOF
fi
print_ok "环境变量文件已创建: $CONFIG_DIR/env"

# ============================================================
# 6. 修改配置文件中的路径 (适配安装路径)
# ============================================================
print_info "适配安装路径..."
if [ -f "$CONFIG_DIR/config.yaml" ]; then
    # 使用 sed 替换路径
    sed -i \
        -e "s|db_path: \"data/|db_path: \"$DATA_DIR/|g" \
        -e "s|knowledge_dir: \"data/|knowledge_dir: \"$DATA_DIR/|g" \
        -e "s|vectordb_dir: \"data/|vectordb_dir: \"$DATA_DIR/|g" \
        -e "s|model_dir: \"models/|model_dir: \"$DATA_DIR/models/|g" \
        -e "s|model_path: \"models/|model_path: \"$DATA_DIR/models/|g" \
        -e "s|partition_a: \"models/|partition_a: \"$DATA_DIR/models/|g" \
        -e "s|partition_b: \"models/|partition_b: \"$DATA_DIR/models/|g" \
        -e "s|log_file: \"logs/|log_file: \"$LOG_DIR/|g" \
        -e "s|workflow_dir: \"workflows\"|workflow_dir: \"$CONFIG_DIR/workflows\"|g" \
        "$CONFIG_DIR/config.yaml"
    print_ok "配置路径已适配"
fi

# ============================================================
# 7. 复制工作流示例
# ============================================================
print_info "工作流配置..."
WORKFLOW_SOURCE=""
if [ -d "$SCRIPT_DIR/workflows" ]; then
    WORKFLOW_SOURCE="$SCRIPT_DIR/workflows"
elif [ -d "$(dirname $SCRIPT_DIR)/workflows" ]; then
    WORKFLOW_SOURCE="$(dirname $SCRIPT_DIR)/workflows"
fi

if [ -n "$WORKFLOW_SOURCE" ]; then
    cp -r "$WORKFLOW_SOURCE"/*.yaml "$CONFIG_DIR/workflows/" 2>/dev/null || true
    print_ok "工作流示例已复制"
else
    print_warn "未找到工作流示例目录"
fi

# ============================================================
# 8. 设置权限
# ============================================================
print_info "设置权限..."
chown -R "$SERVICE_USER:$SERVICE_USER" "$DATA_DIR" "$LOG_DIR"
chown -R root:"$SERVICE_USER" "$CONFIG_DIR"
chmod 750 "$CONFIG_DIR"
chmod 640 "$CONFIG_DIR/config.yaml" "$CONFIG_DIR/env" 2>/dev/null || true
print_ok "权限设置完成"

# ============================================================
# 9. 安装 systemd 服务
# ============================================================
print_info "安装 systemd 服务..."

# 如果有打包的 service 文件就用，否则生成
if [ -f "$SCRIPT_DIR/edgeagent-hub.service" ]; then
    cp "$SCRIPT_DIR/edgeagent-hub.service" "/etc/systemd/system/${SERVICE_NAME}.service"
else
    cat > "/etc/systemd/system/${SERVICE_NAME}.service" << EOF
[Unit]
Description=EdgeAgent Hub - 边缘智能体管理平台
Documentation=https://github.com/edgelite/edgeagent-hub
After=network.target nats-server.service
Wants=network.target

[Service]
Type=simple
User=$SERVICE_USER
Group=$SERVICE_USER
EnvironmentFile=$CONFIG_DIR/env
ExecStart=$INSTALL_DIR/$BINARY_NAME -config $CONFIG_DIR/config.yaml
WorkingDirectory=$INSTALL_DIR
Restart=always
RestartSec=5
LimitNOFILE=65535

# 安全加固
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=$DATA_DIR $LOG_DIR $CONFIG_DIR
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true

# 资源限制
LimitNPROC=65535

StandardOutput=journal
StandardError=journal
SyslogIdentifier=$SERVICE_NAME

[Install]
WantedBy=multi-user.target
EOF
fi

# 环境变量文件路径修正
if [ -f "$CONFIG_DIR/env" ]; then
    sed -i "s|HUB_CONFIG=.*|HUB_CONFIG=$CONFIG_DIR/config.yaml|" "$CONFIG_DIR/env"
fi

systemctl daemon-reload
systemctl enable "$SERVICE_NAME"
print_ok "systemd 服务已安装并启用自启动"

# ============================================================
# 10. 检查 NATS 依赖
# ============================================================
print_info "检查 NATS 依赖..."
if systemctl is-active --quiet nats-server 2>/dev/null; then
    print_ok "NATS 服务已在运行"
elif command -v nats-server &>/dev/null; then
    print_warn "NATS 已安装但未运行，请启动: systemctl start nats-server"
else
    print_warn "NATS 未安装"
    print_info "正在尝试安装 NATS..."
    
    # 尝试通过包管理器安装
    if command -v apt-get &>/dev/null; then
        # 下载并安装 NATS
        NATS_VERSION="0.25.6"
        ARCH_NATS=$(dpkg --print-architecture)
        NATS_URL=""
        if [ "$ARCH_NATS" = "amd64" ]; then
            NATS_URL="https://github.com/nats-io/nats-server/releases/download/v${NATS_VERSION}/nats-server-v${NATS_VERSION}-linux-amd64.tar.gz"
        elif [ "$ARCH_NATS" = "arm64" ]; then
            NATS_URL="https://github.com/nats-io/nats-server/releases/download/v${NATS_VERSION}/nats-server-v${NATS_VERSION}-linux-arm64.tar.gz"
        fi
        
        if [ -n "$NATS_URL" ]; then
            print_info "下载 NATS v$NATS_VERSION..."
            curl -fsSL "$NATS_URL" -o /tmp/nats-server.tar.gz && \
                tar -xzf /tmp/nats-server.tar.gz -C /tmp && \
                mv /tmp/nats-server-v${NATS_VERSION}-linux-*/nats-server /usr/local/bin/ && \
                rm -rf /tmp/nats-server* && \
                chmod +x /usr/local/bin/nats-server
            
            # 创建 NATS systemd 服务
            cat > /etc/systemd/system/nats-server.service << 'EOF'
[Unit]
Description=NATS Server
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/nats-server -js -sd /var/lib/nats
Restart=always
RestartSec=5
LimitNOFILE=65536
User=nats
Group=nats

[Install]
WantedBy=multi-user.target
EOF
            useradd --system --no-create-home --shell /usr/sbin/nologin nats 2>/dev/null || true
            mkdir -p /var/lib/nats
            chown -R nats:nats /var/lib/nats
            systemctl daemon-reload
            systemctl enable nats-server
            systemctl start nats-server
            print_ok "NATS Server 安装并启动成功"
        else
            print_warn "无法确定 NATS 下载 URL，请手动安装 NATS"
        fi
    else
        print_warn "不支持的包管理器，请手动安装 NATS Server"
    fi
fi

# ============================================================
# 11. 初始化数据库
# ============================================================
print_info "初始化数据库..."
su -s /bin/bash "$SERVICE_USER" -c "$INSTALL_DIR/$BINARY_NAME init -config $CONFIG_DIR/config.yaml" 2>/dev/null || true
print_ok "数据库初始化完成"

# ============================================================
# 12. 启动服务
# ============================================================
print_info "启动服务..."
systemctl start "$SERVICE_NAME"
sleep 2

if systemctl is-active --quiet "$SERVICE_NAME"; then
    print_ok "服务已启动"
else
    print_err "服务启动失败，请检查日志:"
    print_err "  journalctl -u $SERVICE_NAME -f"
    exit 1
fi

# ============================================================
# 13. 输出结果
# ============================================================
echo ""
echo "=========================================="
echo -e "${GREEN}  EdgeAgent Hub 安装成功!${NC}"
echo "=========================================="
echo ""
echo "安装路径:     $INSTALL_DIR/$BINARY_NAME"
echo "配置文件:     $CONFIG_DIR/config.yaml"
echo "数据目录:     $DATA_DIR"
echo "日志目录:     $LOG_DIR"
echo ""
echo "服务管理:"
echo "  启动:       systemctl start $SERVICE_NAME"
echo "  停止:       systemctl stop $SERVICE_NAME"
echo "  重启:       systemctl restart $SERVICE_NAME"
echo "  状态:       systemctl status $SERVICE_NAME"
echo "  日志:       journalctl -u $SERVICE_NAME -f"
echo ""
echo -e "${YELLOW}默认管理员账号: admin${NC}"
echo -e "${YELLOW}默认管理员密码: admin123${NC}"
echo -e "${YELLOW}请登录后立即修改密码!${NC}"
echo ""
echo -e "${CYAN}访问地址: http://$(hostname -I 2>/dev/null | awk '{print $1}' || echo 'localhost'):8080${NC}"
echo ""
echo "首次使用建议:"
echo "  1. 登录 Web 管理界面修改默认密码"
echo "  2. 编辑 $CONFIG_DIR/config.yaml 配置 MQTT/NATS 等"
echo "  3. 将 ONNX 模型放入 $DATA_DIR/models/onnx/"
echo "  4. 运行自检: $INSTALL_DIR/$BINARY_NAME selftest -config $CONFIG_DIR/config.yaml"
echo ""
