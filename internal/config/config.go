package config

import (
	"os"

	"gopkg.in/yaml.v3"

	"github.com/cloud-print/agent/internal/domain"
	"github.com/cloud-print/agent/internal/errs"
)

func Load(path string) (*domain.AgentConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, errs.Wrap(errs.ErrConfigMissing, "read config file", err)
	}

	var cfg domain.AgentConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, errs.Wrap(errs.ErrConfigInvalid, "parse config yaml", err)
	}

	cfg.ApplyFixedDefaults()
	ApplyEnvOverrides(&cfg)
	cfg.Cloud.DerivedURL = cfg.DeriveCloudURL()

	if err := Validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}