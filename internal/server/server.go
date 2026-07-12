package server

import (
	"encoding/json"
	"net/http"

	"github.com/sengkong/knowledge-server/internal/vault"
)

type healthResponse struct {
	VaultPath string `json:"vault_path"`
	NoteCount int    `json:"note_count"`
}

func New(vaultPath string, provider vault.VaultProvider) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		notes, err := provider.ListNotes()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(healthResponse{
			VaultPath: vaultPath,
			NoteCount: len(notes),
		})
	})

	return mux
}
