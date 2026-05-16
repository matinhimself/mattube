package api

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/matinhimself/mattube/client/auth"
	"github.com/matinhimself/mattube/client/db"
	"github.com/matinhimself/mattube/client/fronting"
)

// ChunkRef mirrors a single uploaded TS segment.
type ChunkRef struct {
	Index       int     `json:"index"`
	DriveFileID string  `json:"drive_file_id"`
	DurationS   float64 `json:"duration_s"`
}

// JobStatus mirrors the server's status-<id>.json schema.
type JobStatus struct {
	JobID         string     `json:"job_id"`
	Status        string     `json:"status"`
	Progress      int        `json:"progress"`
	DriveFileID   string     `json:"drive_file_id,omitempty"`
	DriveFileName string     `json:"drive_file_name,omitempty"`
	Error         string     `json:"error,omitempty"`
	UpdatedAt     string     `json:"updated_at"`
	TotalChunks   int        `json:"total_chunks,omitempty"`
	ChunkSizeMB   int        `json:"chunk_size_mb,omitempty"`
	Chunks        []ChunkRef `json:"chunks,omitempty"`
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
	db        *sql.DB
	drive     *fronting.DriveClient
	yt        *fronting.YouTubeClient
	front     *http.Client // fronted HTTP client for arbitrary Google fetches (thumbnails)
	thumbs    *ThumbCache
	folderID  string
	cache     *jobCache
	localMode bool

	driveCredsFile  string
	driveTokenFile  string
	driveStateMu    sync.Mutex
	drivePendState  map[string]time.Time // OAuth state -> expiry
	drivePendRedir  map[string]string    // OAuth state -> redirect URL
	jobPollInterval time.Duration
	jobTimeout      time.Duration
	jobsListMu      sync.RWMutex
	jobsList        []JobStatus
}

func NewServer(
	db *sql.DB,
	dc *fronting.DriveClient,
	yt *fronting.YouTubeClient,
	front *http.Client,
	thumbs *ThumbCache,
	folderID string,
	localMode bool,
	driveCredsFile string,
	driveTokenFile string,
	jobPollInterval time.Duration,
	jobTimeout time.Duration,
) *Server {
	return &Server{
		db:              db,
		drive:           dc,
		yt:              yt,
		front:           front,
		thumbs:          thumbs,
		folderID:        folderID,
		cache:           newJobCache(),
		localMode:       localMode,
		driveCredsFile:  driveCredsFile,
		driveTokenFile:  driveTokenFile,
		drivePendState:  make(map[string]time.Time),
		drivePendRedir:  make(map[string]string),
		jobPollInterval: jobPollInterval,
		jobTimeout:      jobTimeout,
	}
}

func (s *Server) Router() chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Post("/auth/login", s.login)
	r.Post("/auth/logout", s.logout)
	r.Get("/admin/drive/callback", s.driveCallback) // outside auth — Google redirects here

	requireAuth := auth.RequireAuth(s.db)
	if s.localMode {
		requireAuth = auth.LocalModeMiddleware()
	}

	r.Group(func(r chi.Router) {
		r.Use(requireAuth)

		r.Get("/auth/me", s.me)

		r.Get("/api/search", s.search)
		r.Get("/api/video/{videoId}", s.videoInfo)
		r.Get("/api/video/{videoId}/related", s.videoRelated)
		r.Get("/api/video/{videoId}/captions", s.videoCaptions)
		r.Get("/api/channel/{channelId}", s.channelInfo)
		r.Get("/api/channel/{channelId}/videos", s.channelVideos)
		r.Get("/api/thumbnail", s.thumbnail)

		r.Post("/api/jobs", s.submitJob)
		r.Get("/api/jobs", s.listJobs)
		r.Get("/api/jobs/{jobId}/status", s.jobStatus)
		r.Get("/api/jobs/{jobId}/stream", s.streamJob)
		r.Get("/api/jobs/{jobId}/chunk/{chunkIdx}", s.streamChunk)

		r.Group(func(r chi.Router) {
			r.Use(auth.RequireAdmin)
			r.Get("/admin/users", s.listUsers)
			r.Post("/admin/users", s.createUser)
			r.Put("/admin/users/{id}/password", s.resetPassword)
			r.Delete("/admin/users/{id}", s.deleteUser)

			r.Get("/admin/drive/status", s.driveStatus)
			r.Get("/admin/drive/connect", s.driveConnect)
		})
	})

	return r
}

