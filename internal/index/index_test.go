package index

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/sengkong/knowledge-server/internal/notes"
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

// buildIndex builds an Index over root, returning the provider and store
// used so callers can exercise Upsert without rebuilding them.
func buildIndex(t *testing.T, root string) (*Index, BuildReport, vault.VaultProvider, notes.NoteStore) {
	t.Helper()
	provider := vault.NewLocalVaultProvider(root)
	store := notes.NewVaultNoteStore(provider)

	idx, report, err := Build(provider, store)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	return idx, report, provider, store
}

const validNote = `---
title: Process
tags: [linux, kernel]
created: 2026-07-12
---
Body text.
`

func TestBuild_IndexesNotesFromVault(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "linux/process.md", validNote)

	idx, _, _, _ := buildIndex(t, root)

	entry, ok := idx.ByID("linux/process")
	if !ok {
		t.Fatalf("ByID(%q) not found", "linux/process")
	}
	if entry.Title != "Process" {
		t.Errorf("Title = %q, want %q", entry.Title, "Process")
	}
	if entry.Path != "linux/process.md" {
		t.Errorf("Path = %q, want %q", entry.Path, "linux/process.md")
	}
}

func TestBuild_SkipsAndRecordsMalformedNotes(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "linux/process.md", validNote)
	writeFile(t, root, "linux/broken.md", "# No frontmatter here.\n")

	idx, report, _, _ := buildIndex(t, root)

	if _, ok := idx.ByID("linux/process"); !ok {
		t.Error("ByID(linux/process) not found, want the good note still indexed")
	}
	if _, ok := idx.ByID("linux/broken"); ok {
		t.Error("ByID(linux/broken) found, want the malformed note skipped")
	}
	if _, ok := report.Failed["linux/broken"]; !ok {
		t.Errorf("report.Failed = %v, want an entry for linux/broken", report.Failed)
	}
}

func TestByTag_ReturnsEntriesWithMatchingTag(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "linux/process.md", validNote)
	writeFile(t, root, "cooking/pasta.md", `---
title: Pasta
tags: [food]
created: 2026-07-12
---
Boil water.
`)

	idx, _, _, _ := buildIndex(t, root)

	got := idx.ByTag("kernel")
	if len(got) != 1 || got[0].ID != "linux/process" {
		t.Fatalf("ByTag(kernel) = %+v, want only linux/process", got)
	}

	if got := idx.ByTag("nonexistent"); len(got) != 0 {
		t.Errorf("ByTag(nonexistent) = %+v, want empty", got)
	}
}

func TestUpsert_AddsNewNoteToIndex(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "linux/process.md", validNote)

	idx, _, _, _ := buildIndex(t, root)

	writeFile(t, root, "linux/memory.md", `---
title: Memory
created: 2026-07-13
---
Memory body.
`)

	if err := idx.Upsert("linux/memory"); err != nil {
		t.Fatalf("Upsert returned error: %v", err)
	}

	entry, ok := idx.ByID("linux/memory")
	if !ok {
		t.Fatal("ByID(linux/memory) not found after Upsert")
	}
	if entry.Title != "Memory" {
		t.Errorf("Title = %q, want %q", entry.Title, "Memory")
	}
}

func TestUpsert_ReplacesExistingEntry(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "linux/process.md", validNote)

	idx, _, _, _ := buildIndex(t, root)

	writeFile(t, root, "linux/process.md", `---
title: Process (renamed)
created: 2026-07-12
---
Body text.
`)

	if err := idx.Upsert("linux/process"); err != nil {
		t.Fatalf("Upsert returned error: %v", err)
	}

	entry, ok := idx.ByID("linux/process")
	if !ok {
		t.Fatal("ByID(linux/process) not found after Upsert")
	}
	if entry.Title != "Process (renamed)" {
		t.Errorf("Title = %q, want %q", entry.Title, "Process (renamed)")
	}
}

func TestRemove_DropsEntryFromIndex(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "linux/process.md", validNote)

	idx, _, _, _ := buildIndex(t, root)

	idx.Remove("linux/process")

	if _, ok := idx.ByID("linux/process"); ok {
		t.Error("ByID(linux/process) found after Remove, want it gone")
	}
}

func TestSaveLoad_RoundTripsEntries(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "linux/process.md", validNote)

	idx, _, provider, store := buildIndex(t, root)

	cachePath := filepath.Join(t.TempDir(), "index.gob")
	if err := idx.Save(cachePath); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	loaded, err := Load(cachePath, provider, store)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	entry, ok := loaded.ByID("linux/process")
	if !ok {
		t.Fatal("ByID(linux/process) not found after Load")
	}
	if entry.Title != "Process" {
		t.Errorf("Title = %q, want %q", entry.Title, "Process")
	}
}

func TestDeleteAndRebuild_ProducesSemanticallyIdenticalIndex(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "linux/process.md", validNote)
	writeFile(t, root, "cooking/pasta.md", `---
title: Pasta
tags: [food]
created: 2026-07-12
---
Boil water.
`)

	cachePath := filepath.Join(t.TempDir(), "index.gob")

	first, _, _, _ := buildIndex(t, root)
	if err := first.Save(cachePath); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	if err := os.Remove(cachePath); err != nil {
		t.Fatalf("removing cache file: %v", err)
	}

	second, _, provider, store := buildIndex(t, root)
	if err := second.Save(cachePath); err != nil {
		t.Fatalf("rebuild: Save returned error: %v", err)
	}

	rebuilt, err := Load(cachePath, provider, store)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if !reflect.DeepEqual(first.entries, rebuilt.entries) {
		t.Errorf("rebuilt entries = %+v, want %+v", rebuilt.entries, first.entries)
	}
}

func TestUpsert_ReturnsErrorForNoteMissingFromVault(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "linux/process.md", validNote)

	idx, _, _, _ := buildIndex(t, root)

	if err := idx.Upsert("linux/does-not-exist"); err == nil {
		t.Fatal("Upsert returned nil error for a note absent from the vault, want error")
	}
}
