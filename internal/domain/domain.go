// Package domain 定义 Cloud Print Agent 核心领域对象与枚举。
//
// 本包包含设备、打印任务、Agent 配置、凭证、网络拓扑等核心数据结构，
// 以及协议类型、设备状态、任务状态、网络故障分类等枚举。
// 所有结构体字段与 design.md 2.3.2.1 类图及 spec.md 第 6 章数据约束保持一致。
package domain

import (
	"encoding/json"
	"fmt"
	"time"
)

// Protocol 打印协议类型枚举。
type Protocol string

const (
	ProtocolRAW Protocol = "RAW" // RAW/JetDirect，端口 9100
	ProtocolLPR Protocol = "LPR" // LPR，端口 515，RFC 1179
	ProtocolIPP Protocol = "IPP" // IPP，端口 631，RFC 8011
	ProtocolUnknown Protocol = "UNKNOWN" // 未知协议（探测全失败）
)

// String 返回协议类型的字符串表示。
func (p Protocol) String() string { return string(p) }

// MarshalJSON 实现 json.Marshaler。
func (p Protocol) MarshalJSON() ([]byte, error) { return json.Marshal(string(p)) }

// UnmarshalJSON 实现 json.Unmarshaler。
func (p *Protocol) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	*p = Protocol(s)
	return nil
}

// DeviceStatus 设备状态枚举。
type DeviceStatus string

const (
	DeviceStatusOnline      DeviceStatus = "ONLINE"       // 在线可用
	DeviceStatusOffline     DeviceStatus = "OFFLINE"      // 离线（探测失败或人工标记）
	DeviceStatusProbeFailed DeviceStatus = "PROBE_FAILED" // 协议探测失败
)

// String 返回设备状态的字符串表示。
func (s DeviceStatus) String() string { return string(s) }

// MarshalJSON 实现 json.Marshaler。
func (s DeviceStatus) MarshalJSON() ([]byte, error) { return json.Marshal(string(s)) }

// UnmarshalJSON 实现 json.Unmarshaler。
func (s *DeviceStatus) UnmarshalJSON(b []byte) error {
	var v string
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*s = DeviceStatus(v)
	return nil
}

// TaskStatus 打印任务状态枚举。
type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "PENDING"   // 待执行（入队未开始）
	TaskStatusRunning   TaskStatus = "RUNNING"   // 执行中
	TaskStatusSuccess   TaskStatus = "SUCCESS"   // 成功完成
	TaskStatusFailed    TaskStatus = "FAILED"    // 最终失败（重试耗尽）
	TaskStatusRetrying  TaskStatus = "RETRYING"  // 重试等待中
	TaskStatusCancelled TaskStatus = "CANCELLED" // 已取消
)

// String 返回任务状态的字符串表示。
func (s TaskStatus) String() string { return string(s) }

// IsTerminal 判断任务状态是否为终态（不再变化）。
func (s TaskStatus) IsTerminal() bool {
	return s == TaskStatusSuccess || s == TaskStatusFailed || s == TaskStatusCancelled
}

// MarshalJSON 实现 json.Marshaler。
func (s TaskStatus) MarshalJSON() ([]byte, error) { return json.Marshal(string(s)) }

// UnmarshalJSON 实现 json.Unmarshaler。
func (s *TaskStatus) UnmarshalJSON(b []byte) error {
	var v string
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*s = TaskStatus(v)
	return nil
}

// NetClass 网络故障分类枚举。
type NetClass string

const (
	NetClassOK                     NetClass = "OK"                     // 网络正常
	NetClassLocalNetFail           NetClass = "LOCAL_NET_FAIL"         // 本地网络故障（内网/网卡异常）
	NetClassCloudGatewayUnreachable NetClass = "CLOUD_GATEWAY_UNREACHABLE" // 云端/网关不可达
	NetClassDNSResolveFail         NetClass = "DNS_RESOLVE_FAIL"       // DNS 解析失败
)

// String 返回网络故障分类的字符串表示。
func (c NetClass) String() string { return string(c) }

// IsOK 判断网络是否正常。
func (c NetClass) IsOK() bool { return c == NetClassOK }

// IsLocalFail 判断是否为本地网络故障。
func (c NetClass) IsLocalFail() bool { return c == NetClassLocalNetFail }

