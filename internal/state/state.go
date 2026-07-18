// Package state guards concurrent access to Index, SearchStore, and Graph
// from the Watcher (writes) and HTTP handlers (reads) that both need to
// touch them once the server is long-running (see ADR-0008).
package state

import (
	"sync"

	"github.com/sengkong/knowledge-server/internal/graph"
	"github.com/sengkong/knowledge-server/internal/index"
	"github.com/sengkong/knowledge-server/internal/search"
)

// State wraps Index, SearchStore, and Graph behind one mutex. The three
// engines themselves stay single-threaded/lock-free as designed; this is
// the one place concurrent access is actually introduced.
type State struct {
	mu  sync.RWMutex
	idx *index.Index
	ss  *search.SearchStore
	g   *graph.Graph

	subMu sync.Mutex
	subs  map[chan struct{}]struct{}
}

// New wraps idx, ss, and g behind a shared mutex.
func New(idx *index.Index, ss *search.SearchStore, g *graph.Graph) *State {
	return &State{idx: idx, ss: ss, g: g}
}

// Upsert re-indexes id across Index, SearchStore, and Graph.
func (s *State) Upsert(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.idx.Upsert(id); err != nil {
		return err
	}
	if err := s.ss.Upsert(id); err != nil {
		return err
	}
	if err := s.g.Upsert(id); err != nil {
		return err
	}

	s.notify()
	return nil
}

// Remove drops id from Index, SearchStore, and Graph.
func (s *State) Remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.idx.Remove(id)
	s.ss.Remove(id)
	s.g.Remove(id)

	s.notify()
}

// Save persists Index, SearchStore, and Graph to indexPath, searchPath, and
// graphPath respectively, under the same lock Upsert/Remove use — unlike
// calling each engine's own Save directly, this can't race a concurrent
// Upsert/Remove from the Watcher or a concurrent read from an HTTP handler
// (see ADR-0008; a caller outside State bypassing this lock was the bug this
// method exists to close).
func (s *State) Save(indexPath, searchPath, graphPath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.idx.Save(indexPath); err != nil {
		return err
	}
	if err := s.ss.Save(searchPath); err != nil {
		return err
	}
	return s.g.Save(graphPath)
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
	return s.idx.ByID(id)
}

// Query runs a SearchStore query.
func (s *State) Query(q string) []search.SearchResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ss.Query(q)
}

// Neighbors returns the Graph neighbors of id.
func (s *State) Neighbors(id string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.g.Neighbors(id)
}

// ByTag returns every Index entry tagged with tag.
func (s *State) ByTag(tag string) []index.IndexEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.idx.ByTag(tag)
}

// IndexAll returns every Index entry.
func (s *State) IndexAll() []index.IndexEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.idx.All()
}

// ShortestPath returns the shortest Graph path between fromID and toID.
func (s *State) ShortestPath(fromID, toID string) ([]string, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.g.ShortestPath(fromID, toID)
}

// Orphans returns every Graph node with zero edges.
func (s *State) Orphans() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.g.Orphans()
}

// GraphAll returns every Graph node.
func (s *State) GraphAll() []graph.GraphEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.g.All()
}
