// Package watcher monitors the Vault for filesystem changes and drives
// incremental updates into Index, SearchStore, and Graph (see CONTEXT.md's
// Watcher entry and ADR-0008).
package watcher

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// debounceDelay coalesces bursts of events on the same path (e.g. an
// editor's write-then-rename save) into a single dispatch.
const debounceDelay = 250 * time.Millisecond

// Target is anything that can be incrementally updated from a changed note
// file. *index.Index, *search.SearchStore, and *graph.Graph all satisfy it.
type Target interface {
	Upsert(id string) error
	Remove(id string)
}

// Watcher watches root for Markdown file changes and dispatches Upsert/Remove
// calls to every target.
type Watcher struct {
	root    string
	targets []Target
	fsw     *fsnotify.Watcher

	mu      sync.Mutex
	timers  map[string]*time.Timer
	pending map[string]fsnotify.Op

	wg      sync.WaitGroup
	runDone chan struct{}
}

// New creates a Watcher over root. Call Start to begin watching.
func New(root string, targets ...Target) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	return &Watcher{
		root:    root,
		targets: targets,
		fsw:     fsw,
		timers:  make(map[string]*time.Timer),
		pending: make(map[string]fsnotify.Op),
		runDone: make(chan struct{}),
	}, nil
}

// Start begins watching root (recursively) and dispatching changes in the
// background.
func (w *Watcher) Start() error {
	if err := w.watchDir(w.root, false); err != nil {
		return err
	}

	go w.run()
	return nil
}

// watchDir adds dir and every subdirectory beneath it to the underlying
// fsnotify watch list. When a directory is discovered after Start (a
// subdirectory created while already running), fsnotify.Add can race with
// files written into it before the watch was established, so synthesize
// dispatches for any Markdown files already present.
func (w *Watcher) watchDir(dir string, synthesize bool) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return w.fsw.Add(path)
		}
		if synthesize && filepath.Ext(path) == ".md" {
			w.debounce(path, fsnotify.Create)
		}
		return nil
	})
}

// Close stops the Watcher and waits for any in-flight debounced dispatch to
// finish, so a caller that Saves state right after Close (see ADR-0010) sees
// every change the Watcher had already accepted. It waits for run() to fully
// exit before waiting on the dispatch WaitGroup — run() is the only caller
// that adds to it, so this ordering guarantees no Add can race the Wait
// below (sync.WaitGroup forbids a concurrent Add(positive) and Wait when the
// counter may be zero).
func (w *Watcher) Close() error {
	err := w.fsw.Close()
	<-w.runDone
	w.wg.Wait()
	return err
}

func (w *Watcher) run() {
	defer close(w.runDone)

	for event := range w.fsw.Events {
		if event.Op.Has(fsnotify.Create) {
			if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
				w.watchDir(event.Name, true)
				continue
			}
		}

		if filepath.Ext(event.Name) != ".md" {
			continue
		}

		w.debounce(event.Name, event.Op)
	}
}

// debounce records the latest operation seen for path and (re)starts a timer
// that dispatches it after debounceDelay, coalescing any events that arrive
// before the timer fires.
func (w *Watcher) debounce(path string, op fsnotify.Op) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.pending[path] = op
	if timer, ok := w.timers[path]; ok {
		if timer.Stop() {
			// Successfully canceled before it fired, so its dispatch will
			// never run and never call wg.Done() itself.
			w.wg.Done()
		}
		// If Stop returns false, the timer already fired (or is about to);
		// its own dispatch goroutine owns that wg slot and will call Done.
	}

	w.wg.Add(1)
	w.timers[path] = time.AfterFunc(debounceDelay, func() {
		defer w.wg.Done()
		w.dispatch(path)
	})
}

func (w *Watcher) dispatch(path string) {
	w.mu.Lock()
	op := w.pending[path]
	delete(w.pending, path)
	delete(w.timers, path)
	w.mu.Unlock()

	id := w.idFor(path)
	switch {
	case op.Has(fsnotify.Create) || op.Has(fsnotify.Write):
		for _, target := range w.targets {
			target.Upsert(id)
		}
	case op.Has(fsnotify.Remove) || op.Has(fsnotify.Rename):
		for _, target := range w.targets {
			target.Remove(id)
		}
	}
}

func (w *Watcher) idFor(path string) string {
	rel, err := filepath.Rel(w.root, path)
	if err != nil {
		rel = path
	}
	return strings.TrimSuffix(filepath.ToSlash(rel), ".md")
}