// MarshalJSON 实现 json.Marshaler。
func (c NetClass) MarshalJSON() ([]byte, error) { return json.Marshal(string(c)) }

// UnmarshalJSON 实现 json.Unmarshaler。
func (c *NetClass) UnmarshalJSON(b []byte) error {
	var v string
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*c = NetClass(v)
	return nil
}

// CloudProtocol 云端连接协议枚举。
type CloudProtocol string

const (
	CloudProtocolWSS   CloudProtocol = "wss"   // WebSocket Secure（默认）
	CloudProtocolHTTPS CloudProtocol = "https" // HTTPS
)

// String 返回云端连接协议的字符串表示。
func (p CloudProtocol) String() string { return string(p) }

// Device 打印设备领域对象。
//
// 对应 spec.md 6.1 打印设备数据约束。
type Device struct {
	DeviceID    string       `json:"device_id" yaml:"device_id"`         // 设备唯一标识（3-32 字符大写字母+数字）
	Name        string       `json:"name" yaml:"name"`                   // 设备名称（1-64 字符）
	IP          string       `json:"ip" yaml:"ip"`                       // 设备 IPv4 地址
	Hostname    string       `json:"hostname,omitempty" yaml:"hostname"` // 主机名（如 ps01）
	Model       string       `json:"model" yaml:"model"`                 // 设备型号（如 EPSON LQ-630KII）
	Protocol    Protocol     `json:"protocol" yaml:"protocol"`           // 打印协议（RAW/LPR/IPP）
	Status      DeviceStatus `json:"status" yaml:"status"`               // 设备状态
	Factory     string       `json:"factory" yaml:"factory"`             // 所属工厂（如 宝山工厂）
	Port        int          `json:"port,omitempty" yaml:"port"`         // 端口号（可选，默认按协议）
	LastProbeAt time.Time    `json:"last_probe_at,omitempty" yaml:"last_probe_at"` // 最近探测时间
	CreatedAt   time.Time    `json:"created_at,omitempty" yaml:"created_at"`       // 创建时间
	UpdatedAt   time.Time    `json:"updated_at,omitempty" yaml:"updated_at"`       // 更新时间
}

// DefaultPort 返回协议对应的默认端口。
func (d *Device) DefaultPort() int {
	if d.Port > 0 {
		return d.Port
	}
	switch d.Protocol {
	case ProtocolRAW:
		return 9100
	case ProtocolLPR:
		return 515
	case ProtocolIPP:
		return 631
	default:
		return 0
	}
}

// PrintParams 打印参数。
//
// 对应 spec.md 6.2-5 打印参数约束。
type PrintParams struct {
	Copies      int               `json:"copies,omitempty" yaml:"copies"`           // 份数（1-99，默认 1）
	Orientation string            `json:"orientation,omitempty" yaml:"orientation"` // 方向（portrait/landscape）
	Extra       map[string]string `json:"extra,omitempty" yaml:"extra"`             // 协议特定额外参数
}

// PrintTask 打印任务领域对象。
//
// 对应 spec.md 6.2 打印任务数据约束。
type PrintTask struct {
	TaskID      string     `json:"task_id"`                       // 任务唯一标识
	DeviceID    string     `json:"device_id"`                     // 目标设备 ID
	DocumentRef string     `json:"document_ref,omitempty"`        // 文档引用（暂存文件路径或云端 URL）
	Checksum    string     `json:"checksum,omitempty"`            // 文档校验和（SHA-256）
	Params      PrintParams `json:"params,omitempty"`             // 打印参数
	Status      TaskStatus `json:"status"`                        // 任务状态
	RetryCount  int        `json:"retry_count"`                   // 已重试次数
	TraceID     string     `json:"trace_id,omitempty"`            // 链路追踪 ID
	ReceivedAt  time.Time  `json:"received_at"`                   // 接收时间
	StartedAt   time.Time  `json:"started_at,omitempty"`          // 开始执行时间
	FinishedAt  time.Time  `json:"finished_at,omitempty"`         // 完成时间
	NextRetryAt time.Time  `json:"next_retry_at,omitempty"`       // 下次重试时间
	ErrorCode   string     `json:"error_code,omitempty"`          // 失败错误码
	ErrorMsg    string     `json:"error_msg,omitempty"`           // 失败错误消息
	SeqNo       int64      `json:"seq_no,omitempty"`              // 到达序号（持久化键）
}

