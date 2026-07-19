package server

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sengkong/knowledge-server/internal/activevault"
	"github.com/sengkong/knowledge-server/internal/state"
	"github.com/sengkong/knowledge-server/internal/vault"
	"github.com/sengkong/knowledge-server/internal/vaultfixture"
)

func setCacheHome(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
}

func setConfigHome(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
}

func newTestHandler(t *testing.T, root string) http.Handler {
	t.Helper()
	handler, _ := newTestHandlerWithState(t, root)
	return handler
}

func newTestHandlerWithState(t *testing.T, root string) (http.Handler, *state.State) {
	t.Helper()
	setCacheHome(t)

	av := activevault.New("light")
	if err := av.Switch(root); err != nil {
		t.Fatalf("Switch returned error: %v", err)
	}

	_, _, _, s, ok := av.Snapshot()
	if !ok {
		t.Fatal("Snapshot ok = false after Switch")
	}
	return New(av), s
}

const clockNote = `---
title: Hybrid Logical Clock
tags: [distributed-systems]
created: 2026-07-12
---
Combines physical time with a logical counter.
`

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

func TestNoteDetail_RendersGoldmarkHTMLOfBody(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "distributed-systems/hlc.md", `---
title: Hybrid Logical Clock
created: 2026-07-12
---
# Overview

Combines **physical time** with a logical counter.
`)

	handler := newTestHandler(t, root)

	req := httptest.NewRequest(http.MethodGet, "/notes/distributed-systems/hlc", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Hybrid Logical Clock") {
		t.Errorf("body = %q, want it to contain the note's title", body)
	}
	if !strings.Contains(body, "<h1>Overview</h1>") {
		t.Errorf("body = %q, want goldmark-rendered <h1>Overview</h1>", body)
	}
	if !strings.Contains(body, "<strong>physical time</strong>") {
		t.Errorf("body = %q, want goldmark-rendered <strong>physical time</strong>", body)
	}
}

func TestNoteDetail_ListsGraphNeighborsAsRelatedNotes(t *testing.T) {
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

	req := httptest.NewRequest(http.MethodGet, "/notes/a", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `href="/notes/b"`) {
		t.Errorf("body = %q, want a link to related note b", body)
	}
}

func TestNoteDetail_ReturnsNotFoundForUnknownID(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "a.md", `---
title: A
created: 2026-07-12
---
Body.
`)

	handler := newTestHandler(t, root)

	req := httptest.NewRequest(http.MethodGet, "/notes/does-not-exist", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestAssets_ServesVaultRelativeFile(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "diagrams/thing.svg", `<svg></svg>`)

	handler := newTestHandler(t, root)

	req := httptest.NewRequest(http.MethodGet, "/assets/diagrams/thing.svg", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Body.String() != `<svg></svg>` {
		t.Errorf("body = %q, want the raw asset bytes", rec.Body.String())
	}
}

// Mirrors docs/architecture.md's real setup: a single-path-segment note
// (ID "architecture", no slashes) embedding an image via a "../assets/..."
// relative path. This proves that path actually resolves end-to-end through
// a browser's relative-URL rules — /notes/architecture + "../assets/x.svg"
// -> /assets/x.svg -- not just that each route works in isolation.
func TestNoteDetail_RelativeImagePathResolvesThroughAssetsRoute(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "architecture.md", `---
title: Architecture
created: 2026-07-18
---
![diagram](../assets/diagrams/thing.svg)
`)
	vaultfixture.WriteNote(t, root, "diagrams/thing.svg", `<svg></svg>`)

	handler := newTestHandler(t, root)

	noteReq := httptest.NewRequest(http.MethodGet, "/notes/architecture", nil)
	noteRec := httptest.NewRecorder()
	handler.ServeHTTP(noteRec, noteReq)

	const wantSrc = `src="../assets/diagrams/thing.svg"`
	if !strings.Contains(noteRec.Body.String(), wantSrc) {
		t.Fatalf("note body = %q, want it to contain %q", noteRec.Body.String(), wantSrc)
	}

	resolved, err := url.Parse("/notes/architecture")
	if err != nil {
		t.Fatalf("parsing base URL: %v", err)
	}
	imgURL, err := url.Parse("../assets/diagrams/thing.svg")
	if err != nil {
		t.Fatalf("parsing image URL: %v", err)
	}
	assetPath := resolved.ResolveReference(imgURL).Path
	if assetPath != "/assets/diagrams/thing.svg" {
		t.Fatalf("resolved asset path = %q, want %q", assetPath, "/assets/diagrams/thing.svg")
	}

	assetReq := httptest.NewRequest(http.MethodGet, assetPath, nil)
	assetRec := httptest.NewRecorder()
	handler.ServeHTTP(assetRec, assetReq)

	if assetRec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", assetRec.Code, http.StatusOK)
	}
	if assetRec.Body.String() != `<svg></svg>` {
		t.Errorf("body = %q, want the raw asset bytes", assetRec.Body.String())
	}
}

