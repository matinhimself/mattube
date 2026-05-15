package config

import (
	"encoding/json"
	"fmt"
	"os"
)

const Path = "/etc/mattube/config.json"

type Config struct {
	// Fronting
	FrontingIP string `json:"fronting_ip"`
	AllowedSNI string `json:"allowed_sni"`

	// Drive
	DriveFolderID    string `json:"drive_folder_id"`
	DriveAccessToken string `json:"drive_access_token"`

	// YouTube
	YouTubeAPIKey string `json:"youtube_api_key"`

	// HTTP
	HTTPAddr string `json:"http_addr"`

	// DB
	DBPath string `json:"db_path"`

	// Bootstrap admin (used only when users table is empty)
	AdminUsername string `json:"admin_username"`
	AdminPassword string `json:"admin_password"`
}

func Load() (*Config, error) {
	data, err := os.ReadFile(Path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", Path, err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", Path, err)
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = ":8080"
	}
	if cfg.DBPath == "" {
		cfg.DBPath = "/var/lib/mattube/mattube-client.db"
	}
	return &cfg, nil
}
