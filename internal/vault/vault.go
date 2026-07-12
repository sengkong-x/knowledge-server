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
