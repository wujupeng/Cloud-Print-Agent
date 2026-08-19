// Package errs 定义 Cloud Print Agent 统一错误码与错误类型。
//
// 错误码与 design.md 2.2.2.1 异常映射表一一对应，
// 包含 4 类新增网络错误（DNS_RESOLVE_FAIL/CLOUD_GATEWAY_UNREACHABLE/
// LOCAL_NET_FAIL/DOMAIN_UPDATE_FAIL）。
// 网络类错误携带 NetClass 故障分类，供 CloudLink 决策重连策略。
package errs

import (
	"errors"
	"fmt"

	"github.com/cloud-print/agent/internal/domain"
)

// ErrorCode 错误码类型。
type ErrorCode string

// 错误码常量（与 design.md 2.2.2.1 异常映射表一一对应）。
const (
	// 认证类
	ErrAuthInvalid       ErrorCode = "AUTH_INVALID"        // 凭证失效（401/403）
	ErrAuthMissing       ErrorCode = "AUTH_MISSING"        // 凭证未配置

	// 网络类（含 4 类新增网络错误）
	ErrDNSResolveFail          ErrorCode = "DNS_RESOLVE_FAIL"          // DNS 解析失败
	ErrCloudGatewayUnreachable ErrorCode = "CLOUD_GATEWAY_UNREACHABLE" // 云端/网关不可达
	ErrLocalNetFail            ErrorCode = "LOCAL_NET_FAIL"            // 本地网络故障
	ErrTLSVerifyFail           ErrorCode = "TLS_VERIFY_FAIL"           // TLS 证书校验失败
	ErrDomainUpdateFail        ErrorCode = "DOMAIN_UPDATE_FAIL"        // 域名远程更新失败
	ErrWSSHandshakeFail        ErrorCode = "WSS_HANDSHAKE_FAIL"        // WSS 握手失败
	ErrCloudDisconnected       ErrorCode = "CLOUD_DISCONNECTED"       // 云端连接断开

	// 任务类
	ErrTaskDataInvalid ErrorCode = "TASK_DATA_INVALID" // 任务数据不完整
	ErrTaskNotFound    ErrorCode = "TASK_NOT_FOUND"    // 任务不存在
	ErrTaskCancelFail  ErrorCode = "TASK_CANCEL_FAIL"  // 任务取消失败（非 PENDING）
	ErrTaskTimeout     ErrorCode = "TASK_TIMEOUT"      // 打印发送超时

	// 设备类
	ErrDeviceNotFound   ErrorCode = "DEVICE_NOT_FOUND"   // 目标设备不存在
	ErrDeviceOffline    ErrorCode = "DEVICE_OFFLINE"     // 设备离线
	ErrDeviceIDConflict ErrorCode = "DEVICE_ID_CONFLICT" // 设备 ID 冲突
	ErrDeviceFieldInvalid ErrorCode = "DEVICE_FIELD_INVALID" // 设备字段校验失败

	// 队列类
	ErrQueueFull  ErrorCode = "QUEUE_FULL"  // 队列已满（100/设备）
	ErrQueueEmpty ErrorCode = "QUEUE_EMPTY" // 队列为空

	// 配置类
	ErrConfigInvalid  ErrorCode = "CONFIG_INVALID"  // 配置校验失败
	ErrConfigMissing  ErrorCode = "CONFIG_MISSING"  // 配置缺失
	ErrConfigVersion  ErrorCode = "CONFIG_VERSION"  // 配置版本不兼容

	// 凭证类
	ErrCredentialInvalid ErrorCode = "CREDENTIAL_INVALID" // 凭证解密失败
	ErrCredentialMissing ErrorCode = "CREDENTIAL_MISSING" // 凭证文件缺失

	// 持久化类
	ErrStorageIO    ErrorCode = "STORAGE_IO"    // 存储读写错误
	ErrStorageCorrupt ErrorCode = "STORAGE_CORRUPT" // 存储数据损坏

	// 协议类
	ErrProtocolProbeFail ErrorCode = "PROTOCOL_PROBE_FAIL" // 协议探测失败
	ErrProtocolSendFail  ErrorCode = "PROTOCOL_SEND_FAIL"  // 协议发送失败

	// 运维类
	ErrOpsAccessDenied ErrorCode = "OPS_ACCESS_DENIED" // 运维接口越权访问

	// 升级类
	ErrUpgradeDownload  ErrorCode = "UPGRADE_DOWNLOAD"  // 升级下载失败
	ErrUpgradeVerify    ErrorCode = "UPGRADE_VERIFY"    // 升级校验失败
	ErrUpgradeRollback  ErrorCode = "UPGRADE_ROLLBACK"  // 升级回滚
)

// AgentError Agent 统一错误类型。
//
// 携带错误码、消息、原始错误与网络故障分类（仅网络类错误）。
type AgentError struct {
	Code     ErrorCode        // 错误码
	Message  string           // 人可读错误消息
	Cause    error            // 原始错误
	NetClass domain.NetClass  // 网络故障分类（仅网络类错误非空）
}

