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
	// Claude CLI .credentials.json format should be loaded transparently.
	claudePath := filepath.Join(t.TempDir(), "credentials.json")
	claudeJSON := `{"claudeAiOauth":{"accessToken":"claude-access","refreshToken":"claude-refresh","expiresAt":1774396800,"scopes":["user:inference"]}}`
	if err := os.WriteFile(claudePath, []byte(claudeJSON), 0o600); err != nil {
		t.Fatalf("write claude credentials: %v", err)
	}
	claudeTokens, err := NewFileStore(claudePath).Load(context.Background())
	if err != nil {
		t.Fatalf("load claude credentials: %v", err)
	}
	if claudeTokens.AccessToken != "claude-access" || claudeTokens.RefreshToken != "claude-refresh" || claudeTokens.Scope != "user:inference" {
		t.Fatalf("unexpected claude tokens: %+v", claudeTokens)
	}
	if claudeTokens.ExpiresAt.Unix() != 1774396800 {
		t.Fatalf("unexpected expires_at: %v", claudeTokens.ExpiresAt)
	}

	parentFile := filepath.Join(t.TempDir(), "file-parent")
	if err := os.WriteFile(parentFile, []byte("x"), 0o600); err != nil {
		t.Fatalf("write parent file: %v", err)
	}
	if err := NewFileStore(filepath.Join(parentFile, "tokens.json")).Save(context.Background(), TokenSet{AccessToken: "x"}); err == nil {
		t.Fatal("expected save failure when parent is a file")
	}
}

func TestFallbackStore(t *testing.T) {
	ctx := context.Background()

	// Set up a seed file with Claude CLI credentials.
	seedPath := filepath.Join(t.TempDir(), "seed.json")
	if err := os.WriteFile(seedPath, []byte(`{"claudeAiOauth":{"accessToken":"seed-token","refreshToken":"seed-refresh","expiresAt":1774396800,"scopes":["user:inference"]}}`), 0o600); err != nil {
		t.Fatalf("write seed: %v", err)
	}

	primaryPath := filepath.Join(t.TempDir(), "tokens.json")
	store := NewFallbackStore(NewFileStore(primaryPath), NewFileStore(seedPath))

	// Load should fall back to seed when primary doesn't exist.
	ts, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("fallback load: %v", err)
	}
	if ts.AccessToken != "seed-token" {
		t.Fatalf("expected seed token, got %q", ts.AccessToken)
	}

	// Save writes to primary.
	saved := TokenSet{AccessToken: "refreshed", RefreshToken: "new-refresh", ExpiresAt: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)}
	if err := store.Save(ctx, saved); err != nil {
		t.Fatalf("save: %v", err)
	}

	// After save, Load returns from primary.
	ts, err = store.Load(ctx)
	if err != nil {
		t.Fatalf("load after save: %v", err)
	}
	if ts.AccessToken != "refreshed" {
		t.Fatalf("expected primary token, got %q", ts.AccessToken)
	}

	// Delete removes primary; next Load falls back to seed again.
	if err := store.Delete(ctx); err != nil {
		t.Fatalf("delete: %v", err)
	}
	ts, err = store.Load(ctx)
	if err != nil {
		t.Fatalf("load after delete: %v", err)
	}
	if ts.AccessToken != "seed-token" {
		t.Fatalf("expected seed token after delete, got %q", ts.AccessToken)
	}
}
