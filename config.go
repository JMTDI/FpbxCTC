package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config holds all user-configurable settings.
type Config struct {
	Domain      string `json:"domain"`
	APIKey      string `json:"api_key"`
	AgentNumber string `json:"agent_number"`
}

func configDir() string {
	return filepath.Join(os.Getenv("APPDATA"), "FpbxCTC")
}

func configPath() string {
	return filepath.Join(configDir(), "config.json")
}

// LoadConfig reads config from disk. Returns an empty Config if the file does not exist yet.
func LoadConfig() (*Config, error) {
	f, err := os.Open(configPath())
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, err
	}
	defer f.Close()

	var cfg Config
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// SaveConfig writes config to disk, creating the directory if needed.
func SaveConfig(cfg *Config) error {
	if err := os.MkdirAll(configDir(), 0700); err != nil {
		return err
	}
	f, err := os.Create(configPath())
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(cfg)
}
