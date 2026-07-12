package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Vault  VaultConfig  `yaml:"vault"`
	Server ServerConfig `yaml:"server"`
	Theme  ThemeConfig  `yaml:"theme"`
}

type VaultConfig struct {
	Path string `yaml:"path"`
}

type ServerConfig struct {
	Port int `yaml:"port"`
}

type ThemeConfig struct {
	Default string `yaml:"default"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}

	if strings.HasPrefix(cfg.Vault.Path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("expanding vault path: %w", err)
		}
		cfg.Vault.Path = filepath.Join(home, strings.TrimPrefix(cfg.Vault.Path, "~"))
	}

	return &cfg, nil
}
