package vault

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
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

func TestListNotes_DiscoversMarkdownFilesRecursively(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "linux/process.md", "# Process")
	writeFile(t, root, "database/postgres/wal.md", "# WAL")

	provider := NewLocalVaultProvider(root)

	notes, err := provider.ListNotes()
	if err != nil {
		t.Fatalf("ListNotes returned error: %v", err)
	}

	got := make([]NoteRef, len(notes))
	copy(got, notes)
	sort.Slice(got, func(i, j int) bool { return got[i].ID < got[j].ID })

	want := []NoteRef{
		{ID: "database/postgres/wal", Path: "database/postgres/wal.md"},
		{ID: "linux/process", Path: "linux/process.md"},
	}

	if len(got) != len(want) {
		t.Fatalf("ListNotes returned %d notes, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("note %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestReadNote_ReturnsFileContentsByID(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "linux/process.md", "# Process\n\nBody text.")

	provider := NewLocalVaultProvider(root)

	data, err := provider.ReadNote("linux/process")
	if err != nil {
		t.Fatalf("ReadNote returned error: %v", err)
	}

	want := "# Process\n\nBody text."
	if string(data) != want {
		t.Errorf("ReadNote content = %q, want %q", string(data), want)
	}
}

func TestReadAsset_ReturnsFileContentsByRelativePath(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "assets/diagram.png", "fake-png-bytes")

	provider := NewLocalVaultProvider(root)

	data, err := provider.ReadAsset("assets/diagram.png")
	if err != nil {
		t.Fatalf("ReadAsset returned error: %v", err)
	}

	want := "fake-png-bytes"
	if string(data) != want {
		t.Errorf("ReadAsset content = %q, want %q", string(data), want)
	}
}

func TestReadAsset_RejectsPathEscapingRoot(t *testing.T) {
	outside := t.TempDir()
	writeFile(t, outside, "secret.txt", "outside the vault")

	root := filepath.Join(outside, "vault")
	writeFile(t, root, "assets/diagram.png", "fake-png-bytes")

	provider := NewLocalVaultProvider(root)

	_, err := provider.ReadAsset("../secret.txt")
	if err == nil {
		t.Fatal("ReadAsset(\"../secret.txt\") returned no error, want it rejected for escaping the vault root")
	}
}

func TestValidateRoot_ErrorsWhenPathDoesNotExist(t *testing.T) {
	root := filepath.Join(t.TempDir(), "does-not-exist")

	if err := ValidateRoot(root); err == nil {
		t.Fatal("ValidateRoot returned nil error for a nonexistent path, want error")
	}
}

func TestValidateRoot_ErrorsWhenPathIsNotADirectory(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("writing fixture file: %v", err)
	}

	if err := ValidateRoot(file); err == nil {
		t.Fatal("ValidateRoot returned nil error for a file path, want error")
	}
}

func TestValidateRoot_NoErrorWhenPathIsADirectory(t *testing.T) {
	root := t.TempDir()

	if err := ValidateRoot(root); err != nil {
		t.Errorf("ValidateRoot returned error for a valid directory: %v", err)
	}
}
