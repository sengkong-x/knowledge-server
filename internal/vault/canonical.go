package vault

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// cacheKeyLength is the number of hex characters kept from the SHA-256
// digest — plenty of collision resistance for a directory name, without
// the noise of the full 64-character digest.
const cacheKeyLength = 32

// CanonicalPath expands a leading "~" (bare "~" only — "~user" is not
// supported), makes the result absolute, and resolves symlinks, so the
// same physical vault always canonicalizes identically regardless of how
// its path was written.
//
// The path must exist for symlink resolution to succeed; a nonexistent
// path is treated as an error here rather than deferred, since a cache
// key can't be reproducibly derived from a path that doesn't yet resolve
// to anything on disk. Full semantic validation (e.g. "is this a
// directory") is a separate concern handled by ValidateRoot at
// vault-switch time.
func CanonicalPath(path string) (string, error) {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("expanding vault path: %w", err)
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~"))
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolving absolute path for %s: %w", path, err)
	}

	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("resolving symlinks for %s: %w", abs, err)
	}

	return resolved, nil
}

// CacheKey returns a reproducible identifier for a canonicalized vault
// path, suitable for use as a directory name under ~/.cache/ks/. Pure
// function, no I/O — callers must pass an already-canonicalized path
// (see CanonicalPath) for the "same vault, same key" property to hold.
func CacheKey(canonicalPath string) string {
	sum := sha256.Sum256([]byte(canonicalPath))
	return hex.EncodeToString(sum[:])[:cacheKeyLength]
}