// --- Auth ---

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if s.localMode {
		jsonOK(w, map[string]any{"id": 0, "username": "local", "is_admin": true, "last_login": nil, "local_mode": true})
		return
	}
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
	user, _ := auth.Validate(s.db, token)
	jsonOK(w, map[string]any{
		"id":         user.ID,
		"username":   user.Username,
		"is_admin":   user.IsAdmin,
		"last_login": user.LastLogin,
		"local_mode": false,
	})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	jsonOK(w, map[string]any{
		"id":         u.ID,
		"username":   u.Username,
		"is_admin":   u.IsAdmin,
		"last_login": u.LastLogin,
		"local_mode": s.localMode,
	})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if s.localMode {
		jsonOK(w, map[string]string{"status": "ok"})
		return
	}
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
	rewriteSearchThumbs(results)
	jsonOK(w, results)
}

func (s *Server) videoInfo(w http.ResponseWriter, r *http.Request) {
	info, err := s.yt.GetVideoInfo(r.Context(), chi.URLParam(r, "videoId"))
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadGateway)
		return
	}
	info.Thumbnail = proxyThumbURL(info.Thumbnail)
	jsonOK(w, info)
}

func (s *Server) videoRelated(w http.ResponseWriter, r *http.Request) {
	results, err := s.yt.GetRelatedVideos(r.Context(), chi.URLParam(r, "videoId"))
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadGateway)
		return
	}
	if results == nil {
		results = []fronting.SearchResult{}
	}
	rewriteSearchThumbs(results)
	jsonOK(w, results)
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
	info.Avatar = proxyThumbURL(info.Avatar)
	jsonOK(w, info)
}

func (s *Server) channelVideos(w http.ResponseWriter, r *http.Request) {
	videos, err := s.yt.GetChannelVideos(r.Context(), chi.URLParam(r, "channelId"))
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadGateway)
		return
	}
	rewriteSearchThumbs(videos)
	jsonOK(w, videos)
}

// --- Jobs ---

func (s *Server) submitJob(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL         string `json:"url"`
		Quality     string `json:"quality"`
		ChunkSizeMB int    `json:"chunk_size_mb"`
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
		"job_id":        jobID,
		"url":           body.URL,
		"quality":       body.Quality,
		"requested_at":  time.Now().UTC().Format(time.RFC3339),
		"chunk_size_mb": body.ChunkSizeMB,
	}

	uploadCtx, uploadCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer uploadCancel()
	if _, err := s.drive.UploadJSON(uploadCtx, s.folderID, "request-"+jobID+".json", req); err != nil {
		jsonError(w, fmt.Sprintf("submit job: %v", err), http.StatusBadGateway)
		return
	}

	jsonOK(w, map[string]string{"job_id": jobID})
}

func (s *Server) jobStatus(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobId")

	status, err := s.fetchStatus(r.Context(), jobID)
	if err != nil {
		jsonOK(w, &JobStatus{JobID: jobID, Status: "pending"})
		return
	}
	jsonOK(w, &status)
}

func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) {
	s.jobsListMu.RLock()
	list := s.jobsList
	s.jobsListMu.RUnlock()
	if list == nil {
		list = []JobStatus{}
	}
	jsonOK(w, list)
}

func (s *Server) fetchStatus(ctx context.Context, jobID string) (JobStatus, error) {
	if cached, ok := s.cache.get(jobID); ok {
		return *cached, nil
	}
	files, err := s.drive.ListByPrefix(ctx, s.folderID, "status-"+jobID)
	if err != nil || len(files) == 0 {
		return JobStatus{}, fmt.Errorf("job not found")
	}
	var status JobStatus
	if err := s.drive.DownloadJSON(ctx, files[0].ID, &status); err != nil {
		return JobStatus{}, err
	}
	if status.Status == "done" || status.Status == "failed" {
		s.cache.set(jobID, &status)
	}
	return status, nil
}

// StartBackgroundTicker starts the periodic Drive job-status refresh and expiry loop.
func (s *Server) StartBackgroundTicker(ctx context.Context) {
	go func() {
		s.tickJobs(ctx)
		ticker := time.NewTicker(s.jobPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.tickJobs(ctx)
			}
		}
	}()
}

