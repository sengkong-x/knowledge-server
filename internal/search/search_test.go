package search

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/sengkong/knowledge-server/internal/notes"
	"github.com/sengkong/knowledge-server/internal/vault"
	"github.com/sengkong/knowledge-server/internal/vaultfixture"
)

func buildSearchStore(t *testing.T, root string) *SearchStore {
	ss, _, _, _ := buildSearchStoreWithDeps(t, root)
	return ss
}

func buildSearchStoreWithDeps(t *testing.T, root string) (*SearchStore, BuildReport, vault.VaultProvider, notes.NoteStore) {
	t.Helper()
	provider := vault.NewLocalVaultProvider(root)
	store := notes.NewVaultNoteStore(provider)

	ss, report, err := Build(provider, store)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	return ss, report, provider, store
}

const clockNote = `---
title: Hybrid Logical Clock
created: 2026-07-12
---
Combines physical time with a logical counter.
`

func TestQuery_ReturnsNoteMatchingTitleSubstring(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "distributed-systems/hlc.md", clockNote)

	ss := buildSearchStore(t, root)

	got := ss.Query("Logical")
	if len(got) != 1 || got[0].ID != "distributed-systems/hlc" {
		t.Fatalf("Query(Logical) = %+v, want only distributed-systems/hlc", got)
	}
}

func TestQuery_MatchesSubstringInBody(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "distributed-systems/hlc.md", clockNote)

	ss := buildSearchStore(t, root)

	got := ss.Query("physical time")
	if len(got) != 1 || got[0].ID != "distributed-systems/hlc" {
		t.Fatalf("Query(physical time) = %+v, want only distributed-systems/hlc (matched in body)", got)
	}
}

func TestQuery_MatchesSubstringInAlias(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "distributed-systems/hlc.md", `---
title: Hybrid Logical Clock
aliases: [HLC]
created: 2026-07-12
---
Combines physical time with a logical counter.
`)

	ss := buildSearchStore(t, root)

	got := ss.Query("HLC")
	if len(got) != 1 || got[0].ID != "distributed-systems/hlc" {
		t.Fatalf("Query(HLC) = %+v, want only distributed-systems/hlc (matched in alias)", got)
	}
}

func TestQuery_RanksTitleMatchBeforeBodyOnlyMatch(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "animals/aardvark.md", `---
title: Aardvark Facts
created: 2026-07-12
---
Includes elephant behavior notes.
`)
	vaultfixture.WriteNote(t, root, "animals/zebra.md", `---
title: Zebra and Elephant Coexistence
created: 2026-07-12
---
Zebras graze near watering holes.
`)

	ss := buildSearchStore(t, root)

	got := ss.Query("elephant")
	if len(got) != 2 {
		t.Fatalf("Query(elephant) returned %d results, want 2: %+v", len(got), got)
	}
	if got[0].ID != "animals/zebra" {
		t.Errorf("Query(elephant)[0] = %+v, want the title match (animals/zebra) ranked first", got[0])
	}
	if got[1].ID != "animals/aardvark" {
		t.Errorf("Query(elephant)[1] = %+v, want the body-only match (animals/aardvark) ranked second", got[1])
	}
}

func TestQuery_ResultIncludesSnippetOfSurroundingText(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "distributed-systems/hlc.md", `---
title: Hybrid Logical Clock
created: 2026-07-12
---
The quick brown fox jumps over the lazy dog while thinking deeply about hybrid logical clocks and distributed consensus protocols in the morning.
`)

	ss := buildSearchStore(t, root)

	got := ss.Query("logical clocks")
	if len(got) != 1 {
		t.Fatalf("Query(logical clocks) returned %d results, want 1", len(got))
	}

	snippet := got[0].Snippet
	if !strings.Contains(snippet, "hybrid logical clocks and") {
		t.Errorf("Snippet = %q, want it to contain surrounding context %q", snippet, "hybrid logical clocks and")
	}
	if !strings.HasPrefix(snippet, "…") {
		t.Errorf("Snippet = %q, want it to start with an ellipsis (truncated before match)", snippet)
	}
	if !strings.HasSuffix(snippet, "…") {
		t.Errorf("Snippet = %q, want it to end with an ellipsis (truncated after match)", snippet)
	}
}

