package main

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/matinhimself/mattube/client/api"
	"github.com/matinhimself/mattube/client/auth"
	"github.com/matinhimself/mattube/client/cli"
	"github.com/matinhimself/mattube/client/config"
	"github.com/matinhimself/mattube/client/db"
	"github.com/matinhimself/mattube/client/fronting"
)

//go:embed static/dist
var staticFiles embed.FS

func main() {
	configPath, configProvided, args, err := parseGlobalFlags(os.Args[1:])
	if err != nil {
		fatalf("%v\n\n%s", err, usage())
	}
	if len(args) > 0 {
		runCommand(args[0], args[1:], configPath, configProvided)
		return
	}
	serve(configPath)
}

// runCommand dispatches CLI subcommands.
// Usage examples:
//
//	go run . create-admin <username> <password>
//	go run . create-user  <username> <password>
//	go run . list-users
//	go run . get-drive-token [credentials.json] [token_out.json]
//	go run . print-drive-token [credentials.json] [token.json]
//	go run . test-fronting <fronting-ip> <allowed-sni>
func runCommand(cmd string, args []string, configPath string, configProvided bool) {
	switch cmd {
	case "serve":
		serve(configPath)

	case "create-admin":
		if len(args) < 2 {
			fatalf("usage: create-admin <username> <password>")
		}
		database := mustOpenDB(configPath, configProvided)
		defer database.Close()
		cli.CreateUser(database, args[0], args[1], true)

	case "create-user":
		if len(args) < 2 {
			fatalf("usage: create-user <username> <password>")
		}
		database := mustOpenDB(configPath, configProvided)
		defer database.Close()
		cli.CreateUser(database, args[0], args[1], false)

	case "list-users":
		database := mustOpenDB(configPath, configProvided)
		defer database.Close()
		cli.ListUsers(database)

	case "get-drive-token":
		creds := argOr(args, 0, "credentials.json")
		out := argOr(args, 1, "drive_token.json")
		cli.GetDriveToken(creds, out)

	case "print-drive-token":
		creds := argOr(args, 0, "credentials.json")
		tok := argOr(args, 1, "drive_token.json")
		cli.PrintTokenFromFile(creds, tok)

	case "test-fronting":
		ip := argOr(args, 0, os.Getenv("FRONTING_IP"))
		sni := argOr(args, 1, os.Getenv("ALLOWED_SNI"))
		cli.TestFronting(ip, sni)

	case "test-video":
		ip := argOr(args, 0, os.Getenv("FRONTING_IP"))
		sni := argOr(args, 1, os.Getenv("ALLOWED_SNI"))
		vid := argOr(args, 2, "")
		cli.TestVideo(ip, sni, vid)

	default:
		fatalf("unknown command %q\n\n%s", cmd, usage())
	}
}