func TestAssets_ReturnsNotFoundForMissingFile(t *testing.T) {
	root := t.TempDir()

	handler := newTestHandler(t, root)

	req := httptest.NewRequest(http.MethodGet, "/assets/does-not-exist.svg", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// A ".." in the request path never reaches the handler at all: net/http's
// ServeMux redirects it to the cleaned equivalent first (see net/http docs).
// VaultProvider.ReadAsset's own traversal guard is covered directly in
// internal/vault's tests, since that's the layer that owns the invariant.
func TestAssets_DirtyPathIsRedirectedByServeMuxBeforeReachingHandler(t *testing.T) {
	root := t.TempDir()
	handler := newTestHandler(t, root)

	req := httptest.NewRequest(http.MethodGet, "/assets/../server.go", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("status = %d, want %d (ServeMux's redirect for a dirty path)", rec.Code, http.StatusTemporaryRedirect)
	}
}

func TestGraphData_ReturnsFullNodeAndEdgeSet(t *testing.T) {
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

	req := httptest.NewRequest(http.MethodGet, "/graph/data", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got graphDataResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshaling response: %v", err)
	}

	if len(got.Nodes) != 2 {
		t.Fatalf("Nodes = %+v, want 2 nodes", got.Nodes)
	}
	nodeIDs := map[string][]string{}
	for _, n := range got.Nodes {
		nodeIDs[n.ID] = n.Neighbors
	}
	if len(nodeIDs["a"]) != 1 || nodeIDs["a"][0] != "b" {
		t.Errorf("node a neighbors = %v, want [b]", nodeIDs["a"])
	}
	if len(nodeIDs["b"]) != 1 || nodeIDs["b"][0] != "a" {
		t.Errorf("node b neighbors = %v, want [a]", nodeIDs["b"])
	}
}

func TestBrowse_ListsAllNotes(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "linux/process.md", `---
title: Process
created: 2026-07-12
---
Body.
`)
	vaultfixture.WriteNote(t, root, "cooking/pasta.md", `---
title: Pasta
created: 2026-07-12
---
Body.
`)

	handler := newTestHandler(t, root)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Process") || !strings.Contains(body, "Pasta") {
		t.Errorf("body = %q, want it to list both note titles", body)
	}
}

func TestBrowse_FiltersByTag(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "linux/process.md", `---
title: Process
tags: [kernel]
created: 2026-07-12
---
Body.
`)
	vaultfixture.WriteNote(t, root, "cooking/pasta.md", `---
title: Pasta
tags: [food]
created: 2026-07-12
---
Body.
`)

	handler := newTestHandler(t, root)

	req := httptest.NewRequest(http.MethodGet, "/?tag=kernel", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Process") {
		t.Errorf("body = %q, want it to include Process", body)
	}
	if strings.Contains(body, "Pasta") {
		t.Errorf("body = %q, want it to exclude Pasta when filtered by tag=kernel", body)
	}
}

func TestSearchUI_RendersMatchingResults(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "distributed-systems/hlc.md", clockNote)
	vaultfixture.WriteNote(t, root, "cooking/pasta.md", `---
title: Pasta
created: 2026-07-12
---
Boil water.
`)

	handler := newTestHandler(t, root)

	req := httptest.NewRequest(http.MethodGet, "/search/ui?q=logical", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Hybrid Logical Clock") {
		t.Errorf("body = %q, want it to include the matching note's title", body)
	}
	if strings.Contains(body, "Pasta") {
		t.Errorf("body = %q, want it to exclude the non-matching note", body)
	}
}

func TestSearchUI_RendersEmptyFormWithNoQuery(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "distributed-systems/hlc.md", clockNote)

	handler := newTestHandler(t, root)

	req := httptest.NewRequest(http.MethodGet, "/search/ui", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if strings.Contains(rec.Body.String(), "Hybrid Logical Clock") {
		t.Errorf("body = %q, want no results when no query given", rec.Body.String())
	}
}

func TestGraphUI_RendersCytoscapeContainer(t *testing.T) {
	root := t.TempDir()
	handler := newTestHandler(t, root)

	req := httptest.NewRequest(http.MethodGet, "/graph/ui", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `id="cy"`) {
		t.Errorf("body = %q, want a Cytoscape container element", body)
	}
	if !strings.Contains(body, "/graph/data") {
		t.Errorf("body = %q, want it to fetch /graph/data", body)
	}
}

