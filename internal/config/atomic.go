package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/cloud-print/agent/internal/domain"
	"github.com/cloud-print/agent/internal/errs"
)

func SaveAtomic(path string, cfg *domain.AgentConfig) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return errs.Wrap(errs.ErrStorageIO, "mkdir for config", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return errs.Wrap(errs.ErrConfigInvalid, "marshal config yaml", err)
	}

	tmp, err := os.CreateTemp(dir, fmt.Sprintf(".%s-", filepath.Base(path)))
	if err != nil {
		return errs.Wrap(errs.ErrStorageIO, "create temp file", err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}

	if err := tmp.Chmod(0o644); err != nil {
		cleanup()
		return errs.Wrap(errs.ErrStorageIO, "chmod temp file", err)
	}

	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return errs.Wrap(errs.ErrStorageIO, "write temp file", err)
	}

	if err := tmp.Sync(); err != nil {
		cleanup()
		return errs.Wrap(errs.ErrStorageIO, "sync temp file", err)
	}

	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return errs.Wrap(errs.ErrStorageIO, "close temp file", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return errs.Wrap(errs.ErrStorageIO, "rename temp file", err)
	}

	return nil
}