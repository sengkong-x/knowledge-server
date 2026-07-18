package state

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/sengkong/knowledge-server/internal/graph"
	"github.com/sengkong/knowledge-server/internal/index"
	"github.com/sengkong/knowledge-server/internal/notes"
	"github.com/sengkong/knowledge-server/internal/search"
	"github.com/sengkong/knowledge-server/internal/vault"
	"github.com/sengkong/knowledge-server/internal/vaultfixture"
)

func buildState(t *testing.T, root string) (*State, vault.VaultProvider, notes.NoteStore) {
	t.Helper()
	provider := vault.NewLocalVaultProvider(root)
	store := notes.NewVaultNoteStore(provider)

	idx, _, err := index.Build(provider, store)
	if err != nil {
		t.Fatalf("index.Build: %v", err)
	}
	ss, _, err := search.Build(provider, store)
	if err != nil {
		t.Fatalf("search.Build: %v", err)
	}
	g, _, err := graph.Build(provider, store)
	if err != nil {
		t.Fatalf("graph.Build: %v", err)
	}

	return New(idx, ss, g), provider, store
}

func TestUpsert_UpdatesIndex(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "process.md", "---\ntitle: Process\ncreated: 2026-07-12\n---\nBody.\n")

	s, _, _ := buildState(t, root)

	vaultfixture.WriteNote(t, root, "memory.md", "---\ntitle: Memory\ncreated: 2026-07-13\n---\nBody.\n")
	if err := s.Upsert("memory"); err != nil {
		t.Fatalf("Upsert returned error: %v", err)
	}

	entry, ok := s.ByID("memory")
	if !ok {
		t.Fatalf("ByID(memory) not found after Upsert")
	}
	if entry.Title != "Memory" {
		t.Errorf("Title = %q, want %q", entry.Title, "Memory")
	}
}

func TestRemove_DropsFromIndex(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "process.md", "---\ntitle: Process\ncreated: 2026-07-12\n---\nBody.\n")

	s, _, _ := buildState(t, root)

	s.Remove("process")

	if _, ok := s.ByID("process"); ok {
		t.Fatalf("ByID(process) found after Remove, want gone")
	}
}

func TestQuery_MatchesSearchStore(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "process.md", "---\ntitle: Process\ncreated: 2026-07-12\n---\nBody about the Linux kernel.\n")

	s, _, _ := buildState(t, root)

	results := s.Query("kernel")
	if len(results) != 1 || results[0].ID != "process" {
		t.Fatalf("Query(kernel) = %+v, want one result for process", results)
	}
}

func TestNeighbors_MatchesGraph(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "a.md", "---\ntitle: A\nrelated: [b]\ncreated: 2026-07-12\n---\nBody.\n")
	vaultfixture.WriteNote(t, root, "b.md", "---\ntitle: B\ncreated: 2026-07-12\n---\nBody.\n")

	s, _, _ := buildState(t, root)

	got, err := s.Neighbors("a")
	if err != nil {
		t.Fatalf("Neighbors returned error: %v", err)
	}
	if len(got) != 1 || got[0] != "b" {
		t.Fatalf("Neighbors(a) = %v, want [b]", got)
	}
}

func TestByTag_MatchesIndex(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "process.md", "---\ntitle: Process\ntags: [kernel]\ncreated: 2026-07-12\n---\nBody.\n")
	vaultfixture.WriteNote(t, root, "pasta.md", "---\ntitle: Pasta\ntags: [food]\ncreated: 2026-07-12\n---\nBody.\n")

	s, _, _ := buildState(t, root)

	got := s.ByTag("kernel")
	if len(got) != 1 || got[0].ID != "process" {
		t.Fatalf("ByTag(kernel) = %+v, want only process", got)
	}
}

func TestIndexAll_ReturnsEveryEntry(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "process.md", "---\ntitle: Process\ncreated: 2026-07-12\n---\nBody.\n")
	vaultfixture.WriteNote(t, root, "pasta.md", "---\ntitle: Pasta\ncreated: 2026-07-12\n---\nBody.\n")

	s, _, _ := buildState(t, root)

	got := s.IndexAll()
	if len(got) != 2 {
		t.Fatalf("IndexAll() returned %d entries, want 2: %+v", len(got), got)
	}
}

