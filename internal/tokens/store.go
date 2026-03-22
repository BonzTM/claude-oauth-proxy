package tokens

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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
	if tokenSet.Missing() {
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