func TestUpsert_AddsNewNoteToSearchStore(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "distributed-systems/hlc.md", clockNote)

	ss, _, _, _ := buildSearchStoreWithDeps(t, root)

	vaultfixture.WriteNote(t, root, "animals/zebra.md", `---
title: Zebra Migration
created: 2026-07-13
---
Zebras travel in herds across the savanna.
`)

	if err := ss.Upsert("animals/zebra"); err != nil {
		t.Fatalf("Upsert returned error: %v", err)
	}

	got := ss.Query("savanna")
	if len(got) != 1 || got[0].ID != "animals/zebra" {
		t.Fatalf("Query(savanna) = %+v, want only animals/zebra after Upsert", got)
	}
}

func TestRemove_DropsEntryFromSearchStore(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "distributed-systems/hlc.md", clockNote)

	ss := buildSearchStore(t, root)

	ss.Remove("distributed-systems/hlc")

	got := ss.Query("Logical")
	if len(got) != 0 {
		t.Fatalf("Query(Logical) = %+v after Remove, want empty", got)
	}
}

func TestSaveLoad_RoundTripsEntries(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "distributed-systems/hlc.md", clockNote)

	ss, _, provider, store := buildSearchStoreWithDeps(t, root)

	cachePath := filepath.Join(t.TempDir(), "search.gob")
	if err := ss.Save(cachePath); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	loaded, err := Load(cachePath, provider, store)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	got := loaded.Query("Logical")
	if len(got) != 1 || got[0].ID != "distributed-systems/hlc" {
		t.Fatalf("Query(Logical) after Load = %+v, want only distributed-systems/hlc", got)
	}
}

func TestLoadOrBuild_LoadsFromCacheWhenPresent(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "distributed-systems/hlc.md", clockNote)

	ss, _, provider, store := buildSearchStoreWithDeps(t, root)
	cachePath := filepath.Join(t.TempDir(), "search.gob")
	if err := ss.Save(cachePath); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	vaultfixture.WriteNote(t, root, "cooking/pasta.md", `---
title: Pasta
created: 2026-07-12
---
Boil water.
`)

	loaded, _, err := LoadOrBuild(cachePath, provider, store)
	if err != nil {
		t.Fatalf("LoadOrBuild returned error: %v", err)
	}

	if got := loaded.Query("water"); len(got) != 0 {
		t.Fatalf("Query(water) = %+v, want LoadOrBuild to have used the stale cache, not rebuilt", got)
	}
	if got := loaded.Query("Logical"); len(got) != 1 {
		t.Fatalf("Query(Logical) = %+v, want the cached entry present", got)
	}
}

func TestLoadOrBuild_FallsBackToBuildWhenCacheMissing(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "distributed-systems/hlc.md", clockNote)

	provider := vault.NewLocalVaultProvider(root)
	store := notes.NewVaultNoteStore(provider)
	cachePath := filepath.Join(t.TempDir(), "does-not-exist.gob")

	ss, _, err := LoadOrBuild(cachePath, provider, store)
	if err != nil {
		t.Fatalf("LoadOrBuild returned error: %v", err)
	}

	if got := ss.Query("Logical"); len(got) != 1 {
		t.Fatalf("Query(Logical) = %+v, want a fresh Build from the vault", got)
	}
}

func TestQuery_MatchesMidWordSubstringCaseInsensitively(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "distributed-systems/hlc.md", clockNote)

	ss := buildSearchStore(t, root)

	got := ss.Query("OCK")
	if len(got) != 1 || got[0].ID != "distributed-systems/hlc" {
		t.Fatalf("Query(OCK) = %+v, want only distributed-systems/hlc (mid-word substring of Clock, case-insensitive)", got)
	}
}
