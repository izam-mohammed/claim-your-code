package crypt

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestDeriveKeyIsStableAnd32Bytes(t *testing.T) {
	k1, err := deriveKey()
	if err != nil {
		t.Fatalf("deriveKey: %v", err)
	}
	if len(k1) != 32 {
		t.Fatalf("key length = %d, want 32 (AES-256)", len(k1))
	}
	k2, _ := deriveKey()
	if string(k1) != string(k2) {
		t.Error("deriveKey is not deterministic on the same machine")
	}
}

func TestEncryptProducesDistinctCiphertexts(t *testing.T) {
	a, err := Encrypt("same-plaintext")
	if err != nil {
		t.Fatal(err)
	}
	b, err := Encrypt("same-plaintext")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Error("two encryptions of the same plaintext are identical — nonce is not random")
	}
	for _, c := range []string{a, b} {
		got, err := Decrypt(c)
		if err != nil {
			t.Fatalf("Decrypt: %v", err)
		}
		if got != "same-plaintext" {
			t.Errorf("round trip = %q, want %q", got, "same-plaintext")
		}
	}
}

func TestEncryptEmptyString(t *testing.T) {
	enc, err := Encrypt("")
	if err != nil {
		t.Fatalf("Encrypt(\"\"): %v", err)
	}
	got, err := Decrypt(enc)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if got != "" {
		t.Errorf("round trip of empty string = %q", got)
	}
}

func TestEncryptLargePayload(t *testing.T) {
	big := strings.Repeat("claim-your-code ", 10000)
	enc, err := Encrypt(big)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decrypt(enc)
	if err != nil {
		t.Fatal(err)
	}
	if got != big {
		t.Error("large payload did not survive the round trip")
	}
}

func TestDecryptRejectsBadBase64(t *testing.T) {
	if _, err := Decrypt("not!valid!base64!"); err == nil {
		t.Error("Decrypt accepted invalid base64")
	}
}

func TestDecryptRejectsShortCiphertext(t *testing.T) {
	// Shorter than a GCM nonce (12 bytes).
	short := base64.StdEncoding.EncodeToString([]byte("tiny"))
	_, err := Decrypt(short)
	if err == nil {
		t.Fatal("Decrypt accepted a ciphertext shorter than the nonce")
	}
	if !strings.Contains(err.Error(), "too short") {
		t.Errorf("error = %v, want it to mention the ciphertext being too short", err)
	}
}

func TestDecryptRejectsTamperedCiphertext(t *testing.T) {
	enc, err := Encrypt("secret-token")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		t.Fatal(err)
	}
	// Flip a bit in the sealed body, past the nonce.
	raw[len(raw)-1] ^= 0x01
	_, err = Decrypt(base64.StdEncoding.EncodeToString(raw))
	if err == nil {
		t.Fatal("Decrypt accepted a tampered ciphertext — GCM authentication is not working")
	}
	if !strings.Contains(err.Error(), "decryption failed") {
		t.Errorf("error = %v, want a decryption-failed message", err)
	}
}

func TestDecryptBytesPropagatesError(t *testing.T) {
	if _, err := DecryptBytes("!!!not base64!!!"); err == nil {
		t.Error("DecryptBytes accepted invalid base64")
	}
}

func TestEncryptBytesDecryptBytesBinarySafe(t *testing.T) {
	payload := []byte{0x00, 0x01, 0xff, 0xfe, 0x7f, 0x80}
	enc, err := EncryptBytes(payload)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecryptBytes(enc)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Errorf("binary round trip = %v, want %v", got, payload)
	}
}
