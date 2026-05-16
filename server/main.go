package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/matinhimself/mattube/server/api"
	"github.com/matinhimself/mattube/server/cli"
	"github.com/matinhimself/mattube/server/config"
	"github.com/matinhimself/mattube/server/drive"
	"github.com/matinhimself/mattube/server/jobs"
)

func main() {
	configPath, args, err := parseGlobalFlags(os.Args[1:])
	if err != nil {
		fatalf("%v\n\n%s", err, usage())
	}
	if len(args) > 0 {
		runCommand(args[0], args[1:], configPath)
		return
	}
	serve(configPath)
}

// runCommand dispatches CLI subcommands.
//
//	go run . get-drive-token [credentials.json] [token_out.json]
//	go run . print-drive-token [credentials.json] [token.json]
func runCommand(cmd string, args []string, configPath string) {
	switch cmd {
	case "serve":
		serve(configPath)

	case "get-drive-token":
		creds := argOr(args, 0, "/etc/mattube/credentials.json")
		out := argOr(args, 1, "/etc/mattube/drive_token.json")
		cli.GetDriveToken(creds, out)

	case "print-drive-token":
		creds := argOr(args, 0, "/etc/mattube/credentials.json")
		tok := argOr(args, 1, "/etc/mattube/drive_token.json")
		cli.PrintTokenFromFile(creds, tok)

	default:
		fatalf("unknown command %q\n\n%s", cmd, usage())
	}
}

func serve(configPath string) {
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := os.MkdirAll(cfg.DownloadDir, 0755); err != nil {
		log.Fatalf("mkdir download dir: %v", err)
	}

	driveClient, err := drive.New(ctx, cfg.CredentialsFile, cfg.TokenFile, cfg.DriveAccessToken)
	if err != nil {
		log.Fatalf("drive client: %v", err)
	}

	proc := jobs.NewProcessor(
		driveClient,
		cfg.DriveOutputFolderID,
		cfg.DriveFolderID,
		cfg.DownloadDir,
	)
	mgr := jobs.NewManager(cfg.MaxConcurrentJobs, proc)

	go pollDrive(ctx, driveClient, mgr, cfg)
	go mgr.Start(ctx)

	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: api.NewRouter(cfg.TokenFile, cfg.AdminPassword)}
	go func() {
		log.Printf("server listening on %s", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("http server: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down...")
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	srv.Shutdown(shutCtx) //nolint:errcheck
}

func pollDrive(ctx context.Context, dc *drive.Client, mgr *jobs.Manager, cfg *config.Config) {
	log.Printf("drive poller: watching folder %s every %s", cfg.DriveFolderID, cfg.PollInterval)
	ticker := time.NewTicker(cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			files, err := dc.ListByPrefix(ctx, cfg.DriveFolderID, "request-")
			if err != nil {
				log.Printf("drive poll error: %v", err)
				continue
			}
			log.Printf("drive poll: found %d request file(s)", len(files))
			for _, f := range files {
				log.Printf("drive poll: processing request file %s (%s)", f.Id, f.Name)
				var req jobs.Request
				if err := dc.DownloadJSON(ctx, f.Id, &req); err != nil {
					log.Printf("download request file %s: %v", f.Id, err)
					continue
				}
				log.Printf("drive poll: request %s url=%s quality=%s chunk_size_mb=%d", req.JobID, req.URL, req.Quality, req.ChunkDurationS)
				if req.JobID == "" {
					log.Printf("bad request file %s: missing job_id", f.Id)
					dc.Delete(ctx, f.Id) //nolint:errcheck
					continue
				}
				if err := dc.Delete(ctx, f.Id); err != nil {
					log.Printf("delete request file %s: %v", f.Id, err)
				}
				mgr.Enqueue(&req)
			}
		}
	}
}

func parseGlobalFlags(args []string) (string, []string, error) {
	configPath := config.DefaultPath
	remaining := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-c", "--config":
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("%s requires a path", args[i])
			}
			configPath = args[i+1]
			i++
		default:
			const configPrefix = "--config="
			if len(args[i]) > len(configPrefix) && args[i][:len(configPrefix)] == configPrefix {
				configPath = args[i][len(configPrefix):]
				continue
			}
			remaining = append(remaining, args[i])
		}
	}
	return configPath, remaining, nil
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
  -c, --config <path>                         Config file path (default /etc/mattube/config.json)
  serve                                        Start the server (default)
  get-drive-token   [creds.json] [out.json]   Run OAuth flow, save Drive token
  print-drive-token [creds.json] [tok.json]   Print a fresh access token`
}
