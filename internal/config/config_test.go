package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_ParsesValidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(path, []byte(`
vault:
  path: /srv/knowledge
server:
  port: 8080
theme:
  default: dark
`), 0o644)
	if err != nil {
		t.Fatalf("writing fixture config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Vault.Path != "/srv/knowledge" {
		t.Errorf("Vault.Path = %q, want %q", cfg.Vault.Path, "/srv/knowledge")
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("Server.Port = %d, want %d", cfg.Server.Port, 8080)
	}
	if cfg.Theme.Default != "dark" {
		t.Errorf("Theme.Default = %q, want %q", cfg.Theme.Default, "dark")
	}
}

func TestLoad_ExpandsTildeInVaultPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(path, []byte(`
vault:
  path: ~/knowledge
server:
  port: 8080
`), 0o644)
	if err != nil {
		t.Fatalf("writing fixture config: %v", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	want := filepath.Join(home, "knowledge")
	if cfg.Vault.Path != want {
		t.Errorf("Vault.Path = %q, want %q", cfg.Vault.Path, want)
	}
}

func TestLoad_MissingFileReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist.yaml")

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load returned nil error for missing config file, want error")
	}
}