func TestEvents_BroadcastsPingOnChange(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "process.md", "---\ntitle: Process\ncreated: 2026-07-12\n---\nBody.\n")

	handler, s := newTestHandlerWithState(t, root)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/events")
	if err != nil {
		t.Fatalf("GET /events: %v", err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", resp.Header.Get("Content-Type"))
	}

	// Give the handler a moment to register its subscription before we
	// trigger a change, since the HTTP round trip above only guarantees
	// headers were flushed, not that Subscribe has run yet.
	time.Sleep(50 * time.Millisecond)
	if err := s.Upsert("process"); err != nil {
		t.Fatalf("Upsert returned error: %v", err)
	}

	line := make(chan string, 1)
	go func() {
		reader := bufio.NewReader(resp.Body)
		for {
			l, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			if strings.TrimSpace(l) != "" {
				line <- l
				return
			}
		}
	}()

	select {
	case got := <-line:
		if !strings.HasPrefix(got, "data:") {
			t.Errorf("line = %q, want an SSE data: line", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SSE ping after Upsert")
	}
}

func TestHealth_ReturnsOKWithVaultPathAndNoteCount(t *testing.T) {
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "linux/process.md", `---
title: Process
created: 2026-07-12
---
Body.
`)
	vaultfixture.WriteNote(t, root, "database/wal.md", `---
title: WAL
created: 2026-07-12
---
Body.
`)

	handler := newTestHandler(t, root)

	canonical, err := vault.CanonicalPath(root)
	if err != nil {
		t.Fatalf("vault.CanonicalPath: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got healthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshaling response: %v", err)
	}
	if got.VaultPath != canonical {
		t.Errorf("VaultPath = %q, want %q", got.VaultPath, canonical)
	}
	if got.NoteCount != 2 {
		t.Errorf("NoteCount = %d, want 2", got.NoteCount)
	}
}

func TestHealth_ReturnsEmptyWhenNoVaultSelected(t *testing.T) {
	av := activevault.New("light")
	handler := New(av)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got healthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshaling response: %v", err)
	}
	if got.VaultPath != "" || got.NoteCount != 0 {
		t.Errorf("got %+v, want zero-value healthResponse when no vault is selected", got)
	}
}

func TestBrowse_RendersNoVaultSelectedWhenNoneIsActive(t *testing.T) {
	av := activevault.New("light")
	handler := New(av)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "No vault selected") {
		t.Errorf("body = %q, want a no-vault-selected message, not a panic or empty page", rec.Body.String())
	}
}

func TestNoteDetail_RendersNoVaultSelectedWhenNoneIsActive(t *testing.T) {
	av := activevault.New("light")
	handler := New(av)

	req := httptest.NewRequest(http.MethodGet, "/notes/anything", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "No vault selected") {
		t.Errorf("body = %q, want a no-vault-selected message", rec.Body.String())
	}
}

func TestSearch_ReturnsConflictJSONWhenNoVaultSelected(t *testing.T) {
	av := activevault.New("light")
	handler := New(av)

	req := httptest.NewRequest(http.MethodGet, "/search?q=anything", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
	}

	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshaling response: %v", err)
	}
	if got["error"] == "" {
		t.Errorf("got %+v, want a non-empty error message", got)
	}
}

