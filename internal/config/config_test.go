package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloud-print/agent/internal/config"
	"github.com/cloud-print/agent/internal/domain"
	"github.com/cloud-print/agent/internal/errs"
)

const exampleConfigYAML = `config_version: 1
cloud:
  endpoint: print.oascii.com
  protocol: wss
  agent_id: BAOSHAN-AGENT-01
ops:
  port: 8901
storage:
  data_dir: /var/lib/cloud-print-agent
  log_dir: /var/log/cloud-print-agent
log:
  level: info
  retention_days: 30
  max_size_mb: 100
devices:
  - device_id: BAOSHAN-PS01
    name: baoshan-lq630kii
    ip: 192.168.2.81
    hostname: ps01
    model: EPSON LQ-630KII
    protocol: RAW
    status: ONLINE
    factory: baoshan
`

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func TestLoadConfig(t *testing.T) {
	path := writeTempConfig(t, exampleConfigYAML)

	cfg, err := config.Load(path)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, "print.oascii.com", cfg.Cloud.Endpoint)
	assert.Equal(t, domain.CloudProtocolWSS, cfg.Cloud.Protocol)
	assert.Equal(t, "wss://print.oascii.com", cfg.Cloud.DerivedURL)
	assert.Equal(t, "BAOSHAN-AGENT-01", cfg.Cloud.AgentID)
	assert.Equal(t, 8901, cfg.Ops.Port)
	assert.Equal(t, "/var/lib/cloud-print-agent", cfg.Storage.DataDir)
	assert.Equal(t, "/var/log/cloud-print-agent", cfg.Storage.LogDir)
	assert.Equal(t, "info", cfg.Log.Level)
	assert.Equal(t, 30, cfg.Log.RetentionDays)

	require.Len(t, cfg.Devices, 1)
	dev := cfg.Devices[0]
	assert.Equal(t, "BAOSHAN-PS01", dev.DeviceID)
	assert.Equal(t, domain.ProtocolRAW, dev.Protocol)
	assert.Equal(t, "192.168.2.81", dev.IP)

	assert.Equal(t, 30*time.Second, cfg.HeartbeatInterval)
	assert.Equal(t, 3, cfg.MaxRetry)
	assert.Equal(t, 5*time.Second, cfg.RetryInitDelay)
	assert.Equal(t, 100, cfg.QueueCapacity)
	assert.Equal(t, 300*time.Second, cfg.RetryMaxBackoff)
	assert.Equal(t, 60*time.Second, cfg.PrintSendTimeout)
	assert.Equal(t, 443, cfg.CloudOutboundPort)
}

func TestLoadConfig_MissingFile(t *testing.T) {
	_, err := config.Load(filepath.Join(t.TempDir(), "not-exist.yaml"))
	require.Error(t, err)
	var ae *errs.AgentError
	assert.ErrorAs(t, err, &ae)
	assert.Equal(t, errs.ErrConfigMissing, ae.Code)
}

func TestValidateConfig(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		cfg := &domain.AgentConfig{}
		cfg.Cloud.Endpoint = "print.oascii.com"
		cfg.Ops.Port = 8901
		cfg.Storage.DataDir = "/var/lib/cloud-print-agent"
		cfg.Storage.LogDir = "/var/log/cloud-print-agent"
		cfg.Log.Level = "info"
		cfg.Log.RetentionDays = 30
		assert.NoError(t, config.Validate(cfg))
	})

	t.Run("empty endpoint rejected", func(t *testing.T) {
		cfg := &domain.AgentConfig{}
		cfg.Cloud.Endpoint = ""
		cfg.Ops.Port = 8901
		cfg.Storage.DataDir = "/data"
		cfg.Storage.LogDir = "/log"
		cfg.Log.Level = "info"
		cfg.Log.RetentionDays = 30
		err := config.Validate(cfg)
		require.Error(t, err)
		var ae *errs.AgentError
		assert.ErrorAs(t, err, &ae)
		assert.Equal(t, errs.ErrConfigInvalid, ae.Code)
	})

	t.Run("ip endpoint rejected", func(t *testing.T) {
		cfg := &domain.AgentConfig{}
		cfg.Cloud.Endpoint = "210.22.123.254"
		cfg.Ops.Port = 8901
		cfg.Storage.DataDir = "/data"
		cfg.Storage.LogDir = "/log"
		cfg.Log.Level = "info"
		cfg.Log.RetentionDays = 30
		err := config.Validate(cfg)
		require.Error(t, err)
		var ae *errs.AgentError
		assert.ErrorAs(t, err, &ae)
		assert.Equal(t, errs.ErrConfigInvalid, ae.Code)
		assert.Contains(t, err.Error(), "not IP")
	})

	t.Run("ops port out of range", func(t *testing.T) {
		cfg := &domain.AgentConfig{}
		cfg.Cloud.Endpoint = "print.oascii.com"
		cfg.Ops.Port = 80
		cfg.Storage.DataDir = "/data"
		cfg.Storage.LogDir = "/log"
		cfg.Log.Level = "info"
		cfg.Log.RetentionDays = 30
		err := config.Validate(cfg)
		require.Error(t, err)
		var ae *errs.AgentError
		assert.ErrorAs(t, err, &ae)
		assert.Equal(t, errs.ErrConfigInvalid, ae.Code)
	})

	t.Run("invalid log level", func(t *testing.T) {
		cfg := &domain.AgentConfig{}
		cfg.Cloud.Endpoint = "print.oascii.com"
		cfg.Ops.Port = 8901
		cfg.Storage.DataDir = "/data"
		cfg.Storage.LogDir = "/log"
		cfg.Log.Level = "trace"
		cfg.Log.RetentionDays = 30
		err := config.Validate(cfg)
		require.Error(t, err)
		var ae *errs.AgentError
		assert.ErrorAs(t, err, &ae)
		assert.Equal(t, errs.ErrConfigInvalid, ae.Code)
	})

	t.Run("retention out of range", func(t *testing.T) {
		cfg := &domain.AgentConfig{}
		cfg.Cloud.Endpoint = "print.oascii.com"
		cfg.Ops.Port = 8901
		cfg.Storage.DataDir = "/data"
		cfg.Storage.LogDir = "/log"
		cfg.Log.Level = "info"
		cfg.Log.RetentionDays = 3
		err := config.Validate(cfg)
		require.Error(t, err)
		var ae *errs.AgentError
		assert.ErrorAs(t, err, &ae)
		assert.Equal(t, errs.ErrConfigInvalid, ae.Code)
	})
}

