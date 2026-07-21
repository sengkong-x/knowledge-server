// Package activevault owns the single Vault a running instance currently
// has open — the Active Vault glossary entry in CONTEXT.md — and the
// switch orchestration described in ADR-0011 and the 2026-07-19 update to
// ADR-0010: validate the new path, save and discard the outgoing vault's
// Engines, instantly serve the incoming vault's cached Engines if present
// (else build from scratch), and kick off a background staleness
// reconciliation over the existing SSE plumbing.
package activevault

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/sengkong/knowledge-server/internal/engines"
	"github.com/sengkong/knowledge-server/internal/logger"
	"github.com/sengkong/knowledge-server/internal/notes"
	"github.com/sengkong/knowledge-server/internal/state"
	"github.com/sengkong/knowledge-server/internal/vault"
	"github.com/sengkong/knowledge-server/internal/watcher"
)

// ActiveVault holds the one Vault subsystem (provider/store/state/watcher)
// currently live, plus the global theme. Exactly one vault's Engines are
// resident at a time — visiting a different vault discards the outgoing
// one's in-memory Engines (after saving them), rather than keeping every
// visited vault resident, which would grow memory unbounded over a session.
type ActiveVault struct {
	mu sync.RWMutex

	path       string // canonical path, "" if none selected
	provider   vault.VaultProvider
	store      notes.NoteStore
	state      *state.State
	watcher    *watcher.Watcher
	cachePaths engines.Paths

	theme string

	log *slog.Logger

	// reconcileMu guards reconcileCh, which the test suite (same package)
	// uses to wait for a Switch's background reconciliation to finish,
	// rather than sleeping or racing an assertion against it.
	reconcileMu sync.Mutex
	reconcileCh chan struct{}

	// switchSubMu guards switchSubs: subscribers (e.g. server.go's /events
	// SSE handler) notified whenever Switch swaps the active vault. This is
	// how an in-flight SSE connection subscribed to a now-discarded State
	// finds out its subscription is stale — see server.go's /events comment
	// for the full rationale.
	switchSubMu sync.Mutex
	switchSubs  map[chan struct{}]struct{}
}

// New constructs an ActiveVault with no vault selected yet.
func New(theme string) *ActiveVault {
	av := &ActiveVault{theme: theme, log: logger.New()}
	av.reconcileCh = closedChan()
	return av
}

func closedChan() chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

// reconcileDone returns a channel that closes when the most recently
// started reconciliation (if any) finishes. Exported only within the
// package — production callers don't need it; Switch's caller (Ticket 06)
// doesn't wait on reconciliation, it just gets the instant cache-or-build
// result.
func (av *ActiveVault) reconcileDone() <-chan struct{} {
	av.reconcileMu.Lock()
	defer av.reconcileMu.Unlock()
	return av.reconcileCh
}

// Snapshot returns the fields request handlers need to serve a request
// against the currently active vault, copied under a read lock so a
// concurrent Switch can't hand back a half-updated view. ok is false if no
// vault is currently selected.
func (av *ActiveVault) Snapshot() (path string, provider vault.VaultProvider, store notes.NoteStore, s *state.State, ok bool) {
	av.mu.RLock()
	defer av.mu.RUnlock()
	return av.path, av.provider, av.store, av.state, av.path != ""
}

// Theme returns the current global theme.
func (av *ActiveVault) Theme() string {
	av.mu.RLock()
	defer av.mu.RUnlock()
	return av.theme
}

// SetTheme sets the global theme. Independent of any vault selection.
func (av *ActiveVault) SetTheme(theme string) {
	av.mu.Lock()
	defer av.mu.Unlock()
	av.theme = theme
}

// SubscribeSwitch registers for notifications delivered whenever Switch
// swaps the active vault. Call the returned unsubscribe func to stop
// receiving them and release the channel.
func (av *ActiveVault) SubscribeSwitch() (ch <-chan struct{}, unsubscribe func()) {
	c := make(chan struct{}, 1)

	av.switchSubMu.Lock()
	if av.switchSubs == nil {
		av.switchSubs = make(map[chan struct{}]struct{})
	}
	av.switchSubs[c] = struct{}{}
	av.switchSubMu.Unlock()

	return c, func() {
		av.switchSubMu.Lock()
		delete(av.switchSubs, c)
		av.switchSubMu.Unlock()
	}
}

// notifySwitch pings every switch subscriber. Sends are non-blocking
// (buffered size 1, dropped if already pending), mirroring state.State's
// own notify — a subscriber only needs to know "a switch happened," and a
// dropped duplicate ping is harmless since the subscriber's response
// (server.go closes the SSE stream) is idempotent.
func (av *ActiveVault) notifySwitch() {
	av.switchSubMu.Lock()
	defer av.switchSubMu.Unlock()

	for c := range av.switchSubs {
		select {
		case c <- struct{}{}:
		default:
		}
	}
}

