#!/bin/bash
set -euo pipefail

SERVICE_NAME="cloud-print-agent"
CONFIG_FILE="/etc/cloud-print-agent/config.yaml"
BIN_PATH="/usr/local/bin/cloud-print-agent"
CPA_PATH="/usr/local/bin/cpa"
INSTALL_SCRIPT_DIR="/usr/local/lib/cloud-print-agent"

get_ops_port() {
    local port
    port=$(grep -A2 '^ops:' "$CONFIG_FILE" 2>/dev/null | grep 'port:' | awk '{print $2}' || true)
    if [ -z "$port" ]; then
        port=8901
    fi
    echo "$port"
}

get_cloud_endpoint() {
    local endpoint
    endpoint=$(grep -A3 '^cloud:' "$CONFIG_FILE" 2>/dev/null | grep 'endpoint:' | awk '{print $2}' || true)
    if [ -z "$endpoint" ]; then
        endpoint="unknown"
    fi
    echo "$endpoint"
}

check_dependencies() {
    for dep in systemctl curl; do
        if ! command -v "$dep" >/dev/null 2>&1; then
            echo "    缺少依赖: $dep"
            exit 1
        fi
    done
    echo "    依赖检查通过"
}

inject_credentials() {
    local token key
    if [ -n "${CPA_DEVICE_TOKEN:-}" ] && [ -n "${CPA_MASTER_KEY:-}" ]; then
        token="$CPA_DEVICE_TOKEN"
        key="$CPA_MASTER_KEY"
        echo "    使用环境变量凭证"
    else
        echo "    请粘贴 base64 编码的设备令牌 (CPA_DEVICE_TOKEN):"
        read -r token
        echo "    请粘贴 base64 编码的主密钥 (CPA_MASTER_KEY):"
        read -r key
    fi

    if [ -z "$token" ] || [ -z "$key" ]; then
        echo "    凭证不能为空"
        exit 1
    fi

    local env_file="/etc/cloud-print-agent/credentials.env"
    mkdir -p /etc/cloud-print-agent
    cat > "$env_file" <<EOF
CPA_DEVICE_TOKEN=$token
CPA_MASTER_KEY=$key
EOF
    chmod 600 "$env_file"
    if id -u debian >/dev/null 2>&1; then
        chown debian:debian "$env_file"
    fi
    echo "    凭证已写入 $env_file"
}

run_install_script() {
    local install_script=""
    if [ -f "${INSTALL_SCRIPT_DIR}/install.sh" ]; then
        install_script="${INSTALL_SCRIPT_DIR}/install.sh"
    elif [ -f "./install.sh" ]; then
        install_script="./install.sh"
    elif [ -f "$(dirname "$0")/install.sh" ]; then
        install_script="$(dirname "$0")/install.sh"
    else
        echo "    未找到 install.sh"
        exit 1
    fi
    bash "$install_script"
}

health_check() {
    local port
    port=$(get_ops_port)
    sleep 2
    if curl -sf "http://127.0.0.1:${port}/api/v1/healthz" >/dev/null 2>&1; then
        echo "    服务健康"
    else
        echo "    服务未就绪，请检查日志: journalctl -u $SERVICE_NAME"
        exit 1
    fi
}

cmd_install() {
    echo "==> [1/7] 检查依赖..."
    check_dependencies

    echo "==> [2/7] 创建目录/用户/拷贝二进制/配置/注册 systemd..."
    run_install_script

    echo "==> [3/7] 凭证注入..."
    inject_credentials

    echo "==> [4/7] 重载 systemd..."
    systemctl daemon-reload

    echo "==> [5/7] 启用服务..."
    systemctl enable "$SERVICE_NAME"

    echo "==> [6/7] 启动服务..."
    systemctl start "$SERVICE_NAME"

    echo "==> [7/7] 健康探针..."
    health_check

    echo "==> 安装完成"
    echo "    二进制: $BIN_PATH"
    echo "    配置:   $CONFIG_FILE"
    echo "    运维:   cpa status / cpa network / cpa logs"
}

