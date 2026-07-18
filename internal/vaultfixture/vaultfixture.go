// Package vaultfixture provides shared test helpers for constructing Vault
// fixtures, used by internal/index, internal/search, internal/server,
// internal/state, and internal/engines's tests.
package vaultfixture

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sengkong/knowledge-server/internal/vault"
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

// HidingProvider wraps a VaultProvider and excludes one note ID from
// ListNotes while still serving its content through ReadNote — a note
// visible to a direct read but not to a directory scan. Index and
// SearchStore's Upsert both resolve a new note's path via a
// ListNotes-based lookup (vault.ResolvePath/RefPath), so hiding an ID makes
// their Upsert fail while Graph's Upsert, which never consults ListNotes,
// still succeeds — useful for constructing a genuine partial-failure case
// across the engine triad.
type HidingProvider struct {
	vault.VaultProvider
	Hide string
}

func (p *HidingProvider) ListNotes() ([]vault.NoteRef, error) {
	refs, err := p.VaultProvider.ListNotes()
	if err != nil {
		return nil, err
	}
	visible := make([]vault.NoteRef, 0, len(refs))
	for _, ref := range refs {
		if ref.ID != p.Hide {
			visible = append(visible, ref)
		}
	}
	return visible, nil
}
