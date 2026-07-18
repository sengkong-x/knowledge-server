// Package graph builds an undirected relationship graph over Notes from
// their `related` frontmatter field (see ADR-0006).
package graph

import (
	"encoding/gob"
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/sengkong/knowledge-server/internal/notes"
	"github.com/sengkong/knowledge-server/internal/parser"
	"github.com/sengkong/knowledge-server/internal/vault"
)

// GraphEntry is a single Graph node: a Note's ID and its resolved set of
// neighbor IDs.
type GraphEntry struct {
	ID        string
	Neighbors map[string]struct{}
}

// Graph is a disposable, rebuildable undirected relationship graph over
// Notes.
type Graph struct {
	entries  map[string]GraphEntry
	provider vault.VaultProvider
	store    notes.NoteStore
}

// BuildReport records notes whose related references could not be resolved
// during a Build.
type BuildReport struct {
	Failed map[string]error
}

// Neighbors returns the direct (1-hop) neighbor IDs of id, sorted.
func (g *Graph) Neighbors(id string) ([]string, error) {
	entry, ok := g.entries[id]
	if !ok {
		return nil, fmt.Errorf("unknown note %q", id)
	}
	return sortedKeys(entry.Neighbors), nil
}

// ShortestPath returns the shortest unweighted path between fromID and toID
// (inclusive of both endpoints), found via BFS. found is false if fromID and
// toID are known nodes with no path connecting them. Returns an error if
// either ID is not a known node.
func (g *Graph) ShortestPath(fromID, toID string) ([]string, bool, error) {
	if _, ok := g.entries[fromID]; !ok {
		return nil, false, fmt.Errorf("unknown note %q", fromID)
	}
	if _, ok := g.entries[toID]; !ok {
		return nil, false, fmt.Errorf("unknown note %q", toID)
	}

	prev := map[string]string{fromID: ""}
	queue := []string{fromID}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for _, n := range sortedKeys(g.entries[current].Neighbors) {
			if _, seen := prev[n]; seen {
				continue
			}
			prev[n] = current
			if n == toID {
				return buildPath(prev, toID), true, nil
			}
			queue = append(queue, n)
		}
	}

	return []string{}, false, nil
}

func buildPath(prev map[string]string, toID string) []string {
	var path []string
	for at := toID; at != ""; at = prev[at] {
		path = append([]string{at}, path...)
	}
	return path
}

func sortedKeys(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Orphans returns every note ID with zero edges, sorted.
func (g *Graph) Orphans() []string {
	var orphans []string
	for id, entry := range g.entries {
		if len(entry.Neighbors) == 0 {
			orphans = append(orphans, id)
		}
	}
	sort.Strings(orphans)
	return orphans
}

// Upsert re-parses id and replaces its outgoing edges. Since edges are
// undirected/shared records (ADR-0006), the other side of each edge updates
// as a side effect — no cross-note recomputation is needed.
func (g *Graph) Upsert(id string) error {
	note, err := g.store.Load(id)
	if err != nil {
		return err
	}

	g.disconnect(id)
	g.entries[id] = GraphEntry{ID: id, Neighbors: make(map[string]struct{})}

	for _, rel := range note.Related {
		if rel == id {
			continue
		}
		if _, ok := g.entries[rel]; !ok {
			continue
		}
		addEdge(g.entries, id, rel)
		addEdge(g.entries, rel, id)
	}

	return nil
}

// Remove drops id and all edges connected to it, if present.
func (g *Graph) Remove(id string) {
	g.disconnect(id)
	delete(g.entries, id)
}

// disconnect removes every edge touching id, without removing id itself.
func (g *Graph) disconnect(id string) {
	for n := range g.entries[id].Neighbors {
		delete(g.entries[n].Neighbors, id)
	}
}

func addEdge(entries map[string]GraphEntry, a, b string) {
	entries[a].Neighbors[b] = struct{}{}
}

// graphData is the gob-encoded on-disk form of a Graph (see ADR-0004).
type graphData struct {
	Entries map[string]GraphEntry
}

// Save persists the Graph to path using gob encoding (see ADR-0004).
func (g *Graph) Save(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	return gob.NewEncoder(f).Encode(graphData{Entries: g.entries})
}

// Load reads a Graph previously written by Save.
func Load(path string, provider vault.VaultProvider, store notes.NoteStore) (*Graph, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var data graphData
	if err := gob.NewDecoder(f).Decode(&data); err != nil {
		return nil, err
	}

	return &Graph{entries: data.Entries, provider: provider, store: store}, nil
}

// Build scans every note in the Vault (via provider and store) and returns a
// Graph of undirected edges declared by each note's related field. Notes
// that fail to parse are skipped and recorded in the returned BuildReport,
// the same as related references that don't resolve to a parsed note.
func Build(provider vault.VaultProvider, store notes.NoteStore) (*Graph, BuildReport, error) {
	refs, err := provider.ListNotes()
	if err != nil {
		return nil, BuildReport{}, err
	}

	parsed := make(map[string]*parser.Note, len(refs))
	report := BuildReport{Failed: make(map[string]error)}
	for _, ref := range refs {
		note, err := store.Load(ref.ID)
		if err != nil {
			report.Failed[ref.ID] = err
			continue
		}
		parsed[ref.ID] = note
	}

	entries := make(map[string]GraphEntry, len(parsed))
	for id := range parsed {
		entries[id] = GraphEntry{ID: id, Neighbors: make(map[string]struct{})}
	}

	for id, note := range parsed {
		var danglingErrs []error
		for _, rel := range note.Related {
			if rel == id {
				continue
			}
			if _, ok := parsed[rel]; !ok {
				danglingErrs = append(danglingErrs, fmt.Errorf("related reference %q does not exist in the vault", rel))
				continue
			}
			addEdge(entries, id, rel)
			addEdge(entries, rel, id)
		}
		if len(danglingErrs) > 0 {
			report.Failed[id] = errors.Join(danglingErrs...)
		}
	}

	return &Graph{entries: entries, provider: provider, store: store}, report, nil
}