func (s *Server) tickJobs(ctx context.Context) {
	files, err := s.drive.ListByPrefix(ctx, s.folderID, "status-")
	if err != nil {
		log.Printf("tickJobs: list status files: %v", err)
		return
	}

	var list []JobStatus
	for _, f := range files {
		var st JobStatus
		if err := s.drive.DownloadJSON(ctx, f.ID, &st); err != nil {
			log.Printf("tickJobs: download %s: %v", f.Name, err)
			continue
		}

		if st.Status != "done" && st.Status != "failed" {
			updated, parseErr := time.Parse(time.RFC3339, st.UpdatedAt)
			if parseErr == nil && time.Since(updated) > s.jobTimeout {
				log.Printf("[%s] job timed out after %s — marking failed and deleting files", st.JobID, s.jobTimeout)
				st.Status = "failed"
				st.Error = "timed out"
				st.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
				if updateErr := s.drive.UpdateJSON(ctx, f.ID, st); updateErr != nil {
					log.Printf("[%s] update status failed: %v", st.JobID, updateErr)
				}
				// Delete the status file
				if deleteErr := s.drive.Delete(ctx, f.ID); deleteErr != nil {
					log.Printf("[%s] delete status file: %v", st.JobID, deleteErr)
				}
				// Delete the request file if it exists
				reqs, _ := s.drive.ListByPrefix(ctx, s.folderID, "request-"+st.JobID)
				for _, req := range reqs {
					if err := s.drive.Delete(ctx, req.ID); err != nil {
						log.Printf("[%s] delete request file: %v", st.JobID, err)
					}
				}
				s.cache.set(st.JobID, &st)
				continue
			}
		}

		if st.Status == "done" || st.Status == "failed" {
			s.cache.set(st.JobID, &st)
		}
		list = append(list, st)
	}

	s.jobsListMu.Lock()
	s.jobsList = list
	s.jobsListMu.Unlock()
}

func (s *Server) streamJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobId")

	status, err := s.fetchStatus(r.Context(), jobID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusNotFound)
		return
	}

	// Chunked mode: serve HLS playlist
	if status.TotalChunks > 0 {
		s.servePlaylist(w, jobID, &status)
		return
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

	log.Printf("[%s] download start: fileID=%s size=%.2f MB", jobID, status.DriveFileID, float64(size)/(1024*1024))

	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Accept-Ranges", "bytes")
	if size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	t0 := time.Now()
	n, _ := io.Copy(w, body)
	elapsed := time.Since(t0)
	log.Printf("[%s] download done: %.2f MB in %s (%.2f MiB/s)", jobID, float64(n)/(1024*1024), elapsed.Round(time.Millisecond), float64(n)/elapsed.Seconds()/(1024*1024))
}

func (s *Server) servePlaylist(w http.ResponseWriter, jobID string, status *JobStatus) {
	// Compute TARGETDURATION from actual chunk durations (HLS spec: max segment duration, rounded up).
	maxDur := 0
	for _, c := range status.Chunks {
		if d := int(c.DurationS) + 1; d > maxDur {
			maxDur = d
		}
	}
	if maxDur <= 0 {
		maxDur = 120 // conservative default while no chunks uploaded yet
	}

	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-cache")

	fmt.Fprintln(w, "#EXTM3U")
	fmt.Fprintln(w, "#EXT-X-VERSION:3")
	fmt.Fprintf(w, "#EXT-X-TARGETDURATION:%d\n", maxDur)
	if status.Status == "done" {
		fmt.Fprintln(w, "#EXT-X-PLAYLIST-TYPE:VOD")
	} else {
		fmt.Fprintln(w, "#EXT-X-PLAYLIST-TYPE:EVENT")
	}
	for _, chunk := range status.Chunks {
		fmt.Fprintf(w, "#EXTINF:%.3f,\n", chunk.DurationS)
		fmt.Fprintf(w, "/api/jobs/%s/chunk/%d\n", jobID, chunk.Index)
	}
	if status.Status == "done" {
		fmt.Fprintln(w, "#EXT-X-ENDLIST")
	}
}

