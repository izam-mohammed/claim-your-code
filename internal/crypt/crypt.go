package crypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/user"

	"golang.org/x/crypto/pbkdf2"
)

// deriveKey generates a 32-byte AES key from machine-specific data.
func deriveKey() ([]byte, error) {
	hostname, _ := os.Hostname()
	u, _ := user.Current()
	username := ""
	if u != nil {
		username = u.Username
	}
	passphrase := fmt.Sprintf("claim:%s:%s", hostname, username)

	salt := []byte("claim-your-code-v1")
	return pbkdf2.Key([]byte(passphrase), salt, 100000, 32, sha256.New), nil
}

// Encrypt encrypts plaintext using AES-256-GCM.
func Encrypt(plaintext string) (string, error) {
	key, err := deriveKey()
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts a base64-encoded AES-256-GCM ciphertext.
func Decrypt(encoded string) (string, error) {
	key, err := deriveKey()
	if err != nil {
		return "", err
	}

	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decryption failed (data may be from a different machine)")
	}

	return string(plaintext), nil
}

// EncryptBytes encrypts raw bytes.
func EncryptBytes(data []byte) (string, error) {
	return Encrypt(string(data))
}

// DecryptBytes decrypts to raw bytes.
func DecryptBytes(encoded string) ([]byte, error) {
	s, err := Decrypt(encoded)
	if err != nil {
		return nil, err
	}
	return []byte(s), nil
}
