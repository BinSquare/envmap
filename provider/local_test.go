package provider

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLocalFileStoresCreatedAt(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	keyPath := filepath.Join(dir, "key")
	if err := os.WriteFile(keyPath, bytesOfLen(32), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	cfg := ProviderConfig{
		Path: filepath.Join(dir, "secrets.db"),
		Encryption: &EncryptionConfig{
			KeyFile: keyPath,
		},
	}
	envCfg := EnvConfig{PathPrefix: "/app/dev"}

	p, err := newLocalFile(envCfg, cfg)
	if err != nil {
		t.Fatalf("newLocalFile: %v", err)
	}
	lf := p.(*localFile)

	ctx := context.Background()
	fullKey := ApplyPrefix(envCfg, "DB_URL")
	if err := lf.Set(ctx, fullKey, "postgres://example"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	records, err := lf.ListWithMetadata(ctx, ResolvedPrefix(envCfg))
	if err != nil {
		t.Fatalf("ListWithMetadata: %v", err)
	}
	rec, ok := records["DB_URL"]
	if !ok {
		t.Fatalf("expected DB_URL to be returned, got keys %v", records)
	}
	if rec.Value != "postgres://example" {
		t.Fatalf("value mismatch: got %q", rec.Value)
	}
	if rec.CreatedAt.IsZero() {
		t.Fatalf("expected CreatedAt to be set")
	}

	// Reopen provider to ensure CreatedAt persisted to disk.
	time.Sleep(10 * time.Millisecond) // allow measurable difference if overwritten
	p2, err := newLocalFile(envCfg, cfg)
	if err != nil {
		t.Fatalf("re-open newLocalFile: %v", err)
	}
	records2, err := p2.(*localFile).ListWithMetadata(ctx, ResolvedPrefix(envCfg))
	if err != nil {
		t.Fatalf("ListWithMetadata second read: %v", err)
	}
	rec2 := records2["DB_URL"]
	if !rec.CreatedAt.Equal(rec2.CreatedAt) {
		t.Fatalf("created timestamps differ: first=%v second=%v", rec.CreatedAt, rec2.CreatedAt)
	}
}

func bytesOfLen(n int) []byte {
	buf := make([]byte, n)
	for i := range buf {
		buf[i] = byte(1 + (i % 250))
	}
	return buf
}
