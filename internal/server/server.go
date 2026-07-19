package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"time"

	"github.com/sengkong/knowledge-server/internal/activevault"
	"github.com/sengkong/knowledge-server/internal/index"
	"github.com/sengkong/knowledge-server/internal/notes"
	"github.com/sengkong/knowledge-server/internal/settings"
	"github.com/sengkong/knowledge-server/web"
	"github.com/yuin/goldmark"
)

// layoutTemplate wraps a page's content in the full HTML document: theme
// CSS (driven by ActiveVault's current theme, switchable at runtime — see
// ADR-0011), vendored HTMX/Alpine.js, and a live-update script that reacts
// to the /events SSE ping (ADR-0009) by re-fetching the current page's
// content in place.
var layoutTemplate = template.Must(template.New("layout").Parse(`<!doctype html>
<html data-theme="{{.Theme}}">
<head>
<meta charset="utf-8">
<title>{{.Title}}</title>
<link rel="stylesheet" href="/themes/base.css">
<link rel="stylesheet" href="/themes/{{.Theme}}.css">
<script src="/vendor/htmx.min.js"></script>
<script src="/vendor/alpine.min.js" defer></script>
</head>
<body>
{{.Content}}
<script>
new EventSource("/events").onmessage = function () {
  htmx.ajax("GET", window.location.pathname + window.location.search, {target: "body", swap: "innerHTML"});
};
</script>
</body>
</html>`))

type layoutView struct {
	Theme   string
	Title   string
	Content template.HTML
}

// render writes content wrapped in the full page layout for a normal
// browser navigation, or just content on its own for an HTMX request
// (identified by the HX-Request header) — the live-update script above
// swaps the latter into <body> without re-loading <head>.
func render(w http.ResponseWriter, r *http.Request, theme, title string, content template.HTML) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if r.Header.Get("HX-Request") == "true" {
		w.Write([]byte(content))
		return
	}
	layoutTemplate.Execute(w, layoutView{Theme: theme, Title: title, Content: content})
}

func renderFragment(tmpl *template.Template, data any) (template.HTML, error) {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return template.HTML(buf.String()), nil
}

// noVaultSelectedTemplate is the fallback content for HTML routes when no
// Active Vault is selected (a first-class, expected state — the server
// boots into it and stays in it until the picker UI selects a vault).
var noVaultSelectedTemplate = template.Must(template.New("noVault").Parse(
	`<p>No vault selected. Pick one to get started.</p>`))

// noVaultSelected renders the "no vault selected" fallback for an HTML
// route so a missing Active Vault never reaches a nil provider/store/state
// and panics.
func noVaultSelected(w http.ResponseWriter, r *http.Request, theme string) {
	content, _ := renderFragment(noVaultSelectedTemplate, nil)
	render(w, r, theme, "No vault selected", content)
}

// noVaultSelectedJSON is the equivalent fallback for API routes.
func noVaultSelectedJSON(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)
	json.NewEncoder(w).Encode(map[string]string{"error": "no vault selected"})
}

// noteDetailTemplate renders a note's title, goldmark-rendered body, and its
// Graph neighbors as related-note links. Body is produced by goldmark from
// trusted, locally-authored Vault content and is intentionally not escaped
// further; Title and Neighbors are escaped normally.
var noteDetailTemplate = template.Must(template.New("note").Parse(`<h1>{{.Title}}</h1>
{{.Body}}
{{if .Neighbors}}<h2>Related notes</h2>
<ul>
{{range .Neighbors}}<li><a href="/notes/{{.}}">{{.}}</a></li>
{{end}}</ul>{{end}}`))

type noteDetailView struct {
	Title     string
	Body      template.HTML
	Neighbors []string
}

// browseTemplate lists notes as links to their detail page.
var browseTemplate = template.Must(template.New("browse").Parse(`<ul>
{{range .Entries}}<li><a href="/notes/{{.ID}}">{{.Title}}</a></li>
{{end}}</ul>`))

