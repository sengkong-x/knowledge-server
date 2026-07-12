package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/sengkong/knowledge-server/internal/config"
	"github.com/sengkong/knowledge-server/internal/logger"
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
	handler := server.New(cfg.Vault.Path, provider)

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Info("starting server", "addr", addr, "vault", cfg.Vault.Path)

	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Error("server exited", "error", err)
		os.Exit(1)
	}
}
