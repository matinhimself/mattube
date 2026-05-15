package api

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/matinhimself/mattube/client/auth"
	"github.com/matinhimself/mattube/client/fronting"
)

// JobStatus mirrors the server's status-<id>.json schema.
type JobStatus struct {
	JobID         string `json:"job_id"`
	Status        string `json:"status"`
	Progress      int    `json:"progress"`
	DriveFileID   string `json:"drive_file_id,omitempty"`
	DriveFileName string `json:"drive_file_name,omitempty"`
	Error         string `json:"error,omitempty"`
	UpdatedAt     string `json:"updated_at"`
}

// jobCache avoids hitting Drive on every poll for finished jobs.
type jobCache struct {
	mu    sync.RWMutex
	cache map[string]*JobStatus
}

func newJobCache() *jobCache { return &jobCache{cache: make(map[string]*JobStatus)} }

func (c *jobCache) get(id string) (*JobStatus, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.cache[id]
	return v, ok
}

func (c *jobCache) set(id string, s *JobStatus) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[id] = s
}

// Server holds all handler dependencies.
type Server struct {
	db       *sql.DB
	drive    *fronting.DriveClient
	yt       *fronting.YouTubeClient
	folderID string
	cache    *jobCache
}

func NewServer(db *sql.DB, dc *fronting.DriveClient, yt *fronting.YouTubeClient, folderID string) *Server {
	return &Server{db: db, drive: dc, yt: yt, folderID: folderID, cache: newJobCache()}
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Auth endpoints (public)
	r.Post("/auth/login", s.login)
	r.Post("/auth/logout", s.logout)

	// All API routes require auth
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAuth(s.db))

		// YouTube discovery
		r.Get("/api/search", s.search)
		r.Get("/api/video/{videoId}", s.videoInfo)
		r.Get("/api/video/{videoId}/captions", s.videoCaptions)
		r.Get("/api/channel/{channelId}", s.channelInfo)
		r.Get("/api/channel/{channelId}/videos", s.channelVideos)

		// Jobs
		r.Post("/api/jobs", s.submitJob)
		r.Get("/api/jobs", s.listJobs)
		r.Get("/api/jobs/{jobId}/status", s.jobStatus)
		r.Get("/api/jobs/{jobId}/stream", s.streamJob)

		// Admin
		r.Group(func(r chi.Router) {
			r.Use(auth.RequireAdmin)
			r.Get("/admin/users", s.listUsers)
			r.Post("/admin/users", s.createUser)
			r.Put("/admin/users/{id}/password", s.resetPassword)
			r.Delete("/admin/users/{id}", s.deleteUser)
		})
	})

	return r
}

// --- Auth ---

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "bad request", http.StatusBadRequest)
		return
	}
	token, err := auth.Login(s.db, body.Username, body.Password)
	if err != nil {
		jsonError(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	auth.SetSessionCookie(w, token)
	jsonOK(w, map[string]string{"status": "ok"})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("session"); err == nil {
		auth.Logout(s.db, c.Value) //nolint:errcheck
	}
	auth.ClearSessionCookie(w)
	jsonOK(w, map[string]string{"status": "ok"})
}

// --- YouTube ---

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		jsonError(w, "q required", http.StatusBadRequest)
		return
	}
	n, _ := strconv.Atoi(r.URL.Query().Get("n"))
	if n <= 0 || n > 50 {
		n = 20
	}
	results, err := s.yt.Search(r.Context(), q, n)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadGateway)
		return
	}
	jsonOK(w, results)
}

func (s *Server) videoInfo(w http.ResponseWriter, r *http.Request) {
	info, err := s.yt.GetVideoInfo(r.Context(), chi.URLParam(r, "videoId"))
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadGateway)
		return
	}
	jsonOK(w, info)
}

func (s *Server) videoCaptions(w http.ResponseWriter, r *http.Request) {
	lang := r.URL.Query().Get("lang") // optional, e.g. "en", "fa"
	segs, err := s.yt.GetCaptions(r.Context(), chi.URLParam(r, "videoId"), lang)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadGateway)
		return
	}
	jsonOK(w, segs)
}

func (s *Server) channelInfo(w http.ResponseWriter, r *http.Request) {
	info, err := s.yt.GetChannel(r.Context(), chi.URLParam(r, "channelId"))
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadGateway)
		return
	}
	jsonOK(w, info)
}

