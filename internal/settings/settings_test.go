package settings

import (
	"path/filepath"
	"reflect"
	"testing"
)

func setConfigHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	return dir
}

func TestPath_ResolvesUnderConfigHome(t *testing.T) {
	dir := setConfigHome(t)

	got, err := Path()
	if err != nil {
		t.Fatalf("Path() returned error: %v", err)
	}

	want := filepath.Join(dir, "ks", "settings.json")
	if got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}

func TestLoad_MissingFileReturnsZeroValueAndNoError(t *testing.T) {
	setConfigHome(t)

	got, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error for a missing file: %v", err)
	}

	want := Settings{}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Load() = %+v, want zero-value %+v", got, want)
	}
}

func TestSaveLoad_RoundTripsExactly(t *testing.T) {
	setConfigHome(t)

	s := Settings{
		VaultPath:    "/home/user/vault",
		Theme:        "dark",
		VaultHistory: []string{"/home/user/vault", "/home/user/other"},
	}

	if err := Save(s); err != nil {
		t.Fatalf("Save() returned error: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if got.VaultPath != s.VaultPath {
		t.Errorf("VaultPath = %q, want %q", got.VaultPath, s.VaultPath)
	}
	if got.Theme != s.Theme {
		t.Errorf("Theme = %q, want %q", got.Theme, s.Theme)
	}
	if len(got.VaultHistory) != len(s.VaultHistory) {
		t.Fatalf("VaultHistory = %v, want %v", got.VaultHistory, s.VaultHistory)
	}
	for i := range s.VaultHistory {
		if got.VaultHistory[i] != s.VaultHistory[i] {
			t.Errorf("VaultHistory[%d] = %q, want %q", i, got.VaultHistory[i], s.VaultHistory[i])
		}
	}
}

func TestSave_WritesViaTempFileThenRename(t *testing.T) {
	dir := setConfigHome(t)

	if err := Save(Settings{VaultPath: "/vault"}); err != nil {
		t.Fatalf("Save() returned error: %v", err)
	}

	entries, err := filepath.Glob(filepath.Join(dir, "ks", "*"))
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}

	// Only the final settings.json should remain — no leftover temp file,
	// confirming the write-then-rename completed cleanly.
	if len(entries) != 1 || filepath.Base(entries[0]) != "settings.json" {
		t.Errorf("directory contents after Save = %v, want exactly [settings.json]", entries)
	}
}

func TestWithVault_PushesToFrontOfHistory(t *testing.T) {
	s := Settings{VaultHistory: []string{"/a", "/b"}}

	got := s.WithVault("/c")

	if got.VaultPath != "/c" {
		t.Errorf("VaultPath = %q, want %q", got.VaultPath, "/c")
	}
	want := []string{"/c", "/a", "/b"}
	if len(got.VaultHistory) != len(want) {
		t.Fatalf("VaultHistory = %v, want %v", got.VaultHistory, want)
	}
	for i := range want {
		if got.VaultHistory[i] != want[i] {
			t.Errorf("VaultHistory[%d] = %q, want %q", i, got.VaultHistory[i], want[i])
		}
	}
}

func TestWithVault_DedupesAlreadyPresentPathMovingItToFront(t *testing.T) {
	s := Settings{VaultHistory: []string{"/a", "/b", "/c"}}

	got := s.WithVault("/b")

	want := []string{"/b", "/a", "/c"}
	if len(got.VaultHistory) != len(want) {
		t.Fatalf("VaultHistory = %v, want %v", got.VaultHistory, want)
	}
	for i := range want {
		if got.VaultHistory[i] != want[i] {
			t.Errorf("VaultHistory[%d] = %q, want %q", i, got.VaultHistory[i], want[i])
		}
	}
}

func TestWithVault_CapsHistoryLength(t *testing.T) {
	s := Settings{}
	for i := range 20 {
		s = s.WithVault(filepath.Join("/vault", string(rune('a'+i))))
	}

	if len(s.VaultHistory) > 10 {
		t.Errorf("VaultHistory length = %d, want capped at 10", len(s.VaultHistory))
	}
}

func TestWithTheme_SetsTheme(t *testing.T) {
	s := Settings{}

	got := s.WithTheme("dark")

	if got.Theme != "dark" {
		t.Errorf("Theme = %q, want %q", got.Theme, "dark")
	}
}
