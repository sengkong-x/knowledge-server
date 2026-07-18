// Package state guards concurrent access to the Engines triad from the
// Watcher (writes) and HTTP handlers (reads) that both need to touch them
// once the server is long-running (see ADR-0008).
package state

import (
	"sync"

	"github.com/sengkong/knowledge-server/internal/engines"
	"github.com/sengkong/knowledge-server/internal/graph"
	"github.com/sengkong/knowledge-server/internal/index"
	"github.com/sengkong/knowledge-server/internal/search"
)

// State wraps Engines behind one mutex. Engines itself stays
// single-threaded/lock-free as designed; this is the one place concurrent
// access is actually introduced.
type State struct {
	mu sync.RWMutex
	e  *engines.Engines

	subMu sync.Mutex
	subs  map[chan struct{}]struct{}
}

// New wraps e behind a shared mutex.
func New(e *engines.Engines) *State {
	return &State{e: e}
}

// Upsert re-indexes id across Index, SearchStore, and Graph.
func (s *State) Upsert(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.e.UpsertAll(id); err != nil {
		return err
	}

	s.notify()
	return nil
}

// Remove drops id from Index, SearchStore, and Graph.
func (s *State) Remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.e.RemoveAll(id)

	s.notify()
}

// Save persists Index, SearchStore, and Graph to paths, under the same lock
// Upsert/Remove use — unlike calling each engine's own Save directly, this
// can't race a concurrent Upsert/Remove from the Watcher or a concurrent
// read from an HTTP handler (see ADR-0008; a caller outside State bypassing
// this lock was the bug this method exists to close).
func (s *State) Save(paths engines.Paths) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.e.SaveAll(paths)
}

// Subscribe registers for change notifications, delivered on every
// successful Upsert or Remove (a generic ping, no per-note payload — see
// ADR-0009). Call the returned unsubscribe func to stop receiving them and
// release the channel.
func (s *State) Subscribe() (ch <-chan struct{}, unsubscribe func()) {
	c := make(chan struct{}, 1)

	s.subMu.Lock()
	if s.subs == nil {
		s.subs = make(map[chan struct{}]struct{})
	}
	s.subs[c] = struct{}{}
	s.subMu.Unlock()

	return c, func() {
		s.subMu.Lock()
		delete(s.subs, c)
		s.subMu.Unlock()
	}
}

// notify pings every subscriber. Sends are non-blocking (buffered size 1,
// dropped if already pending) since subscribers only need to know "something
// changed," not receive every individual change (ADR-0009).
func (s *State) notify() {
	s.subMu.Lock()
	defer s.subMu.Unlock()

	for c := range s.subs {
		select {
		case c <- struct{}{}:
		default:
		}
	}
}

// ByID returns the Index entry for id, if indexed.
func (s *State) ByID(id string) (index.IndexEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.e.Index().ByID(id)
}

// Query runs a SearchStore query.
func (s *State) Query(q string) []search.SearchResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.e.Search().Query(q)
}

// Neighbors returns the Graph neighbors of id.
func (s *State) Neighbors(id string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.e.Graph().Neighbors(id)
}

// ByTag returns every Index entry tagged with tag.
func (s *State) ByTag(tag string) []index.IndexEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.e.Index().ByTag(tag)
}

// IndexAll returns every Index entry.
func (s *State) IndexAll() []index.IndexEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.e.Index().All()
}

// ShortestPath returns the shortest Graph path between fromID and toID.
func (s *State) ShortestPath(fromID, toID string) ([]string, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.e.Graph().ShortestPath(fromID, toID)
}

// Orphans returns every Graph node with zero edges.
func (s *State) Orphans() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.e.Graph().Orphans()
}

// GraphAll returns every Graph node.
func (s *State) GraphAll() []graph.GraphEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.e.Graph().All()
}
