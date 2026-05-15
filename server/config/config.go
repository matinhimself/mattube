package config

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

const DefaultPath = "/etc/mattube/config.json"

type Config struct {
	DriveFolderID       string `json:"drive_folder_id"`
	DriveOutputFolderID string `json:"drive_output_folder_id"`
	CredentialsFile     string `json:"credentials_file"`
	TokenFile           string `json:"token_file"`
	DriveAccessToken    string `json:"drive_access_token"`
	DownloadDir         string `json:"download_dir"`
	PollIntervalS       int    `json:"poll_interval_s"`
	MaxConcurrentJobs   int    `json:"max_concurrent_jobs"`
	HTTPAddr            string `json:"http_addr"`
	AdminPassword       string `json:"admin_password"`

	// derived
	PollInterval time.Duration `json:"-"`
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
	if cfg.CredentialsFile == "" {
		cfg.CredentialsFile = "/etc/mattube/credentials.json"
	}
	if cfg.TokenFile == "" {
		cfg.TokenFile = "/etc/mattube/drive_token.json"
	}
	if cfg.DownloadDir == "" {
		cfg.DownloadDir = "/tmp/mattube"
	}
	if cfg.PollIntervalS <= 0 {
		cfg.PollIntervalS = 5
	}
	if cfg.MaxConcurrentJobs <= 0 {
		cfg.MaxConcurrentJobs = 2
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = ":8081"
	}
	cfg.PollInterval = time.Duration(cfg.PollIntervalS) * time.Second
	return &cfg, nil
}