type browseView struct {
	Entries []index.IndexEntry
}

// searchUITemplate renders a search form plus any matching results.
var searchUITemplate = template.Must(template.New("searchUI").Parse(`<form hx-get="/search/ui" hx-target="body">
<input type="text" name="q" value="{{.Query}}">
</form>
<ul>
{{range .Results}}<li><a href="/notes/{{.ID}}">{{.Title}}</a></li>
{{end}}</ul>`))

type searchUIView struct {
	Query   string
	Results []searchResultResponse
}

// graphUITemplate is the Cytoscape.js graph visualization shell. Cytoscape
// itself and the script that fetches /graph/data into it are vendored
// frontend assets (see ADR-0007's companion asset-vendoring deliverable),
// not written here.
const graphUITemplate = `<div id="cy"></div>
<script src="/vendor/cytoscape.min.js"></script>
<script src="/js/graph.js" data-source="/graph/data"></script>`

type healthResponse struct {
	VaultPath string `json:"vault_path"`
	NoteCount int    `json:"note_count"`
}

type searchResultResponse struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Path    string   `json:"path"`
	Tags    []string `json:"tags"`
	Snippet string   `json:"snippet"`
}

type neighborsResponse struct {
	Neighbors []string `json:"neighbors"`
}

type pathResponse struct {
	Path  []string `json:"path"`
	Found bool     `json:"found"`
}

type orphansResponse struct {
	Orphans []string `json:"orphans"`
}

type graphNodeResponse struct {
	ID        string   `json:"id"`
	Neighbors []string `json:"neighbors"`
}

type graphDataResponse struct {
	Nodes []graphNodeResponse `json:"nodes"`
}

type settingsResponse struct {
	VaultPath    string   `json:"vault_path"`
	Theme        string   `json:"theme"`
	VaultHistory []string `json:"vault_history"`
}

type switchVaultRequest struct {
	Path string `json:"path"`
}

type switchThemeRequest struct {
	Theme string `json:"theme"`
}

