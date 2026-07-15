// Package vaultfixture provides a shared test helper for writing Vault
// fixture files, used by internal/index, internal/search, and
// internal/server's tests.
package vaultfixture

import (
	"os"
	"path/filepath"
	"testing"
)

// WriteNote writes content to root/rel, creating parent directories as
// needed.
func WriteNote(tb testing.TB, root, rel, content string) {
	tb.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		tb.Fatalf("creating dir for %s: %v", rel, err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		tb.Fatalf("writing %s: %v", rel, err)
	}
}
