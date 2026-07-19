package graph

import (
	"encoding/gob"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sengkong/knowledge-server/internal/notes"
	"github.com/sengkong/knowledge-server/internal/vault"
	"github.com/sengkong/knowledge-server/internal/vaultfixture"
)

// buildGraph builds a Graph over root, returning the provider and store used
// so callers can exercise Upsert without rebuilding them.
func buildGraph(t *testing.T, root string) (*Graph, BuildReport, vault.VaultProvider, notes.NoteStore) {
	t.Helper()
	provider := vault.NewLocalVaultProvider(root)
	store := notes.NewVaultNoteStore(provider)

	g, report, err := Build(provider, store)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	return g, report, provider, store
}

func TestBuild_LinksNotesSymmetricallyViaRelated(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "linux/process.md", `---
title: Process
related: [linux/memory]
created: 2026-07-12
---
Body text.
`)
	vaultfixture.WriteNote(t, root, "linux/memory.md", `---
title: Memory
created: 2026-07-13
---
Memory body.
`)

	g, _, _, _ := buildGraph(t, root)

	got, err := g.Neighbors("linux/memory")
	if err != nil {
		t.Fatalf("Neighbors returned error: %v", err)
	}
	if len(got) != 1 || got[0] != "linux/process" {
		t.Fatalf("Neighbors(linux/memory) = %v, want [linux/process]", got)
	}
}

func TestBuild_DropsDanglingRelatedReferenceAndReportsIt(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "linux/process.md", `---
title: Process
related: [linux/does-not-exist]
created: 2026-07-12
---
Body text.
`)

	g, report, _, _ := buildGraph(t, root)

	got, err := g.Neighbors("linux/process")
	if err != nil {
		t.Fatalf("Neighbors returned error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("Neighbors(linux/process) = %v, want empty (dangling ref dropped)", got)
	}
	if _, ok := report.Failed["linux/process"]; !ok {
		t.Errorf("report.Failed = %v, want an entry for linux/process's dangling reference", report.Failed)
	}
}

func TestBuild_ReportsAllDanglingReferencesOnANote(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "linux/process.md", `---
title: Process
related: [linux/ghost-one, linux/ghost-two]
created: 2026-07-12
---
Body text.
`)

	_, report, _, _ := buildGraph(t, root)

	err, ok := report.Failed["linux/process"]
	if !ok {
		t.Fatalf("report.Failed = %v, want an entry for linux/process", report.Failed)
	}
	msg := err.Error()
	if !strings.Contains(msg, "linux/ghost-one") || !strings.Contains(msg, "linux/ghost-two") {
		t.Errorf("report.Failed[linux/process] = %q, want it to mention both dangling references", msg)
	}
}

func TestBuild_DoesNotLinkToANoteThatFailedToParse(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "linux/process.md", `---
title: Process
related: [linux/broken]
created: 2026-07-12
---
Body text.
`)
	vaultfixture.WriteNote(t, root, "linux/broken.md", "# No frontmatter here.\n")

	g, _, _, _ := buildGraph(t, root)

	got, err := g.Neighbors("linux/process")
	if err != nil {
		t.Fatalf("Neighbors returned error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("Neighbors(linux/process) = %v, want empty (related note failed to parse)", got)
	}
	if _, err := g.Neighbors("linux/broken"); err == nil {
		t.Fatal("Neighbors(linux/broken) returned nil error, want error (never a node since it failed to parse)")
	}
}

func TestBuild_NormalizesSelfReferencesAndDuplicates(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "linux/process.md", `---
title: Process
related: [linux/process, linux/memory, linux/memory]
created: 2026-07-12
---
Body text.
`)
	vaultfixture.WriteNote(t, root, "linux/memory.md", `---
title: Memory
created: 2026-07-13
---
Memory body.
`)

	g, _, _, _ := buildGraph(t, root)

	got, err := g.Neighbors("linux/process")
	if err != nil {
		t.Fatalf("Neighbors returned error: %v", err)
	}
	if len(got) != 1 || got[0] != "linux/memory" {
		t.Fatalf("Neighbors(linux/process) = %v, want [linux/memory] (self-ref and duplicate collapsed)", got)
	}
}

func TestNeighbors_ReturnsErrorForUnknownID(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "linux/process.md", `---
title: Process
created: 2026-07-12
---
Body text.
`)

	g, _, _, _ := buildGraph(t, root)

	if _, err := g.Neighbors("linux/does-not-exist"); err == nil {
		t.Fatal("Neighbors returned nil error for an unknown ID, want error")
	}
}

func TestOrphans_ReturnsNotesWithZeroEdges(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "linux/process.md", `---
title: Process
related: [linux/memory]
created: 2026-07-12
---
Body text.
`)
	vaultfixture.WriteNote(t, root, "linux/memory.md", `---
title: Memory
created: 2026-07-13
---
Memory body.
`)
	vaultfixture.WriteNote(t, root, "cooking/pasta.md", `---
title: Pasta
created: 2026-07-13
---
Boil water.
`)

	g, _, _, _ := buildGraph(t, root)

	got := g.Orphans()
	if len(got) != 1 || got[0] != "cooking/pasta" {
		t.Fatalf("Orphans() = %v, want [cooking/pasta]", got)
	}
}

