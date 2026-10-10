package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Vera keeps two files under the user's config directory, and the split is
// deliberate: settings are shareable, a token is not.
//
//	config.json   the API address and output preferences   0644
//	session.json  the bearer token and who it belongs to   0600
//
// The desktop stores its token in the OS keyring instead, which is right for a
// windowed app on a laptop and wrong here: a headless server has no keyring, and
// a CI job has no keyring either. A file with 0600 is the honest equivalent, and
// VERA_TOKEN overrides it entirely for scripts that should not write one at all.

// Session is the signed-in state: which server, who, and the token.
type Session struct {
	API         string    `json:"api"`
	Token       string    `json:"token"`
	Email       string    `json:"email"`
	DisplayName string    `json:"displayName,omitempty"`
	Role        string    `json:"role,omitempty"`
	SignedInAt  time.Time `json:"signedInAt"`
}

// Config is the non-secret half.
type Config struct {
	API  string `json:"api,omitempty"`
	JSON bool   `json:"json,omitempty"`
}

func configDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate the config directory: %w", err)
	}
	return filepath.Join(base, "vera"), nil
}

func sessionPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "session.json"), nil
}

func configPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// LoadSession returns the stored session, or nil when there is none.
//
// A missing file is not an error -- it is the state of anyone who has not signed
// in yet -- but a corrupt or unreadable one is, because silently treating it as
// "signed out" would send someone hunting for a password problem that is really
// a permissions problem.
func LoadSession() (*Session, error) {
	path, err := sessionPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("%s is not valid JSON (%v); delete it and sign in again", path, err)
	}
	return &s, nil
}

// SaveSession writes the session with owner-only permissions.
func SaveSession(s *Session) error {
	dir, err := configDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	path := filepath.Join(dir, "session.json")
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("encode session: %w", err)
	}
	// Write then move, so an interrupted write cannot leave a truncated token
	// on disk for the next command to read.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}

// ClearSession removes the stored token. Absent is success: signing out twice
// is not an error.
func ClearSession() error {
	path, err := sessionPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return nil
}

// LoadConfig reads the settings file; a missing one is simply empty settings.
func LoadConfig() (Config, error) {
	path, err := configPath()
	if err != nil {
		return Config{}, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read %s: %w", path, err)
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}, fmt.Errorf("%s is not valid JSON (%v); delete it and run 'vera config set' again", path, err)
	}
	return c, nil
}

// SaveConfig writes the settings file.
func SaveConfig(c Config) error {
	dir, err := configDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	path := filepath.Join(dir, "config.json")
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
