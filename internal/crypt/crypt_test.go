package crypt

import "testing"

func TestEncryptDecrypt(t *testing.T) {
	original := `{"id":"clm_test","repo":"/tmp/test"}`
	encrypted, err := Encrypt(original)
	if err != nil {
		t.Fatal(err)
	}
	if encrypted == original {
		t.Error("encrypted should differ from original")
	}
	decrypted, err := Decrypt(encrypted)
	if err != nil {
		t.Fatal(err)
	}
	if decrypted != original {
		t.Errorf("got %q, want %q", decrypted, original)
	}
}

func TestEncryptBytesDecryptBytes(t *testing.T) {
	data := []byte(`{"commits":[{"hash":"abc"}]}`)
	encrypted, err := EncryptBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	decrypted, err := DecryptBytes(encrypted)
	if err != nil {
		t.Fatal(err)
	}
	if string(decrypted) != string(data) {
		t.Errorf("round-trip failed")
	}
}

func TestDecryptInvalid(t *testing.T) {
	if _, err := Decrypt("!!!"); err == nil {
		t.Error("expected error for invalid base64")
	}
	if _, err := Decrypt("dGVzdA=="); err == nil {
		t.Error("expected error for invalid ciphertext")
	}
}
