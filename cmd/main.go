package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/sengkong/knowledge-server/internal/config"
	"github.com/sengkong/knowledge-server/internal/engines"
	"github.com/sengkong/knowledge-server/internal/logger"
	"github.com/sengkong/knowledge-server/internal/notes"
	"github.com/sengkong/knowledge-server/internal/server"
	"github.com/sengkong/knowledge-server/internal/state"
	"github.com/sengkong/knowledge-server/internal/vault"
	"github.com/sengkong/knowledge-server/internal/watcher"
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

	// Deliberately outside the Vault root: the Watcher recursively watches
	// the whole Vault, and a cache dir living inside it would both get
	// scanned as content and get its own writes watched as if they were
	// Vault edits.
	cacheDir := filepath.Join(filepath.Dir(*configPath), ".cache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		log.Error("creating cache dir", "path", cacheDir, "error", err)
		os.Exit(1)
	}
	cachePaths := engines.Paths{
		Index:  filepath.Join(cacheDir, "index.gob"),
		Search: filepath.Join(cacheDir, "search.gob"),
		Graph:  filepath.Join(cacheDir, "graph.gob"),
	}

	e, report, err := engines.LoadOrBuild(cachePaths, provider, store)
	if err != nil {
		log.Error("loading or building engines", "error", err)
		os.Exit(1)
	}
	for id, parseErr := range report.Index.Failed {
		log.Warn("note failed to index", "id", id, "error", parseErr)
	}
	for id, parseErr := range report.Search.Failed {
		log.Warn("note failed to index for search", "id", id, "error", parseErr)
	}
	for id, buildErr := range report.Graph.Failed {
		log.Warn("note failed to graph", "id", id, "error", buildErr)
	}

	knowledge := state.New(e)

	w, err := watcher.New(cfg.Vault.Path, knowledge)
	if err != nil {
		log.Error("creating watcher", "error", err)
		os.Exit(1)
	}
	if err := w.Start(); err != nil {
		log.Error("starting watcher", "error", err)
		os.Exit(1)
	}

	handler := server.New(cfg.Vault.Path, provider, store, knowledge, cfg.Theme.Default)

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	httpServer := &http.Server{Addr: addr, Handler: handler}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Info("starting server", "addr", addr, "vault", cfg.Vault.Path)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server exited", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")

	if err := w.Close(); err != nil {
		log.Warn("closing watcher", "error", err)
	}
	if err := knowledge.Save(cachePaths); err != nil {
		log.Warn("saving cache", "error", err)
	}

	if err := httpServer.Shutdown(context.Background()); err != nil {
		log.Warn("server shutdown", "error", err)
	}
}
