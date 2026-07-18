package index

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/sengkong/knowledge-server/internal/notes"
	"github.com/sengkong/knowledge-server/internal/vault"
	"github.com/sengkong/knowledge-server/internal/vaultfixture"
)

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
	vaultfixture.WriteNote(t, root, "linux/process.md", validNote)

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
	vaultfixture.WriteNote(t, root, "linux/process.md", validNote)
	vaultfixture.WriteNote(t, root, "linux/broken.md", "# No frontmatter here.\n")

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

func TestAll_ReturnsEveryIndexedEntry(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "linux/process.md", validNote)
	vaultfixture.WriteNote(t, root, "cooking/pasta.md", `---
title: Pasta
tags: [food]
created: 2026-07-12
---
Boil water.
`)

	idx, _, _, _ := buildIndex(t, root)

	got := idx.All()
	if len(got) != 2 {
		t.Fatalf("All() returned %d entries, want 2: %+v", len(got), got)
	}

	ids := map[string]bool{}
	for _, entry := range got {
		ids[entry.ID] = true
	}
	if !ids["linux/process"] || !ids["cooking/pasta"] {
		t.Errorf("All() = %+v, want entries for linux/process and cooking/pasta", got)
	}
}

func TestByTag_ReturnsEntriesWithMatchingTag(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "linux/process.md", validNote)
	vaultfixture.WriteNote(t, root, "cooking/pasta.md", `---
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
	vaultfixture.WriteNote(t, root, "linux/process.md", validNote)

	idx, _, _, _ := buildIndex(t, root)

	vaultfixture.WriteNote(t, root, "linux/memory.md", `---
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
	vaultfixture.WriteNote(t, root, "linux/process.md", validNote)

	idx, _, _, _ := buildIndex(t, root)

	vaultfixture.WriteNote(t, root, "linux/process.md", `---
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
	vaultfixture.WriteNote(t, root, "linux/process.md", validNote)

	idx, _, _, _ := buildIndex(t, root)

	idx.Remove("linux/process")

	if _, ok := idx.ByID("linux/process"); ok {
		t.Error("ByID(linux/process) found after Remove, want it gone")
	}
}

func TestSaveLoad_RoundTripsEntries(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "linux/process.md", validNote)

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

func TestLoadOrBuild_LoadsFromCacheWhenPresent(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "linux/process.md", validNote)

	idx, _, provider, store := buildIndex(t, root)
	cachePath := filepath.Join(t.TempDir(), "index.gob")
	if err := idx.Save(cachePath); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	// Change the vault after saving; if LoadOrBuild used the cache instead
	// of rebuilding, it won't see this new note.
	vaultfixture.WriteNote(t, root, "linux/memory.md", `---
title: Memory
created: 2026-07-13
---
Memory body.
`)

	loaded, _, err := LoadOrBuild(cachePath, provider, store)
	if err != nil {
		t.Fatalf("LoadOrBuild returned error: %v", err)
	}

	if _, ok := loaded.ByID("linux/memory"); ok {
		t.Fatal("ByID(linux/memory) found, want LoadOrBuild to have used the stale cache, not rebuilt")
	}
	if _, ok := loaded.ByID("linux/process"); !ok {
		t.Fatal("ByID(linux/process) not found, want the cached entry present")
	}
}

func TestLoadOrBuild_FallsBackToBuildWhenCacheMissing(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "linux/process.md", validNote)

	provider := vault.NewLocalVaultProvider(root)
	store := notes.NewVaultNoteStore(provider)
	cachePath := filepath.Join(t.TempDir(), "does-not-exist.gob")

	idx, _, err := LoadOrBuild(cachePath, provider, store)
	if err != nil {
		t.Fatalf("LoadOrBuild returned error: %v", err)
	}

	if _, ok := idx.ByID("linux/process"); !ok {
		t.Fatal("ByID(linux/process) not found, want a fresh Build from the vault")
	}
}

func TestDeleteAndRebuild_ProducesSemanticallyIdenticalIndex(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "linux/process.md", validNote)
	vaultfixture.WriteNote(t, root, "cooking/pasta.md", `---
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
	vaultfixture.WriteNote(t, root, "linux/process.md", validNote)

	idx, _, _, _ := buildIndex(t, root)

	if err := idx.Upsert("linux/does-not-exist"); err == nil {
		t.Fatal("Upsert returned nil error for a note absent from the vault, want error")
	}
}
