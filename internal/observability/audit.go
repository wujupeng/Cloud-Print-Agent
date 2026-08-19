package observability

import (
	"os"
	"path/filepath"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/cloud-print/agent/internal/errs"
)

const (
	auditLogFilename   = "audit.log"
	auditMaxSizeMB     = 100
	auditRetentionDays = 30
)

type AuditLogger struct {
	logger *zap.Logger
	mu     sync.RWMutex
}

func NewAuditLogger(logDir string) (*AuditLogger, error) {
	if logDir == "" {
		return nil, errs.New(errs.ErrConfigInvalid, "logDir is empty")
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, errs.Wrap(errs.ErrStorageIO, "mkdir audit log dir", err)
	}

	w := &lumberjack.Logger{
		Filename:   filepath.Join(logDir, auditLogFilename),
		MaxSize:    auditMaxSizeMB,
		MaxBackups: auditRetentionDays,
		MaxAge:     auditRetentionDays,
		Compress:   true,
	}

	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(newEncoderConfig()),
		zapcore.AddSync(w),
		zapcore.InfoLevel,
	)

	logger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(0))
	return &AuditLogger{logger: logger}, nil
}

func (a *AuditLogger) Log(event string, fields ...zap.Field) {
	a.mu.RLock()
	l := a.logger
	a.mu.RUnlock()
	l.Info(event, fields...)
}

func (a *AuditLogger) LogTaskReceived(taskID, deviceID, traceID string) {
	a.Log("task_received",
		FieldEvent("task_received"),
		FieldTaskID(taskID),
		FieldDeviceID(deviceID),
		FieldTraceID(traceID),
	)
}

func (a *AuditLogger) LogTaskExecuted(taskID, deviceID, traceID string, retryCount int) {
	a.Log("task_executed",
		FieldEvent("task_executed"),
		FieldTaskID(taskID),
		FieldDeviceID(deviceID),
		FieldTraceID(traceID),
		FieldRetryCount(retryCount),
	)
}

func (a *AuditLogger) LogTaskRetry(taskID, deviceID, traceID string, retryCount int, errMsg string) {
	a.Log("task_retry",
		FieldEvent("task_retry"),
		FieldTaskID(taskID),
		FieldDeviceID(deviceID),
		FieldTraceID(traceID),
		FieldRetryCount(retryCount),
		FieldError(errMsg),
	)
}

func (a *AuditLogger) LogTaskCancelled(taskID, traceID string) {
	a.Log("task_cancelled",
		FieldEvent("task_cancelled"),
		FieldTaskID(taskID),
		FieldTraceID(traceID),
	)
}

func (a *AuditLogger) LogDeviceAdded(deviceID, name string) {
	a.Log("device_added",
		FieldEvent("device_added"),
		FieldDeviceID(deviceID),
		zap.String("name", name),
	)
}

func (a *AuditLogger) LogDeviceRemoved(deviceID string) {
	a.Log("device_removed",
		FieldEvent("device_removed"),
		FieldDeviceID(deviceID),
	)
}

func (a *AuditLogger) LogDeviceUpdated(deviceID string) {
	a.Log("device_updated",
		FieldEvent("device_updated"),
		FieldDeviceID(deviceID),
	)
}

func (a *AuditLogger) LogUpgrade(version string, traceID string) {
	a.Log("upgrade",
		FieldEvent("upgrade"),
		zap.String("version", version),
		FieldTraceID(traceID),
	)
}

func (a *AuditLogger) LogRollback(version string, traceID string) {
	a.Log("rollback",
		FieldEvent("rollback"),
		zap.String("version", version),
		FieldTraceID(traceID),
	)
}

func (a *AuditLogger) LogDomainUpdate(endpoint, traceID string) {
	a.Log("domain_update",
		FieldEvent("domain_update"),
		FieldCloudEndpoint(endpoint),
		FieldTraceID(traceID),
	)
}

func (a *AuditLogger) LogDomainRollback(endpoint, traceID string) {
	a.Log("domain_rollback",
		FieldEvent("domain_rollback"),
		FieldCloudEndpoint(endpoint),
		FieldTraceID(traceID),
	)
}

func (a *AuditLogger) LogOpsAccess(endpoint, method, clientIP string) {
	a.Log("ops_access",
		FieldEvent("ops_access"),
		zap.String("method", method),
		zap.String("client_ip", clientIP),
		zap.String("path", endpoint),
	)
}

func (a *AuditLogger) LogAuthFailed(reason, traceID string) {
	a.Log("auth_failed",
		FieldEvent("auth_failed"),
		FieldError(reason),
		FieldTraceID(traceID),
	)
}

func (a *AuditLogger) LogNetClassChange(from, to string, traceID string) {
	a.Log("net_class_change",
		FieldEvent("net_class_change"),
		zap.String("from", from),
		zap.String("to", to),
		FieldTraceID(traceID),
	)
}

func (a *AuditLogger) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.logger.Sync()
}