// Shutdown persists the currently active vault's Engines (if any) and
// closes its Watcher, for a graceful process shutdown — the same Save
// semantics as the outgoing-vault side of Switch, just triggered by process
// exit instead of a new vault being selected (see the 2026-07-19 update to
// ADR-0010).
func (av *ActiveVault) Shutdown() error {
	av.mu.Lock()
	defer av.mu.Unlock()

	if av.path == "" {
		return nil
	}

	saveErr := av.state.Save(av.cachePaths)
	closeErr := av.watcher.Close()
	return errors.Join(saveErr, closeErr)
}

// Switch validates newPath, saves and discards the outgoing vault's
// Engines (if any), loads-or-builds the incoming vault's Engines from its
// ~/.cache/ks/<hash>/ cache dir, swaps in the new provider/store/state/
// watcher, and kicks off a background staleness reconciliation. newPath
// must pass vault.ValidateRoot before anything about the outgoing vault is
// touched — a bad path must never tear down a perfectly good vault.
//
// Deliberately builds the entire incoming subsystem (provider, store,
// LoadOrBuild, watcher.Start) before taking av.mu, rather than holding the
// write lock across that whole (potentially slow) sequence: this keeps
// concurrent Snapshot() reads unblocked for longer, and as a side effect
// means a failure partway through building the incoming vault (a LoadOrBuild
// or watcher error) also leaves the outgoing vault completely untouched, not
// just a failed ValidateRoot. The two watchers briefly coexist (incoming
// started, outgoing not yet closed) only for the few instructions between
// building the incoming one and taking the lock — harmless, since each
// watches a disjoint filesystem root.
func (av *ActiveVault) Switch(newPath string) error {
	canonical, err := vault.CanonicalPath(newPath)
	if err != nil {
		return fmt.Errorf("canonicalizing vault path: %w", err)
	}
	if err := vault.ValidateRoot(canonical); err != nil {
		return fmt.Errorf("invalid vault path: %w", err)
	}

	cacheDir, err := vaultCacheDir(canonical)
	if err != nil {
		return fmt.Errorf("resolving cache dir: %w", err)
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return fmt.Errorf("creating cache dir %s: %w", cacheDir, err)
	}
	cachePaths := engines.Paths{
		Index:  filepath.Join(cacheDir, "index.gob"),
		Search: filepath.Join(cacheDir, "search.gob"),
		Graph:  filepath.Join(cacheDir, "graph.gob"),
	}

	provider := vault.NewLocalVaultProvider(canonical)
	store := notes.NewVaultNoteStore(provider)

	e, report, err := engines.LoadOrBuild(cachePaths, provider, store)
	if err != nil {
		return fmt.Errorf("loading or building engines for %s: %w", canonical, err)
	}

	newState := state.New(e)
	w, err := watcher.New(canonical, newState)
	if err != nil {
		return fmt.Errorf("creating watcher for %s: %w", canonical, err)
	}
	if err := w.Start(); err != nil {
		return fmt.Errorf("starting watcher for %s: %w", canonical, err)
	}

	av.mu.Lock()
	if av.path != "" {
		// Best-effort: a failed save of the outgoing (disposable, per
		// ADR-0004) cache shouldn't block switching to a validated new
		// vault.
		if err := av.state.Save(av.cachePaths); err != nil {
			av.log.Warn("saving outgoing vault cache", "vault", av.path, "error", err)
		}
		if err := av.watcher.Close(); err != nil {
			av.log.Warn("closing outgoing vault watcher", "vault", av.path, "error", err)
		}
	}

	av.path = canonical
	av.provider = provider
	av.store = store
	av.state = newState
	av.watcher = w
	av.cachePaths = cachePaths
	av.mu.Unlock()

	av.notifySwitch()
	av.logBuildReport(canonical, report)

	reconcileCh := make(chan struct{})
	av.reconcileMu.Lock()
	av.reconcileCh = reconcileCh
	av.reconcileMu.Unlock()
	go av.reconcile(provider, newState, reconcileCh)

	return nil
}