// New builds the http.Handler for a running instance. Every route reads the
// Active Vault fresh per request via av.Snapshot()/av.Theme() rather than
// closing over a vault/theme captured once at construction time, since
// ADR-0011 lets av.Switch/SetTheme change what's active at any point in the
// server's lifetime.
func New(av *activevault.ActiveVault) http.Handler {
	mux := http.NewServeMux()

	assets := http.FileServerFS(web.FS)
	mux.Handle("GET /vendor/", assets)
	mux.Handle("GET /themes/", assets)
	mux.Handle("GET /js/", assets)

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		theme := av.Theme()
		_, _, _, s, ok := av.Snapshot()
		if !ok {
			noVaultSelected(w, r, theme)
			return
		}

		tag := r.URL.Query().Get("tag")

		var entries []index.IndexEntry
		if tag != "" {
			entries = s.ByTag(tag)
		} else {
			entries = s.IndexAll()
		}

		content, err := renderFragment(browseTemplate, browseView{Entries: entries})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		render(w, r, theme, "Browse", content)
	})

	mux.HandleFunc("GET /search/ui", func(w http.ResponseWriter, r *http.Request) {
		theme := av.Theme()
		_, _, _, s, ok := av.Snapshot()
		if !ok {
			noVaultSelected(w, r, theme)
			return
		}

		q := r.URL.Query().Get("q")

		var results []searchResultResponse
		if q != "" {
			for _, m := range s.Query(q) {
				entry, ok := s.ByID(m.ID)
				if !ok {
					continue
				}
				results = append(results, searchResultResponse{
					ID:      entry.ID,
					Title:   entry.Title,
					Path:    entry.Path,
					Tags:    entry.Tags,
					Snippet: m.Snippet,
				})
			}
		}

		content, err := renderFragment(searchUITemplate, searchUIView{Query: q, Results: results})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		render(w, r, theme, "Search", content)
	})

	mux.HandleFunc("GET /graph/ui", func(w http.ResponseWriter, r *http.Request) {
		render(w, r, av.Theme(), "Graph", template.HTML(graphUITemplate))
	})

	// /events: an SSE connection subscribes to the currently active vault's
	// State for content-change pings (ADR-0009). A vault switch discards
	// that State and swaps in a new one (Ticket 05), which would otherwise
	// leave any already-open /events connection subscribed to a State
	// nobody will ever notify again — a silent "stops updating" gap rather
	// than a visible failure. Resolution (deliberately chosen over the
	// alternative of leaving it silently stale): also subscribe to
	// av.SubscribeSwitch(), and end the stream the moment a switch happens.
	// The browser's EventSource auto-reconnects on a closed connection,
	// which re-runs this handler, re-Snapshots, and subscribes fresh to the
	// new State — so a switch costs one reconnect, not a silently stopped
	// feed.
	mux.HandleFunc("GET /events", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		_, _, _, s, hasVault := av.Snapshot()

		var stateCh <-chan struct{}
		unsubState := func() {}
		if hasVault {
			stateCh, unsubState = s.Subscribe()
		}
		defer unsubState()

		switchCh, unsubSwitch := av.SubscribeSwitch()
		defer unsubSwitch()

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		for {
			select {
			case <-r.Context().Done():
				return
			case <-switchCh:
				return
			case <-stateCh:
				// Generic "something changed" ping, no per-note payload
				// (see ADR-0009) — every listening view re-fetches its own
				// current content rather than reasoning about what changed.
				fmt.Fprint(w, "data: change\n\n")
				flusher.Flush()
			}
		}
	})

	mux.HandleFunc("GET /assets/{path...}", func(w http.ResponseWriter, r *http.Request) {
		// No path-traversal guard needed here: ServeMux itself redirects any
		// request path containing ".." to its cleaned equivalent before a
		// handler ever runs (see net/http's ServeMux docs), and
		// VaultProvider.ReadAsset rejects any escape of the Vault root on
		// its own behalf, since it's the abstraction that owns filesystem
		// safety, not this transport-layer handler.
		_, provider, _, _, ok := av.Snapshot()
		if !ok {
			http.NotFound(w, r)
			return
		}

		reqPath := r.PathValue("path")

		data, err := provider.ReadAsset(reqPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.ServeContent(w, r, reqPath, time.Time{}, bytes.NewReader(data))
	})

	mux.HandleFunc("GET /notes/{id...}", func(w http.ResponseWriter, r *http.Request) {
		theme := av.Theme()
		_, _, store, s, ok := av.Snapshot()
		if !ok {
			noVaultSelected(w, r, theme)
			return
		}

		id := r.PathValue("id")
		note, err := store.Load(id)
		if err != nil {
			if errors.Is(err, notes.ErrNotFound) {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var body bytes.Buffer
		if err := goldmark.Convert([]byte(note.Body), &body); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// A note missing from the Graph (e.g. it failed to parse during the
		// last Build) simply has no related notes to show, rather than
		// failing the whole page.
		neighbors, _ := s.Neighbors(id)

		content, err := renderFragment(noteDetailTemplate, noteDetailView{
			Title:     note.Title,
			Body:      template.HTML(body.String()),
			Neighbors: neighbors,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		render(w, r, theme, note.Title, content)
	})

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		path, provider, _, _, ok := av.Snapshot()

		w.Header().Set("Content-Type", "application/json")
		if !ok {
			json.NewEncoder(w).Encode(healthResponse{})
			return
		}

		notes, err := provider.ListNotes()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(healthResponse{
			VaultPath: path,
			NoteCount: len(notes),
		})
	})

	mux.HandleFunc("GET /search", func(w http.ResponseWriter, r *http.Request) {
		_, _, _, s, ok := av.Snapshot()
		if !ok {
			noVaultSelectedJSON(w)
			return
		}

		q := r.URL.Query().Get("q")
		tag := r.URL.Query().Get("tag")
		if q == "" && tag == "" {
			http.Error(w, "missing q or tag parameter", http.StatusBadRequest)
			return
		}

		// snippets holds the matched-text excerpt per note ID when q is
		// given; tag-only results have no snippet, since they didn't come
		// from a text query.
		var candidates []index.IndexEntry
		snippets := make(map[string]string)
		if q != "" {
			for _, m := range s.Query(q) {
				if entry, ok := s.ByID(m.ID); ok {
					candidates = append(candidates, entry)
					snippets[m.ID] = m.Snippet
				}
			}
		} else {
			candidates = s.ByTag(tag)
		}

		results := make([]searchResultResponse, 0, len(candidates))
		for _, entry := range candidates {
			if q != "" && tag != "" && !entry.HasTag(tag) {
				continue
			}
			results = append(results, searchResultResponse{
				ID:      entry.ID,
				Title:   entry.Title,
				Path:    entry.Path,
				Tags:    entry.Tags,
				Snippet: snippets[entry.ID],
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
	})

	mux.HandleFunc("GET /graph/neighbors", func(w http.ResponseWriter, r *http.Request) {
		_, _, _, s, ok := av.Snapshot()
		if !ok {
			noVaultSelectedJSON(w)
			return
		}

		id := r.URL.Query().Get("id")
		neighbors, err := s.Neighbors(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(neighborsResponse{Neighbors: neighbors})
	})

	mux.HandleFunc("GET /graph/path", func(w http.ResponseWriter, r *http.Request) {
		_, _, _, s, ok := av.Snapshot()
		if !ok {
			noVaultSelectedJSON(w)
			return
		}

		from := r.URL.Query().Get("from")
		to := r.URL.Query().Get("to")
		path, found, err := s.ShortestPath(from, to)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(pathResponse{Path: path, Found: found})
	})

	mux.HandleFunc("GET /graph/orphans", func(w http.ResponseWriter, r *http.Request) {
		_, _, _, s, ok := av.Snapshot()
		if !ok {
			noVaultSelectedJSON(w)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(orphansResponse{Orphans: s.Orphans()})
	})

	mux.HandleFunc("GET /graph/data", func(w http.ResponseWriter, r *http.Request) {
		_, _, _, s, ok := av.Snapshot()
		if !ok {
			noVaultSelectedJSON(w)
			return
		}

		entries := s.GraphAll()
		nodes := make([]graphNodeResponse, 0, len(entries))
		for _, entry := range entries {
			neighbors := make([]string, 0, len(entry.Neighbors))
			for n := range entry.Neighbors {
				neighbors = append(neighbors, n)
			}
			sort.Strings(neighbors)
			nodes = append(nodes, graphNodeResponse{ID: entry.ID, Neighbors: neighbors})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(graphDataResponse{Nodes: nodes})
	})

	mux.HandleFunc("GET /settings", func(w http.ResponseWriter, r *http.Request) {
		s, err := settings.Load()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(settingsResponse{
			VaultPath:    s.VaultPath,
			Theme:        s.Theme,
			VaultHistory: s.VaultHistory,
		})
	})

	// PUT, not POST: switching the vault sets the one current selection
	// (an idempotent "this is now the active vault" state change) rather
	// than creating a new resource — same reasoning for PUT /theme below.
	// No existing route in this table sets a precedent either way (all are
	// GET), so this is a fresh choice, documented here rather than left
	// implicit.
	mux.HandleFunc("PUT /vault", func(w http.ResponseWriter, r *http.Request) {
		var req switchVaultRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		if err := av.Switch(req.Path); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		canonical, _, _, _, _ := av.Snapshot()
		saved, err := settings.Load()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := settings.Save(saved.WithVault(canonical)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("PUT /theme", func(w http.ResponseWriter, r *http.Request) {
		var req switchThemeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		av.SetTheme(req.Theme)

		saved, err := settings.Load()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := settings.Save(saved.WithTheme(req.Theme)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	})

	return mux
}
