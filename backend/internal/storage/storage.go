// Package storage stores uploaded files (invoices, warranty cards, photos).
// V1 uses the local filesystem (a Docker volume); an S3/MinIO implementation can be added
// behind the same interface without touching handlers.
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Storage abstracts blob storage.
type Storage interface {
	Put(ctx context.Context, key string, r io.Reader) (int64, error)
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}

// ErrNotFound is returned when a key does not exist.
var ErrNotFound = errors.New("object not found")

var validKey = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9/_.-]{0,200}$`)

// Local stores files under Dir.
type Local struct{ Dir string }

// NewLocal creates the directory if needed.
func NewLocal(dir string) (*Local, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	return &Local{Dir: dir}, nil
}

func (l *Local) path(key string) (string, error) {
	if !validKey.MatchString(key) || strings.Contains(key, "..") {
		return "", fmt.Errorf("invalid storage key %q", key)
	}
	return filepath.Join(l.Dir, filepath.FromSlash(key)), nil
}

// Put writes r to key atomically (temp file + rename).
func (l *Local) Put(_ context.Context, key string, r io.Reader) (int64, error) {
	p, err := l.path(key)
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return 0, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".upload-*")
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(tmp, r)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
		return 0, err
	}
	if err := os.Rename(tmp.Name(), p); err != nil {
		_ = os.Remove(tmp.Name())
		return 0, err
	}
	return n, nil
}

// Open opens key for reading.
func (l *Local) Open(_ context.Context, key string) (io.ReadCloser, error) {
	p, err := l.path(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	return f, err
}

// Delete removes key (missing keys are not an error).
func (l *Local) Delete(_ context.Context, key string) error {
	p, err := l.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