func TestShortestPath_FindsMultiHopPath(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "a.md", `---
title: A
related: [b]
created: 2026-07-12
---
A body.
`)
	vaultfixture.WriteNote(t, root, "b.md", `---
title: B
related: [c]
created: 2026-07-12
---
B body.
`)
	vaultfixture.WriteNote(t, root, "c.md", `---
title: C
created: 2026-07-12
---
C body.
`)

	g, _, _, _ := buildGraph(t, root)

	path, found, err := g.ShortestPath("a", "c")
	if err != nil {
		t.Fatalf("ShortestPath returned error: %v", err)
	}
	if !found {
		t.Fatal("ShortestPath(a, c) found = false, want true")
	}
	want := []string{"a", "b", "c"}
	if len(path) != len(want) {
		t.Fatalf("ShortestPath(a, c) = %v, want %v", path, want)
	}
	for i := range want {
		if path[i] != want[i] {
			t.Fatalf("ShortestPath(a, c) = %v, want %v", path, want)
		}
	}
}

func TestShortestPath_ReturnsNotFoundForDisconnectedNotes(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "a.md", `---
title: A
created: 2026-07-12
---
A body.
`)
	vaultfixture.WriteNote(t, root, "b.md", `---
title: B
created: 2026-07-12
---
B body.
`)

	g, _, _, _ := buildGraph(t, root)

	path, found, err := g.ShortestPath("a", "b")
	if err != nil {
		t.Fatalf("ShortestPath returned error: %v", err)
	}
	if found {
		t.Fatalf("ShortestPath(a, b) found = true, want false (disconnected); path = %v", path)
	}
	if len(path) != 0 {
		t.Errorf("ShortestPath(a, b) path = %v, want empty", path)
	}
}

func TestShortestPath_ReturnsErrorForUnknownID(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "a.md", `---
title: A
created: 2026-07-12
---
A body.
`)

	g, _, _, _ := buildGraph(t, root)

	if _, _, err := g.ShortestPath("a", "does-not-exist"); err == nil {
		t.Fatal("ShortestPath returned nil error for an unknown ID, want error")
	}
}

func TestUpsert_ReplacesOutgoingEdgesAndUpdatesSymmetricSide(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "a.md", `---
title: A
related: [b]
created: 2026-07-12
---
A body.
`)
	vaultfixture.WriteNote(t, root, "b.md", `---
title: B
created: 2026-07-12
---
B body.
`)
	vaultfixture.WriteNote(t, root, "c.md", `---
title: C
created: 2026-07-12
---
C body.
`)

	g, _, _, _ := buildGraph(t, root)

	vaultfixture.WriteNote(t, root, "a.md", `---
title: A
related: [c]
created: 2026-07-12
---
A body, now related to C instead.
`)

	if err := g.Upsert("a"); err != nil {
		t.Fatalf("Upsert returned error: %v", err)
	}

	aNeighbors, err := g.Neighbors("a")
	if err != nil {
		t.Fatalf("Neighbors(a) returned error: %v", err)
	}
	if len(aNeighbors) != 1 || aNeighbors[0] != "c" {
		t.Fatalf("Neighbors(a) = %v, want [c]", aNeighbors)
	}

	bNeighbors, err := g.Neighbors("b")
	if err != nil {
		t.Fatalf("Neighbors(b) returned error: %v", err)
	}
	if len(bNeighbors) != 0 {
		t.Fatalf("Neighbors(b) = %v, want empty (edge to A removed)", bNeighbors)
	}

	cNeighbors, err := g.Neighbors("c")
	if err != nil {
		t.Fatalf("Neighbors(c) returned error: %v", err)
	}
	if len(cNeighbors) != 1 || cNeighbors[0] != "a" {
		t.Fatalf("Neighbors(c) = %v, want [a] (symmetric edge added)", cNeighbors)
	}
}

func TestRemove_DropsNodeAndItsEdges(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "a.md", `---
title: A
related: [b]
created: 2026-07-12
---
A body.
`)
	vaultfixture.WriteNote(t, root, "b.md", `---
title: B
created: 2026-07-12
---
B body.
`)

	g, _, _, _ := buildGraph(t, root)

	g.Remove("a")

	if _, err := g.Neighbors("a"); err == nil {
		t.Fatal("Neighbors(a) returned nil error after Remove, want error (unknown node)")
	}

	bNeighbors, err := g.Neighbors("b")
	if err != nil {
		t.Fatalf("Neighbors(b) returned error: %v", err)
	}
	if len(bNeighbors) != 0 {
		t.Fatalf("Neighbors(b) = %v, want empty after Remove(a)", bNeighbors)
	}
}

