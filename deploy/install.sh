#!/bin/bash
set -euo pipefail

SERVICE_NAME="cloud-print-agent"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"
SUDOERS_FILE="/etc/sudoers.d/cloud-print-agent"
SUDOERS_SRC="sudoers.cloud-print-agent"
CONFIG_DIR="/etc/cloud-print-agent"
DATA_DIR="/var/lib/cloud-print-agent"
LOG_DIR="/var/log/cloud-print-agent"
LIB_DIR="/usr/local/lib/cloud-print-agent"
BIN_PATH="/usr/local/bin/cloud-print-agent"
CPA_PATH="/usr/local/bin/cpa"
USER="debian"
GROUP="debian"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "==> 创建用户/组..."
if ! getent group "$GROUP" >/dev/null 2>&1; then
    groupadd --system "$GROUP"
    echo "    创建组: $GROUP"
fi
if ! id -u "$USER" >/dev/null 2>&1; then
    useradd --system --no-create-home --shell /usr/sbin/nologin --gid "$GROUP" "$USER"
    echo "    创建用户: $USER"
fi

echo "==> 创建目录..."
mkdir -p "$CONFIG_DIR" "$DATA_DIR" "$LOG_DIR" "$LIB_DIR"
chown -R "${USER}:${GROUP}" "$CONFIG_DIR" "$DATA_DIR" "$LOG_DIR"
chmod 700 "$CONFIG_DIR"
echo "    $CONFIG_DIR"
echo "    $DATA_DIR"
echo "    $LOG_DIR"

echo "==> 拷贝二进制..."
BIN_SRC=""
for candidate in "$SCRIPT_DIR/cloud-print-agent" "./cloud-print-agent" "/tmp/cloud-print-agent"; do
    if [ -f "$candidate" ]; then
        BIN_SRC="$candidate"
        break
    fi
done
if [ -z "$BIN_SRC" ]; then
    echo "    未找到二进制文件，请放置 ./cloud-print-agent 或 /tmp/cloud-print-agent"
    exit 1
fi
cp "$BIN_SRC" "$BIN_PATH"
chmod 755 "$BIN_PATH"
echo "    $BIN_PATH (from $BIN_SRC)"

echo "==> 拷贝配置..."
CONFIG_SRC=""
for candidate in "$SCRIPT_DIR/config.yaml" "./config.yaml"; do
    if [ -f "$candidate" ]; then
        CONFIG_SRC="$candidate"
        break
    fi
done
if [ -n "$CONFIG_SRC" ]; then
    cp "$CONFIG_SRC" "${CONFIG_DIR}/config.yaml"
    echo "    ${CONFIG_DIR}/config.yaml (from $CONFIG_SRC)"
elif [ -f "${CONFIG_DIR}/config.yaml" ]; then
    echo "    配置已存在，跳过"
else
    echo "    未找到 config.yaml，请放置 ./config.yaml"
    exit 1
fi
chown "${USER}:${GROUP}" "${CONFIG_DIR}/config.yaml"
chmod 640 "${CONFIG_DIR}/config.yaml"

echo "==> 安装 systemd 服务单元..."
SERVICE_SRC=""
for candidate in "$SCRIPT_DIR/cloud-print-agent.service" "./cloud-print-agent.service"; do
    if [ -f "$candidate" ]; then
        SERVICE_SRC="$candidate"
        break
    fi
done
if [ -z "$SERVICE_SRC" ]; then
    echo "    未找到 cloud-print-agent.service"
    exit 1
fi
cp "$SERVICE_SRC" "$SERVICE_FILE"
chmod 644 "$SERVICE_FILE"
echo "    $SERVICE_FILE"

echo "==> 安装 sudoers..."
SUDOERS_SRC_FILE=""
for candidate in "$SCRIPT_DIR/$SUDOERS_SRC" "./$SUDOERS_SRC"; do
    if [ -f "$candidate" ]; then
        SUDOERS_SRC_FILE="$candidate"
        break
    fi
done
if [ -z "$SUDOERS_SRC_FILE" ]; then
    echo "    未找到 $SUDOERS_SRC"
    exit 1
fi
cp "$SUDOERS_SRC_FILE" "$SUDOERS_FILE"
chmod 440 "$SUDOERS_FILE"
echo "    $SUDOERS_FILE"

echo "==> 安装 cpa 脚本..."
CPA_SRC=""
for candidate in "$SCRIPT_DIR/cpa.sh" "./cpa.sh"; do
    if [ -f "$candidate" ]; then
        CPA_SRC="$candidate"
        break
    fi
done
if [ -z "$CPA_SRC" ]; then
    echo "    未找到 cpa.sh"
    exit 1
fi
cp "$CPA_SRC" "$CPA_PATH"
chmod 755 "$CPA_PATH"
echo "    $CPA_PATH"

echo "==> 安装 install.sh..."
cp "$0" "${LIB_DIR}/install.sh"
chmod 755 "${LIB_DIR}/install.sh"
echo "    ${LIB_DIR}/install.sh"

echo "==> 重载 systemd 并启用服务..."
systemctl daemon-reload
systemctl enable "$SERVICE_NAME"

echo "==> 安装脚本执行完成"