func TestApplyFixedDefaults(t *testing.T) {
	cfg := &domain.AgentConfig{}
	cfg.HeartbeatInterval = 1 * time.Second
	cfg.MaxRetry = 99
	cfg.RetryInitDelay = 1 * time.Minute
	cfg.QueueCapacity = 999
	cfg.RetryMaxBackoff = 1 * time.Hour
	cfg.PrintSendTimeout = 1 * time.Minute
	cfg.CloudOutboundPort = 8080

	cfg.ApplyFixedDefaults()

	assert.Equal(t, 30*time.Second, cfg.HeartbeatInterval)
	assert.Equal(t, 3, cfg.MaxRetry)
	assert.Equal(t, 5*time.Second, cfg.RetryInitDelay)
	assert.Equal(t, 100, cfg.QueueCapacity)
	assert.Equal(t, 300*time.Second, cfg.RetryMaxBackoff)
	assert.Equal(t, 60*time.Second, cfg.PrintSendTimeout)
	assert.Equal(t, 443, cfg.CloudOutboundPort)

	cfg.HeartbeatInterval = 999 * time.Second
	cfg.MaxRetry = 0
	cfg.ApplyFixedDefaults()
	assert.Equal(t, 30*time.Second, cfg.HeartbeatInterval)
	assert.Equal(t, 3, cfg.MaxRetry)
}

func TestSaveAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "saved.yaml")

	cfg := &domain.AgentConfig{}
	cfg.ConfigVersion = 1
	cfg.Cloud.Endpoint = "print.oascii.com"
	cfg.Cloud.Protocol = domain.CloudProtocolWSS
	cfg.Cloud.AgentID = "BAOSHAN-AGENT-01"
	cfg.Ops.Port = 8901
	cfg.Storage.DataDir = "/var/lib/cloud-print-agent"
	cfg.Storage.LogDir = "/var/log/cloud-print-agent"
	cfg.Log.Level = "info"
	cfg.Log.RetentionDays = 30
	cfg.Log.MaxSizeMB = 100

	require.NoError(t, config.SaveAtomic(path, cfg))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.False(t, info.IsDir())
	assert.Greater(t, info.Size(), int64(0))

	loaded, err := config.Load(path)
	require.NoError(t, err)
	assert.Equal(t, "print.oascii.com", loaded.Cloud.Endpoint)
	assert.Equal(t, domain.CloudProtocolWSS, loaded.Cloud.Protocol)
	assert.Equal(t, 8901, loaded.Ops.Port)

	matches, err := filepath.Glob(filepath.Join(dir, ".*-saved.yaml*"))
	require.NoError(t, err)
	for _, m := range matches {
		if m != path {
			t.Errorf("leftover temp file: %s", m)
		}
	}
}

func TestSaveAtomic_NestedDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "deep", "config.yaml")
	cfg := &domain.AgentConfig{}
	cfg.Cloud.Endpoint = "print.oascii.com"
	cfg.Cloud.Protocol = domain.CloudProtocolWSS
	cfg.Ops.Port = 8901
	cfg.Storage.DataDir = "/data"
	cfg.Storage.LogDir = "/log"
	cfg.Log.Level = "info"
	cfg.Log.RetentionDays = 30

	require.NoError(t, config.SaveAtomic(path, cfg))
	_, err := os.Stat(path)
	require.NoError(t, err)
}

func TestLoadConfig_RejectsIP(t *testing.T) {
	bad := strings.Replace(exampleConfigYAML, "print.oascii.com", "210.22.123.254", 1)
	path := writeTempConfig(t, bad)
	_, err := config.Load(path)
	require.Error(t, err)
	var ae *errs.AgentError
	assert.ErrorAs(t, err, &ae)
	assert.Equal(t, errs.ErrConfigInvalid, ae.Code)
}