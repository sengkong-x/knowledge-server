package vault

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCanonicalPath_ExpandsTildeToAbsolutePath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir: %v", err)
	}

	got, err := CanonicalPath("~")
	if err != nil {
		t.Fatalf("CanonicalPath(\"~\") returned error: %v", err)
	}

	want, err := filepath.EvalSymlinks(home)
	if err != nil {
		t.Fatalf("filepath.EvalSymlinks(%q): %v", home, err)
	}

	if got != want {
		t.Errorf("CanonicalPath(\"~\") = %q, want %q", got, want)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("CanonicalPath(\"~\") = %q, want absolute path", got)
	}
}

func TestCanonicalPath_SameDirectoryViaSymlinkProducesSameCacheKey(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real-vault")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatalf("creating real vault dir: %v", err)
	}

	link := filepath.Join(root, "link-vault")
	if err := os.Symlink(real, link); err != nil {
		t.Fatalf("creating symlink: %v", err)
	}

	canonReal, err := CanonicalPath(real)
	if err != nil {
		t.Fatalf("CanonicalPath(real): %v", err)
	}
	canonLink, err := CanonicalPath(link)
	if err != nil {
		t.Fatalf("CanonicalPath(link): %v", err)
	}

	if canonReal != canonLink {
		t.Fatalf("canonical paths differ: real=%q link=%q, want equal", canonReal, canonLink)
	}

	if CacheKey(canonReal) != CacheKey(canonLink) {
		t.Errorf("CacheKey differs for the same physical vault reached two ways: %q vs %q", CacheKey(canonReal), CacheKey(canonLink))
	}
}

func TestCacheKey_DifferentDirectoriesProduceDifferentKeys(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "vault-a")
	b := filepath.Join(root, "vault-b")
	if err := os.Mkdir(a, 0o755); err != nil {
		t.Fatalf("creating vault-a: %v", err)
	}
	if err := os.Mkdir(b, 0o755); err != nil {
		t.Fatalf("creating vault-b: %v", err)
	}

	canonA, err := CanonicalPath(a)
	if err != nil {
		t.Fatalf("CanonicalPath(a): %v", err)
	}
	canonB, err := CanonicalPath(b)
	if err != nil {
		t.Fatalf("CanonicalPath(b): %v", err)
	}

	if CacheKey(canonA) == CacheKey(canonB) {
		t.Errorf("CacheKey collided for two different directories: %q", CacheKey(canonA))
	}
}

func TestCanonicalPath_ErrorsWhenPathDoesNotExist(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "does-not-exist")

	if _, err := CanonicalPath(missing); err == nil {
		t.Fatal("CanonicalPath returned no error for a nonexistent path, want error (existence/validation deferred to ValidateRoot at switch time is documented, not silently swallowed here)")
	}
}

func TestCacheKey_IsDeterministicAndReasonableLength(t *testing.T) {
	key1 := CacheKey("/some/canonical/path")
	key2 := CacheKey("/some/canonical/path")

	if key1 != key2 {
		t.Errorf("CacheKey is not deterministic: %q vs %q", key1, key2)
	}
	if len(key1) < 16 || len(key1) > 32 {
		t.Errorf("CacheKey length = %d, want between 16 and 32 hex chars", len(key1))
	}
}