func TestShortestPath_MatchesGraph(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "a.md", "---\ntitle: A\nrelated: [b]\ncreated: 2026-07-12\n---\nBody.\n")
	vaultfixture.WriteNote(t, root, "b.md", "---\ntitle: B\ncreated: 2026-07-12\n---\nBody.\n")

	s, _, _ := buildState(t, root)

	path, found, err := s.ShortestPath("a", "b")
	if err != nil {
		t.Fatalf("ShortestPath returned error: %v", err)
	}
	if !found || len(path) != 2 {
		t.Fatalf("ShortestPath(a, b) = %v, %v, want [a b], true", path, found)
	}
}

func TestOrphans_MatchesGraph(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "a.md", "---\ntitle: A\nrelated: [b]\ncreated: 2026-07-12\n---\nBody.\n")
	vaultfixture.WriteNote(t, root, "b.md", "---\ntitle: B\ncreated: 2026-07-12\n---\nBody.\n")
	vaultfixture.WriteNote(t, root, "c.md", "---\ntitle: C\ncreated: 2026-07-12\n---\nBody.\n")

	s, _, _ := buildState(t, root)

	got := s.Orphans()
	if len(got) != 1 || got[0] != "c" {
		t.Fatalf("Orphans() = %v, want [c]", got)
	}
}

func TestGraphAll_ReturnsEveryNode(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "a.md", "---\ntitle: A\nrelated: [b]\ncreated: 2026-07-12\n---\nBody.\n")
	vaultfixture.WriteNote(t, root, "b.md", "---\ntitle: B\ncreated: 2026-07-12\n---\nBody.\n")

	s, _, _ := buildState(t, root)

	got := s.GraphAll()
	if len(got) != 2 {
		t.Fatalf("GraphAll() returned %d entries, want 2: %+v", len(got), got)
	}
}

func TestSubscribe_NotifiedOnUpsert(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "process.md", "---\ntitle: Process\ncreated: 2026-07-12\n---\nBody.\n")

	s, _, _ := buildState(t, root)

	ch, unsubscribe := s.Subscribe()
	defer unsubscribe()

	if err := s.Upsert("process"); err != nil {
		t.Fatalf("Upsert returned error: %v", err)
	}

	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for change notification after Upsert")
	}
}

func TestSubscribe_NotifiedOnRemove(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "process.md", "---\ntitle: Process\ncreated: 2026-07-12\n---\nBody.\n")

	s, _, _ := buildState(t, root)

	ch, unsubscribe := s.Subscribe()
	defer unsubscribe()

	s.Remove("process")

	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for change notification after Remove")
	}
}

func TestSave_PersistsAllThreeUnderLock(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "process.md", "---\ntitle: Process\ncreated: 2026-07-12\n---\nBody.\n")

	s, provider, store := buildState(t, root)

	cacheDir := t.TempDir()
	indexPath := cacheDir + "/index.gob"
	searchPath := cacheDir + "/search.gob"
	graphPath := cacheDir + "/graph.gob"

	if err := s.Save(indexPath, searchPath, graphPath); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	loadedIdx, err := index.Load(indexPath, provider, store)
	if err != nil {
		t.Fatalf("index.Load returned error: %v", err)
	}
	if _, ok := loadedIdx.ByID("process"); !ok {
		t.Fatal("ByID(process) not found in saved index cache")
	}
}

func TestConcurrentUpsertAndReads_NoDataRace(t *testing.T) {
	root := t.TempDir()
	for i := range 20 {
		vaultfixture.WriteNote(t, root, fmt.Sprintf("note-%d.md", i), "---\ntitle: Note\ncreated: 2026-07-12\n---\nBody.\n")
	}

	s, _, _ := buildState(t, root)

	var wg sync.WaitGroup
	for i := range 20 {
		id := fmt.Sprintf("note-%d", i)

		wg.Add(2)
		go func() {
			defer wg.Done()
			if err := s.Upsert(id); err != nil {
				t.Errorf("Upsert(%s): %v", id, err)
			}
		}()
		go func() {
			defer wg.Done()
			s.ByID(id)
			s.Query("Note")
			s.Neighbors(id)
		}()
	}
	wg.Wait()
}
