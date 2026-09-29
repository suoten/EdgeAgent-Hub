#!/bin/bash
# ============================================================
# EdgeAgent Hub - Linux 卸载脚本
# 用法: sudo bash uninstall.sh
# ============================================================
set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
NC='\033[0m'

print_ok()   { echo -e "${GREEN}[OK]${NC}   $1"; }
print_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
print_info() { echo -e "${CYAN}[INFO]${NC} $1"; }
print_err()  { echo -e "${RED}[ERROR]${NC} $1"; }

if [ "$EUID" -ne 0 ]; then
    print_err "请以 root 权限运行: sudo bash uninstall.sh"
    exit 1
fi

INSTALL_DIR="/opt/edgeagent-hub"
CONFIG_DIR="/etc/edgeagent-hub"
DATA_DIR="/var/lib/edgeagent-hub"
LOG_DIR="/var/log/edgeagent-hub"
SERVICE_USER="edgeagent"
SERVICE_NAME="edgeagent-hub"

echo ""
echo "=========================================="
echo -e "${RED}  EdgeAgent Hub 卸载程序${NC}"
echo "=========================================="
echo ""

# 询问是否保留数据
read -p "是否保留数据目录和配置? [y/N] " KEEP_DATA
KEEP_DATA=$(echo "$KEEP_DATA" | tr '[:upper:]' '[:lower:]')

# 停止服务
print_info "停止服务..."
systemctl stop "$SERVICE_NAME" 2>/dev/null || true
systemctl disable "$SERVICE_NAME" 2>/dev/null || true
rm -f "/etc/systemd/system/${SERVICE_NAME}.service"
systemctl daemon-reload
print_ok "服务已停止并移除"

# 删除二进制
if [ -d "$INSTALL_DIR" ]; then
    rm -rf "$INSTALL_DIR"
    print_ok "已删除: $INSTALL_DIR"
fi

# 删除配置和数据
if [ "$KEEP_DATA" = "y" ] || [ "$KEEP_DATA" = "yes" ]; then
    print_warn "保留配置目录: $CONFIG_DIR"
    print_warn "保留数据目录: $DATA_DIR"
    print_warn "保留日志目录: $LOG_DIR"
else
    if [ -d "$CONFIG_DIR" ]; then
        rm -rf "$CONFIG_DIR"
        print_ok "已删除: $CONFIG_DIR"
    fi
    if [ -d "$DATA_DIR" ]; then
        rm -rf "$DATA_DIR"
        print_ok "已删除: $DATA_DIR"
    fi
    if [ -d "$LOG_DIR" ]; then
        rm -rf "$LOG_DIR"
        print_ok "已删除: $LOG_DIR"
    fi
fi

# 删除服务用户
if id "$SERVICE_USER" &>/dev/null; then
    userdel "$SERVICE_USER" 2>/dev/null || true
    print_ok "已删除用户: $SERVICE_USER"
fi

echo ""
echo "=========================================="
echo -e "${GREEN}  EdgeAgent Hub 卸载完成${NC}"
echo "=========================================="
echo ""
