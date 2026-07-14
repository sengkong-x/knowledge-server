package notes

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/sengkong/knowledge-server/internal/parser"
	"github.com/sengkong/knowledge-server/internal/vault"
)

// ErrNotFound is returned by Load when no note exists for the given ID.
var ErrNotFound = errors.New("note not found")

// NoteStore returns parsed notes, built on top of a VaultProvider.
type NoteStore interface {
	List() ([]parser.Note, error)
	Load(id string) (*parser.Note, error)
}

type vaultNoteStore struct {
	provider vault.VaultProvider
}

// NewVaultNoteStore builds a NoteStore backed by the given VaultProvider.
func NewVaultNoteStore(provider vault.VaultProvider) NoteStore {
	return &vaultNoteStore{provider: provider}
}

func (s *vaultNoteStore) List() ([]parser.Note, error) {
	refs, err := s.provider.ListNotes()
	if err != nil {
		return nil, err
	}

	result := make([]parser.Note, 0, len(refs))
	for _, ref := range refs {
		note, err := s.Load(ref.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, *note)
	}

	return result, nil
}

func (s *vaultNoteStore) Load(id string) (*parser.Note, error) {
	raw, err := s.provider.ReadNote(id)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
		}
		return nil, err
	}

	return parser.Parse(id, raw)
}
