package activevault

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sengkong/knowledge-server/internal/vault"
	"github.com/sengkong/knowledge-server/internal/vaultfixture"
)

func setCacheHome(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
}

const noteA = `---
title: Note A
created: 2026-07-12
---
Body A.
`

const noteB = `---
title: Note B
created: 2026-07-12
---
Body B.
`

// waitReconciled blocks until av's most recent Switch's background
// reconciliation has finished, so tests can assert post-reconcile state
// deterministically instead of racing a goroutine.
func waitReconciled(t *testing.T, av *ActiveVault) {
	t.Helper()
	select {
	case <-av.reconcileDone():
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for reconcile to finish")
	}
}

func TestSwitch_ToValidVaultSucceedsAndSnapshotReflectsIt(t *testing.T) {
	setCacheHome(t)
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "note-a.md", noteA)

	av := New("light")

	if err := av.Switch(root); err != nil {
		t.Fatalf("Switch returned error: %v", err)
	}
	waitReconciled(t, av)

	path, _, _, s, ok := av.Snapshot()
	if !ok {
		t.Fatal("Snapshot ok = false, want true after a successful Switch")
	}
	if path == "" {
		t.Error("Snapshot path is empty, want the canonicalized vault root")
	}
	if _, found := s.ByID("note-a"); !found {
		t.Error(`ByID("note-a") not found, want the switched-in vault's notes indexed`)
	}
}

func TestSwitch_ToInvalidPathLeavesOutgoingVaultUntouched(t *testing.T) {
	setCacheHome(t)
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "note-a.md", noteA)

	av := New("light")
	if err := av.Switch(root); err != nil {
		t.Fatalf("initial Switch returned error: %v", err)
	}
	waitReconciled(t, av)

	badPath := filepath.Join(t.TempDir(), "does-not-exist")
	if err := av.Switch(badPath); err == nil {
		t.Fatal("Switch to a nonexistent path returned no error, want error")
	}

	path, _, _, s, ok := av.Snapshot()
	if !ok {
		t.Fatal("Snapshot ok = false after a failed Switch, want the previous vault to remain active")
	}
	if _, found := s.ByID("note-a"); !found {
		t.Error(`ByID("note-a") not found after a failed Switch, want the previous vault untouched`)
	}
	_ = path
}

func TestRemoveVault_DeletesCacheDirForAnInactiveVaultWithoutTouchingTheActiveOne(t *testing.T) {
	setCacheHome(t)
	active := t.TempDir()
	vaultfixture.WriteNote(t, active, "note-a.md", noteA)
	other := t.TempDir()
	vaultfixture.WriteNote(t, other, "note-b.md", noteB)

	av := New("light")
	if err := av.Switch(active); err != nil {
		t.Fatalf("Switch(active) returned error: %v", err)
	}
	waitReconciled(t, av)
	if err := av.Switch(other); err != nil {
		t.Fatalf("Switch(other) returned error: %v", err)
	}
	waitReconciled(t, av)
	if err := av.Switch(active); err != nil {
		t.Fatalf("Switch(active) again returned error: %v", err)
	}
	waitReconciled(t, av)

	otherCacheDir, err := vaultCacheDir(mustCanonical(t, other))
	if err != nil {
		t.Fatalf("vaultCacheDir(other): %v", err)
	}
	if _, err := os.Stat(otherCacheDir); err != nil {
		t.Fatalf("other's cache dir does not exist before removal: %v", err)
	}

	if _, err := av.RemoveVault(other); err != nil {
		t.Fatalf("RemoveVault(other) returned error: %v", err)
	}

	if _, err := os.Stat(otherCacheDir); !os.IsNotExist(err) {
		t.Errorf("other's cache dir still exists after RemoveVault, want it deleted (stat err = %v)", err)
	}

	path, _, _, s, ok := av.Snapshot()
	if !ok {
		t.Fatal("Snapshot ok = false after removing an inactive vault, want the active vault untouched")
	}
	if path == "" {
		t.Error("Snapshot path is empty, want the still-active vault's path")
	}
	if _, found := s.ByID("note-a"); !found {
		t.Error(`ByID("note-a") not found, want the active vault's notes still indexed`)
	}
}

func TestRemoveVault_RemovingTheActiveVaultClearsActiveState(t *testing.T) {
	setCacheHome(t)
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "note-a.md", noteA)

	av := New("light")
	if err := av.Switch(root); err != nil {
		t.Fatalf("Switch returned error: %v", err)
	}
	waitReconciled(t, av)

	cacheDir, err := vaultCacheDir(mustCanonical(t, root))
	if err != nil {
		t.Fatalf("vaultCacheDir: %v", err)
	}

	if _, err := av.RemoveVault(root); err != nil {
		t.Fatalf("RemoveVault returned error: %v", err)
	}

	if _, _, _, _, ok := av.Snapshot(); ok {
		t.Error("Snapshot ok = true after removing the active vault, want no vault selected")
	}
	if _, err := os.Stat(cacheDir); !os.IsNotExist(err) {
		t.Errorf("cache dir still exists after removing the active vault (stat err = %v)", err)
	}
}

