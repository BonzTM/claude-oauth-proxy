package tokens

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type TokenSet struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	Scope        string    `json:"scope"`
	ExpiresAt    time.Time `json:"expires_at"`
}

func (t TokenSet) Missing() bool {
	return t.AccessToken == ""
}

func (t TokenSet) Expired(now func() time.Time, skew time.Duration) bool {
	if now == nil {
		now = time.Now
	}
	if t.ExpiresAt.IsZero() {
		return true
	}
	return !now().Before(t.ExpiresAt.Add(-skew))
}

type Store interface {
	Load(ctx context.Context) (TokenSet, error)
	Save(ctx context.Context, tokenSet TokenSet) error
	Delete(ctx context.Context) error
	Path() string
}

type FileStore struct {
	path string
}

func NewFileStore(path string) *FileStore {
	return &FileStore{path: path}
}

func (s *FileStore) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

// claudeCredentials represents the Claude CLI ~/.claude/.credentials.json format.
type claudeCredentials struct {
	ClaudeAiOauth *claudeOauth `json:"claudeAiOauth"`
}

type claudeOauth struct {
	AccessToken  string   `json:"accessToken"`
	RefreshToken string   `json:"refreshToken"`
	ExpiresAt    int64    `json:"expiresAt"`
	Scopes       []string `json:"scopes"`
}

func (c *claudeOauth) toTokenSet() TokenSet {
	return TokenSet{
		AccessToken:  c.AccessToken,
		RefreshToken: c.RefreshToken,
		TokenType:    "Bearer",
		Scope:        strings.Join(c.Scopes, " "),
		ExpiresAt:    time.Unix(c.ExpiresAt, 0),
	}
}

func (s *FileStore) Load(_ context.Context) (TokenSet, error) {
	if s == nil {
		return TokenSet{}, os.ErrInvalid
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		return TokenSet{}, err
	}
	var tokenSet TokenSet
	if err := json.Unmarshal(data, &tokenSet); err != nil {
		return TokenSet{}, err
	}
	// If native format didn't yield an access token, try Claude CLI format.
	if tokenSet.Missing() {
		var creds claudeCredentials
		if err := json.Unmarshal(data, &creds); err == nil && creds.ClaudeAiOauth != nil && creds.ClaudeAiOauth.AccessToken != "" {
			return creds.ClaudeAiOauth.toTokenSet(), nil
		}
		return TokenSet{}, errors.New("token file does not contain an access token")
	}
	return tokenSet, nil
}

func (s *FileStore) Save(_ context.Context, tokenSet TokenSet) error {
	if s == nil {
		return os.ErrInvalid
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(tokenSet, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmpPath, s.path)
}

func (s *FileStore) Delete(_ context.Context) error {
	if s == nil {
		return os.ErrInvalid
	}
	err := os.Remove(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// FallbackStore writes to a primary store and falls back to a seed store for
// initial reads. This allows mounting a read-only credentials file (e.g. the
// Claude CLI's .credentials.json) while writing refreshed tokens to a separate
// writable location.
type FallbackStore struct {
	primary Store
	seed    Store
}

func NewFallbackStore(primary, seed Store) *FallbackStore {
	return &FallbackStore{primary: primary, seed: seed}
}

func (f *FallbackStore) Path() string {
	return f.primary.Path()
}

func (f *FallbackStore) Load(ctx context.Context) (TokenSet, error) {
	ts, err := f.primary.Load(ctx)
	if err == nil {
		return ts, nil
	}
	return f.seed.Load(ctx)
}

func (f *FallbackStore) Save(ctx context.Context, tokenSet TokenSet) error {
	return f.primary.Save(ctx, tokenSet)
}

func (f *FallbackStore) Delete(ctx context.Context) error {
	return f.primary.Delete(ctx)
}