// Error 实现 error 接口。
func (e *AgentError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap 支持 errors.Is/errors.As 错误解包。
func (e *AgentError) Unwrap() error { return e.Cause }

// Is 支持 errors.Is 比较。
func (e *AgentError) Is(target error) bool {
	var t *AgentError
	if errors.As(target, &t) {
		return e.Code == t.Code
	}
	return false
}

// New 创建新的 AgentError。
func New(code ErrorCode, message string) *AgentError {
	return &AgentError{Code: code, Message: message}
}

// Newf 创建新的 AgentError（格式化消息）。
func Newf(code ErrorCode, format string, args ...interface{}) *AgentError {
	return &AgentError{Code: code, Message: fmt.Sprintf(format, args...)}
}

// Wrap 包装原始错误为 AgentError。
func Wrap(code ErrorCode, message string, cause error) *AgentError {
	return &AgentError{Code: code, Message: message, Cause: cause}
}

// Wrapf 包装原始错误为 AgentError（格式化消息）。
func Wrapf(code ErrorCode, cause error, format string, args ...interface{}) *AgentError {
	return &AgentError{Code: code, Message: fmt.Sprintf(format, args...), Cause: cause}
}

// WithNetClass 为错误附加网络故障分类。
func (e *AgentError) WithNetClass(class domain.NetClass) *AgentError {
	e.NetClass = class
	return e
}

// NewNetError 创建网络类错误（携带 NetClass）。
func NewNetError(code ErrorCode, message string, class domain.NetClass) *AgentError {
	return &AgentError{Code: code, Message: message, NetClass: class}
}

// WrapNetError 包装网络类错误（携带 NetClass）。
func WrapNetError(code ErrorCode, message string, cause error, class domain.NetClass) *AgentError {
	return &AgentError{Code: code, Message: message, Cause: cause, NetClass: class}
}

// IsRetryable 判断错误是否可重试。
//
// 对应 design.md 2.6 错误处理表中的可重试错误。
func IsRetryable(err error) bool {
	var ae *AgentError
	if !errors.As(err, &ae) {
		return false
	}
	switch ae.Code {
	case ErrCloudGatewayUnreachable,
		ErrDNSResolveFail,
		ErrLocalNetFail,
		ErrWSSHandshakeFail,
		ErrCloudDisconnected,
		ErrDeviceOffline,
		ErrTaskTimeout,
		ErrProtocolSendFail,
		ErrStorageIO:
		return true
	default:
		return false
	}
}

// IsTerminal 判断错误是否为终态错误（不可重试，任务直接失败）。
//
// 对应 design.md 2.6 错误处理表中的终态错误。
func IsTerminal(err error) bool {
	var ae *AgentError
	if !errors.As(err, &ae) {
		return false
	}
	switch ae.Code {
	case ErrAuthInvalid,
		ErrAuthMissing,
		ErrTaskDataInvalid,
		ErrTaskNotFound,
		ErrTaskCancelFail,
		ErrDeviceNotFound,
		ErrDeviceIDConflict,
		ErrDeviceFieldInvalid,
		ErrQueueFull,
		ErrConfigInvalid,
		ErrConfigMissing,
		ErrCredentialInvalid,
		ErrCredentialMissing,
		ErrStorageCorrupt,
		ErrProtocolProbeFail,
		ErrOpsAccessDenied,
		ErrTLSVerifyFail:
		return true
	default:
		return false
	}
}

// IsNetError 判断错误是否为网络类错误。
func IsNetError(err error) bool {
	var ae *AgentError
	if !errors.As(err, &ae) {
		return false
	}
	switch ae.Code {
	case ErrDNSResolveFail,
		ErrCloudGatewayUnreachable,
		ErrLocalNetFail,
		ErrCloudDisconnected,
		ErrWSSHandshakeFail:
		return true
	default:
		return false
	}
}

// NetClassFromErr 从错误中提取网络故障分类。
//
// 若错误不是网络类错误，返回 NetClassOK。
func NetClassFromErr(err error) domain.NetClass {
	var ae *AgentError
	if !errors.As(err, &ae) {
		return domain.NetClassOK
	}
	if ae.NetClass != "" {
		return ae.NetClass
	}
	// 根据错误码推断 NetClass
	switch ae.Code {
	case ErrDNSResolveFail:
		return domain.NetClassDNSResolveFail
	case ErrCloudGatewayUnreachable:
		return domain.NetClassCloudGatewayUnreachable
	case ErrLocalNetFail:
		return domain.NetClassLocalNetFail
	default:
		return domain.NetClassOK
	}
}

// CodeFromErr 从错误中提取错误码。
//
// 若错误不是 AgentError，返回空字符串。
func CodeFromErr(err error) ErrorCode {
	var ae *AgentError
	if !errors.As(err, &ae) {
		return ""
	}
	return ae.Code
}