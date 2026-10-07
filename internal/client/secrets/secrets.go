// Package secrets stores per-node credentials (WireGuard private key and
// device token) in the OS keychain: Keychain on macOS, Credential Manager on
// Windows, Secret Service on Linux. When no keychain is available (common
// on minimal Linux desktops) it falls back to a 0600 file in the app's
// config directory.
package secrets

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/zalando/go-keyring"
)

const service = "ExitLagFree"

var ErrNotFound = errors.New("secret not found")

type Store struct {
	path       string
	useKeyring bool
	mu         sync.Mutex
}

func New(dir string) *Store {
	return &Store{path: filepath.Join(dir, "secrets.json"), useKeyring: true}
}

// NewFileOnly skips the OS keychain entirely.
func NewFileOnly(dir string) *Store {
	return &Store{path: filepath.Join(dir, "secrets.json")}
}

func (s *Store) Set(key, value string) error {
	if s.useKeyring {
		if err := keyring.Set(service, key, value); err == nil {
			_ = s.fileDelete(key)
			return nil
		}
	}
	return s.fileSet(key, value)
}

func (s *Store) Get(key string) (string, error) {
	if s.useKeyring {
		if v, err := keyring.Get(service, key); err == nil {
			return v, nil
		}
	}
	return s.fileGet(key)
}

func (s *Store) Delete(key string) {
	if s.useKeyring {
		_ = keyring.Delete(service, key)
	}
	_ = s.fileDelete(key)
}

func (s *Store) load() (map[string]string, error) {
	m := map[string]string{}
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	return m, json.Unmarshal(b, &m)
}

func (s *Store) save(m map[string]string) error {
	if len(m) == 0 {
		err := os.Remove(s.path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	b, _ := json.Marshal(m)
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Store) fileSet(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.load()
	if err != nil {
		return err
	}
	m[key] = value
	return s.save(m)
}

func (s *Store) fileGet(key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.load()
	if err != nil {
		return "", err
	}
	v, ok := m[key]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

func (s *Store) fileDelete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.load()
	if err != nil {
		return err
	}
	if _, ok := m[key]; !ok {
		return nil
	}
	delete(m, key)
	return s.save(m)
}