func TestSaveLoad_RoundTripsGraph(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "a.md", `---
title: A
related: [b]
created: 2026-07-12
---
A body.
`)
	vaultfixture.WriteNote(t, root, "b.md", `---
title: B
created: 2026-07-12
---
B body.
`)

	g, _, provider, store := buildGraph(t, root)

	cachePath := filepath.Join(t.TempDir(), "graph.gob")
	if err := g.Save(cachePath); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	loaded, err := Load(cachePath, provider, store)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	got, err := loaded.Neighbors("a")
	if err != nil {
		t.Fatalf("Neighbors(a) returned error: %v", err)
	}
	if len(got) != 1 || got[0] != "b" {
		t.Fatalf("Neighbors(a) after Load = %v, want [b]", got)
	}
}

func TestLoadOrBuild_LoadsFromCacheWhenPresent(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "a.md", `---
title: A
related: [b]
created: 2026-07-12
---
A body.
`)
	vaultfixture.WriteNote(t, root, "b.md", `---
title: B
created: 2026-07-12
---
B body.
`)

	g, _, provider, store := buildGraph(t, root)
	cachePath := filepath.Join(t.TempDir(), "graph.gob")
	if err := g.Save(cachePath); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	vaultfixture.WriteNote(t, root, "c.md", `---
title: C
created: 2026-07-12
---
C body.
`)

	loaded, _, err := LoadOrBuild(cachePath, provider, store)
	if err != nil {
		t.Fatalf("LoadOrBuild returned error: %v", err)
	}

	if len(loaded.All()) != 2 {
		t.Fatalf("All() = %+v, want LoadOrBuild to have used the stale cache (2 nodes), not rebuilt", loaded.All())
	}
}

// writeStaleCache writes a cache file with a stale format header followed
// by a validly gob-encoded graphData payload — so a Load that ignored the
// header would decode it successfully (and wrongly). This is the scenario
// version headers exist to catch, distinct from plain corrupt/garbage bytes
// which gob would already reject on its own.
func writeStaleCache(t *testing.T, path string, data graphData) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("creating stale-cache fixture: %v", err)
	}
	defer f.Close()

	if _, err := f.WriteString("KSC0\n"); err != nil {
		t.Fatalf("writing stale header: %v", err)
	}
	if err := gob.NewEncoder(f).Encode(data); err != nil {
		t.Fatalf("encoding stale-cache payload: %v", err)
	}
}

func TestLoad_ErrorsOnStaleCacheHeader(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "graph.gob")
	writeStaleCache(t, cachePath, graphData{Entries: map[string]GraphEntry{}})

	root := t.TempDir()
	provider := vault.NewLocalVaultProvider(root)
	store := notes.NewVaultNoteStore(provider)

	if _, err := Load(cachePath, provider, store); err == nil {
		t.Fatal("Load returned no error for a stale cache header, want error even though the payload decodes validly")
	}
}

func TestLoadOrBuild_FallsBackWhenCacheHeaderStale(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "a.md", `---
title: A
related: [b]
created: 2026-07-12
---
A body.
`)
	vaultfixture.WriteNote(t, root, "b.md", `---
title: B
created: 2026-07-12
---
B body.
`)

	cachePath := filepath.Join(t.TempDir(), "graph.gob")
	writeStaleCache(t, cachePath, graphData{Entries: map[string]GraphEntry{}})

	provider := vault.NewLocalVaultProvider(root)
	store := notes.NewVaultNoteStore(provider)

	g, _, err := LoadOrBuild(cachePath, provider, store)
	if err != nil {
		t.Fatalf("LoadOrBuild returned error: %v", err)
	}
	if len(g.All()) != 2 {
		t.Fatalf("All() = %+v, want LoadOrBuild to have rebuilt from the vault (2 nodes)", g.All())
	}
}

func TestLoadOrBuild_FallsBackToBuildWhenCacheMissing(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "a.md", `---
title: A
created: 2026-07-12
---
A body.
`)

	provider := vault.NewLocalVaultProvider(root)
	store := notes.NewVaultNoteStore(provider)
	cachePath := filepath.Join(t.TempDir(), "does-not-exist.gob")

	g, _, err := LoadOrBuild(cachePath, provider, store)
	if err != nil {
		t.Fatalf("LoadOrBuild returned error: %v", err)
	}

	if len(g.All()) != 1 {
		t.Fatalf("All() = %+v, want a fresh Build from the vault", g.All())
	}
}

func TestAll_ReturnsEveryNode(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "linux/process.md", `---
title: Process
related: [linux/memory]
created: 2026-07-12
---
Body text.
`)
	vaultfixture.WriteNote(t, root, "linux/memory.md", `---
title: Memory
created: 2026-07-13
---
Memory body.
`)

	g, _, _, _ := buildGraph(t, root)

	got := g.All()
	if len(got) != 2 {
		t.Fatalf("All() returned %d entries, want 2: %+v", len(got), got)
	}

	ids := map[string]bool{}
	for _, entry := range got {
		ids[entry.ID] = true
	}
	if !ids["linux/process"] || !ids["linux/memory"] {
		t.Errorf("All() = %+v, want entries for linux/process and linux/memory", got)
	}
}