func serve(configPath string) {
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer database.Close()

	// Bootstrap: create admin if no users exist (skipped in local mode)
	if !cfg.LocalMode {
		if n, _ := auth.CountUsers(database); n == 0 {
			if cfg.AdminUsername == "" || cfg.AdminPassword == "" {
				log.Fatal("no users exist — run: go run . create-admin <username> <password>")
			}
			if _, err := auth.CreateUser(database, cfg.AdminUsername, cfg.AdminPassword, true); err != nil {
				log.Fatalf("create admin: %v", err)
			}
			log.Printf("created admin user: %s", cfg.AdminUsername)
		}
	}

	frontClient := fronting.NewClient(cfg.FrontingIP, cfg.AllowedSNI)

	// Build a Drive client with auto-refreshing token when possible.
	// All OAuth requests (token exchange, refresh) go through the fronting transport.
	var driveClient *fronting.DriveClient
	if cfg.DriveAccessToken != "" {
		driveClient = fronting.NewDriveClient(cfg.FrontingIP, cfg.AllowedSNI, cfg.DriveAccessToken)
		log.Println("Drive: using static access token from config (no auto-refresh)")
	} else if ts, err := cli.LoadTokenSource(cfg.DriveCredsFile, cfg.DriveTokenFile, frontClient); err == nil {
		log.Printf("Drive: token loaded from %s (creds: %s)", cfg.DriveTokenFile, cfg.DriveCredsFile)
		driveClient = fronting.NewDriveClientWithSource(cfg.FrontingIP, cfg.AllowedSNI, ts)
	} else if ts, err := cli.LoadTokenSourceFromDB(cfg.DriveCredsFile, database, frontClient); err == nil {
		log.Println("Drive: token loaded from database")
		driveClient = fronting.NewDriveClientWithSource(cfg.FrontingIP, cfg.AllowedSNI, ts)
	} else {
		log.Printf("Drive: no token found (%v) — connect via admin UI", err)
		driveClient = fronting.NewDriveClient(cfg.FrontingIP, cfg.AllowedSNI, "")
	}
	if _, err := os.Stat(cfg.DriveCredsFile); err != nil {
		log.Printf("Drive: credentials file not found at %s — OAuth connect will be unavailable", cfg.DriveCredsFile)
	}
	ytClient := fronting.NewYouTubeClient(cfg.FrontingIP, cfg.AllowedSNI, cfg.YouTubeAPIKey)
	thumbCache := api.NewThumbCache(64<<20, 6*time.Hour)

	apiServer := api.NewServer(database, driveClient, ytClient, frontClient, thumbCache, cfg.DriveFolderID, cfg.LocalMode, cfg.DriveCredsFile, cfg.DriveTokenFile)

	r := apiServer.Router()

	// Debug: unauthenticated search test — remove before production
	r.Get("/debug/search", apiServer.DebugSearch)

	// Serve SPA only for paths the API router didn't claim.
	dist, err := fs.Sub(staticFiles, "static/dist")
	if err != nil {
		log.Fatalf("embed sub: %v", err)
	}
	fileServer := http.FileServer(http.FS(dist))
	r.NotFound(func(w http.ResponseWriter, req *http.Request) {
		p := req.URL.Path
		if strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/auth/") || strings.HasPrefix(p, "/admin/") {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
		name := strings.TrimPrefix(p, "/")
		if f, err := dist.Open(name); err != nil {
			req.URL.Path = "/"
		} else {
			f.Close()
		}
		fileServer.ServeHTTP(w, req)
	})

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: r}
	go func() {
		log.Printf("client listening on %s", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("http: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down...")
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	srv.Shutdown(shutCtx) //nolint:errcheck
}

func mustOpenDB(configPath string, configProvided bool) *sql.DB {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		if cfg, err := config.Load(configPath); err == nil && cfg.DBPath != "" {
			dbPath = cfg.DBPath
		} else if configProvided {
			log.Fatalf("config: %v", err)
		} else {
			dbPath = "./mattube-client.db"
		}
	}
	database, err := db.Open(dbPath)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	return database
}

func argOr(args []string, i int, fallback string) string {
	if i < len(args) && args[i] != "" {
		return args[i]
	}
	return fallback
}

func parseGlobalFlags(args []string) (string, bool, []string, error) {
	configPath := config.DefaultPath
	configProvided := false
	remaining := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-c", "--config":
			if i+1 >= len(args) {
				return "", false, nil, fmt.Errorf("%s requires a path", args[i])
			}
			configPath = args[i+1]
			configProvided = true
			i++
		default:
			const configPrefix = "--config="
			if len(args[i]) > len(configPrefix) && args[i][:len(configPrefix)] == configPrefix {
				configPath = args[i][len(configPrefix):]
				configProvided = true
				continue
			}
			remaining = append(remaining, args[i])
		}
	}
	return configPath, configProvided, remaining, nil
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func usage() string {
	return `Commands:
  -c, --config <path>                    Config file path (default /etc/mattube/config.json)
  serve                                  Start the web server (default)
  create-admin  <username> <password>    Create an admin user
  create-user   <username> <password>    Create a regular user
  list-users                             List all users
  get-drive-token [creds.json] [out.json] Run OAuth flow, save Drive token
  print-drive-token [creds.json] [tok.json] Print a fresh access token
  test-fronting  <ip> <sni>             Test SNI fronting connectivity
  test-video     <ip> <sni> <video-id>  Fetch metadata, formats, related videos`
}
