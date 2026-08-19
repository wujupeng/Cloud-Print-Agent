package credential

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"io"

	"github.com/cloud-print/agent/internal/errs"
)

const masterKeyLen = 32

func Encrypt(masterKey []byte, plaintext []byte) ([]byte, error) {
	if len(masterKey) != masterKeyLen {
		return nil, errs.Newf(errs.ErrCredentialInvalid, "master key must be %d bytes, got %d", masterKeyLen, len(masterKey))
	}

	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return nil, errs.Wrap(errs.ErrCredentialInvalid, "create aes cipher", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errs.Wrap(errs.ErrCredentialInvalid, "create gcm", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, errs.Wrap(errs.ErrCredentialInvalid, "generate nonce", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

func Decrypt(masterKey []byte, ciphertext []byte) ([]byte, error) {
	if len(masterKey) != masterKeyLen {
		return nil, errs.Newf(errs.ErrCredentialInvalid, "master key must be %d bytes, got %d", masterKeyLen, len(masterKey))
	}

	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return nil, errs.Wrap(errs.ErrCredentialInvalid, "create aes cipher", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errs.Wrap(errs.ErrCredentialInvalid, "create gcm", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errs.New(errs.ErrCredentialInvalid, "ciphertext too short")
	}

	nonce, ct := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, errs.Wrap(errs.ErrCredentialInvalid, "gcm open", err)
	}

	return plaintext, nil
}