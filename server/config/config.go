package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	DriveFolderID       string
	DriveOutputFolderID string
	CredentialsFile     string
	DownloadDir         string
	PollInterval        time.Duration
	MaxConcurrentJobs   int
	HTTPAddr            string
	HTTPSProxy          string // SOCKS5 proxy for yt-dlp
}

func Load() *Config {
	return &Config{
		DriveFolderID:       mustEnv("DRIVE_FOLDER_ID"),
		DriveOutputFolderID: mustEnv("DRIVE_OUTPUT_FOLDER_ID"),
		CredentialsFile:     getEnv("GOOGLE_CREDENTIALS_FILE", "credentials.json"),
		DownloadDir:         getEnv("DOWNLOAD_DIR", "/tmp/mattube"),
		PollInterval:        parseDuration(getEnv("POLL_INTERVAL_S", "5"), time.Second),
		MaxConcurrentJobs:   parseInt(getEnv("MAX_CONCURRENT_JOBS", "2")),
		HTTPAddr:            getEnv("HTTP_ADDR", ":8081"),
		HTTPSProxy:          getEnv("HTTPS_PROXY", "socks5://127.0.0.1:10814"),
	}
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic("required env var not set: " + key)
	}
	return v
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseDuration(s string, unit time.Duration) time.Duration {
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 5 * time.Second
	}
	return time.Duration(n) * unit
}

func parseInt(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 2
	}
	return n
}
