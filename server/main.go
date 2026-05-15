package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/matinhimself/mattube/server/api"
	"github.com/matinhimself/mattube/server/config"
	"github.com/matinhimself/mattube/server/drive"
	"github.com/matinhimself/mattube/server/jobs"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := os.MkdirAll(cfg.DownloadDir, 0755); err != nil {
		log.Fatalf("mkdir download dir: %v", err)
	}

	driveClient, err := drive.New(ctx, cfg.CredentialsFile)
	if err != nil {
		log.Fatalf("drive client: %v", err)
	}

	proc := jobs.NewProcessor(
		driveClient,
		cfg.DriveOutputFolderID,
		cfg.DriveFolderID,
		cfg.DownloadDir,
		cfg.HTTPSProxy,
	)
	mgr := jobs.NewManager(cfg.MaxConcurrentJobs, proc)

	// Drive poller goroutine
	go pollDrive(ctx, driveClient, mgr, cfg)

	// Worker pool
	go mgr.Start(ctx)

	// HTTP admin server
	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: api.NewRouter()}
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
			for _, f := range files {
				var req jobs.Request
				if err := dc.DownloadJSON(ctx, f.Id, &req); err != nil {
					log.Printf("download request file %s: %v", f.Id, err)
					continue
				}
				if req.JobID == "" {
					log.Printf("bad request file %s: missing job_id", f.Id)
					dc.Delete(ctx, f.Id) //nolint:errcheck
					continue
				}
				// Delete before enqueue to avoid re-processing on next poll
				if err := dc.Delete(ctx, f.Id); err != nil {
					log.Printf("delete request file %s: %v", f.Id, err)
				}
				mgr.Enqueue(&req)
			}
		}
	}
}
