package tokens

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTokenSetExpired(t *testing.T) {
	now := time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC)
	valid := TokenSet{AccessToken: "a", ExpiresAt: now.Add(10 * time.Minute)}
	if valid.Expired(func() time.Time { return now }, 5*time.Minute) {
		t.Fatal("expected token to be valid")
	}
	expired := TokenSet{AccessToken: "a", ExpiresAt: now.Add(4 * time.Minute)}
	if !expired.Expired(func() time.Time { return now }, 5*time.Minute) {
		t.Fatal("expected token to be expired within skew")
	}
	if !(TokenSet{}).Expired(nil, 0) {
		t.Fatal("expected zero token to be expired")
	}
}

func TestFileStoreRoundTripAndDelete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tokens.json")
	store := NewFileStore(path)
	ctx := context.Background()
	tokenSet := TokenSet{AccessToken: "access", RefreshToken: "refresh", ExpiresAt: time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC)}
	if err := store.Save(ctx, tokenSet); err != nil {
		t.Fatalf("save tokens: %v", err)
	}
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat token file: %v", err)
	}
	if stat.Mode().Perm() != 0o600 {
		t.Fatalf("unexpected mode: %v", stat.Mode().Perm())
	}
	loaded, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("load tokens: %v", err)
	}
	if loaded.AccessToken != tokenSet.AccessToken || loaded.RefreshToken != tokenSet.RefreshToken {
		t.Fatalf("unexpected loaded tokens: %+v", loaded)
	}
	if err := store.Delete(ctx); err != nil {
		t.Fatalf("delete token file: %v", err)
	}
	if err := store.Delete(ctx); err != nil {
		t.Fatalf("delete missing token file: %v", err)
	}
}

func TestFileStoreMissingAndInvalidCases(t *testing.T) {
	store := NewFileStore(filepath.Join(t.TempDir(), "missing.json"))
	if _, err := store.Load(context.Background()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected missing file error, got %v", err)
	}
	if _, err := (*FileStore)(nil).Load(context.Background()); !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("expected invalid store load, got %v", err)
	}
	if err := (*FileStore)(nil).Save(context.Background(), TokenSet{}); !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("expected invalid store save, got %v", err)
	}
	if err := (*FileStore)(nil).Delete(context.Background()); !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("expected invalid store delete, got %v", err)
	}
	path := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(path, []byte(`{"refresh_token":"only"}`), 0o600); err != nil {
		t.Fatalf("write invalid token file: %v", err)
	}
	if _, err := NewFileStore(path).Load(context.Background()); err == nil {
		t.Fatal("expected invalid token data error")
	}
	if (*FileStore)(nil).Path() != "" {
		t.Fatal("expected nil store path to be empty")
	}
	parentFile := filepath.Join(t.TempDir(), "file-parent")
	if err := os.WriteFile(parentFile, []byte("x"), 0o600); err != nil {
		t.Fatalf("write parent file: %v", err)
	}
	if err := NewFileStore(filepath.Join(parentFile, "tokens.json")).Save(context.Background(), TokenSet{AccessToken: "x"}); err == nil {
		t.Fatal("expected save failure when parent is a file")
	}
}
