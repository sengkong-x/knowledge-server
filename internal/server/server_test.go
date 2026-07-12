package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sengkong/knowledge-server/internal/vault"
)

type fakeVaultProvider struct {
	notes []vault.NoteRef
}

func (f *fakeVaultProvider) ListNotes() ([]vault.NoteRef, error) { return f.notes, nil }
func (f *fakeVaultProvider) ReadNote(id string) ([]byte, error)  { return nil, nil }
func (f *fakeVaultProvider) ReadAsset(path string) ([]byte, error) { return nil, nil }

func TestHealth_ReturnsOKWithVaultPathAndNoteCount(t *testing.T) {
	provider := &fakeVaultProvider{notes: []vault.NoteRef{
		{ID: "linux/process", Path: "linux/process.md"},
		{ID: "database/wal", Path: "database/wal.md"},
	}}

	handler := New("/srv/knowledge", provider)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	wantBody := `{"vault_path":"/srv/knowledge","note_count":2}`
	if rec.Body.String() != wantBody+"\n" && rec.Body.String() != wantBody {
		t.Errorf("body = %q, want %q", rec.Body.String(), wantBody)
	}
}