func TestGraphData_ReturnsConflictJSONWhenNoVaultSelected(t *testing.T) {
	av := activevault.New("light")
	handler := New(av)

	req := httptest.NewRequest(http.MethodGet, "/graph/data", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestAssets_ReturnsNotFoundWhenNoVaultSelected(t *testing.T) {
	av := activevault.New("light")
	handler := New(av)

	req := httptest.NewRequest(http.MethodGet, "/assets/whatever.svg", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestSettings_ReturnsCurrentVaultThemeAndHistory(t *testing.T) {
	setConfigHome(t)

	av := activevault.New("dark")
	handler := New(av)

	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got settingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshaling response: %v", err)
	}
	// No PUT /vault or /theme has run yet in this test, so settings.json
	// doesn't exist yet — Load() (Ticket 03) returns the zero value.
	if got.VaultPath != "" || got.Theme != "" || len(got.VaultHistory) != 0 {
		t.Errorf("got %+v, want zero-value settings before any switch/theme change", got)
	}
}

func TestSwitchVault_SucceedsAndPersistsToSettings(t *testing.T) {
	setCacheHome(t)
	setConfigHome(t)

	av := activevault.New("light")
	handler := New(av)

	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "note-a.md", `---
title: Note A
created: 2026-07-12
---
Body.
`)

	body, err := json.Marshal(switchVaultRequest{Path: root})
	if err != nil {
		t.Fatalf("marshaling request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/vault", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %q", rec.Code, http.StatusOK, rec.Body.String())
	}

	canonical, err := vault.CanonicalPath(root)
	if err != nil {
		t.Fatalf("vault.CanonicalPath: %v", err)
	}

	_, _, _, s, ok := av.Snapshot()
	if !ok {
		t.Fatal("Snapshot ok = false after PUT /vault, want the switch to have taken effect")
	}
	if _, found := s.ByID("note-a"); !found {
		t.Error(`ByID("note-a") not found, want the switched-in vault's notes indexed`)
	}

	settingsReq := httptest.NewRequest(http.MethodGet, "/settings", nil)
	settingsRec := httptest.NewRecorder()
	handler.ServeHTTP(settingsRec, settingsReq)

	var gotSettings settingsResponse
	if err := json.Unmarshal(settingsRec.Body.Bytes(), &gotSettings); err != nil {
		t.Fatalf("unmarshaling settings response: %v", err)
	}
	if gotSettings.VaultPath != canonical {
		t.Errorf("settings VaultPath = %q, want %q", gotSettings.VaultPath, canonical)
	}
}

func TestSwitchVault_ValidationFailureLeavesPreviousVaultActiveAndSurfacesError(t *testing.T) {
	setCacheHome(t)
	setConfigHome(t)

	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "note-a.md", `---
title: Note A
created: 2026-07-12
---
Body.
`)

	av := activevault.New("light")
	if err := av.Switch(root); err != nil {
		t.Fatalf("initial Switch returned error: %v", err)
	}
	handler := New(av)

	badPath := filepath.Join(t.TempDir(), "does-not-exist")
	body, err := json.Marshal(switchVaultRequest{Path: badPath})
	if err != nil {
		t.Fatalf("marshaling request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/vault", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshaling response: %v", err)
	}
	if got["error"] == "" {
		t.Error("got empty error message, want the validation failure surfaced for the picker UI")
	}

	_, _, _, s, ok := av.Snapshot()
	if !ok {
		t.Fatal("Snapshot ok = false, want the previous vault to remain untouched after a failed switch")
	}
	if _, found := s.ByID("note-a"); !found {
		t.Error(`ByID("note-a") not found, want the previous vault's notes still present`)
	}
}

func TestSwitchTheme_SetsThemeAndPersistsToSettings(t *testing.T) {
	setConfigHome(t)

	av := activevault.New("light")
	handler := New(av)

	body, err := json.Marshal(switchThemeRequest{Theme: "dark"})
	if err != nil {
		t.Fatalf("marshaling request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/theme", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %q", rec.Code, http.StatusOK, rec.Body.String())
	}

	if got := av.Theme(); got != "dark" {
		t.Errorf("av.Theme() = %q, want %q", got, "dark")
	}

	settingsReq := httptest.NewRequest(http.MethodGet, "/settings", nil)
	settingsRec := httptest.NewRecorder()
	handler.ServeHTTP(settingsRec, settingsReq)

	var gotSettings settingsResponse
	if err := json.Unmarshal(settingsRec.Body.Bytes(), &gotSettings); err != nil {
		t.Fatalf("unmarshaling settings response: %v", err)
	}
	if gotSettings.Theme != "dark" {
		t.Errorf("settings Theme = %q, want %q", gotSettings.Theme, "dark")
	}
}

// TestEvents_ClosesStreamOnVaultSwitch verifies the documented resolution
// (see server.go's /events comment) for the edge case where an SSE
// connection is open across a vault switch: rather than silently going
// quiet (still subscribed to a State nobody will ever notify again), the
// stream ends, and a real browser's EventSource auto-reconnects to pick up
// the new vault's State.
func TestEvents_ClosesStreamOnVaultSwitch(t *testing.T) {
	setCacheHome(t)

	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "note-a.md", `---
title: Note A
created: 2026-07-12
---
Body.
`)

	av := activevault.New("light")
	if err := av.Switch(root); err != nil {
		t.Fatalf("initial Switch returned error: %v", err)
	}
	handler := New(av)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/events")
	if err != nil {
		t.Fatalf("GET /events: %v", err)
	}
	defer resp.Body.Close()

	time.Sleep(50 * time.Millisecond)

	root2 := t.TempDir()
	vaultfixture.WriteNote(t, root2, "note-b.md", `---
title: Note B
created: 2026-07-12
---
Body.
`)
	if err := av.Switch(root2); err != nil {
		t.Fatalf("Switch returned error: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		reader := bufio.NewReader(resp.Body)
		_, err := reader.ReadString('\n')
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("read from /events succeeded after a vault switch, want the stream to have closed")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for /events to close after a vault switch")
	}
}
