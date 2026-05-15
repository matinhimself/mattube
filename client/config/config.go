package config

import (
	"os"
)

type Config struct {
	// Fronting
	FrontingIP string
	AllowedSNI string

	// Drive
	DriveFolderID   string
	DriveAccessToken string // OAuth token for Drive access

	// YouTube
	YouTubeAPIKey string // optional InnerTube API key

	// HTTP
	HTTPAddr string

	// DB
	DBPath string

	// Bootstrap admin (used only when users table is empty)
	AdminUsername string
	AdminPassword string
}

func Load() *Config {
	return &Config{
		FrontingIP:      mustEnv("FRONTING_IP"),
		AllowedSNI:      mustEnv("ALLOWED_SNI"),
		DriveFolderID:   mustEnv("DRIVE_FOLDER_ID"),
		DriveAccessToken: getEnv("DRIVE_ACCESS_TOKEN", ""),
		YouTubeAPIKey:   getEnv("YOUTUBE_API_KEY", ""),
		HTTPAddr:        getEnv("HTTP_ADDR", ":8080"),
		DBPath:          getEnv("DB_PATH", "./mattube-client.db"),
		AdminUsername:   getEnv("ADMIN_USERNAME", ""),
		AdminPassword:   getEnv("ADMIN_PASSWORD", ""),
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
