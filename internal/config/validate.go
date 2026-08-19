package config

import (
	"net"
	"strings"

	"github.com/cloud-print/agent/internal/domain"
	"github.com/cloud-print/agent/internal/errs"
)

var validLogLevels = map[string]struct{}{
	"debug": {},
	"info":  {},
	"warn":  {},
	"error": {},
}

func Validate(cfg *domain.AgentConfig) error {
	if cfg.Cloud.Endpoint == "" {
		return errs.New(errs.ErrConfigInvalid, "cloud.endpoint is empty")
	}
	if net.ParseIP(cfg.Cloud.Endpoint) != nil {
		return errs.Newf(errs.ErrConfigInvalid, "cloud.endpoint must be a domain, not IP: %s", cfg.Cloud.Endpoint)
	}
	if !strings.Contains(cfg.Cloud.Endpoint, ".") {
		return errs.Newf(errs.ErrConfigInvalid, "cloud.endpoint must contain at least one dot: %s", cfg.Cloud.Endpoint)
	}

	if cfg.Ops.Port < 1024 || cfg.Ops.Port > 65535 {
		return errs.Newf(errs.ErrConfigInvalid, "ops.port must be in [1024, 65535], got %d", cfg.Ops.Port)
	}

	if cfg.Storage.DataDir == "" {
		return errs.New(errs.ErrConfigInvalid, "storage.data_dir is empty")
	}
	if cfg.Storage.LogDir == "" {
		return errs.New(errs.ErrConfigInvalid, "storage.log_dir is empty")
	}

	if cfg.Log.RetentionDays < 7 || cfg.Log.RetentionDays > 90 {
		return errs.Newf(errs.ErrConfigInvalid, "log.retention_days must be in [7, 90], got %d", cfg.Log.RetentionDays)
	}

	if _, ok := validLogLevels[strings.ToLower(cfg.Log.Level)]; !ok {
		return errs.Newf(errs.ErrConfigInvalid, "log.level must be one of debug/info/warn/error, got %q", cfg.Log.Level)
	}

	return nil
}