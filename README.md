# Cloud Print Agent - 云打印代理

[![Version](https://img.shields.io/badge/version-v1.1.0-blue.svg)](https://github.com/wujupeng/Cloud-Print-Agent/releases/tag/v1.1.0)
[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8.svg)](https://go.dev/)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

> 部署在工厂本地的轻量级云打印代理，通过 WebSocket Secure (WSS) 连接云打印平台，接收云端打印任务并分发至本地打印机执行。

## 目录

- [系统概述](#系统概述)
- [系统架构](#系统架构)
- [核心功能](#核心功能)
- [支持的硬件设备](#支持的硬件设备)
- [项目结构](#项目结构)
- [部署方式](#部署方式)
- [部署教程](#部署教程)
- [配置说明](#配置说明)
- [使用指南](#使用指南)
- [运维手册](#运维手册)
- [故障排查](#故障排查)
- [API 接口](#api-接口)
- [版本历史](#版本历史)

---

## 系统概述

Cloud Print Agent 是华为云码道(CodeArts)云打印系统的工厂端组件。它以 systemd 服务形式运行在工厂本地服务器上，通过 WSS 长连接与云打印平台通信，实现以下核心流程：

```
用户在 Web 端创建打印任务 → 云打印平台分发任务 → Agent 接收并执行 → 打印机输出 → Agent 上报结果
```

### 网络拓扑

```
┌──────────────┐     WSS/443      ┌──────────────┐    反向代理    ┌──────────────────┐
│  云打印平台   │ ◄────────────── ► │  网关/Nginx  │ ◄─────────── ► │  Cloud Print Agent │
│ print.oascii.com│                │ 210.22.123.254│               │  192.168.2.40     │
└──────────────┘                  └──────────────┘                └────────┬─────────┘
                                                                        │
                                              ┌─────────────────────────┼─────────────────────────┐
                                              │                         │                         │
                                    ┌─────────▼────────┐    ┌─────────▼────────┐    ┌─────────▼────────┐
                                    │  EPSON LQ-630KII  │    │  Canon iR-ADV    │    │  Deli888 标签    │
                                    │  192.168.2.81:9100│    │  C3530           │    │  192.168.2.118   │
                                    │  (RAW 协议)       │    │  192.168.0.231   │    │  :9100 (RAW)     │
                                    └──────────────────┘    └──────────────────┘    └3└──────────────────┘
```

---

## 系统架构

### 组件关系

```
Cloud Print Agent (本仓库)
    ├── cloudlink     — 云端 WSS 连接管理（握手/心跳/重连/消息收发）
    ├── executor      — 打印任务执行引擎（队列/重试/超时控制）
    ├── protocol      — 多协议打印适配器（RAW/LPR/IPP/CUPS）
    ├── device        — 设备管理与健康探测
    ├── taskqueue     — 本地持久化任务队列（bbolt 嵌入式 KV）
    ├── config        — 配置加载与热校验
    ├── credential    — 凭证加密存储
    ├── netprobe      — 网络拓扑探测（DNS/网关/本地链路）
    ├── opsapi        — 运维 HTTP 接口（状态/网络诊断/日志）
    ├── observability — 日志/审计/链路追踪
    └── lifecycle     — 生命周期管理（systemd 集成/信号处理）

Cloud Print Server (姊妹仓库: wujupeng/Cloud-Print-Server)
    提供 Web 管理界面、REST API、WSS Hub、任务调度、文档存储
```

### 通信协议

| 通道 | 协议 | 方向 | 用途 |
|------|------|------|------|
| 云端连接 | WSS (WebSocket Secure) | 双向 | 任务下发、状态上报、心跳保活 |
| 运维接口 | HTTP | 本地 | 状态查询、网络诊断、健康检查 |
| 打印通道 | RAW/LPR/IPP/CUPS | Agent → 打印机 | 实际打印数据发送 |

### 消息类型

| 消息 | 方向 | 说明 |
|------|------|------|
| `task.dispatch` | Server → Agent | 下发打印任务（含文档内容） |
| `task.ack` | Agent → Server | 任务接收确认 |
| `task.result` | Agent → Server | 任务执行结果上报 |
| `heartbeat` | Agent → Server | 心跳保活（含设备/网络状态） |
| `device.status` | Agent → Server | 设备状态变更上报 |
| `net.event` | Agent → Server | 网络故障事件上报 |
| `config.update` | Server → Agent | 配置下发更新 |
| `config.ack` | Agent → Server | 配置应用确认 |

---

## 核心功能

### 1. 多协议打印支持

| 协议 | 适用场景 | 实现方式 | 端口 |
|------|---------|---------|------|
| **RAW** | 针式打印机、标签打印机 | TCP 直连 9100 端口发送原始数据 | 9100 |
| **LPR** | 传统网络打印机 | RFC 1179 LPD 协议 | 515 |
| **IPP** | 现代网络打印机 | RFC 8011 IPP 协议 | 631 |
| **CUPS** | 需要驱动转换的打印机（如 Canon UFR II） | 调用 `lp` 命令通过本地 CUPS 打印 | - |

### 2. 任务执行引擎

- **本地持久化队列**：基于 bbolt 嵌入式 KV 存储，断电不丢任务
- **自动重试**：失败任务自动重试，最多 3 次，指数退避（初始 5s，上限 300s）
- **超时控制**：单次打印发送超时 60s
- **并发安全**：单任务串行执行，避免打印机冲突

### 3. 云端连接管理

- **WSS 自动重连**：断线后自动重连，支持指数退避
- **心跳保活**：30s 间隔发送心跳，90s 超时检测
- **网络拓扑探测**：实时检测 DNS 解析、网关连通性、本地网络状态
- **故障分类**：自动分类网络故障（本地网络故障/DNS失败/网关不可达）

### 4. 设备管理

- **多设备支持**：单 Agent 可管理多台打印机
- **健康探测**：定期探测设备在线状态
- **协议自动识别**：支持 RAW/LPR/IPP 端口探测
- **状态上报**：设备状态变更实时上报云端

### 5. 安全特性

- **凭证加密存储**：设备令牌使用 AES 加密存储
- **systemd 沙箱**：ProtectSystem=strict, ProtectHome=true, PrivateTmp=true
- **内存限制**：MemoryMax=200M
- **最小权限**：专用用户运行，sudoers 白名单

---

## 支持的硬件设备

### 已验证打印机

| 设备型号 | 类型 | 协议 | 连接方式 | 驱动/指令 | 状态 |
|---------|------|------|---------|---------|------|
| EPSON LQ-630KII | 针式打印机 | RAW | 192.168.2.81:9100 | 原始文本/ESC指令 | ✅ 已验证 |
| Canon iR-ADV C3530 | A4彩色复合机 | CUPS | socket://192.168.0.231:9100 | Canon UFR II v6.40 | ✅ 已验证 |
| Deli888 | 标签打印机 | RAW | 192.168.2.118:9100 | TSPL/TSPL2 指令 | ✅ 已验证 |

### 设备配置指南

#### 针式打印机（RAW 协议）

针式打印机通常支持 RAW/JetDirect 协议，直接通过 9100 端口发送文本或 ESC 指令。

```yaml
devices:
  - device_id: EPSON-LQ630KII
    name: EPSON针式打印机
    ip: 192.168.2.81
    model: EPSON LQ-630KII
    protocol: RAW
    port: 9100
```

#### Canon 复合机（CUPS + UFR II）

Canon imageRUNNER ADVANCE 系列使用 UFR II 专有页面描述语言，需安装 Canon UFR II Linux 驱动。

**安装 Canon UFR II 驱动：**

```bash
# 1. 下载驱动包（从 Canon 官网获取）
# linux-UFRII-drv-v640-m17n-04.tar.gz

# 2. 解压并安装
tar xzf linux-UFRII-drv-v640-m17n-04.tar.gz
sudo apt-get install -y libcupsimage2t64 cups-bsd
sudo dpkg -i linux-UFRII-drv-v640-m17n/x64/Debian/cnrdrvcups-ufr2-uk_6.40-1.04_amd64.deb

# 3. 查找正确的 PPD 文件
ls /usr/share/cups/model/CNRCUPSIRADVC* | while read f; do
  echo "$f: $(grep 'ShortNickName' "$f")"
done

# 4. 配置 CUPS 打印机（以 iR-ADV C3530 为例）
sudo lpadmin -p canon3530 -v socket://192.168.0.231:9100 \
  -m CNRCUPSIRADVC3525ZK.ppd -E
```

**Agent 配置：**

```yaml
devices:
  - device_id: Canon3530-A4
    name: Canon A4复合机
    ip: canon3530          # CUPS 打印机名称
    model: Canon iR-ADV C3530
    protocol: CUPS
```

> **注意**：`ip` 字段填 CUPS 打印机名称（`lpadmin -p` 指定的名称），不是 IP 地址。CUPS 协议跳过 IP 验证。

#### 标签打印机（RAW + TSPL）

标签打印机通常支持 TSPL/TSPL2 指令集，通过 RAW 协议直接发送指令到 9100 端口。

```yaml
devices:
  - device_id: Deli888-Label
    name: Deli888标签打印机
    ip: 192.168.2.118
    model: Deli888 80x50mm
    protocol: RAW
    port: 9100
```

**TSPL 指令示例（80mm×50mm 标签）：**

```
SIZE 80 mm,50 mm
GAP 2 mm,0 mm
CLS
TEXT 10,10,"3",0,1,1,"标签内容"
PRINT 1,1
```

### 添加新设备

1. **确认打印机网络连通性**：`ping <打印机IP>` 和 `nc -zv <打印机IP> 9100`
2. **确定协议类型**：
   - 针式/标签打印机 → RAW（端口 9100）
   - Canon 复合机 → CUPS（需安装 UFR II 驱动）
   - HP/Brother 激光机 → RAW 或 IPP
3. **添加 Agent 配置**：编辑 `/etc/cloud-print-agent/config.yaml`
4. **在云平台注册设备**：通过 Web 管理界面添加设备记录
5. **重启 Agent**：`sudo systemctl restart cloud-print-agent`

---

## 项目结构

```
Cloud-Print-Agent/
├── cmd/
│   ├── cloud-print-agent/         # 主程序入口
│   │   └── main.go
│   └── init-credential/           # 凭证初始化工具
│       └── main.go
├── internal/
│   ├── cloudlink/                 # 云端 WSS 连接
│   │   ├── client.go              #   连接客户端
│   │   ├── handshake.go           #   握手协议
│   │   ├── heartbeat.go           #   心跳保活
│   │   ├── reconnect.go           #   自动重连
│   │   ├── dispatcher.go          #   消息分发
│   │   ├── domain_update.go       #   设备状态上报
│   │   ├── reporter.go            #   任务结果上报
│   │   └── resolve.go             #   DNS 解析
│   ├── config/                    # 配置管理
│   ├── credential/                # 凭证加密
│   ├── device/                    # 设备管理
│   ├── domain/                    # 领域模型
│   ├── errs/                      # 错误定义
│   ├── executor/                  # 任务执行引擎
│   ├── lifecycle/                 # 生命周期
│   ├── netprobe/                  # 网络探测
│   ├── observability/             # 可观测性
│   ├── opsapi/                    # 运维 API
│   ├── protocol/                  # 打印协议适配器
│   │   ├── raw.go                 #   RAW/JetDirect
│   │   ├── lpr.go                 #   LPR/LPD
│   │   ├── ipp.go                 #   IPP
│   │   ├── cups.go                #   CUPS
│   │   └── probe.go               #   协议探测
│   ├── storage/                   # 持久化存储
│   ├── taskqueue/                 # 任务队列
│   └── updater/                   # 自动更新
├── deploy/                        # 部署文件
│   ├── install.sh                 #   安装脚本
│   ├── cpa.sh                     #   运维工具
│   ├── cloud-print-agent.service  #   systemd 服务
│   ├── config.example.yaml        #   配置示例
│   └── sudoers.cloud-print-agent  #   sudoers 白名单
├── test/                          # 测试
├── go.mod                         # Go 模块定义
└── go.sum                         # 依赖校验
```

---

## 部署方式

### 方式一：源码编译部署（推荐）

适用于工厂本地服务器，从源码编译并安装。

**前置条件：**
- Linux 服务器（Debian 12+/Ubuntu 22.04+）
- Go 1.22+
- 网络可访问云打印平台域名
- 网络可访问目标打印机

### 方式二：二进制直接部署

适用于无 Go 编译环境的服务器，直接使用预编译二进制。

### 方式三：配置热更新部署

适用于已部署环境，仅需更新配置文件并重启服务。

---

## 部署教程

### 完整部署流程（源码编译）

#### 1. 环境准备

```bash
# 安装 Go 1.22+
wget https://go.dev/dl/go1.22.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.22.0.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin

# 安装 CUPS（CUPS 协议打印机需要）
sudo apt-get install -y cups cups-client

# 安装 Canon UFR II 驱动（Canon 打印机需要）
# 参见"支持的硬件设备"章节
```

#### 2. 编译 Agent

```bash
# 克隆仓库
git clone https://github.com/wujupeng/Cloud-Print-Agent.git
cd Cloud-Print-Agent

# 编译
go build -o cloud-print-agent ./cmd/cloud-print-agent

# 验证
./cloud-print-agent -version
```

#### 3. 配置 Agent

```bash
# 创建配置目录
sudo mkdir -p /etc/cloud-print-agent

# 复制并编辑配置
sudo cp deploy/config.example.yaml /etc/cloud-print-agent/config.yaml
sudo nano /etc/cloud-print-agent/config.yaml
```

**配置文件示例：**

```yaml
config_version: 1
cloud:
  endpoint: print.oascii.com    # 云打印平台域名
  protocol: wss                  # 连接协议
  agent_id: BAOSHAN-AGENT-01    # Agent 实例 ID
ops:
  port: 8901                     # 运维接口端口
storage:
  data_dir: /var/lib/cloud-print-agent
  log_dir: /var/log/cloud-print-agent
log:
  level: info                    # 日志级别: debug/info/warn/error
  retention_days: 30
  max_size_mb: 100
devices:
  - device_id: 3aec85cd-8819-4cc0-8dee-343a166cca33
    name: EPSON-LQ630KII
    ip: 192.168.2.81
    model: EPSON LQ-630KII
    protocol: RAW
    port: 9100
    status: ONLINE
    factory: BAOSHAN
  - device_id: 4c36ba6c-0c99-49a2-ba45-3605d9c7991c
    name: Deli888-Label
    ip: 192.168.2.118
    model: Deli888 80x50mm
    protocol: RAW
    port: 9100
    status: ONLINE
    factory: BAOSHAN
  - device_id: f993e25b-198f-4e30-a355-91b3ddfda08e
    name: Canon3530-A4
    ip: canon3530                 # CUPS 打印机名称
    model: Canon iR-ADV C3530
    protocol: CUPS
    status: ONLINE
    factory: BAOSHAN
```

#### 4. 安装系统服务

```bash
# 使用安装脚本
sudo bash deploy/install.sh

# 或手动安装
sudo cp cloud-print-agent /usr/local/bin/
sudo cp deploy/cloud-print-agent.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable cloud-print-agent
```

#### 5. 注入凭证

```bash
# 创建凭证文件
sudo nano /etc/cloud-print-agent/credentials.env
```

```env
CPA_MASTER_KEY=<base64编码的主密钥>
CPA_DEVICE_TOKEN=<base64编码的设备令牌>
```

```bash
sudo chmod 600 /etc/cloud-print-agent/credentials.env
sudo chown debian:debian /etc/cloud-print-agent/credentials.env
```

#### 6. 启动服务

```bash
sudo systemctl start cloud-print-agent
sudo systemctl status cloud-print-agent
```

#### 7. 验证部署

```bash
# 检查服务状态
cpa status

# 检查网络连通性
cpa network

# 查看实时日志
cpa logs
```

### CUPS 打印机配置（Canon UFR II）

如果使用 CUPS 协议打印机（如 Canon），需要额外配置 CUPS：

```bash
# 1. 安装 Canon UFR II 驱动
sudo dpkg -i cnrdrvcups-ufr2-uk_6.40-1.04_amd64.deb

# 2. 查找正确的 PPD 文件
# iR-ADV C3530 对应 CNRCUPSIRADVC3525ZK.ppd (iR-ADV C3525/3530)
grep 'ShortNickName' /usr/share/cups/model/CNRCUPSIRADVC3525ZK.ppd

# 3. 配置 CUPS 打印机
sudo lpadmin -p canon3530 \
  -v socket://192.168.0.231:9100 \
  -m CNRCUPSIRADVC3525ZK.ppd \
  -E

# 4. 验证
lpstat -p canon3530
echo "test" | lp -d canon3530
```

---

## 配置说明

### 完整配置项

| 配置项 | 说明 | 默认值 | 约束 |
|--------|------|--------|------|
| `config_version` | 配置版本号 | 1 | 正整数 |
| `cloud.endpoint` | 云平台域名 | - | 禁止 IP，必须为域名 |
| `cloud.protocol` | 连接协议 | wss | wss/https |
| `cloud.agent_id` | Agent 实例 ID | - | 3-32 字符 |
| `ops.port` | 运维接口端口 | 8901 | 1024-65535 |
| `storage.data_dir` | 数据目录 | /var/lib/cloud-print-agent | 绝对路径 |
| `storage.log_dir` | 日志目录 | /var/log/cloud-print-agent | 绝对路径 |
| `log.level` | 日志级别 | info | debug/info/warn/error |
| `log.retention_days` | 日志保留天数 | 30 | 7-90 |
| `log.max_size_mb` | 单日志文件最大大小 | 100 | 正整数 |

### 固定运行参数（代码强制覆盖）

| 参数 | 值 | 说明 |
|------|-----|------|
| 心跳间隔 | 30s | 不可修改 |
| 最大重试次数 | 3 | 不可修改 |
| 重试初始延迟 | 5s | 不可修改 |
| 队列容量 | 100 | 不可修改 |
| 重试退避上限 | 300s | 不可修改 |
| 打印发送超时 | 60s | 不可修改 |
| 云端出站端口 | 443 | 不可修改 |

### 设备配置项

| 字段 | 说明 | 示例 |
|------|------|------|
| `device_id` | 设备唯一标识 | 3aec85cd-8819-4cc0-8dee-343a166cca33 |
| `name` | 设备名称 | EPSON-LQ630KII |
| `ip` | IP 地址或 CUPS 打印机名 | 192.168.2.81 或 canon3530 |
| `hostname` | 主机名（可选） | epson-printer |
| `model` | 设备型号 | EPSON LQ-630KII |
| `protocol` | 打印协议 | RAW/LPR/IPP/CUPS |
| `port` | 端口号（可选） | 9100 |
| `status` | 初始状态 | ONLINE |
| `factory` | 所属工厂 | BAOSHAN |

---

## 使用指南

### 运维工具 cpa

Agent 提供 `cpa` 命令行运维工具：

```bash
# 查看服务状态
cpa status
# 输出: 状态/版本/运行时长/云端地址/网络分类/设备总数/在线设备/待处理任务

# 网络诊断
cpa network
# 输出: 云端域名/解析IP/DNS耗时/网关连通/本地网络/故障分类

# 查看实时日志
cpa logs

# 重启服务
cpa restart

# 安装/卸载
cpa install
cpa uninstall
```

### 打印流程

#### 普通文档打印（针式/激光打印机）

1. 在云打印平台 Web 界面上传文档（支持 txt/pdf/doc 等格式）
2. 选择目标打印机和打印参数（份数/纸张/方向）
3. 创建打印任务
4. Agent 自动接收并执行打印
5. 在 Web 界面查看任务状态（PENDING → RUNNING → SUCCESS）

#### 标签打印（TSPL 指令）

1. 准备 TSPL 指令文件（.tspl）
2. 在云打印平台上传该文件
3. 选择标签打印机创建任务
4. Agent 通过 RAW 协议直接发送 TSPL 指令到打印机

**TSPL 指令模板：**

```
SIZE 80 mm,50 mm
GAP 2 mm,0 mm
CLS
TEXT 10,10,"3",0,1,1,"标签内容"
BARCODE 10,50,"128",80,2,0,2,2,"1234567890"
PRINT 1,1
```

### API 接口

Agent 运维接口（默认端口 8901）：

| 接口 | 方法 | 说明 |
|------|------|------|
| `/api/v1/healthz` | GET | 健康检查 |
| `/api/v1/status` | GET | 服务状态 |
| `/api/v1/network` | GET | 网络诊断 |

---

## 运维手册

### 日常运维命令

```bash
# 服务管理
sudo systemctl start cloud-print-agent      # 启动
sudo systemctl stop cloud-print-agent       # 停止
sudo systemctl restart cloud-print-agent    # 重启
sudo systemctl status cloud-print-agent     # 状态

# 日志查看
sudo journalctl -u cloud-print-agent -f                    # 实时日志
sudo journalctl -u cloud-print-agent --since "1 hour ago"  # 最近1小时
sudo journalctl -u cloud-print-agent -p err                # 仅错误日志

# 配置更新
sudo nano /etc/cloud-print-agent/config.yaml
sudo systemctl restart cloud-print-agent

# 设备管理
lpstat -p                    # 查看所有 CUPS 打印机
lpstat -p canon3530          # 查看指定打印机
lpstat -o                    # 查看打印队列
ping 192.168.2.81            # 测试打印机网络
```

### 添加新打印机

1. **确认网络连通**：
   ```bash
   ping <打印机IP>
   nc -zv <打印机IP> 9100    # RAW 端口
   ```

2. **编辑 Agent 配置**：
   ```bash
   sudo nano /etc/cloud-print-agent/config.yaml
   # 在 devices 列表添加新设备
   ```

3. **在云平台注册设备**：
   - 登录 Web 管理界面
   - 进入"设备管理"页面
   - 添加设备'设备记录（device_id 需与配置一致）
   - 为用户分配设备权限

4. **重启 Agent**：
   ```bash
   sudo systemctl restart cloud-print-agent
   ```

5. **验证**：
   ```bash
   cpa status    # 确认设备在线
   ```

### 更新 Agent

```bash
# 1. 编译新版本
cd Cloud-Print-Agent
git pull
go build -o cloud-print-agent ./cmd/cloud-print-agent

# 2. 停止服务
sudo systemctl stop cloud-print-agent

# 3. 替换二进制
sudo cp cloud-print-agent /usr/local/bin/

# 4. 启动服务
sudo systemctl start cloud-print-agent

# 5. 验证
cpa status
```

### 备份与恢复

```bash
# 备份
sudo tar -czf cloud-print-agent-backup-$(date +%Y%m%d).tar.gz \
  /etc/cloud-print-agent/ \
  /var/lib/cloud-print-agent/

# 恢复
sudo tar -xzf cloud-print-agent-backup-YYYYMMDD.tar.gz -C /
sudo systemctl restart cloud-print-agent
```

---

## 故障排查

### 常见问题

#### 1. Agent 无法连接云端

```bash
# 检查网络
cpa network

# 常见原因：
# - DNS 解析失败：检查 /etc/resolv.conf
# - 网关不可达：检查路由和网络配置
# - 防火墙阻断：确认 443 端口出站放行
# - 域名错误：检查 config.yaml 中 cloud.endpoint
```

#### 2. 打印机不输出

**RAW 协议打印机：**
```bash
# 测试网络连通性
ping <打印机IP>
nc -zv <打印机IP> 9100

# 直接发送测试数据
echo "test" | nc -w 5 <打印机IP> 9100
```

**CUPS 协议打印机：**
```bash
# 检查 CUPS 打印机状态
lpstat -p <打印机名>

# 检查 CUPS 队列
lpstat -o <打印机名>

# 查看 CUPS 错误日志
sudo tail -f /var/log/cups/error_log

# 手动测试打印
echo "test" | lp -d <打印机名>
```

#### 3. Canon 打印机报"PDL IMG 无效"

**原因**：打印机不支持发送的数据格式（PCL/PostScript）。

**解决**：安装 Canon UFR II 驱动，使用正确的 PPD 文件配置 CUPS。

```bash
# 确认 UFR II 驱动已安装
dpkg -l | grep cnrdrvcups

# 确认 PPD 文件正确
grep 'ShortNickName' /etc/cups/ppd/<打印机名>.ppd

# 重新配置
sudo lpadmin -p <打印机名> -v socket://<IP>:9100 -m <正确的PPD> -E
```

#### 4. Canon 打印机报"PDL UFR II版本错误"

**原因**：使用了错误型号的 PPD 文件。

**解决**：查找正确的 PPD 文件。

```bash
# 搜索包含打印机型号的 PPD
grep -rl 'C3530\|c3530' /usr/share/cups/model/CNRCUPS*.ppd

# 查看所有 iR-ADV C 系列 PPD
for f in /usr/share/cups/model/CNRCUPSIRADVC*; do
  echo "$f: $(grep 'ShortNickName' "$f")"
done
```

#### 5. 标签打印机不输出

**原因**：标签打印机需要特定指令集（TSPL/ZPL），直接发送文本不会打印。

**解决**：发送正确的 TSPL 指令。

```bash
# 测试 TSPL 指令
printf 'SIZE 80 mm,50 mm\nGAP 2 mm,0 mm\nCLS\nTEXT 10,10,"3",0,1,1,"Test"\nPRINT 1,1\n' \
  | nc -w 5 <打印机IP> 9100
```

#### 6. 任务状态一直 RUNNING

**原因**：Agent 的 OnTaskResult 回调未注册或上报失败。

**解决**：确认 Agent 版本 ≥ v1.1.0，检查日志：

```bash
sudo journalctl -u cloud-print-agent | grep -i "task.*result"
```

### 日志级别说明

| 级别 | 说明 | 使用场景 |
|------|------|---------|
| debug | 详细调试信息 | 开发/排障 |
| info | 关键操作日志（默认） | 日常运行 |
| warn | 警告信息 | 需关注但不影响运行 |
| error | 错误信息 | 需立即处理 |

### 网络故障分类

| 分类 | 说明 | 排查方向 |
|------|------|---------|
| OK | 网络正常 | - |
| LOCAL_NET_FAIL | 本地网络故障 | 检查网卡/IP/路由 |
| DNS_RESOLVE_FAIL | DNS 解析失败 | 检查 DNS 服务器配置 |
| CLOUD_GATEWAY_UNREACHABLE | 云端/网关不可达 | 检查防火墙/网关 |

---

## API 接口

### 运维接口

**健康检查**
```
GET http://<agent-ip>:8901/api/v1/healthz
```

**服务状态**
```
GET http://<agent-ip>:8901/api/v1/status
```

响应示例：
```json
{
  "status": "running",
  "version": "0.1.0",
  "process_uptime": "2h30m",
  "derived_url": "wss://print.oascii.com",
  "net_class": "OK",
  "device_count": 3,
  "online_devices": 3,
  "pending_tasks": 0
}
```

**网络诊断**
```
GET http://<agent-ip>:8901/api/v1/network
```

响应示例：
```json
{
  "endpoint": "print.oascii.com",
  "resolved_ip": "210.22.123.254",
  "dns_latency_ms": 15,
  "gateway_reach": true,
  "local_net_reach": true,
  "net_class": "OK"
}
```

---

## 版本历史

### v1.1.0 (2026-08-20)

**新增功能：**
- CUPS 协议适配器（支持 Canon UFR II 驱动）
- PrintTask Content 字段（嵌入文档内容传输，避免 Agent 回拉文档）
- OnTaskResult 回调注册（任务状态可靠上报）
- CUPS 协议跳过 IPv4 验证（使用 CUPS 打印机名称）
- cmd/init-credential 凭证初始化工具

**优化：**
- 简化 CUPS adapter（直接使用 `lp` 命令，UFR II 驱动处理格式转换）
- Deli888 标签打印机改为 RAW 协议（TSPL 指令直发）

**验证：**
- 三台打印机端到端测试全部通过
  - EPSON LQ-630KII (RAW 协议)
  - Canon iR-ADV C3530 (CUPS + UFR II)
  - Deli888 标签打印机 (RAW + TSPL)

### v1.0.0 (初始版本)

- Cloud Print Agent MVP
- WSS 云端连接
- RAW/LPR/IPP 协议支持
- 本地持久化任务队列
- 设备健康探测
- 网络拓扑诊断
- systemd 集成
- 运维 API 接口

---

## 相关仓库

- [Cloud-Print-Server](https://github.com/wujupeng/Cloud-Print-Server) — 云打印平台（Web 管理界面 + REST API + WSS Hub）

## 技术栈

| 组件 | 技术 |
|------|------|
| 语言 | Go 1.22 |
| WebSocket | nhooyr.io/websocket |
| 持久化 | bbolt (嵌入式 KV) |
| 日志 | zap + lumberjack |
| 配置 | YAML (yaml.v3) |
| HTTP 路由 | chi/v5 |
| 服务管理 | systemd |
| 打印系统 | CUPS (CUPS 协议打印机) |

## 许可证

MIT License