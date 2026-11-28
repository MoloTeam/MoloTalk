package access

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
)

// LoadSigningKey читает PKCS#8 PEM с закрытым ключом Ed25519. Сообщения об
// ошибках не содержат байтов ключа.
func LoadSigningKey(path string) (ed25519.PrivateKey, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("access: read signing key: %w", err)
	}

	block, _ := pem.Decode(contents)
	if block == nil || block.Type != "PRIVATE KEY" {
		return nil, errors.New("access: signing key must be a PEM PKCS#8 private key")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.New("access: signing key must be a PKCS#8 Ed25519 private key")
	}
	key, ok := parsed.(ed25519.PrivateKey)
	if !ok || len(key) != ed25519.PrivateKeySize {
		return nil, errors.New("access: signing key must be an Ed25519 private key")
	}
	return key, nil
}
