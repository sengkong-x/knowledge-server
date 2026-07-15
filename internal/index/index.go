package index

import (
	"encoding/gob"
	"os"
	"slices"
	"time"

	"github.com/sengkong/knowledge-server/internal/notes"
	"github.com/sengkong/knowledge-server/internal/parser"
	"github.com/sengkong/knowledge-server/internal/vault"
)

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

	return gob.NewEncoder(f).Encode(idx.entries)
}

// Load reads an Index previously written by Save.
func Load(path string, provider vault.VaultProvider, store notes.NoteStore) (*Index, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	entries := make(map[string]IndexEntry)
	if err := gob.NewDecoder(f).Decode(&entries); err != nil {
		return nil, err
	}

	return &Index{entries: entries, provider: provider, store: store}, nil
}

// Remove drops the entry for id, if present.
func (idx *Index) Remove(id string) {
	delete(idx.entries, id)
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
