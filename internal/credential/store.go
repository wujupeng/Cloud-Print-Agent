package credential

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/cloud-print/agent/internal/domain"
	"github.com/cloud-print/agent/internal/errs"
)

type encryptedFields struct {
	DeviceToken []byte `json:"device_token"`
	MTLSCert    []byte `json:"mtls_cert,omitempty"`
	MTLSKey     []byte `json:"mtls_key,omitempty"`
}

func Save(path string, creds *domain.Credentials, masterKey []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return errs.Wrap(errs.ErrStorageIO, "mkdir for credentials", err)
	}

	enc := encryptedFields{}
	if creds.DeviceToken != "" {
		c, err := Encrypt(masterKey, []byte(creds.DeviceToken))
		if err != nil {
			return err
		}
		enc.DeviceToken = c
	}
	if creds.MTLSCert != "" {
		c, err := Encrypt(masterKey, []byte(creds.MTLSCert))
		if err != nil {
			return err
		}
		enc.MTLSCert = c
	}
	if creds.MTLSKey != "" {
		c, err := Encrypt(masterKey, []byte(creds.MTLSKey))
		if err != nil {
			return err
		}
		enc.MTLSKey = c
	}

	data, err := json.Marshal(enc)
	if err != nil {
		return errs.Wrap(errs.ErrCredentialInvalid, "marshal credentials", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return errs.Wrap(errs.ErrStorageIO, "write credentials file", err)
	}

	return nil
}

func Load(path string, masterKey []byte) (*domain.Credentials, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errs.Wrap(errs.ErrCredentialMissing, "read credentials file", err)
	}

	var enc encryptedFields
	if err := json.Unmarshal(data, &enc); err != nil {
		return nil, errs.Wrap(errs.ErrCredentialInvalid, "unmarshal credentials", err)
	}

	creds := &domain.Credentials{}
	if len(enc.DeviceToken) > 0 {
		pt, err := Decrypt(masterKey, enc.DeviceToken)
		if err != nil {
			return nil, err
		}
		creds.DeviceToken = string(pt)
	}
	if len(enc.MTLSCert) > 0 {
		pt, err := Decrypt(masterKey, enc.MTLSCert)
		if err != nil {
			return nil, err
		}
		creds.MTLSCert = string(pt)
	}
	if len(enc.MTLSKey) > 0 {
		pt, err := Decrypt(masterKey, enc.MTLSKey)
		if err != nil {
			return nil, err
		}
		creds.MTLSKey = string(pt)
	}

	return creds, nil
}