package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type encryptedFields struct {
	DeviceToken []byte `json:"device_token"`
}

func encrypt(masterKey []byte, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func main() {
	token := flag.String("token", "", "device token to encrypt")
	outPath := flag.String("out", "/etc/cloud-print-agent/credentials.enc", "output path")
	flag.Parse()

	if *token == "" {
		fmt.Fprintln(os.Stderr, "token is required")
		os.Exit(1)
	}

	masterKey := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, masterKey); err != nil {
		fmt.Fprintln(os.Stderr, "gen master key:", err)
		os.Exit(1)
	}

	encToken, err := encrypt(masterKey, []byte(*token))
	if err != nil {
		fmt.Fprintln(os.Stderr, "encrypt token:", err)
		os.Exit(1)
	}

	enc := encryptedFields{DeviceToken: encToken}
	data, err := json.Marshal(enc)
	if err != nil {
		fmt.Fprintln(os.Stderr, "marshal:", err)
		os.Exit(1)
	}

	dir := filepath.Dir(*outPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		fmt.Fprintln(os.Stderr, "mkdir:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*outPath, data, 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "write:", err)
		os.Exit(1)
	}

	fmt.Println("MasterKey (base64):", base64.StdEncoding.EncodeToString(masterKey))
	fmt.Println("Credentials saved to:", *outPath)
}