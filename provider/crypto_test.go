package provider

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestEncryptDecryptRoundtrip(t *testing.T) {
	key, err := deriveKey([]byte("test-key-material-at-least-16-bytes"))
	if err != nil {
		t.Fatalf("deriveKey: %v", err)
	}
	plaintext := []byte("secret database connection string")

	ciphertext, err := encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	if bytes.Equal(ciphertext, plaintext) {
		t.Error("ciphertext equals plaintext")
	}

	decrypted, err := decrypt(ciphertext, key)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("roundtrip failed: got %q, want %q", decrypted, plaintext)
	}
}

func TestEncryptProducesDifferentCiphertext(t *testing.T) {
	key, _ := deriveKey([]byte("test-key-material-16"))
	plaintext := []byte("same input")

	c1, _ := encrypt(plaintext, key)
	c2, _ := encrypt(plaintext, key)

	if bytes.Equal(c1, c2) {
		t.Error("encrypt should produce different ciphertext each time (random nonce)")
	}
}

func TestDecryptWrongKey(t *testing.T) {
	key1, _ := deriveKey([]byte("key1-must-be-16-bytes"))
	key2, _ := deriveKey([]byte("key2-must-be-16-bytes"))

	ciphertext, _ := encrypt([]byte("secret"), key1)

	_, err := decrypt(ciphertext, key2)
	if err == nil {
		t.Error("decrypt with wrong key should fail")
	}
}

func TestDecryptTruncated(t *testing.T) {
	key, _ := deriveKey([]byte("key-material-16-"))
	ciphertext, _ := encrypt([]byte("secret"), key)

	// Truncate to less than nonce size
	_, err := decrypt(ciphertext[:5], key)
	if err == nil {
		t.Error("decrypt of truncated ciphertext should fail")
	}
}

func TestDecryptCorrupted(t *testing.T) {
	key, _ := deriveKey([]byte("key-material-16-"))
	ciphertext, _ := encrypt([]byte("secret"), key)

	// Flip a bit in the ciphertext
	corrupted := make([]byte, len(ciphertext))
	copy(corrupted, ciphertext)
	corrupted[len(corrupted)-1] ^= 0xff

	_, err := decrypt(corrupted, key)
	if err == nil {
		t.Error("decrypt of corrupted ciphertext should fail")
	}
}

func TestDeriveKeyDeterministic(t *testing.T) {
	material := []byte("consistent-key-material")

	k1, err := deriveKey(material)
	if err != nil {
		t.Fatal(err)
	}
	k2, err := deriveKey(material)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(k1, k2) {
		t.Error("deriveKey should be deterministic")
	}
}

func TestDeriveKeyLength(t *testing.T) {
	key, err := deriveKey([]byte("any-material-here"))
	if err != nil {
		t.Fatal(err)
	}

	// AES-256 requires 32-byte key
	if len(key) != 32 {
		t.Errorf("key length = %d, want 32", len(key))
	}
}

func TestGenerateKeyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "key")

	if err := GenerateKeyFile(path); err != nil {
		t.Fatalf("GenerateKeyFile: %v", err)
	}

	// Check file exists with correct permissions
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat key file: %v", err)
	}

	if info.Mode().Perm() != 0o600 {
		t.Errorf("key file permissions = %o, want 600", info.Mode().Perm())
	}

	// Check content is 32 bytes
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read key file: %v", err)
	}
	if len(data) != 32 {
		t.Errorf("key file length = %d, want 32", len(data))
	}

	// Check directory permissions
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat key dir: %v", err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Errorf("key dir permissions = %o, want 700", dirInfo.Mode().Perm())
	}
}

func TestLoadKeyMaterialMinLength(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "shortkey")

	// Write a key that's too short
	if err := os.WriteFile(path, []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := loadKeyMaterial(&EncryptionConfig{KeyFile: path})
	if err == nil {
		t.Error("expected error for short key file")
	}
}

func TestLoadKeyMaterialPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "badperms")

	if err := os.WriteFile(path, []byte("good-key-material-16"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := loadKeyMaterial(&EncryptionConfig{KeyFile: path})
	if err == nil {
		t.Error("expected error for permissive key file")
	}
}
