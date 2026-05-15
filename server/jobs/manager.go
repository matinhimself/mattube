package jobs

import (
	"context"
	"log"
	"sync"
	"time"
)

// Status values for a download job.
const (
	StatusQueued      = "queued"
	StatusDownloading = "downloading"
	StatusUploading   = "uploading"
	StatusDone        = "done"
	StatusFailed      = "failed"
)

// Request is read from request-<id>.json on Drive.
type Request struct {
	JobID       string `json:"job_id"`
	URL         string `json:"url"`
	Quality     string `json:"quality"`
	RequestedAt string `json:"requested_at"`
}

// Status is written to status-<id>.json on Drive.
type Status struct {
	JobID         string `json:"job_id"`
	Status        string `json:"status"`
	Progress      int    `json:"progress"`
	DriveFileID   string `json:"drive_file_id,omitempty"`
	DriveFileName string `json:"drive_file_name,omitempty"`
	Error         string `json:"error,omitempty"`
	UpdatedAt     string `json:"updated_at"`
}

// StatusWriter is implemented by the poller to persist status back to Drive.
type StatusWriter interface {
	WriteStatus(ctx context.Context, status *Status) error
}

// Manager owns the goroutine pool and job queue.
type Manager struct {
	maxWorkers int
	processor  *Processor
	queue      chan *Request
	mu         sync.Mutex
	seen       map[string]struct{} // job IDs currently in queue or processing
}

func NewManager(maxWorkers int, proc *Processor) *Manager {
	m := &Manager{
		maxWorkers: maxWorkers,
		processor:  proc,
		queue:      make(chan *Request, 64),
		seen:       make(map[string]struct{}),
	}
	return m
}

// Start launches worker goroutines. Blocks until ctx is cancelled.
func (m *Manager) Start(ctx context.Context) {
	log.Printf("job manager: starting %d workers", m.maxWorkers)
	var wg sync.WaitGroup
	for i := range m.maxWorkers {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			m.worker(ctx, id)
		}(i)
	}
	wg.Wait()
}

// Enqueue adds a job to the queue if not already seen.
// Returns false if the job was already enqueued.
func (m *Manager) Enqueue(req *Request) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.seen[req.JobID]; exists {
		return false
	}
	m.seen[req.JobID] = struct{}{}
	m.queue <- req
	log.Printf("job manager: enqueued %s (%s)", req.JobID, req.URL)
	return true
}

func (m *Manager) worker(ctx context.Context, id int) {
	log.Printf("worker %d ready", id)
	for {
		select {
		case <-ctx.Done():
			return
		case req := <-m.queue:
			log.Printf("worker %d: picked up job %s (%s)", id, req.JobID, req.URL)
			start := time.Now()
			m.processor.Process(ctx, req)
			log.Printf("worker %d: finished job %s in %s", id, req.JobID, time.Since(start).Round(time.Second))
			m.mu.Lock()
			delete(m.seen, req.JobID)
			m.mu.Unlock()
		case <-time.After(100 * time.Millisecond):
			// keep loop responsive to ctx cancellation
		}
	}
}