// RemoveVault deletes path's on-disk Engines cache. If path is the
// currently active vault, the active state is cleared entirely (closing its
// watcher first) rather than falling back to another vault — the caller
// (server.go's DELETE /vault handler) is responsible for also dropping path
// from settings.VaultHistory. Returns the canonicalized path so the caller
// doesn't need to canonicalize it a second time.
//
// path must already be canonical — the only caller passes entries read
// back out of settings.VaultHistory, which are canonicalized once by
// WithVault at the time they're added (see settings.Settings.WithVault).
// RemoveVault relies on that: if path can no longer be resolved (e.g. its
// directory was deleted or moved), it falls back to using path as-is
// rather than failing, so a vault that no longer exists on disk can still
// be cleared from history and cache. That fallback trusts path only as
// far as it can verify without filesystem access — see the abs/clean
// check below — so passing an arbitrary non-canonical path (relative,
// unclean, or one that was simply never canonicalized) still fails rather
// than silently corrupting the active-vault comparison or cache-dir
// cleanup.
func (av *ActiveVault) RemoveVault(path string) (string, error) {
	canonical, err := vault.CanonicalPath(path)
	if err != nil {
		// EvalSymlinks (inside CanonicalPath) requires the path to exist,
		// so it fails here whenever the vault's directory has already been
		// deleted — exactly the case this fallback exists for. Filesystem
		// resolution is unavailable at that point, but every truly
		// canonical path is by construction already absolute and clean
		// (filepath.Abs + filepath.EvalSymlinks never produce anything
		// else), so requiring that much still catches a caller passing a
		// relative or unclean path through this branch instead of
		// silently trusting it outright.
		abs, absErr := filepath.Abs(path)
		if absErr != nil || abs != path {
			return "", fmt.Errorf("canonicalizing vault path: %w", err)
		}
		canonical = path
	}

	av.mu.Lock()
	wasActive := av.path == canonical
	if wasActive {
		if err := av.watcher.Close(); err != nil {
			av.log.Warn("closing removed vault's watcher", "vault", canonical, "error", err)
		}
		// No state.Save here (unlike Switch's outgoing-vault teardown and
		// Shutdown): the cache dir this would persist to is deleted a few
		// lines below, so saving first would just be immediately discarded
		// work.
		av.path = ""
		av.provider = nil
		av.store = nil
		av.state = nil
		av.watcher = nil
		av.cachePaths = engines.Paths{}
	}
	av.mu.Unlock()

	if wasActive {
		// Mirrors Switch's own notifySwitch call: an /events subscriber
		// (server.go) still thinks its State is live and needs telling
		// otherwise, exactly as when Switch swaps in a different vault.
		av.notifySwitch()
	}

	cacheDir, err := vaultCacheDir(canonical)
	if err != nil {
		return "", fmt.Errorf("resolving cache dir: %w", err)
	}
	if err := os.RemoveAll(cacheDir); err != nil {
		return "", fmt.Errorf("removing cache dir %s: %w", cacheDir, err)
	}
	return canonical, nil
}

func (av *ActiveVault) logBuildReport(vaultPath string, report engines.BuildReport) {
	for id, err := range report.Index.Failed {
		av.log.Warn("note failed to index", "vault", vaultPath, "id", id, "error", err)
	}
	for id, err := range report.Search.Failed {
		av.log.Warn("note failed to index for search", "vault", vaultPath, "id", id, "error", err)
	}
	for id, err := range report.Graph.Failed {
		av.log.Warn("note failed to graph", "vault", vaultPath, "id", id, "error", err)
	}
}

// reconcile rescans the vault for notes added or removed since s's Index
// was loaded (whether from cache or a fresh Build) and drives Upsert/Remove
// through s for the difference — a cheap check-first pass, not a full
// rebuild, so a cache hit still avoids re-parsing every note. s already
// pings SSE subscribers on every Upsert/Remove (see internal/state), so
// this reuses that plumbing with no server-side changes needed.
//
// Deliberate limitation: this only catches notes added or removed while the
// vault was inactive, by diffing IDs — it does not detect a note whose ID
// is unchanged but whose content was edited in place during that window
// (that would need per-file mtime tracking, which none of IndexEntry,
// vault.VaultProvider, or notes.NoteStore carry today, and adding it would
// mean reopening the already-settled cache-entry shape from Ticket 01/02).
// A content-only edit made while a vault wasn't the Active Vault surfaces
// once the Watcher notices it live, the next time that vault becomes active
// and gets re-watched — not before.
func (av *ActiveVault) reconcile(provider vault.VaultProvider, s *state.State, done chan struct{}) {
	defer close(done)

	refs, err := provider.ListNotes()
	if err != nil {
		av.log.Warn("reconcile: listing notes", "error", err)
		return
	}

	onDisk := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		onDisk[ref.ID] = struct{}{}
	}

	indexed := make(map[string]struct{})
	for _, entry := range s.IndexAll() {
		indexed[entry.ID] = struct{}{}
	}

	for id := range onDisk {
		if _, ok := indexed[id]; !ok {
			if err := s.Upsert(id); err != nil {
				av.log.Warn("reconcile: upserting note", "id", id, "error", err)
			}
		}
	}
	for id := range indexed {
		if _, ok := onDisk[id]; !ok {
			s.Remove(id)
		}
	}
}

// vaultCacheDir returns ~/.cache/ks/<hash>/ for the given canonical vault
// path, where <hash> is vault.CacheKey(canonical) (see ADR-0011).
func vaultCacheDir(canonical string) (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", errors.New("resolving user cache dir: " + err.Error())
	}
	return filepath.Join(base, "ks", vault.CacheKey(canonical)), nil
}
