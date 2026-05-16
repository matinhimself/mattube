package config

import (
	"encoding/json"
	"fmt"
	"os"
)

const DefaultPath = "/etc/mattube/config.json"

type Config struct {
	// Fronting
	FrontingIP string `json:"fronting_ip"`
	AllowedSNI string `json:"allowed_sni"`

	// Drive
	DriveFolderID    string `json:"drive_folder_id"`
	DriveAccessToken string `json:"drive_access_token"`
	DriveCredsFile   string `json:"drive_creds_file"`
	DriveTokenFile   string `json:"drive_token_file"`

	// YouTube
	YouTubeAPIKey string `json:"youtube_api_key"`

	// HTTP
	HTTPAddr string `json:"http_addr"`

	// DB
	DBPath string `json:"db_path"`

	// Bootstrap admin (used only when users table is empty)
	AdminUsername string `json:"admin_username"`
	AdminPassword string `json:"admin_password"`

	// LocalMode disables authentication entirely — useful for single-user local installs.
	LocalMode bool `json:"local_mode"`

	// JobPollInterval is how often the background ticker refreshes job statuses from Drive.
	// Format: Go duration string, e.g. "2m", "90s". Default: "2m".
	JobPollInterval string `json:"job_poll_interval"`

	// JobTimeout is how long a job can stay in pending/processing before being marked failed
	// and its Drive files deleted. Format: Go duration string, e.g. "2h". Default: "2h".
	JobTimeout string `json:"job_timeout"`
}

func Load(path string) (*Config, error) {
	if path == "" {
		path = DefaultPath
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = ":8080"
	}
	if cfg.DBPath == "" {
		cfg.DBPath = "/var/lib/mattube/mattube-client.db"
	}
	if cfg.DriveCredsFile == "" {
		cfg.DriveCredsFile = "/etc/mattube/credentials.json"
	}
	if cfg.DriveTokenFile == "" {
		cfg.DriveTokenFile = "/etc/mattube/drive_token.json"
	}
	return &cfg, nil
}
