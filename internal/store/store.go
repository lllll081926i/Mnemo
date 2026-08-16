// Package store provides local persistence for accounts, settings, tags,
// favorites, transfer history and share history. Backing store is a set of
// atomic JSON files under one directory (no cgo/sqlite dependency).
package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Store is the local persistence root.
type Store struct {
	dir string
	mu  sync.Mutex
}

// Open creates (if needed) and opens the store directory.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

// Dir returns the backing directory.
func (s *Store) Dir() string { return s.dir }

// readJSON loads a collection file; returns os.ErrNotExist when missing.
func (s *Store) readJSON(name string, v any) error {
	b, err := os.ReadFile(filepath.Join(s.dir, name))
	if err != nil {
		return err
	}
	if len(b) == 0 {
		return nil
	}
	return json.Unmarshal(b, v)
}

// writeJSON persists a collection atomically (tmp + rename).
func (s *Store) writeJSON(name string, v any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(s.dir, name+".tmp")
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return renameWithRetry(tmp, filepath.Join(s.dir, name))
}

// renameWithRetry wraps os.Rename with a short retry loop. On Windows the
// target may be briefly locked by antivirus/indexing; a few retries with
// backoff avoid spurious persistence failures.
func renameWithRetry(src, dst string) error {
	var err error
	for i := 0; i < 3; i++ {
		if err = os.Rename(src, dst); err == nil {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return err
}

// listFiles returns collection file names matching the prefix/suffix.
func (s *Store) listFiles(prefix, suffix string) ([]string, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if prefix != "" && len(name) < len(prefix)+len(suffix) {
			continue
		}
		if prefix != "" && name[:len(prefix)] != prefix {
			continue
		}
		if suffix != "" && !hasSuffix(name, suffix) {
			continue
		}
		out = append(out, name)
	}
	return out, nil
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

// errInvalid is a generic store error helper.
func errInvalid(msg string) error { return fmt.Errorf("store: %s", msg) }
