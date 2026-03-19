package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	RegistryPath   string `json:"registry_path"`
	ProxyContainer string `json:"proxy_container"`
	DBContainer    string `json:"db_container"`
	IncusSocket    string `json:"incus_socket"`
	Version        string `json:"-"`
}

func DefaultConfig() *Config {
	return &Config{
		RegistryPath:   "/etc/freedb/registry.json",
		ProxyContainer: "proxy1",
		DBContainer:    "db1",
		IncusSocket:    "",
	}
}

func Load() (*Config, error) {
	cfg := DefaultConfig()

	paths := []string{
		"/etc/freedb/config.json",
		filepath.Join(os.Getenv("HOME"), ".config", "freedb", "config.json"),
	}

	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, err
		}
		return cfg, nil
	}

	return cfg, nil
}
