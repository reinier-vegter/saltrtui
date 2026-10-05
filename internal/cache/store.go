// Package cache persists the application-wide release update check, shared
// by every Salt master configuration this process is pointed at.
package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

const (
	schemaVersion  = 1
	updateCheckTTL = time.Hour
)

// ErrNotFound distinguishes an absent cache file from an unreadable one.
var ErrNotFound = errors.New("cache entry not found")

// ErrUnsupportedSchema indicates a cache file written by an incompatible version.
var ErrUnsupportedSchema = errors.New("unsupported cache schema")

// UpdateCheck records the most recent GitHub release lookup.
type UpdateCheck struct {
	CheckedAt     time.Time `json:"checkedAt"`
	LatestVersion string    `json:"latestVersion"`
}

type updateCheckEnvelope struct {
	SchemaVersion int         `json:"schemaVersion"`
	Data          UpdateCheck `json:"data"`
}

// Store provides access to the local, application-wide saltrtui cache.
type Store struct {
	directory string
}

// NewStore creates a store rooted at the user's cache directory, independent
// of any active Salt master configuration.
func NewStore() (*Store, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("resolve user cache directory: %w", err)
	}
	return &Store{directory: filepath.Join(cacheDir, "saltrtui")}, nil
}

// LoadUpdateCheck reads the most recent release lookup.
func (s *Store) LoadUpdateCheck() (UpdateCheck, error) {
	var envelope updateCheckEnvelope
	if err := s.load("update-check.json", &envelope); err != nil {
		return UpdateCheck{}, err
	}
	if envelope.SchemaVersion != schemaVersion {
		return UpdateCheck{}, fmt.Errorf("update check: %w: %d", ErrUnsupportedSchema, envelope.SchemaVersion)
	}
	return envelope.Data, nil
}

// SaveUpdateCheck atomically saves the most recent release lookup.
func (s *Store) SaveUpdateCheck(check UpdateCheck) error {
	return s.save("update-check.json", updateCheckEnvelope{SchemaVersion: schemaVersion, Data: check})
}

// IsUpdateCheckFresh reports whether a release lookup was attempted within an hour.
func IsUpdateCheckFresh(check UpdateCheck, now time.Time) bool {
	return !check.CheckedAt.IsZero() && !check.CheckedAt.After(now) && now.Sub(check.CheckedAt) <= updateCheckTTL
}

func (s *Store) load(name string, destination any) error {
	contents, err := os.ReadFile(filepath.Join(s.directory, name))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%s: %w", name, ErrNotFound)
		}
		return fmt.Errorf("read %s: %w", name, err)
	}
	if err := json.Unmarshal(contents, destination); err != nil {
		return fmt.Errorf("decode %s: %w", name, err)
	}
	return nil
}

func (s *Store) save(name string, value any) error {
	contents, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode %s: %w", name, err)
	}
	contents = append(contents, '\n')
	if err := os.MkdirAll(s.directory, 0o700); err != nil {
		return fmt.Errorf("create cache directory: %w", err)
	}

	temporary, err := os.CreateTemp(s.directory, "."+name+"-*")
	if err != nil {
		return fmt.Errorf("create temporary %s: %w", name, err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)

	if _, err := temporary.Write(contents); err != nil {
		temporary.Close()
		return fmt.Errorf("write temporary %s: %w", name, err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync temporary %s: %w", name, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary %s: %w", name, err)
	}
	if err := os.Rename(temporaryName, filepath.Join(s.directory, name)); err != nil {
		return fmt.Errorf("replace %s: %w", name, err)
	}
	return nil
}
