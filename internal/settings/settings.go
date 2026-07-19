// Package settings persists the user's durable, cross-restart choices —
// the Active Vault and Settings entries in CONTEXT.md — distinct from the
// disposable per-vault Engines cache. Deliberately dumb: no validation of
// the vault path lives here (that belongs to the vault-switch
// orchestration, the actual decision point for "is this a real vault").
package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

// maxVaultHistory bounds VaultHistory so the file doesn't grow unbounded
// across a long-lived install.
const maxVaultHistory = 10

// Settings is the durable, cross-restart state written to settings.json.
type Settings struct {
	VaultPath    string   `json:"vault_path"`
	Theme        string   `json:"theme"`
	VaultHistory []string `json:"vault_history"`
}

// Path returns ~/.config/ks/settings.json. os.UserConfigDir() resolves to
// $XDG_CONFIG_HOME (or $HOME/.config if unset) on Linux, which is exactly
// ~/.config — confirmed via `go doc os UserConfigDir`.
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolving user config dir: %w", err)
	}
	return filepath.Join(dir, "ks", "settings.json"), nil
}

// Load reads settings.json. A missing file is the normal first-run case
// (no vault selected yet) and returns a zero-value Settings with no error,
// not a failure.
func Load() (Settings, error) {
	path, err := Path()
	if err != nil {
		return Settings{}, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Settings{}, nil
		}
		return Settings{}, fmt.Errorf("reading settings %s: %w", path, err)
	}

	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		return Settings{}, fmt.Errorf("parsing settings %s: %w", path, err)
	}
	return s, nil
}

// Save writes settings atomically: encode to a temp file in the same
// directory, then os.Rename over the real path, so a crash mid-write
// can't leave settings.json corrupt — this is durable state, unlike the
// disposable gob cache (see ADR-0011).
func Save(s Settings) error {
	path, err := Path()
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating settings dir %s: %w", dir, err)
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding settings: %w", err)
	}

	tmp, err := os.CreateTemp(dir, "settings-*.json.tmp")
	if err != nil {
		return fmt.Errorf("creating temp settings file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("writing temp settings file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("closing temp settings file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming temp settings file to %s: %w", path, err)
	}
	return nil
}

// WithVault returns a copy of s with VaultPath set to path, pushed to the
// front of VaultHistory (deduplicated — an already-present entry moves to
// the front rather than duplicating — and capped at maxVaultHistory).
func (s Settings) WithVault(path string) Settings {
	history := make([]string, 0, len(s.VaultHistory)+1)
	history = append(history, path)
	for _, p := range s.VaultHistory {
		if p != path {
			history = append(history, p)
		}
	}
	history = slices.Clone(history)
	if len(history) > maxVaultHistory {
		history = history[:maxVaultHistory]
	}

	s.VaultPath = path
	s.VaultHistory = history
	return s
}

// WithTheme returns a copy of s with Theme set to theme.
func (s Settings) WithTheme(theme string) Settings {
	s.Theme = theme
	return s
}
