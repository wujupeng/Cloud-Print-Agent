package credential_test

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloud-print/agent/internal/credential"
	"github.com/cloud-print/agent/internal/domain"
	"github.com/cloud-print/agent/internal/errs"
)

func genMasterKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)
	return key
}

func TestEncryptDecrypt(t *testing.T) {
	key := genMasterKey(t)

	cases := []string{
		"",
		"hello",
		"device-token-1234567890",
		"-----BEGIN CERTIFICATE-----\nMIIBxTCCAWugAwIBAgIU\n-----END CERTIFICATE-----",
	}
	for _, plain := range cases {
		ct, err := credential.Encrypt(key, []byte(plain))
		require.NoError(t, err)
		assert.NotEmpty(t, ct)
		assert.NotEqual(t, []byte(plain), ct)

		pt, err := credential.Decrypt(key, ct)
		require.NoError(t, err)
		assert.Equal(t, plain, string(pt))
	}
}

func TestEncrypt_DifferentCiphertext(t *testing.T) {
	key := genMasterKey(t)
	plain := []byte("same-plaintext")

	c1, err := credential.Encrypt(key, plain)
	require.NoError(t, err)
	c2, err := credential.Encrypt(key, plain)
	require.NoError(t, err)
	assert.NotEqual(t, c1, c2, "GCM nonce should randomize ciphertext")

	pt1, err := credential.Decrypt(key, c1)
	require.NoError(t, err)
	pt2, err := credential.Decrypt(key, c2)
	require.NoError(t, err)
	assert.Equal(t, pt1, pt2)
}

func TestWrongKey(t *testing.T) {
	key1 := genMasterKey(t)
	key2 := genMasterKey(t)
	require.NotEqual(t, key1, key2)

	ct, err := credential.Encrypt(key1, []byte("secret"))
	require.NoError(t, err)

	_, err = credential.Decrypt(key2, ct)
	require.Error(t, err)
	var ae *errs.AgentError
	assert.ErrorAs(t, err, &ae)
	assert.Equal(t, errs.ErrCredentialInvalid, ae.Code)
}

func TestEncrypt_InvalidKeyLen(t *testing.T) {
	_, err := credential.Encrypt([]byte("short"), []byte("x"))
	require.Error(t, err)
	var ae *errs.AgentError
	assert.ErrorAs(t, err, &ae)
	assert.Equal(t, errs.ErrCredentialInvalid, ae.Code)
}

func TestDecrypt_ShortCiphertext(t *testing.T) {
	key := genMasterKey(t)
	_, err := credential.Decrypt(key, []byte("x"))
	require.Error(t, err)
	var ae *errs.AgentError
	assert.ErrorAs(t, err, &ae)
	assert.Equal(t, errs.ErrCredentialInvalid, ae.Code)
}

func TestCredentialStore(t *testing.T) {
	key := genMasterKey(t)
	path := filepath.Join(t.TempDir(), "creds.json")

	original := &domain.Credentials{
		DeviceToken: "device-token-abc-123",
		MTLSCert:    "cert-content",
		MTLSKey:     "key-content",
	}

	require.NoError(t, credential.Save(path, original, key))

	info, err := os.Stat(path)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}

	loaded, err := credential.Load(path, key)
	require.NoError(t, err)
	assert.Equal(t, original.DeviceToken, loaded.DeviceToken)
	assert.Equal(t, original.MTLSCert, loaded.MTLSCert)
	assert.Equal(t, original.MTLSKey, loaded.MTLSKey)
}

func TestCredentialStore_PartialFields(t *testing.T) {
	key := genMasterKey(t)
	path := filepath.Join(t.TempDir(), "creds_partial.json")

	original := &domain.Credentials{
		DeviceToken: "only-token",
	}
	require.NoError(t, credential.Save(path, original, key))

	loaded, err := credential.Load(path, key)
	require.NoError(t, err)
	assert.Equal(t, "only-token", loaded.DeviceToken)
	assert.Empty(t, loaded.MTLSCert)
	assert.Empty(t, loaded.MTLSKey)
}

func TestCredentialStore_WrongKeyFailsLoad(t *testing.T) {
	key1 := genMasterKey(t)
	key2 := genMasterKey(t)
	path := filepath.Join(t.TempDir(), "creds_wk.json")

	require.NoError(t, credential.Save(path, &domain.Credentials{DeviceToken: "x"}, key1))

	_, err := credential.Load(path, key2)
	require.Error(t, err)
	var ae *errs.AgentError
	assert.ErrorAs(t, err, &ae)
	assert.Equal(t, errs.ErrCredentialInvalid, ae.Code)
}

func TestCredentialStore_MissingFile(t *testing.T) {
	key := genMasterKey(t)
	_, err := credential.Load(filepath.Join(t.TempDir(), "missing.json"), key)
	require.Error(t, err)
	var ae *errs.AgentError
	assert.ErrorAs(t, err, &ae)
	assert.Equal(t, errs.ErrCredentialMissing, ae.Code)
}