package index

import (
	"bufio"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"time"

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
		return errors.New("index: cache format version mismatch")
	}
	return nil
}

// IndexEntry is a single Index record: a Note's metadata without its Body.
type IndexEntry struct {
	ID      string
	Title   string
	Tags    []string
	Path    string
	Created time.Time
}

// HasTag reports whether tag is among the entry's Tags.
func (e IndexEntry) HasTag(tag string) bool {
	return slices.Contains(e.Tags, tag)
}

// Index is a disposable, rebuildable projection of Vault metadata.
type Index struct {
	entries  map[string]IndexEntry
	provider vault.VaultProvider
	store    notes.NoteStore
}

func entryFromNote(note *parser.Note, path string) IndexEntry {
	return IndexEntry{
		ID:      note.ID,
		Title:   note.Title,
		Tags:    note.Tags,
		Path:    path,
		Created: note.Created,
	}
}

// ByID returns the entry for id, if indexed.
func (idx *Index) ByID(id string) (IndexEntry, bool) {
	entry, ok := idx.entries[id]
	return entry, ok
}

// Upsert re-indexes a single note, adding or replacing its entry.
func (idx *Index) Upsert(id string) error {
	note, err := idx.store.Load(id)
	if err != nil {
		return err
	}

	path, err := vault.ResolvePath(idx.entries[id].Path, idx.provider, id)
	if err != nil {
		return err
	}

	idx.entries[id] = entryFromNote(note, path)
	return nil
}

// Save persists the Index to path using gob encoding (see ADR-0004).
func (idx *Index) Save(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := writeCacheHeader(f); err != nil {
		return err
	}
	return gob.NewEncoder(f).Encode(idx.entries)
}

// Load reads an Index previously written by Save.
func Load(path string, provider vault.VaultProvider, store notes.NoteStore) (*Index, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := bufio.NewReader(f)
	if err := readCacheHeader(r); err != nil {
		return nil, err
	}

	entries := make(map[string]IndexEntry)
	if err := gob.NewDecoder(r).Decode(&entries); err != nil {
		return nil, err
	}

	return &Index{entries: entries, provider: provider, store: store}, nil
}

// Remove drops the entry for id, if present.
func (idx *Index) Remove(id string) {
	delete(idx.entries, id)
}

// All returns every entry in the Index.
func (idx *Index) All() []IndexEntry {
	entries := make([]IndexEntry, 0, len(idx.entries))
	for _, entry := range idx.entries {
		entries = append(entries, entry)
	}
	return entries
}

// ByTag returns every entry that has tag among its Tags.
func (idx *Index) ByTag(tag string) []IndexEntry {
	var matches []IndexEntry
	for _, entry := range idx.entries {
		if entry.HasTag(tag) {
			matches = append(matches, entry)
		}
	}
	return matches
}

// LoadOrBuild loads the Index from path if present, falling back to a full
// Build from the Vault if the cache is missing or fails to decode (see
// ADR-0010).
func LoadOrBuild(path string, provider vault.VaultProvider, store notes.NoteStore) (*Index, BuildReport, error) {
	if idx, err := Load(path, provider, store); err == nil {
		return idx, BuildReport{Failed: make(map[string]error)}, nil
	}
	return Build(provider, store)
}

// BuildReport records notes that failed to parse during a Build.
type BuildReport struct {
	Failed map[string]error
}

// Build scans every note in the Vault (via provider and store) and returns
// an Index of the ones that parse successfully. Notes that fail to parse
// are skipped and recorded in the returned BuildReport.
func Build(provider vault.VaultProvider, store notes.NoteStore) (*Index, BuildReport, error) {
	refs, err := provider.ListNotes()
	if err != nil {
		return nil, BuildReport{}, err
	}

	idx := &Index{entries: make(map[string]IndexEntry, len(refs)), provider: provider, store: store}
	report := BuildReport{Failed: make(map[string]error)}
	for _, ref := range refs {
		note, err := store.Load(ref.ID)
		if err != nil {
			report.Failed[ref.ID] = err
			continue
		}
		idx.entries[ref.ID] = entryFromNote(note, ref.Path)
	}

	return idx, report, nil
}
