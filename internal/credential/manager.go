package credential

import (
	"encoding/base64"
	"os"

	"github.com/cloud-print/agent/internal/domain"
	"github.com/cloud-print/agent/internal/errs"
)

type Manager struct {
	masterKey []byte
	creds     *domain.Credentials
	envKey    string
	encPath   string
}

func NewManager(envKey string, encPath string) (*Manager, error) {
	raw := os.Getenv(envKey)
	if raw == "" {
		return nil, errs.Newf(errs.ErrCredentialMissing, "master key env %s is empty", envKey)
	}

	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, errs.Wrap(errs.ErrCredentialInvalid, "decode master key base64", err)
	}
	if len(key) != masterKeyLen {
		return nil, errs.Newf(errs.ErrCredentialInvalid, "master key must be %d bytes after base64 decode, got %d", masterKeyLen, len(key))
	}

	creds, err := Load(encPath, key)
	if err != nil {
		return nil, err
	}

	return &Manager{
		masterKey: key,
		creds:     creds,
		envKey:    envKey,
		encPath:   encPath,
	}, nil
}

func (m *Manager) GetDeviceToken() string {
	return m.creds.DeviceToken
}

func (m *Manager) GetMTLSCert() string {
	return m.creds.MTLSCert
}

func (m *Manager) GetMTLSKey() string {
	return m.creds.MTLSKey
}

func (m *Manager) Credentials() *domain.Credentials {
	return m.creds
}

func (m *Manager) Reload() error {
	creds, err := Load(m.encPath, m.masterKey)
	if err != nil {
		return err
	}
	m.creds = creds
	return nil
}

func (m *Manager) Save(creds *domain.Credentials) error {
	if err := Save(m.encPath, creds, m.masterKey); err != nil {
		return err
	}
	m.creds = creds
	return nil
}