func (s *Server) streamChunk(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobId")
	chunkIdx, err := strconv.Atoi(chi.URLParam(r, "chunkIdx"))
	if err != nil {
		jsonError(w, "invalid chunk index", http.StatusBadRequest)
		return
	}

	status, err := s.fetchStatus(r.Context(), jobID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusNotFound)
		return
	}
	if chunkIdx >= len(status.Chunks) {
		jsonError(w, "chunk not available", http.StatusNotFound)
		return
	}

	chunk := status.Chunks[chunkIdx]
	body, _, err := s.drive.Download(r.Context(), chunk.DriveFileID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer body.Close()

	w.Header().Set("Content-Type", "video/mp2t")
	log.Printf("[%s] chunk %d start", jobID, chunkIdx)
	t0 := time.Now()
	n, _ := io.Copy(w, body)
	log.Printf("[%s] chunk %d done: %.2f MB in %s", jobID, chunkIdx, float64(n)/(1024*1024), time.Since(t0).Round(time.Millisecond))
}

// --- Thumbnail proxy ---

// thumbnail streams a Google-hosted thumbnail through the fronting
// transport. Cache hits are served from memory; misses fetch, store,
// and stream the response.
func (s *Server) thumbnail(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("u")
	if raw == "" {
		jsonError(w, "u required", http.StatusBadRequest)
		return
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || !fronting.IsGoogleHost(parsed.Host) {
		jsonError(w, "invalid url", http.StatusBadRequest)
		return
	}
	key := parsed.String()

	if data, ct, ok := s.thumbs.Get(key); ok {
		writeThumb(w, data, ct, "hit")
		return
	}

	req, err := fronting.NewRequest("GET", key, nil)
	if err != nil {
		jsonError(w, "bad url", http.StatusBadRequest)
		return
	}
	req = req.WithContext(r.Context())
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept", "image/*,*/*;q=0.8")

	resp, err := s.front.Do(req)
	if err != nil {
		jsonError(w, "fetch: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		jsonError(w, fmt.Sprintf("upstream %d", resp.StatusCode), http.StatusBadGateway)
		return
	}

	ct := resp.Header.Get("Content-Type")
	if ct == "" || !strings.HasPrefix(ct, "image/") {
		ct = "image/jpeg"
	}

	buf := s.thumbs.getBuf()
	defer s.thumbs.putBuf(buf)
	if _, err := io.CopyN(buf, resp.Body, thumbMaxItemSize+1); err != nil && err != io.EOF {
		jsonError(w, "read: "+err.Error(), http.StatusBadGateway)
		return
	}
	if buf.Len() > thumbMaxItemSize {
		jsonError(w, "thumbnail too large", http.StatusBadGateway)
		return
	}

	data := make([]byte, buf.Len())
	copy(data, buf.Bytes())
	s.thumbs.Put(key, data, ct)
	writeThumb(w, data, ct, "miss")
}

func writeThumb(w http.ResponseWriter, data []byte, contentType, status string) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "public, max-age=86400, immutable")
	w.Header().Set("X-Thumb-Cache", status)
	w.Write(data) //nolint:errcheck
}

// proxyThumbURL turns an absolute Google thumbnail URL into a relative
// proxy URL routed through /api/thumbnail. Empty / non-Google inputs
// are returned unchanged so the frontend never breaks on edge cases.
func proxyThumbURL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || !fronting.IsGoogleHost(u.Host) {
		return raw
	}
	return "/api/thumbnail?u=" + url.QueryEscape(raw)
}

func rewriteSearchThumbs(results []fronting.SearchResult) {
	for i := range results {
		results[i].Thumbnail = proxyThumbURL(results[i].Thumbnail)
	}
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

// --- Drive OAuth ---

const driveOAuthScope = "https://www.googleapis.com/auth/drive.file"

func (s *Server) driveStatus(w http.ResponseWriter, r *http.Request) {
	_, credsErr := os.Stat(s.driveCredsFile)
	jsonOK(w, map[string]any{
		"connected":   s.drive.IsConnected(),
		"creds_ready": credsErr == nil,
	})
}

func (s *Server) driveConnect(w http.ResponseWriter, r *http.Request) {
	creds, err := os.ReadFile(s.driveCredsFile)
	if err != nil {
		jsonError(w, "credentials.json not found on server", http.StatusServiceUnavailable)
		return
	}
	cfg, err := google.ConfigFromJSON(creds, driveOAuthScope)
	if err != nil {
		jsonError(w, "invalid credentials.json: "+err.Error(), http.StatusInternalServerError)
		return
	}

	scheme := "http"
	if r.Header.Get("X-Forwarded-Proto") == "https" || r.TLS != nil {
		scheme = "https"
	}
	redirectURL := fmt.Sprintf("%s://%s/admin/drive/callback", scheme, r.Host)
	cfg.RedirectURL = redirectURL

	state := newJobID() + newJobID()
	s.driveStateMu.Lock()
	s.drivePendState[state] = time.Now().Add(10 * time.Minute)
	s.drivePendRedir[state] = redirectURL
	s.driveStateMu.Unlock()

	authURL := cfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)
	http.Redirect(w, r, authURL, http.StatusFound)
}

