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

func (p *localVaultProvider) ReadAsset(path string) ([]byte, error) {
	return os.ReadFile(filepath.Join(p.root, filepath.FromSlash(path)))
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
