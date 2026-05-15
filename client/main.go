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
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
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
	if len(os.Args) > 1 {
		runCommand(os.Args[1], os.Args[2:])
		return
	}
	serve()
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
func runCommand(cmd string, args []string) {
	switch cmd {
	case "create-admin":
		if len(args) < 2 {
			fatalf("usage: create-admin <username> <password>")
		}
		database := mustOpenDB()
		defer database.Close()
		cli.CreateUser(database, args[0], args[1], true)

	case "create-user":
		if len(args) < 2 {
			fatalf("usage: create-user <username> <password>")
		}
		database := mustOpenDB()
		defer database.Close()
		cli.CreateUser(database, args[0], args[1], false)

	case "list-users":
		database := mustOpenDB()
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

func serve() {
	cfg := config.Load()

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer database.Close()

	// Bootstrap: create admin if no users exist
	if n, _ := auth.CountUsers(database); n == 0 {
		if cfg.AdminUsername == "" || cfg.AdminPassword == "" {
			log.Fatal("no users exist — run: go run . create-admin <username> <password>")
		}
		if _, err := auth.CreateUser(database, cfg.AdminUsername, cfg.AdminPassword, true); err != nil {
			log.Fatalf("create admin: %v", err)
		}
		log.Printf("created admin user: %s", cfg.AdminUsername)
	}

	// If a token file exists and DRIVE_ACCESS_TOKEN is not set, load it automatically
	driveToken := cfg.DriveAccessToken
	if driveToken == "" {
		if tok, err := cli.LoadTokenFromFile("credentials.json", "drive_token.json"); err == nil {
			driveToken = tok
			log.Println("loaded Drive token from drive_token.json")
		}
	}

	driveClient := fronting.NewDriveClient(cfg.FrontingIP, cfg.AllowedSNI, driveToken)
	ytClient := fronting.NewYouTubeClient(cfg.FrontingIP, cfg.AllowedSNI, cfg.YouTubeAPIKey)

	apiServer := api.NewServer(database, driveClient, ytClient, cfg.DriveFolderID)

	r := chi.NewRouter()
	r.Mount("/", apiServer.Router())

	// Serve SPA with client-side routing fallback
	dist, err := fs.Sub(staticFiles, "static/dist")
	if err != nil {
		log.Fatalf("embed sub: %v", err)
	}
	fileServer := http.FileServer(http.FS(dist))
	r.Get("/*", func(w http.ResponseWriter, req *http.Request) {
		if _, err := dist.(fs.StatFS).Stat(req.URL.Path[1:]); err != nil {
			req.URL.Path = "/"
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

func mustOpenDB() *sql.DB {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./mattube-client.db"
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

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func usage() string {
	return `Commands:
  serve                                  Start the web server (default)
  create-admin  <username> <password>    Create an admin user
  create-user   <username> <password>    Create a regular user
  list-users                             List all users
  get-drive-token [creds.json] [out.json] Run OAuth flow, save Drive token
  print-drive-token [creds.json] [tok.json] Print a fresh access token
  test-fronting  <ip> <sni>             Test SNI fronting connectivity
  test-video     <ip> <sni> <video-id>  Fetch metadata, formats, related videos`
}