func (s *Server) channelVideos(w http.ResponseWriter, r *http.Request) {
	videos, err := s.yt.GetChannelVideos(r.Context(), chi.URLParam(r, "channelId"))
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadGateway)
		return
	}
	jsonOK(w, videos)
}

// --- Jobs ---

func (s *Server) submitJob(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL     string `json:"url"`
		Quality string `json:"quality"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.URL == "" {
		jsonError(w, "url required", http.StatusBadRequest)
		return
	}
	if body.Quality == "" {
		body.Quality = "1080p"
	}

	jobID := newJobID()
	req := map[string]any{
		"job_id":       jobID,
		"url":          body.URL,
		"quality":      body.Quality,
		"requested_at": time.Now().UTC().Format(time.RFC3339),
	}

	if _, err := s.drive.UploadJSON(r.Context(), s.folderID, "request-"+jobID+".json", req); err != nil {
		jsonError(w, fmt.Sprintf("submit job: %v", err), http.StatusBadGateway)
		return
	}

	jsonOK(w, map[string]string{"job_id": jobID})
}

func (s *Server) jobStatus(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobId")

	// Serve from cache if already done/failed
	if cached, ok := s.cache.get(jobID); ok {
		if cached.Status == "done" || cached.Status == "failed" {
			jsonOK(w, cached)
			return
		}
	}

	// Download latest status file from Drive
	files, err := s.drive.ListByPrefix(r.Context(), s.folderID, "status-"+jobID)
	if err != nil || len(files) == 0 {
		jsonOK(w, &JobStatus{JobID: jobID, Status: "pending"})
		return
	}

	var status JobStatus
	if err := s.drive.DownloadJSON(r.Context(), files[0].ID, &status); err != nil {
		jsonError(w, err.Error(), http.StatusBadGateway)
		return
	}

	if status.Status == "done" || status.Status == "failed" {
		s.cache.set(jobID, &status)
	}
	jsonOK(w, &status)
}

func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) {
	// List all status-*.json files from the Drive folder
	files, err := s.drive.ListByPrefix(r.Context(), s.folderID, "status-")
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadGateway)
		return
	}

	var statuses []JobStatus
	for _, f := range files {
		var st JobStatus
		if err := s.drive.DownloadJSON(r.Context(), f.ID, &st); err == nil {
			statuses = append(statuses, st)
		}
	}
	jsonOK(w, statuses)
}

func (s *Server) streamJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobId")

	var status JobStatus
	if cached, ok := s.cache.get(jobID); ok {
		status = *cached
	} else {
		files, err := s.drive.ListByPrefix(r.Context(), s.folderID, "status-"+jobID)
		if err != nil || len(files) == 0 {
			jsonError(w, "job not found", http.StatusNotFound)
			return
		}
		if err := s.drive.DownloadJSON(r.Context(), files[0].ID, &status); err != nil {
			jsonError(w, err.Error(), http.StatusBadGateway)
			return
		}
	}

	if status.Status != "done" || status.DriveFileID == "" {
		jsonError(w, "video not ready", http.StatusConflict)
		return
	}

	body, size, err := s.drive.Download(r.Context(), status.DriveFileID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer body.Close()

	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Accept-Ranges", "bytes")
	if size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	io.Copy(w, body) //nolint:errcheck
}

// --- Admin ---

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := auth.ListUsers(s.db)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, users)
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
		IsAdmin  bool   `json:"is_admin"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Username == "" || body.Password == "" {
		jsonError(w, "username and password required", http.StatusBadRequest)
		return
	}
	id, err := auth.CreateUser(s.db, body.Username, body.Password, body.IsAdmin)
	if err != nil {
		jsonError(w, err.Error(), http.StatusConflict)
		return
	}
	jsonOK(w, map[string]any{"id": id})
}

func (s *Server) resetPassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Password == "" || id == 0 {
		jsonError(w, "id and password required", http.StatusBadRequest)
		return
	}
	if err := auth.ChangePassword(s.db, id, body.Password); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "ok"})
}

func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if id == 0 {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := auth.DeleteUser(s.db, id); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "ok"})
}

// --- Helpers ---

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg}) //nolint:errcheck
}

func newJobID() string {
	b := make([]byte, 4)
	rand.Read(b) //nolint:errcheck
	return hex.EncodeToString(b)
}
