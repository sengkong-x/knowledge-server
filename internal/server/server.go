package server

import (
	"encoding/json"
	"net/http"

	"github.com/sengkong/knowledge-server/internal/graph"
	"github.com/sengkong/knowledge-server/internal/index"
	"github.com/sengkong/knowledge-server/internal/search"
	"github.com/sengkong/knowledge-server/internal/vault"
)

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

func New(vaultPath string, provider vault.VaultProvider, idx *index.Index, ss *search.SearchStore, g *graph.Graph) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		notes, err := provider.ListNotes()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(healthResponse{
			VaultPath: vaultPath,
			NoteCount: len(notes),
		})
	})

	mux.HandleFunc("GET /search", func(w http.ResponseWriter, r *http.Request) {
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
			for _, m := range ss.Query(q) {
				if entry, ok := idx.ByID(m.ID); ok {
					candidates = append(candidates, entry)
					snippets[m.ID] = m.Snippet
				}
			}
		} else {
			candidates = idx.ByTag(tag)
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
		id := r.URL.Query().Get("id")
		neighbors, err := g.Neighbors(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(neighborsResponse{Neighbors: neighbors})
	})

	mux.HandleFunc("GET /graph/path", func(w http.ResponseWriter, r *http.Request) {
		from := r.URL.Query().Get("from")
		to := r.URL.Query().Get("to")
		path, found, err := g.ShortestPath(from, to)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(pathResponse{Path: path, Found: found})
	})

	mux.HandleFunc("GET /graph/orphans", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(orphansResponse{Orphans: g.Orphans()})
	})

	return mux
}
