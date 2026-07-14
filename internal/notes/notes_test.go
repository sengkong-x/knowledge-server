package notes

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/sengkong/knowledge-server/internal/vault"
)

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("creating dir for %s: %v", rel, err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", rel, err)
	}
}

const validNote = `---
title: Process
created: 2026-07-12
---
Body text.
`

func TestList_ReturnsParsedNotesFromVault(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "linux/process.md", validNote)
	writeFile(t, root, "linux/memory.md", `---
title: Memory
created: 2026-07-13
---
Memory body.
`)

	store := NewVaultNoteStore(vault.NewLocalVaultProvider(root))

	got, err := store.List()
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}

	sort.Slice(got, func(i, j int) bool { return got[i].ID < got[j].ID })

	if len(got) != 2 {
		t.Fatalf("List returned %d notes, want 2: %+v", len(got), got)
	}
	if got[0].ID != "linux/memory" || got[0].Title != "Memory" {
		t.Errorf("note 0 = %+v, want ID=linux/memory Title=Memory", got[0])
	}
	if got[1].ID != "linux/process" || got[1].Title != "Process" {
		t.Errorf("note 1 = %+v, want ID=linux/process Title=Process", got[1])
	}
}

func TestList_FailsFastOnFirstMalformedNote(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "linux/process.md", validNote)
	writeFile(t, root, "linux/broken.md", "# No frontmatter here.\n")

	store := NewVaultNoteStore(vault.NewLocalVaultProvider(root))

	_, err := store.List()
	if err == nil {
		t.Fatal("List returned nil error with a malformed note present, want error")
	}
}

func TestLoad_ReturnsParsedNote(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "linux/process.md", validNote)

	store := NewVaultNoteStore(vault.NewLocalVaultProvider(root))

	note, err := store.Load("linux/process")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if note.Title != "Process" {
		t.Errorf("Title = %q, want %q", note.Title, "Process")
	}
}

func TestLoad_ReturnsErrNotFoundForMissingID(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "linux/process.md", validNote)

	store := NewVaultNoteStore(vault.NewLocalVaultProvider(root))

	_, err := store.Load("linux/does-not-exist")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Load error = %v, want errors.Is(err, ErrNotFound)", err)
	}
}

func TestLoad_ReturnsParseErrorForMalformedNote(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "linux/broken.md", "# No frontmatter here.\n")

	store := NewVaultNoteStore(vault.NewLocalVaultProvider(root))

	_, err := store.Load("linux/broken")
	if err == nil {
		t.Fatal("Load returned nil error for malformed note, want error")
	}
	if errors.Is(err, ErrNotFound) {
		t.Error("Load returned ErrNotFound for a malformed (but existing) note, want a parse error")
	}
}
