package vault

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type VaultProvider interface {
	ListNotes() ([]NoteRef, error)
	ReadNote(id string) ([]byte, error)
	ReadAsset(path string) ([]byte, error)
}

type NoteRef struct {
	ID   string
	Path string
}

type localVaultProvider struct {
	root string
}

func NewLocalVaultProvider(root string) VaultProvider {
	return &localVaultProvider{root: root}
}

func (p *localVaultProvider) ListNotes() ([]NoteRef, error) {
	var notes []NoteRef

	err := filepath.WalkDir(p.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}

		rel, err := filepath.Rel(p.root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		notes = append(notes, NoteRef{
			ID:   strings.TrimSuffix(rel, ".md"),
			Path: rel,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	return notes, nil
}

func (p *localVaultProvider) ReadNote(id string) ([]byte, error) {
	path := filepath.Join(p.root, filepath.FromSlash(id)+".md")
	return os.ReadFile(path)
}

// ReadAsset serves arbitrary Vault-relative files (e.g. images referenced
// from a note's body), so unlike ReadNote (whose id always comes from a
// ListNotes-derived NoteRef) path here may be attacker-controlled directly
// from an HTTP route — it's sanitized against escaping root before use.
func (p *localVaultProvider) ReadAsset(path string) ([]byte, error) {
	full, err := safeJoin(p.root, path)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(full)
}

// safeJoin joins root and rel, rejecting any rel that would resolve outside
// root (e.g. via ".." segments).
func safeJoin(root, rel string) (string, error) {
	full := filepath.Join(root, filepath.FromSlash(rel))
	if full != root && !strings.HasPrefix(full, root+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes vault root", rel)
	}
	return full, nil
}

// ResolvePath returns existingPath if non-empty, otherwise resolves id's
// path via RefPath. Used by disposable-index Upsert methods (Index,
// SearchStore) to avoid a full vault scan when the entry is already known.
func ResolvePath(existingPath string, provider VaultProvider, id string) (string, error) {
	if existingPath != "" {
		return existingPath, nil
	}
	return RefPath(provider, id)
}

// RefPath resolves id's path by listing the Vault, since VaultProvider has
// no single-ref lookup.
func RefPath(provider VaultProvider, id string) (string, error) {
	refs, err := provider.ListNotes()
	if err != nil {
		return "", err
	}
	for _, ref := range refs {
		if ref.ID == id {
			return ref.Path, nil
		}
	}
	return "", fmt.Errorf("vault: no path found for note %q", id)
}

func ValidateRoot(root string) error {
	info, err := os.Stat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("vault path %s is not a directory", root)
	}
	return nil
}
