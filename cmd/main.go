package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/sengkong/knowledge-server/internal/config"
	"github.com/sengkong/knowledge-server/internal/graph"
	"github.com/sengkong/knowledge-server/internal/index"
	"github.com/sengkong/knowledge-server/internal/logger"
	"github.com/sengkong/knowledge-server/internal/notes"
	"github.com/sengkong/knowledge-server/internal/search"
	"github.com/sengkong/knowledge-server/internal/server"
	"github.com/sengkong/knowledge-server/internal/vault"
)

func main() {
	configPath := flag.String("config", "./config.yaml", "path to config.yaml")
	flag.Parse()

	log := logger.New()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Error("loading config", "error", err)
		os.Exit(1)
	}

	if err := vault.ValidateRoot(cfg.Vault.Path); err != nil {
		log.Error("invalid vault path", "path", cfg.Vault.Path, "error", err)
		os.Exit(1)
	}

	provider := vault.NewLocalVaultProvider(cfg.Vault.Path)
	store := notes.NewVaultNoteStore(provider)

	idx, idxReport, err := index.Build(provider, store)
	if err != nil {
		log.Error("building index", "error", err)
		os.Exit(1)
	}
	for id, parseErr := range idxReport.Failed {
		log.Warn("note failed to index", "id", id, "error", parseErr)
	}

	ss, ssReport, err := search.Build(provider, store)
	if err != nil {
		log.Error("building search store", "error", err)
		os.Exit(1)
	}
	for id, parseErr := range ssReport.Failed {
		log.Warn("note failed to index for search", "id", id, "error", parseErr)
	}

	g, gReport, err := graph.Build(provider, store)
	if err != nil {
		log.Error("building graph", "error", err)
		os.Exit(1)
	}
	for id, buildErr := range gReport.Failed {
		log.Warn("note failed to graph", "id", id, "error", buildErr)
	}

	handler := server.New(cfg.Vault.Path, provider, idx, ss, g)

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Info("starting server", "addr", addr, "vault", cfg.Vault.Path)

	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Error("server exited", "error", err)
		os.Exit(1)
	}
}