// This is the exact scenario RemoveVault's fallback exists for: the vault
// directory is gone by the time removal is requested, so CanonicalPath can
// no longer resolve it via EvalSymlinks. The path passed in must still be
// usable for removal — see the precondition documented on RemoveVault.
func TestRemoveVault_RemovingTheActiveVaultAfterItsDirectoryWasDeletedStillClearsState(t *testing.T) {
	setCacheHome(t)
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "note-a.md", noteA)

	av := New("light")
	if err := av.Switch(root); err != nil {
		t.Fatalf("Switch returned error: %v", err)
	}
	waitReconciled(t, av)

	canonical := mustCanonical(t, root)
	cacheDir, err := vaultCacheDir(canonical)
	if err != nil {
		t.Fatalf("vaultCacheDir: %v", err)
	}

	if err := os.RemoveAll(root); err != nil {
		t.Fatalf("removing vault directory: %v", err)
	}

	if _, err := av.RemoveVault(canonical); err != nil {
		t.Fatalf("RemoveVault returned error: %v", err)
	}

	if _, _, _, _, ok := av.Snapshot(); ok {
		t.Error("Snapshot ok = true after removing the active vault, want no vault selected")
	}
	if _, err := os.Stat(cacheDir); !os.IsNotExist(err) {
		t.Errorf("cache dir still exists after removing the active vault (stat err = %v)", err)
	}
}

// RemoveVault's fallback for an unresolvable path only trusts it as far
// as it can verify without filesystem access (absolute and clean); a
// relative path — which no successful CanonicalPath call could ever have
// produced — must still be rejected rather than silently accepted.
func TestRemoveVault_RejectsANonCanonicalPathEvenWhenUnresolvable(t *testing.T) {
	setCacheHome(t)
	av := New("light")

	if _, err := av.RemoveVault("relative/does-not-exist"); err == nil {
		t.Fatal("RemoveVault with a relative, nonexistent path returned no error, want error")
	}
}

func mustCanonical(t *testing.T, path string) string {
	t.Helper()
	canonical, err := vault.CanonicalPath(path)
	if err != nil {
		t.Fatalf("vault.CanonicalPath(%q): %v", path, err)
	}
	return canonical
}

func TestSwitch_AToBToA_ReusesCachedEnginesForA(t *testing.T) {
	setCacheHome(t)
	rootA := t.TempDir()
	vaultfixture.WriteNote(t, rootA, "note-a.md", noteA)
	rootB := t.TempDir()
	vaultfixture.WriteNote(t, rootB, "note-b.md", noteB)

	av := New("light")

	if err := av.Switch(rootA); err != nil {
		t.Fatalf("Switch(A) returned error: %v", err)
	}
	waitReconciled(t, av)

	if err := av.Switch(rootB); err != nil {
		t.Fatalf("Switch(B) returned error: %v", err)
	}
	waitReconciled(t, av)

	// Add a new note to A's vault on disk after leaving it. If switching
	// back to A rebuilds from scratch, this note would be picked up
	// immediately. If it instead loads A's persisted cache (written when we
	// switched away from it above), the new note is absent until the
	// background reconcile eventually catches it — so we assert on the
	// state immediately after Switch returns, before waiting on reconcile.
	vaultfixture.WriteNote(t, rootA, "note-a2.md", `---
title: Note A2
created: 2026-07-13
---
Body A2.
`)

	if err := av.Switch(rootA); err != nil {
		t.Fatalf("Switch(A) again returned error: %v", err)
	}

	_, _, _, s, ok := av.Snapshot()
	if !ok {
		t.Fatal("Snapshot ok = false, want true")
	}
	if _, found := s.ByID("note-a2"); found {
		t.Error(`ByID("note-a2") found immediately after Switch, want the cached (pre-addition) Index to have been loaded instead of a fresh Build`)
	}
	if _, found := s.ByID("note-a"); !found {
		t.Error(`ByID("note-a") not found, want the cached entry for A present`)
	}

	waitReconciled(t, av)
	if _, found := s.ByID("note-a2"); !found {
		t.Error(`ByID("note-a2") not found after reconcile, want the background rescan to have caught the new note`)
	}
}

func TestNew_NoVaultSelected_SnapshotReturnsNotOK(t *testing.T) {
	av := New("light")

	_, _, _, _, ok := av.Snapshot()
	if ok {
		t.Error("Snapshot ok = true with no vault selected, want false")
	}
}

func TestThemeDefaultsAndCanBeSet(t *testing.T) {
	av := New("dark")

	if got := av.Theme(); got != "dark" {
		t.Errorf("Theme() = %q, want %q", got, "dark")
	}

	av.SetTheme("light")
	if got := av.Theme(); got != "light" {
		t.Errorf("Theme() after SetTheme = %q, want %q", got, "light")
	}
}

func TestSnapshot_ConcurrentWithSwitch_NoRace(t *testing.T) {
	setCacheHome(t)
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "note-a.md", noteA)

	av := New("light")
	if err := av.Switch(root); err != nil {
		t.Fatalf("initial Switch returned error: %v", err)
	}
	waitReconciled(t, av)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 50 {
			av.Snapshot()
		}
	}()

	root2 := t.TempDir()
	vaultfixture.WriteNote(t, root2, "note-b.md", noteB)
	if err := av.Switch(root2); err != nil {
		t.Fatalf("concurrent Switch returned error: %v", err)
	}
	<-done
	waitReconciled(t, av)
}

func TestSwitch_LogsIndexBuildFailuresWithoutErroring(t *testing.T) {
	setCacheHome(t)
	root := t.TempDir()
	vaultfixture.WriteNote(t, root, "note-a.md", noteA)
	if err := os.WriteFile(filepath.Join(root, "broken.md"), []byte("no frontmatter"), 0o644); err != nil {
		t.Fatalf("writing broken note: %v", err)
	}

	av := New("light")
	if err := av.Switch(root); err != nil {
		t.Fatalf("Switch returned error: %v", err)
	}
	waitReconciled(t, av)
}
