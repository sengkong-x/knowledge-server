package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sengkong/knowledge-server/internal/graph"
	"github.com/sengkong/knowledge-server/internal/index"
	"github.com/sengkong/knowledge-server/internal/notes"
	"github.com/sengkong/knowledge-server/internal/search"
	"github.com/sengkong/knowledge-server/internal/vault"
	"github.com/sengkong/knowledge-server/internal/vaultfixture"
)

func newTestHandler(t *testing.T, root string) http.Handler {
	t.Helper()
	provider := vault.NewLocalVaultProvider(root)
	store := notes.NewVaultNoteStore(provider)

	idx, _, err := index.Build(provider, store)
	if err != nil {
		t.Fatalf("index.Build returned error: %v", err)
	}
	ss, _, err := search.Build(provider, store)
	if err != nil {
		t.Fatalf("search.Build returned error: %v", err)
	}
	g, _, err := graph.Build(provider, store)
	if err != nil {
		t.Fatalf("graph.Build returned error: %v", err)
	}

	return New(root, provider, idx, ss, g)
}

const clockNote = `---
title: Hybrid Logical Clock
tags: [distributed-systems]
created: 2026-07-12
---
Combines physical time with a logical counter.
`

type fakeVaultProvider struct {
	notes []vault.NoteRef
}

func (f *fakeVaultProvider) ListNotes() ([]vault.NoteRef, error)   { return f.notes, nil }
func (f *fakeVaultProvider) ReadNote(id string) ([]byte, error)    { return nil, nil }
func (f *fakeVaultProvider) ReadAsset(path string) ([]byte, error) { return nil, nil }

func TestSearch_ReturnsMatchesForTextQuery(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "distributed-systems/hlc.md", clockNote)

	handler := newTestHandler(t, root)

	req := httptest.NewRequest(http.MethodGet, "/search?q=logical", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got []searchResultResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshaling response: %v", err)
	}

	if len(got) != 1 || got[0].ID != "distributed-systems/hlc" {
		t.Fatalf("results = %+v, want only distributed-systems/hlc", got)
	}
}

func TestSearch_ReturnsMatchesForTagOnly(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "distributed-systems/hlc.md", clockNote)
	vaultfixture.WriteNote(t, root, "cooking/pasta.md", `---
title: Pasta
tags: [food]
created: 2026-07-12
---
Boil water.
`)

	handler := newTestHandler(t, root)

	req := httptest.NewRequest(http.MethodGet, "/search?tag=distributed-systems", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got []searchResultResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshaling response: %v", err)
	}

	if len(got) != 1 || got[0].ID != "distributed-systems/hlc" {
		t.Fatalf("results = %+v, want only distributed-systems/hlc", got)
	}
}

func TestSearch_CombinesQAndTagWithANDSemantics(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "distributed-systems/hlc.md", clockNote)
	vaultfixture.WriteNote(t, root, "distributed-systems/vector-clock.md", `---
title: Vector Clock
tags: [distributed-systems]
created: 2026-07-12
---
Tracks logical causality across processes.
`)
	vaultfixture.WriteNote(t, root, "cooking/pasta.md", `---
title: Pasta Logical Steps
tags: [food]
created: 2026-07-12
---
Boil water, add pasta.
`)

	handler := newTestHandler(t, root)

	req := httptest.NewRequest(http.MethodGet, "/search?q=logical&tag=distributed-systems", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got []searchResultResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshaling response: %v", err)
	}

	gotIDs := make(map[string]bool)
	for _, r := range got {
		gotIDs[r.ID] = true
	}
	want := map[string]bool{"distributed-systems/hlc": true, "distributed-systems/vector-clock": true}
	if len(got) != len(want) {
		t.Fatalf("results = %+v, want exactly %v (matches both q and tag)", got, want)
	}
	for id := range want {
		if !gotIDs[id] {
			t.Errorf("results = %+v, missing %q", got, id)
		}
	}
}

func TestSearch_ReturnsBadRequestWhenNeitherQNorTagGiven(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "distributed-systems/hlc.md", clockNote)

	handler := newTestHandler(t, root)

	req := httptest.NewRequest(http.MethodGet, "/search", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGraphNeighbors_ReturnsDirectNeighbors(t *testing.T) {
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

	handler := newTestHandler(t, root)

	req := httptest.NewRequest(http.MethodGet, "/graph/neighbors?id=b", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got neighborsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshaling response: %v", err)
	}
	if len(got.Neighbors) != 1 || got.Neighbors[0] != "a" {
		t.Fatalf("neighbors = %v, want [a]", got.Neighbors)
	}
}

func TestGraphNeighbors_ReturnsNotFoundForUnknownID(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "a.md", `---
title: A
created: 2026-07-12
---
A body.
`)

	handler := newTestHandler(t, root)

	req := httptest.NewRequest(http.MethodGet, "/graph/neighbors?id=does-not-exist", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestGraphPath_ReturnsShortestPath(t *testing.T) {
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

	handler := newTestHandler(t, root)

	req := httptest.NewRequest(http.MethodGet, "/graph/path?from=a&to=b", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got pathResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshaling response: %v", err)
	}
	if !got.Found {
		t.Fatal("found = false, want true")
	}
	want := []string{"a", "b"}
	if len(got.Path) != len(want) || got.Path[0] != want[0] || got.Path[1] != want[1] {
		t.Fatalf("path = %v, want %v", got.Path, want)
	}
}

func TestGraphPath_ReturnsNotFoundForUnknownID(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "a.md", `---
title: A
created: 2026-07-12
---
A body.
`)

	handler := newTestHandler(t, root)

	req := httptest.NewRequest(http.MethodGet, "/graph/path?from=a&to=does-not-exist", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestGraphPath_ReturnsEmptyArrayNotNullWhenDisconnected(t *testing.T) {
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

	handler := newTestHandler(t, root)

	req := httptest.NewRequest(http.MethodGet, "/graph/path?from=a&to=b", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if strings.Contains(rec.Body.String(), `"path":null`) {
		t.Errorf("body = %q, want \"path\":[] not \"path\":null", rec.Body.String())
	}
}

func TestGraphOrphans_ReturnsNotesWithZeroEdges(t *testing.T) {
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

	handler := newTestHandler(t, root)

	req := httptest.NewRequest(http.MethodGet, "/graph/orphans", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got orphansResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshaling response: %v", err)
	}
	if len(got.Orphans) != 1 || got.Orphans[0] != "c" {
		t.Fatalf("orphans = %v, want [c]", got.Orphans)
	}
}

func TestHealth_ReturnsOKWithVaultPathAndNoteCount(t *testing.T) {
	provider := &fakeVaultProvider{notes: []vault.NoteRef{
		{ID: "linux/process", Path: "linux/process.md"},
		{ID: "database/wal", Path: "database/wal.md"},
	}}

	store := notes.NewVaultNoteStore(provider)
	idx, _, err := index.Build(provider, store)
	if err != nil {
		t.Fatalf("index.Build returned error: %v", err)
	}
	ss, _, err := search.Build(provider, store)
	if err != nil {
		t.Fatalf("search.Build returned error: %v", err)
	}
	g, _, err := graph.Build(provider, store)
	if err != nil {
		t.Fatalf("graph.Build returned error: %v", err)
	}

	handler := New("/srv/knowledge", provider, idx, ss, g)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	wantBody := `{"vault_path":"/srv/knowledge","note_count":2}`
	if rec.Body.String() != wantBody+"\n" && rec.Body.String() != wantBody {
		t.Errorf("body = %q, want %q", rec.Body.String(), wantBody)
	}
}
