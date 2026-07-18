package watcher

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sengkong/knowledge-server/internal/vaultfixture"
)

// spyTarget records Upsert/Remove calls on channels so tests can wait for a
// specific call without a fixed sleep.
type spyTarget struct {
	upserts chan string
	removes chan string
}

func newSpyTarget() *spyTarget {
	return &spyTarget{
		upserts: make(chan string, 10),
		removes: make(chan string, 10),
	}
}

func (s *spyTarget) Upsert(id string) error {
	s.upserts <- id
	return nil
}

func (s *spyTarget) Remove(id string) {
	s.removes <- id
}

func waitForUpsert(t *testing.T, ch chan string, want string) {
	t.Helper()
	select {
	case got := <-ch:
		if got != want {
			t.Fatalf("Upsert called with %q, want %q", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for Upsert(%q)", want)
	}
}

func drainUpserts(ch chan string) []string {
	var got []string
	for {
		select {
		case id := <-ch:
			got = append(got, id)
		default:
			return got
		}
	}
}

func TestWatcher_DebouncesRapidSavesIntoOneUpsert(t *testing.T) {
	root := t.TempDir()
	target := newSpyTarget()

	w, err := New(root, target)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer w.Close()

	if err := w.Start(); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	for i := 0; i < 5; i++ {
		vaultfixture.WriteNote(t, root, "process.md", "---\ntitle: Process\n---\nBody.\n")
	}

	waitForUpsert(t, target.upserts, "process")

	// Give any further debounced fires a chance to land before asserting
	// there weren't any more.
	time.Sleep(500 * time.Millisecond)
	if extra := drainUpserts(target.upserts); len(extra) != 0 {
		t.Fatalf("got %d extra Upsert calls after debounce settled, want 0: %v", len(extra), extra)
	}
}

func waitForRemove(t *testing.T, ch chan string, want string) {
	t.Helper()
	select {
	case got := <-ch:
		if got != want {
			t.Fatalf("Remove called with %q, want %q", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for Remove(%q)", want)
	}
}

func TestWatcher_RemovesOnNoteDelete(t *testing.T) {
	root := t.TempDir()
	target := newSpyTarget()
	vaultfixture.WriteNote(t, root, "process.md", "---\ntitle: Process\n---\nBody.\n")

	w, err := New(root, target)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer w.Close()

	if err := w.Start(); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	if err := os.Remove(filepath.Join(root, "process.md")); err != nil {
		t.Fatalf("os.Remove: %v", err)
	}

	waitForRemove(t, target.removes, "process")
}

func TestWatcher_RenameRemovesOldIDAndUpsertsNewID(t *testing.T) {
	root := t.TempDir()
	target := newSpyTarget()
	vaultfixture.WriteNote(t, root, "process.md", "---\ntitle: Process\n---\nBody.\n")

	w, err := New(root, target)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer w.Close()

	if err := w.Start(); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	if err := os.Rename(filepath.Join(root, "process.md"), filepath.Join(root, "renamed.md")); err != nil {
		t.Fatalf("os.Rename: %v", err)
	}

	waitForRemove(t, target.removes, "process")
	waitForUpsert(t, target.upserts, "renamed")
}

func TestWatcher_UpsertsOnNoteCreateInSubdirectory(t *testing.T) {
	root := t.TempDir()
	target := newSpyTarget()

	w, err := New(root, target)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer w.Close()

	if err := w.Start(); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	vaultfixture.WriteNote(t, root, "linux/process.md", "---\ntitle: Process\n---\nBody.\n")

	waitForUpsert(t, target.upserts, "linux/process")
}

func TestWatcher_IgnoresNonMarkdownFiles(t *testing.T) {
	root := t.TempDir()
	target := newSpyTarget()

	w, err := New(root, target)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer w.Close()

	if err := w.Start(); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	if err := os.WriteFile(filepath.Join(root, "diagram.png"), []byte("fake image bytes"), 0o644); err != nil {
		t.Fatalf("writing asset: %v", err)
	}
	// A real note write afterward proves the watcher is alive and simply
	// ignored the asset, rather than the test racing a slow watcher.
	vaultfixture.WriteNote(t, root, "process.md", "---\ntitle: Process\n---\nBody.\n")
	waitForUpsert(t, target.upserts, "process")

	if extra := drainUpserts(target.upserts); len(extra) != 0 {
		t.Fatalf("got Upsert calls for non-Markdown file: %v", extra)
	}
}

func TestClose_WaitsForInFlightDispatch(t *testing.T) {
	root := t.TempDir()
	target := newSpyTarget()

	w, err := New(root, target)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	if err := w.Start(); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	vaultfixture.WriteNote(t, root, "process.md", "---\ntitle: Process\n---\nBody.\n")

	// Give fsnotify a moment to actually deliver the event and register the
	// debounce timer — comfortably under debounceDelay, so the timer is
	// still pending (not yet fired) when Close runs below.
	time.Sleep(100 * time.Millisecond)

	// Close before the debounce timer would normally fire; if Close
	// doesn't wait for it, the pending Upsert never happens and target.upserts
	// stays empty.
	if err := w.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	select {
	case id := <-target.upserts:
		if id != "process" {
			t.Fatalf("Upsert called with %q, want %q", id, "process")
		}
	default:
		t.Fatal("Close returned before the pending debounced Upsert ran")
	}
}

func TestWatcher_UpsertsOnNoteCreate(t *testing.T) {
	root := t.TempDir()
	target := newSpyTarget()

	w, err := New(root, target)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer w.Close()

	if err := w.Start(); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	vaultfixture.WriteNote(t, root, "process.md", "---\ntitle: Process\n---\nBody.\n")

	waitForUpsert(t, target.upserts, "process")
}