func (s *Server) driveCallback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")

	s.driveStateMu.Lock()
	expiry, ok := s.drivePendState[state]
	redirectURL := s.drivePendRedir[state]
	if ok {
		delete(s.drivePendState, state)
		delete(s.drivePendRedir, state)
	}
	s.driveStateMu.Unlock()

	if !ok || state == "" || time.Now().After(expiry) {
		http.Error(w, "invalid or expired OAuth state — please try again", http.StatusBadRequest)
		return
	}
	if code == "" {
		errMsg := r.URL.Query().Get("error")
		if errMsg == "" {
			errMsg = "no authorization code received"
		}
		http.Error(w, errMsg, http.StatusBadRequest)
		return
	}

	creds, err := os.ReadFile(s.driveCredsFile)
	if err != nil {
		http.Error(w, "credentials.json not found on server", http.StatusInternalServerError)
		return
	}
	cfg, err := google.ConfigFromJSON(creds, driveOAuthScope)
	if err != nil {
		http.Error(w, "invalid credentials.json: "+err.Error(), http.StatusInternalServerError)
		return
	}
	cfg.RedirectURL = redirectURL

	// All OAuth HTTP calls (exchange + future refreshes) go through the fronting transport.
	frontCtx := context.WithValue(context.Background(), oauth2.HTTPClient, s.front)

	ctx, cancel := context.WithTimeout(frontCtx, 30*time.Second)
	defer cancel()

	token, err := cfg.Exchange(ctx, code)
	if err != nil {
		http.Error(w, "token exchange failed: "+err.Error(), http.StatusBadGateway)
		return
	}

	if b, err := json.MarshalIndent(token, "", "  "); err == nil {
		db.SetSetting(s.db, "drive_token", string(b)) //nolint:errcheck
	}
	if s.driveTokenFile != "" {
		if b, err := json.MarshalIndent(token, "", "  "); err == nil {
			os.WriteFile(s.driveTokenFile, b, 0600) //nolint:errcheck
		}
	}

	ts := fronting.NewPersistingDBTokenSource(
		cfg.TokenSource(frontCtx, token),
		s.db,
		token.AccessToken,
	)
	s.drive.SetTokenSource(ts)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, driveConnectedHTML)
}

const driveConnectedHTML = `<!DOCTYPE html>
<html>
<head><title>Drive Connected</title></head>
<body style="background:#080808;color:#fff;font-family:system-ui,sans-serif;display:flex;align-items:center;justify-content:center;height:100vh;margin:0;text-align:center">
<div>
  <div style="font-size:3rem;margin-bottom:12px">&#10003;</div>
  <p style="font-size:1.1rem;font-weight:600;margin:0 0 8px">Google Drive Connected</p>
  <p style="color:#666;font-size:0.85rem;margin:0">This tab will close automatically&hellip;</p>
</div>
<script>
  if (window.opener) window.opener.postMessage('drive-connected', '*');
  setTimeout(function(){ window.close(); }, 1500);
</script>
</body>
</html>`

// --- Debug ---

func (s *Server) DebugSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		jsonError(w, "q required", http.StatusBadRequest)
		return
	}
	n, _ := strconv.Atoi(r.URL.Query().Get("n"))
	if n <= 0 || n > 50 {
		n = 5
	}
	results, err := s.yt.Search(r.Context(), q, n)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadGateway)
		return
	}
	jsonOK(w, map[string]any{"count": len(results), "results": results})
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

