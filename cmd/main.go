package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/sengkong/knowledge-server/internal/activevault"
	"github.com/sengkong/knowledge-server/internal/logger"
	"github.com/sengkong/knowledge-server/internal/server"
	"github.com/sengkong/knowledge-server/internal/settings"
)

func main() {
	port := flag.Int("port", 8080, "HTTP port to listen on")
	flag.Parse()

	log := logger.New()

	s, err := settings.Load()
	if err != nil {
		log.Error("loading settings", "error", err)
		os.Exit(1)
	}

	av := activevault.New(s.Theme)
	if s.VaultPath != "" {
		if err := av.Switch(s.VaultPath); err != nil {
			// A stale settings.json entry (e.g. the vault directory was
			// deleted since last run) shouldn't crash the whole server —
			// boot with no Active Vault and let the picker UI select one.
			log.Warn("re-opening last vault from settings", "vault", s.VaultPath, "error", err)
		}
	}

	handler := server.New(av)

	addr := fmt.Sprintf(":%d", *port)
	httpServer := &http.Server{Addr: addr, Handler: handler}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Info("starting server", "addr", addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server exited", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")

	if err := av.Shutdown(); err != nil {
		log.Warn("shutting down active vault", "error", err)
	}

	if err := httpServer.Shutdown(context.Background()); err != nil {
		log.Warn("server shutdown", "error", err)
	}
}
