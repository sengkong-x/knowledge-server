package engines_test

import (
	"os"
	"testing"

	"github.com/sengkong/knowledge-server/internal/engines"
	"github.com/sengkong/knowledge-server/internal/notes"
	"github.com/sengkong/knowledge-server/internal/vault"
	"github.com/sengkong/knowledge-server/internal/vaultfixture"
)

func buildEngines(t *testing.T, root string) (*engines.Engines, vault.VaultProvider, notes.NoteStore) {
	t.Helper()
	provider := vault.NewLocalVaultProvider(root)
	store := notes.NewVaultNoteStore(provider)

	e, _, err := engines.Build(provider, store)
	if err != nil {
		t.Fatalf("engines.Build: %v", err)
	}
	return e, provider, store
}

func TestUpsertAll_UpdatesIndexSearchAndGraph(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "process.md", "---\ntitle: Process\ncreated: 2026-07-12\n---\nBody.\n")

	e, _, _ := buildEngines(t, root)

	vaultfixture.WriteNote(t, root, "memory.md", "---\ntitle: Memory\ncreated: 2026-07-13\nrelated: [process]\n---\nBody.\n")
	if err := e.UpsertAll("memory"); err != nil {
		t.Fatalf("UpsertAll returned error: %v", err)
	}

	if entry, ok := e.Index().ByID("memory"); !ok || entry.Title != "Memory" {
		t.Errorf("Index().ByID(memory) = %+v, %v, want Title=Memory, ok=true", entry, ok)
	}
	if got := e.Search().Query("Memory"); len(got) == 0 {
		t.Errorf("Search().Query(Memory) returned no results")
	}
	neighbors, err := e.Graph().Neighbors("memory")
	if err != nil {
		t.Fatalf("Graph().Neighbors(memory) returned error: %v", err)
	}
	if len(neighbors) != 1 || neighbors[0] != "process" {
		t.Errorf("Graph().Neighbors(memory) = %v, want [process]", neighbors)
	}
}

func TestUpsertAll_AggregatesErrorAcrossAllThreeEngines(t *testing.T) {
	root := t.TempDir()
	e, _, _ := buildEngines(t, root)

	// "missing" was never written to the Vault, so every engine's Upsert
	// fails the same way (store.Load can't find it) — this proves UpsertAll
	// attempts and reports all three rather than stopping at the first.
	err := e.UpsertAll("missing")
	if err == nil {
		t.Fatal("UpsertAll(missing) returned nil, want an aggregated error")
	}
	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		t.Fatalf("error is not a joined error: %v", err)
	}
	if got := len(joined.Unwrap()); got != 3 {
		t.Errorf("joined error count = %d, want 3 (one per engine)", got)
	}
}

func TestUpsertAll_PartialFailure_GraphSucceedsWhileIndexAndSearchFail(t *testing.T) {
	root := t.TempDir()
	provider := &vaultfixture.HidingProvider{VaultProvider: vault.NewLocalVaultProvider(root), Hide: "memory"}
	store := notes.NewVaultNoteStore(provider)

	e, _, err := engines.Build(provider, store)
	if err != nil {
		t.Fatalf("engines.Build: %v", err)
	}

	vaultfixture.WriteNote(t, root, "memory.md", "---\ntitle: Memory\ncreated: 2026-07-13\n---\nBody.\n")

	err = e.UpsertAll("memory")
	if err == nil {
		t.Fatal("UpsertAll(memory) returned nil, want an error from Index and SearchStore")
	}
	if _, ok := e.Index().ByID("memory"); ok {
		t.Error("Index().ByID(memory) found an entry despite Index's Upsert failing")
	}
	if got := e.Search().Query("Memory"); len(got) != 0 {
		t.Errorf("Search().Query(Memory) = %v, want no results (SearchStore's Upsert should have failed)", got)
	}
	if _, err := e.Graph().Neighbors("memory"); err != nil {
		t.Errorf("Graph().Neighbors(memory) returned error %v, want Graph's Upsert to have succeeded", err)
	}
}

func TestRemoveAll_DropsFromIndexSearchAndGraph(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "process.md", "---\ntitle: Process\ncreated: 2026-07-12\n---\nBody.\n")

	e, _, _ := buildEngines(t, root)

	e.RemoveAll("process")

	if _, ok := e.Index().ByID("process"); ok {
		t.Error("Index().ByID(process) still found after RemoveAll")
	}
	if got := e.Search().Query("Process"); len(got) != 0 {
		t.Errorf("Search().Query(Process) = %v, want no results after RemoveAll", got)
	}
	if _, err := e.Graph().Neighbors("process"); err == nil {
		t.Error("Graph().Neighbors(process) succeeded after RemoveAll, want error")
	}
}

func TestSaveAll_PartialFailureStillPersistsOtherEngines(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "process.md", "---\ntitle: Process\ncreated: 2026-07-12\n---\nBody.\n")

	e, _, _ := buildEngines(t, root)

	cacheDir := t.TempDir()
	paths := engines.Paths{
		// A directory, not a file: os.Create fails for Index specifically,
		// while Search and Graph get valid paths.
		Index:  cacheDir,
		Search: cacheDir + "/search.gob",
		Graph:  cacheDir + "/graph.gob",
	}

	if err := e.SaveAll(paths); err == nil {
		t.Fatal("SaveAll returned nil, want an error from the Index save")
	}

	if _, err := vault.NewLocalVaultProvider(root).ListNotes(); err != nil {
		t.Fatalf("vault should still be intact: %v", err)
	}
	for _, p := range []string{paths.Search, paths.Graph} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected %s to be persisted despite Index save failing: %v", p, err)
		}
	}
}
