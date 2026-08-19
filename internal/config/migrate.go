package config

import (
	"fmt"

	"github.com/cloud-print/agent/internal/domain"
	"github.com/cloud-print/agent/internal/errs"
)

const CurrentConfigVersion = 1

type MigrateFunc func(cfg *domain.AgentConfig) error

type Migrator struct {
	funcs map[[2]int]MigrateFunc
}

func NewMigrator() *Migrator {
	return &Migrator{funcs: make(map[[2]int]MigrateFunc)}
}

func (m *Migrator) Register(from, to int, fn MigrateFunc) {
	m.funcs[[2]int{from, to}] = fn
}

func (m *Migrator) Migrate(cfg *domain.AgentConfig, targetVersion int) error {
	if cfg.ConfigVersion > targetVersion {
		return errs.Newf(errs.ErrConfigVersion, "config version %d is newer than supported %d", cfg.ConfigVersion, targetVersion)
	}
	for cfg.ConfigVersion < targetVersion {
		from := cfg.ConfigVersion
		to := from + 1
		fn, ok := m.funcs[[2]int{from, to}]
		if !ok {
			return errs.Newf(errs.ErrConfigVersion, "no migrator registered for %d -> %d", from, to)
		}
		if err := fn(cfg); err != nil {
			return errs.Wrapf(errs.ErrConfigVersion, err, "migrate %d -> %d", from, to)
		}
		cfg.ConfigVersion = to
	}
	return nil
}

func DefaultMigrator() *Migrator {
	m := NewMigrator()
	m.Register(0, 1, func(cfg *domain.AgentConfig) error {
		if cfg.ConfigVersion == 0 {
			cfg.ConfigVersion = 1
		}
		return nil
	})
	return m
}

func (m *Migrator) Describe() string {
	var keys []string
	for k := range m.funcs {
		keys = append(keys, fmt.Sprintf("%d->%d", k[0], k[1]))
	}
	return fmt.Sprintf("migrator with %d migrations: %v", len(m.funcs), keys)
}