// AgentConfig Agent 运行配置。
//
// 对应 spec.md 6.3 Agent 运行配置数据约束。
// 固定项（heartbeat/max_retry/retry_init/queue_capacity/retry_max_backoff/
// print_send_timeout/cloud_outbound_port）在代码中强制覆盖，禁止用户修改。
type AgentConfig struct {
	ConfigVersion int `json:"config_version" yaml:"config_version"` // 配置版本号

	// 云端连接配置
	Cloud CloudConfig `json:"cloud" yaml:"cloud"`

	// 运维接口配置
	Ops OpsConfig `json:"ops" yaml:"ops"`

	// 存储配置
	Storage StorageConfig `json:"storage" yaml:"storage"`

	// 日志配置
	Log LogConfig `json:"log" yaml:"log"`

	// 设备列表
	Devices []Device `json:"devices,omitempty" yaml:"devices"`

	// 固定运行参数（代码强制覆盖，禁止用户修改）
	HeartbeatInterval  time.Duration `json:"-" yaml:"-"` // 心跳间隔，固定 30s
	MaxRetry           int           `json:"-" yaml:"-"` // 最大重试次数，固定 3
	RetryInitDelay     time.Duration `json:"-" yaml:"-"` // 重试初始延迟，固定 5s
	QueueCapacity      int           `json:"-" yaml:"-"` // 队列容量，固定 100
	RetryMaxBackoff    time.Duration `json:"-" yaml:"-"` // 重试退避上限，固定 300s
	PrintSendTimeout   time.Duration `json:"-" yaml:"-"` // 打印发送超时，固定 60s
	CloudOutboundPort  int           `json:"-" yaml:"-"` // 云端出站端口，固定 443
}

// CloudConfig 云端连接配置。
type CloudConfig struct {
	Endpoint   string        `json:"endpoint" yaml:"endpoint"`     // 云端域名（如 print.oascii.com），禁止 IP
	Protocol   CloudProtocol `json:"protocol" yaml:"protocol"`     // 连接协议（wss/https）
	DerivedURL string        `json:"derived_url,omitempty" yaml:"-"` // 派生接入地址（protocol://endpoint），程序派生
	AgentID    string        `json:"agent_id,omitempty" yaml:"agent_id"` // Agent 实例 ID
}

// OpsConfig 运维接口配置。
type OpsConfig struct {
	Port int `json:"port" yaml:"port"` // 运维接口端口（1024-65535）
}

// StorageConfig 存储配置。
type StorageConfig struct {
	DataDir string `json:"data_dir" yaml:"data_dir"` // 数据目录
	LogDir  string `json:"log_dir" yaml:"log_dir"`   // 日志目录
}

// LogConfig 日志配置。
type LogConfig struct {
	Level          string `json:"level" yaml:"level"`                     // 日志级别（debug/info/warn/error）
	RetentionDays  int    `json:"retention_days" yaml:"retention_days"`   // 日志保留天数（7-90，默认 30）
	MaxSizeMB      int    `json:"max_size_mb,omitempty" yaml:"max_size_mb"` // 单文件最大大小（默认 100MB）
}

// Credentials Agent 凭证（加密存储）。
//
// 对应 spec.md 6.5 部署凭证（脱敏）。
// 三类凭证（device_token/mtls_cert/mtls_key）独立字段加密。
type Credentials struct {
	DeviceToken string `json:"device_token"` // 设备认证令牌
	MTLSCert    string `json:"mtls_cert,omitempty"` // mTLS 证书（可选）
	MTLSKey     string `json:"mtls_key,omitempty"`  // mTLS 私钥（可选）
}

// NetTopology 网络拓扑与链路状态。
//
// 对应 design.md 2.3.2.1 NetTopology 对象。
type NetTopology struct {
	Endpoint      string    `json:"endpoint"`                 // 云端域名
	ResolvedIP    string    `json:"resolved_ip,omitempty"`    // DNS 解析得到的 IP
	DNSLatencyMs  int       `json:"dns_latency_ms,omitempty"` // DNS 解析耗时（毫秒）
	GatewayIP     string    `json:"gateway_ip,omitempty"`     // 网关公网 IP
	GatewayReach  bool      `json:"gateway_reach"`            // 网关连通性
	LocalNetReach bool      `json:"local_net_reach"`          // 本地网络连通性
	NetClass      NetClass  `json:"net_class"`                // 故障分类
	LastCheckAt   time.Time `json:"last_check_at"`            // 最近检查时间
}

