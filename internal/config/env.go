package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/cloud-print/agent/internal/domain"
)

const envPrefix = "CPA_"

func ApplyEnvOverrides(cfg *domain.AgentConfig) {
	if v := os.Getenv(envPrefix + "CLOUD_ENDPOINT"); v != "" {
		cfg.Cloud.Endpoint = v
	}
	if v := os.Getenv(envPrefix + "CLOUD_PROTOCOL"); v != "" {
		cfg.Cloud.Protocol = domain.CloudProtocol(v)
	}
	if v := os.Getenv(envPrefix + "CLOUD_AGENT_ID"); v != "" {
		cfg.Cloud.AgentID = v
	}
	if v := os.Getenv(envPrefix + "OPS_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Ops.Port = p
		}
	}
	if v := os.Getenv(envPrefix + "DATA_DIR"); v != "" {
		cfg.Storage.DataDir = v
	}
	if v := os.Getenv(envPrefix + "LOG_DIR"); v != "" {
		cfg.Storage.LogDir = v
	}
	if v := os.Getenv(envPrefix + "LOG_LEVEL"); v != "" {
		cfg.Log.Level = strings.ToLower(v)
	}
	if v := os.Getenv(envPrefix + "LOG_RETENTION_DAYS"); v != "" {
		if d, err := strconv.Atoi(v); err == nil {
			cfg.Log.RetentionDays = d
		}
	}
	if v := os.Getenv(envPrefix + "LOG_MAX_SIZE_MB"); v != "" {
		if s, err := strconv.Atoi(v); err == nil {
			cfg.Log.MaxSizeMB = s
		}
	}
	if v := os.Getenv(envPrefix + "CONFIG_VERSION"); v != "" {
		if ver, err := strconv.Atoi(v); err == nil {
			cfg.ConfigVersion = ver
		}
	}

	cfg.Cloud.DerivedURL = cfg.DeriveCloudURL()
}