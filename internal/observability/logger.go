package observability

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/cloud-print/agent/internal/errs"
)

const (
	defaultLogMaxSizeMB = 100
	defaultRetentionDays = 30
	defaultLogFilename   = "agent.log"
)

func NewLogger(logDir string, level string) (*zap.Logger, error) {
	if logDir == "" {
		return nil, errs.New(errs.ErrConfigInvalid, "logDir is empty")
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, errs.Wrap(errs.ErrStorageIO, "mkdir log dir", err)
	}

	lvl, err := parseLevel(level)
	if err != nil {
		return nil, err
	}

	w := &lumberjack.Logger{
		Filename:   filepath.Join(logDir, defaultLogFilename),
		MaxSize:    defaultLogMaxSizeMB,
		MaxBackups: defaultRetentionDays,
		MaxAge:     defaultRetentionDays,
		Compress:   true,
	}

	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(newEncoderConfig()),
		zapcore.AddSync(w),
		lvl,
	)

	logger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(0))
	return logger, nil
}

func newEncoderConfig() zapcore.EncoderConfig {
	return zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "level",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}
}

func parseLevel(level string) (zapcore.Level, error) {
	switch strings.ToLower(level) {
	case "debug":
		return zapcore.DebugLevel, nil
	case "info":
		return zapcore.InfoLevel, nil
	case "warn":
		return zapcore.WarnLevel, nil
	case "error":
		return zapcore.ErrorLevel, nil
	case "":
		return zapcore.InfoLevel, nil
	default:
		return 0, fmt.Errorf("invalid log level: %q", level)
	}
}

func FieldTraceID(v string) zap.Field       { return zap.String("trace_id", v) }
func FieldEvent(v string) zap.Field         { return zap.String("event", v) }
func FieldDeviceID(v string) zap.Field      { return zap.String("device_id", v) }
func FieldTaskID(v string) zap.Field        { return zap.String("task_id", v) }
func FieldRetryCount(v int) zap.Field       { return zap.Int("retry_count", v) }
func FieldError(v string) zap.Field         { return zap.String("error", v) }
func FieldNetClass(v string) zap.Field      { return zap.String("net_class", v) }
func FieldCloudEndpoint(v string) zap.Field { return zap.String("cloud_endpoint", v) }
func FieldResolvedIP(v string) zap.Field    { return zap.String("resolved_ip", v) }