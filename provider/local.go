package provider

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gofrs/flock"
	"golang.org/x/crypto/hkdf"
)

func init() {
	Register(Info{
		Type:           "local-file",
		Description:    "Encrypted local file storage",
		Factory:        newLocalFile,
		RequiredFields: []string{"path", "encryption"},
		OptionalFields: []string{},
	})

	Register(Info{
		Type:           "local-store",
		Description:    "Encrypted local file storage (alias for local-file)",
		Factory:        newLocalFile,
		RequiredFields: []string{"path", "encryption"},
		OptionalFields: []string{},
	})
}

type localFile struct {
	envCfg      EnvConfig
	providerCfg ProviderConfig
	key         []byte
	path        string
	lock        *flock.Flock
	mu          sync.Mutex
}

func newLocalFile(envCfg EnvConfig, providerCfg ProviderConfig) (Provider, error) {
	if providerCfg.Path == "" {
		return nil, fmt.Errorf("local-file provider missing path")
	}
	if providerCfg.Encryption == nil {
		return nil, fmt.Errorf("local-file provider requires encryption configuration")
	}
	keyMaterial, err := loadKeyMaterial(providerCfg.Encryption)
	if err != nil {
		return nil, err
	}
	key, err := deriveKey(keyMaterial)
	if err != nil {
		return nil, fmt.Errorf("derive encryption key: %w", err)
	}
	lockPath := providerCfg.Path + ".lock"
	return &localFile{
		envCfg:      envCfg,
		providerCfg: providerCfg,
		key:         key,
		path:        providerCfg.Path,
		lock:        flock.New(lockPath),
	}, nil
}

func (p *localFile) Get(_ context.Context, name string) (string, error) {
	var value string
	err := p.withExclusiveLock(func() error {
		entries, err := p.readAllUnlocked()
		if err != nil {
			return err
		}
		val, ok := entries[name]
		if !ok {
			return fmt.Errorf("missing secret %s for env (expected from %s)", name, p.path)
		}
		value = val
		return nil
	})
	return value, err
}

func (p *localFile) List(_ context.Context, prefix string) (map[string]string, error) {
	out := make(map[string]string)
	err := p.withExclusiveLock(func() error {
		entries, err := p.readAllUnlocked()
		if err != nil {
			return err
		}
		for name, val := range entries {
			if prefix != "" && !strings.HasPrefix(name, ensurePrefixSlash(prefix)) {
				continue
			}
			base := TrimPrefix(p.envCfg, name)
			out[base] = val
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (p *localFile) Set(_ context.Context, name, value string) error {
	return p.withExclusiveLock(func() error {
		entries, err := p.readAllUnlocked()
		if err != nil {
			return err
		}
		entries[name] = value
		return p.writeAllUnlocked(entries)
	})
}

func (p *localFile) readAllUnlocked() (map[string]string, error) {
	entries := map[string]string{}
	raw, err := os.ReadFile(p.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return entries, nil
		}
		return nil, fmt.Errorf("read local store: %w", err)
	}
	if len(raw) == 0 {
		return entries, nil
	}
	plaintext, err := decrypt(raw, p.key)
	if err != nil {
		return nil, fmt.Errorf("decrypt local store: %w", err)
	}
	if err := json.Unmarshal(plaintext, &entries); err != nil {
		return nil, fmt.Errorf("parse local store: %w", err)
	}
	return entries, nil
}

func (p *localFile) writeAllUnlocked(entries map[string]string) error {
	encoded, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("encode local store: %w", err)
	}
	ciphertext, err := encrypt(encoded, p.key)
	if err != nil {
		return fmt.Errorf("encrypt local store: %w", err)
	}

	// Atomic write: write to temp file, then rename
	dir := filepath.Dir(p.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create local store dir: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".secrets-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	// Ensure cleanup on failure
	success := false
	defer func() {
		if !success {
			os.Remove(tmpPath)
		}
	}()

	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if _, err := tmp.Write(ciphertext); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, p.path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}

	success = true
	return nil
}

func (p *localFile) withExclusiveLock(fn func() error) error {
	if err := p.lock.Lock(); err != nil {
		return fmt.Errorf("acquire lock %s: %w", p.lock.Path(), err)
	}
	defer func() {
		if err := p.lock.Unlock(); err != nil {
			log.Printf("envmap: unlock %s: %v", p.lock.Path(), err)
		}
	}()

	p.mu.Lock()
	defer p.mu.Unlock()

	return fn()
}

// --- Key Management ---

// deriveKey uses HKDF to derive a 256-bit encryption key from arbitrary key material.
// HKDF is appropriate for both high-entropy (random file) and lower-entropy (passphrase) inputs.
func deriveKey(material []byte) ([]byte, error) {
	// HKDF with SHA-256, no salt (we use random nonces per encryption),
	// and a fixed info string to bind the key to this purpose.
	hkdfReader := hkdf.New(sha256.New, material, nil, []byte("envmap-local-encryption-v1"))

	key := make([]byte, 32) // AES-256
	if _, err := io.ReadFull(hkdfReader, key); err != nil {
		return nil, err
	}
	return key, nil
}

func loadKeyMaterial(cfg *EncryptionConfig) ([]byte, error) {
	if cfg.KeyEnv != "" {
		if v := os.Getenv(cfg.KeyEnv); v != "" {
			return []byte(v), nil
		}
		return nil, fmt.Errorf("key env var %s is empty or not set", cfg.KeyEnv)
	}

	if cfg.KeyFile == "" {
		return nil, fmt.Errorf("no key source provided; set encryption.key_env or encryption.key_file")
	}
	info, err := os.Stat(cfg.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("stat key file: %w", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("key file %s is too permissive (%#o); run: chmod 600 %s", cfg.KeyFile, info.Mode().Perm(), cfg.KeyFile)
	}
	data, err := os.ReadFile(cfg.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("read key file: %w", err)
	}
	if len(data) < 16 {
		return nil, fmt.Errorf("key file %s is too short (%d bytes); use at least 16 bytes of random data", cfg.KeyFile, len(data))
	}
	return data, nil
}

// --- Encryption ---

func encrypt(plaintext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
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
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

func decrypt(ciphertext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	size := gcm.NonceSize()
	if len(ciphertext) < size {
		return nil, errors.New("ciphertext too short")
	}
	nonce := ciphertext[:size]
	data := ciphertext[size:]
	return gcm.Open(nil, nonce, data, nil)
}

// GenerateKeyFile creates a new cryptographically secure key file.
// Call this to bootstrap local storage.
func GenerateKeyFile(path string) error {
	key := make([]byte, 32) // 256 bits of entropy
	if _, err := rand.Read(key); err != nil {
		return fmt.Errorf("generate random key: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create key directory: %w", err)
	}

	if err := os.WriteFile(path, key, 0o600); err != nil {
		return fmt.Errorf("write key file: %w", err)
	}

	return nil
}
