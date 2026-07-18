// Package engines owns Index, SearchStore, and Graph as a single unit:
// constructing them together from the Vault, fanning writes out across all
// three, and persisting all three. It does not add concurrency safety — see
// internal/state for that.
package engines

import (
	"errors"

	"github.com/sengkong/knowledge-server/internal/graph"
	"github.com/sengkong/knowledge-server/internal/index"
	"github.com/sengkong/knowledge-server/internal/notes"
	"github.com/sengkong/knowledge-server/internal/search"
	"github.com/sengkong/knowledge-server/internal/vault"
)

// Paths locates the on-disk gob cache for each engine (see ADR-0004,
// ADR-0010).
type Paths struct {
	Index  string
	Search string
	Graph  string
}

// BuildReport aggregates each engine's own BuildReport, one field per
// engine, so callers can still log per-engine failures with per-engine
// messages.
type BuildReport struct {
	Index  index.BuildReport
	Search search.BuildReport
	Graph  graph.BuildReport
}

// Engines holds Index, SearchStore, and Graph together.
type Engines struct {
	idx *index.Index
	ss  *search.SearchStore
	g   *graph.Graph
}

// Index returns the underlying Index, for reads. Engines' own interface
// (UpsertAll/RemoveAll/SaveAll) only covers the three engines moving
// together as a unit; reads were never the duplicated concern this package
// exists to fix, so callers read through these accessors rather than
// Engines growing its own copy of Index/SearchStore/Graph's read methods.
func (e *Engines) Index() *index.Index { return e.idx }

// Search returns the underlying SearchStore, for reads (see Index).
func (e *Engines) Search() *search.SearchStore { return e.ss }

// Graph returns the underlying Graph, for reads (see Index).
func (e *Engines) Graph() *graph.Graph { return e.g }

// LoadOrBuild loads each engine from its path in paths if present, falling
// back to a full Build from the Vault for any engine whose cache is missing
// or fails to decode (see ADR-0010).
func LoadOrBuild(paths Paths, provider vault.VaultProvider, store notes.NoteStore) (*Engines, BuildReport, error) {
	idx, idxReport, err := index.LoadOrBuild(paths.Index, provider, store)
	if err != nil {
		return nil, BuildReport{}, err
	}

	ss, ssReport, err := search.LoadOrBuild(paths.Search, provider, store)
	if err != nil {
		return nil, BuildReport{}, err
	}

	g, gReport, err := graph.LoadOrBuild(paths.Graph, provider, store)
	if err != nil {
		return nil, BuildReport{}, err
	}

	return &Engines{idx: idx, ss: ss, g: g}, BuildReport{Index: idxReport, Search: ssReport, Graph: gReport}, nil
}

// Build scans the Vault and builds all three engines from scratch, without
// touching any on-disk cache.
func Build(provider vault.VaultProvider, store notes.NoteStore) (*Engines, BuildReport, error) {
	idx, idxReport, err := index.Build(provider, store)
	if err != nil {
		return nil, BuildReport{}, err
	}

	ss, ssReport, err := search.Build(provider, store)
	if err != nil {
		return nil, BuildReport{}, err
	}

	g, gReport, err := graph.Build(provider, store)
	if err != nil {
		return nil, BuildReport{}, err
	}

	return &Engines{idx: idx, ss: ss, g: g}, BuildReport{Index: idxReport, Search: ssReport, Graph: gReport}, nil
}

// UpsertAll re-indexes id across Index, SearchStore, and Graph. It attempts
// all three regardless of earlier failures and returns their errors joined,
// so a failure in one engine doesn't leave the others out of date for id.
func (e *Engines) UpsertAll(id string) error {
	idxErr := e.idx.Upsert(id)
	ssErr := e.ss.Upsert(id)
	gErr := e.g.Upsert(id)
	return errors.Join(idxErr, ssErr, gErr)
}

// RemoveAll drops id from Index, SearchStore, and Graph. None of the three
// can fail to remove an id.
func (e *Engines) RemoveAll(id string) {
	e.idx.Remove(id)
	e.ss.Remove(id)
	e.g.Remove(id)
}

// SaveAll persists Index, SearchStore, and Graph to their paths. It attempts
// all three regardless of earlier failures and returns their errors joined,
// so a failure saving one engine doesn't stop the others from persisting.
func (e *Engines) SaveAll(paths Paths) error {
	idxErr := e.idx.Save(paths.Index)
	ssErr := e.ss.Save(paths.Search)
	gErr := e.g.Save(paths.Graph)
	return errors.Join(idxErr, ssErr, gErr)
}