// GatewayConfig 网关配置（可选，用于网关探测）。
type GatewayConfig struct {
	PublicIP string `json:"public_ip,omitempty" yaml:"public_ip"` // 网关公网 IP（如 210.22.123.254）
	ProbePort int   `json:"probe_port,omitempty" yaml:"probe_port"` // 探测端口（默认 443）
}

// Envelope 云端通信消息信封。
//
// 对应 design.md 2.2.2.1 JSON 信封格式。
type Envelope struct {
	Type    string          `json:"type"`              // 消息类型
	TraceID string          `json:"trace_id,omitempty"` // 链路追踪 ID
	TS      time.Time      `json:"ts"`                // 时间戳
	Payload json.RawMessage `json:"payload"`          // 消息载荷（JSON）
}

// NewEnvelope 创建消息信封。
func NewEnvelope(msgType string, payload interface{}) (*Envelope, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}
	return &Envelope{
		Type:    msgType,
		TS:      time.Now().UTC(),
		Payload: b,
	}, nil
}

// HeartbeatPayload 心跳消息载荷。
type HeartbeatPayload struct {
	AgentID       string    `json:"agent_id"`
	Version       string    `json:"version"`
	OnlineDevices int       `json:"online_devices"`
	PendingTasks  int       `json:"pending_tasks"`
	CloudEndpoint string    `json:"cloud_endpoint"`
	NetClass      NetClass  `json:"net_class"`
	Timestamp     time.Time `json:"timestamp"`
}

// TaskResultPayload 任务结果上报载荷。
type TaskResultPayload struct {
	TaskID    string     `json:"task_id"`
	DeviceID  string     `json:"device_id"`
	Status    TaskStatus `json:"status"`
	RetryCount int       `json:"retry_count"`
	ErrorCode string     `json:"error_code,omitempty"`
	ErrorMsg  string     `json:"error_msg,omitempty"`
	FinishedAt time.Time `json:"finished_at"`
}

// DeviceStatusPayload 设备状态上报载荷。
type DeviceStatusPayload struct {
	DeviceID   string       `json:"device_id"`
	Status     DeviceStatus `json:"status"`
	Protocol   Protocol     `json:"protocol,omitempty"`
	LastProbeAt time.Time   `json:"last_probe_at,omitempty"`
}

// NetEventPayload 网络事件上报载荷。
type NetEventPayload struct {
	Class    NetClass  `json:"class"`
	Endpoint string    `json:"endpoint,omitempty"`
	Detail   string    `json:"detail,omitempty"`
	TS       time.Time `json:"ts"`
}

// ConfigAckPayload 配置确认上报载荷。
type ConfigAckPayload struct {
	Applied bool   `json:"applied"`
	Reason  string `json:"reason,omitempty"`
	Field   string `json:"field,omitempty"`
}

// TaskAckPayload 任务确认载荷。
type TaskAckPayload struct {
	TaskID  string `json:"task_id"`
	Accepted bool `json:"accepted"`
	Reason  string `json:"reason,omitempty"`
}

// ApplyFixedDefaults 应用固定运行参数（代码强制覆盖，禁止用户修改）。
//
// 对应 spec.md 6.3 固定项约束。
func (c *AgentConfig) ApplyFixedDefaults() {
	c.HeartbeatInterval = 30 * time.Second
	c.MaxRetry = 3
	c.RetryInitDelay = 5 * time.Second
	c.QueueCapacity = 100
	c.RetryMaxBackoff = 300 * time.Second
	c.PrintSendTimeout = 60 * time.Second
	c.CloudOutboundPort = 443
}

// DeriveCloudURL 派生云端接入地址。
func (c *AgentConfig) DeriveCloudURL() string {
	return fmt.Sprintf("%s://%s", c.Cloud.Protocol, c.Cloud.Endpoint)
}

// Version Agent 版本号。
const Version = "0.1.0"