cmd_uninstall() {
    echo "==> 停止服务..."
    systemctl stop "$SERVICE_NAME" 2>/dev/null || true
    systemctl disable "$SERVICE_NAME" 2>/dev/null || true

    echo "==> 移除文件..."
    rm -f "$BIN_PATH"
    rm -f "$CPA_PATH"
    rm -f "/etc/systemd/system/${SERVICE_NAME}.service"
    rm -f /etc/sudoers.d/cloud-print-agent
    rm -rf "$INSTALL_SCRIPT_DIR"
    rm -rf /etc/cloud-print-agent
    rm -rf /var/lib/cloud-print-agent
    rm -rf /var/log/cloud-print-agent

    echo "==> 重载 systemd..."
    systemctl daemon-reload

    echo "==> 卸载完成"
}

cmd_restart() {
    systemctl restart "$SERVICE_NAME"
    echo "服务已重启"
}

cmd_status() {
    local port
    port=$(get_ops_port)
    local endpoint
    endpoint=$(get_cloud_endpoint)

    echo "=== Cloud Print Agent 状态 ==="
    echo "云端域名: $endpoint"
    echo

    local resp
    resp=$(curl -sf "http://127.0.0.1:${port}/api/v1/status" 2>/dev/null) || {
        echo "无法获取状态，服务可能未运行"
        exit 1
    }

    echo "$resp" | python3 -c "
import sys, json
d = json.load(sys.stdin)
print(f\"状态:     {d.get('status', 'unknown')}\")
print(f\"版本:     {d.get('version', 'unknown')}\")
print(f\"运行时长: {d.get('process_uptime', 'unknown')}\")
print(f\"云端地址: {d.get('derived_url', 'unknown')}\")
print(f\"网络分类: {d.get('net_class', 'unknown')}\")
print(f\"设备总数: {d.get('device_count', 0)}\")
print(f\"在线设备: {d.get('online_devices', 0)}\")
print(f\"待处理任务: {d.get('pending_tasks', 0)}\")
" 2>/dev/null || echo "$resp"
}

cmd_network() {
    local port
    port=$(get_ops_port)
    local endpoint
    endpoint=$(get_cloud_endpoint)

    echo "=== 网络诊断 ==="
    echo "云端域名: $endpoint"
    echo

    local resp
    resp=$(curl -sf "http://127.0.0.1:${port}/api/v1/network" 2>/dev/null) || {
        echo "无法获取网络状态，服务可能未运行"
        exit 1
    }

    echo "$resp" | python3 -c "
import sys, json
d = json.load(sys.stdin)
print(f\"云端域名:   {d.get('endpoint', 'unknown')}\")
print(f\"解析IP:     {d.get('resolved_ip', 'unknown')}\")
print(f\"DNS耗时:    {d.get('dns_latency_ms', 0)} ms\")
print(f\"网关连通:   {'是' if d.get('gateway_reach') else '否'}\")
print(f\"本地网络:   {'正常' if d.get('local_net_reach') else '异常'}\")
print(f\"故障分类:   {d.get('net_class', 'unknown')}\")
" 2>/dev/null || echo "$resp"
}

cmd_logs() {
    journalctl -u "$SERVICE_NAME" -f
}

usage() {
    cat <<EOF
用法: cpa <command>

命令:
  install     安装并启动 Cloud Print Agent
  uninstall   卸载 Cloud Print Agent
  restart     重启服务
  status      查看服务状态 (含云端域名)
  network     网络诊断 (云端域名/解析IP/网关连通性/故障分类/DNS耗时)
  logs        查看日志 (实时跟踪)

环境变量 (非交互式凭证注入):
  CPA_DEVICE_TOKEN  base64 编码的设备令牌
  CPA_MASTER_KEY    base64 编码的主密钥
EOF
}

main() {
    if [ $# -eq 0 ]; then
        usage
        exit 1
    fi

    local cmd="$1"
    case "$cmd" in
        install)   cmd_install ;;
        uninstall) cmd_uninstall ;;
        restart)   cmd_restart ;;
        status)    cmd_status ;;
        network)   cmd_network ;;
        logs)      cmd_logs ;;
        -h|--help) usage ;;
        *)         echo "未知命令: $cmd"; echo; usage; exit 1 ;;
    esac
}

main "$@"