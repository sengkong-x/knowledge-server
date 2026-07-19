package search

import (
	"bufio"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/sengkong/knowledge-server/internal/notes"
	"github.com/sengkong/knowledge-server/internal/parser"
	"github.com/sengkong/knowledge-server/internal/vault"
)

// cacheFormatVersion identifies the on-disk shape of the gob-encoded
// entries map. Bump on any change to that shape; a mismatch on Load is
// treated as a cache miss (fall back to Build), never a decode
// panic/silently-wrong data — see ADR-0004 and ADR-0010.
const cacheFormatVersion = 1

var cacheHeader = fmt.Sprintf("KSC%d\n", cacheFormatVersion)

func writeCacheHeader(w io.Writer) error {
	_, err := io.WriteString(w, cacheHeader)
	return err
}

func readCacheHeader(r *bufio.Reader) error {
	line, err := r.ReadString('\n')
	if err != nil {
		return err
	}
	if line != cacheHeader {
		return errors.New("search: cache format version mismatch")
	}
	return nil
}

// SearchResult is a single Query match.
type SearchResult struct {
	ID      string
	Title   string
	Snippet string
}

type searchEntry struct {
	ID         string
	Title      string
	Path       string
	TitleLower string
	Text       string
	TextLower  string
}

// snippetContextChars is how many characters of surrounding text are kept on
// each side of a match when building a Snippet.
const snippetContextChars = 20

// snippet returns a window of text around the first case-insensitive
// occurrence of q, with an ellipsis where the window was truncated.
func snippet(text, textLower, q string) string {
	idx := strings.Index(textLower, q)
	if idx < 0 {
		return ""
	}

	rawStart := idx - snippetContextChars
	rawEnd := idx + len(q) + snippetContextChars
	start := max(rawStart, 0)
	end := min(rawEnd, len(text))
	truncatedStart := rawStart > 0
	truncatedEnd := rawEnd < len(text)

	var b strings.Builder
	if truncatedStart {
		b.WriteString("…")
	}
	b.WriteString(text[start:end])
	if truncatedEnd {
		b.WriteString("…")
	}
	return b.String()
}

func entryFromNote(note *parser.Note, path string) searchEntry {
	text := note.Title + "\n" + note.Body + "\n" + strings.Join(note.Aliases, "\n")
	return searchEntry{
		ID:         note.ID,
		Title:      note.Title,
		Path:       path,
		TitleLower: strings.ToLower(note.Title),
		Text:       text,
		TextLower:  strings.ToLower(text),
	}
}

// SearchStore is a disposable, rebuildable structure holding normalized
// text per Note, for substring search (see ADR-0005).
type SearchStore struct {
	entries  map[string]searchEntry
	provider vault.VaultProvider
	store    notes.NoteStore
}

// LoadOrBuild loads the SearchStore from path if present, falling back to a
// full Build from the Vault if the cache is missing or fails to decode (see
// ADR-0010).
func LoadOrBuild(path string, provider vault.VaultProvider, store notes.NoteStore) (*SearchStore, BuildReport, error) {
	if ss, err := Load(path, provider, store); err == nil {
		return ss, BuildReport{Failed: make(map[string]error)}, nil
	}
	return Build(provider, store)
}

// BuildReport records notes that failed to parse during a Build.
type BuildReport struct {
	Failed map[string]error
}

// Build scans every note in the Vault (via provider and store) and returns
// a SearchStore of the ones that parse successfully.
func Build(provider vault.VaultProvider, store notes.NoteStore) (*SearchStore, BuildReport, error) {
	refs, err := provider.ListNotes()
	if err != nil {
		return nil, BuildReport{}, err
	}

	ss := &SearchStore{entries: make(map[string]searchEntry, len(refs)), provider: provider, store: store}
	report := BuildReport{Failed: make(map[string]error)}
	for _, ref := range refs {
		note, err := store.Load(ref.ID)
		if err != nil {
			report.Failed[ref.ID] = err
			continue
		}
		ss.entries[ref.ID] = entryFromNote(note, ref.Path)
	}

	return ss, report, nil
}

// Upsert re-indexes a single note, adding or replacing its entry.
func (ss *SearchStore) Upsert(id string) error {
	note, err := ss.store.Load(id)
	if err != nil {
		return err
	}

	path, err := vault.ResolvePath(ss.entries[id].Path, ss.provider, id)
	if err != nil {
		return err
	}

	ss.entries[id] = entryFromNote(note, path)
	return nil
}

// Remove drops the entry for id, if present.
func (ss *SearchStore) Remove(id string) {
	delete(ss.entries, id)
}

// Save persists the SearchStore to path using gob encoding, mirroring
// Index (see ADR-0004).
func (ss *SearchStore) Save(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := writeCacheHeader(f); err != nil {
		return err
	}
	return gob.NewEncoder(f).Encode(ss.entries)
}

// Load reads a SearchStore previously written by Save.
func Load(path string, provider vault.VaultProvider, store notes.NoteStore) (*SearchStore, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := bufio.NewReader(f)
	if err := readCacheHeader(r); err != nil {
		return nil, err
	}

	entries := make(map[string]searchEntry)
	if err := gob.NewDecoder(r).Decode(&entries); err != nil {
		return nil, err
	}

	return &SearchStore{entries: entries, provider: provider, store: store}, nil
}

// Query returns every entry whose title, body, or aliases contain q as a
// substring (case-insensitive, including mid-word matches).
func (ss *SearchStore) Query(q string) []SearchResult {
	q = strings.ToLower(q)

	type scored struct {
		result     SearchResult
		titleMatch bool
	}
	var matches []scored
	for _, e := range ss.entries {
		if strings.Contains(e.TextLower, q) {
			matches = append(matches, scored{
				result:     SearchResult{ID: e.ID, Title: e.Title, Snippet: snippet(e.Text, e.TextLower, q)},
				titleMatch: strings.Contains(e.TitleLower, q),
			})
		}
	}

	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].titleMatch != matches[j].titleMatch {
			return matches[i].titleMatch
		}
		return matches[i].result.Title < matches[j].result.Title
	})

	results := make([]SearchResult, len(matches))
	for i, m := range matches {
		results[i] = m.result
	}
	return results
}
