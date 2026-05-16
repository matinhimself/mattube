package jobs

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/matinhimself/mattube/server/drive"
)

// Processor runs yt-dlp + ffmpeg and uploads the result to Drive.
type Processor struct {
	driveClient   *drive.Client
	outputFolder  string // Drive folder ID for finished videos
	statusFolder  string // Drive folder ID for status files (same as job folder)
	downloadDir   string // local temp base dir
}

func NewProcessor(dc *drive.Client, outputFolder, statusFolder, downloadDir string) *Processor {
	return &Processor{
		driveClient:  dc,
		outputFolder: outputFolder,
		statusFolder: statusFolder,
		downloadDir:  downloadDir,
	}
}

func (p *Processor) Process(ctx context.Context, req *Request) {
	log.Printf("[%s] starting: %s quality=%s chunk_size_mb=%d", req.JobID, req.URL, req.Quality, req.ChunkSizeMB)

	jobDir := filepath.Join(p.downloadDir, req.JobID)
	if err := os.MkdirAll(jobDir, 0755); err != nil {
		p.fail(ctx, req.JobID, fmt.Sprintf("mkdir: %v", err))
		return
	}
	defer os.RemoveAll(jobDir)

	statusFileID, err := p.createStatus(ctx, req.JobID, StatusQueued, 0, "", "")
	if err != nil {
		log.Printf("[%s] warn: could not create status file: %v", req.JobID, err)
	}

	// Streaming chunked mode: segment while downloading via ffmpeg direct URLs
	if req.ChunkSizeMB > 0 {
		log.Printf("[%s] streaming-chunk mode: %d MB segments", req.JobID, req.ChunkSizeMB)
		if err := p.processStreamingChunked(ctx, req, jobDir, statusFileID); err != nil {
			p.updateStatus(ctx, statusFileID, req.JobID, StatusFailed, 0, "", err.Error())
		}
		return
	}

	// Download
	p.updateStatus(ctx, statusFileID, req.JobID, StatusDownloading, 0, "", "")
	outPath, err := p.download(ctx, req, jobDir, func(pct int) {
		p.updateStatus(ctx, statusFileID, req.JobID, StatusDownloading, pct, "", "")
	})
	if err != nil {
		p.updateStatus(ctx, statusFileID, req.JobID, StatusFailed, 0, "", err.Error())
		return
	}

	// Remux with faststart for iOS streaming
	log.Printf("[%s] remuxing with faststart: %s", req.JobID, outPath)
	outPath, err = remuxFaststart(ctx, outPath)
	if err != nil {
		log.Printf("[%s] warn: faststart remux failed, uploading original: %v", req.JobID, err)
	} else {
		log.Printf("[%s] remux done: %s", req.JobID, outPath)
	}

	if fi, err := os.Stat(outPath); err == nil {
		log.Printf("[%s] upload file: %s size=%.2f MB", req.JobID, outPath, float64(fi.Size())/(1024*1024))
	}

	// Upload
	p.updateStatus(ctx, statusFileID, req.JobID, StatusUploading, 0, "", "")
	driveID, err := p.driveClient.UploadFile(ctx, p.outputFolder, outPath, "video/mp4")
	if err != nil {
		log.Printf("[%s] upload failed: %v", req.JobID, err)
		p.updateStatus(ctx, statusFileID, req.JobID, StatusFailed, 0, "", fmt.Sprintf("upload: %v", err))
		return
	}

	p.updateStatus(ctx, statusFileID, req.JobID, StatusDone, 100, driveID, "")
	log.Printf("[%s] done: drive_file_id=%s", req.JobID, driveID)
}

// download runs yt-dlp and returns the path to the final .mp4 file.
func (p *Processor) download(ctx context.Context, req *Request, jobDir string, progress func(int)) (string, error) {
	format := ytdlpFormat(req.Quality)
	outTemplate := filepath.Join(jobDir, "video.%(ext)s")

	args := []string{
		"--format", format,
		"--merge-output-format", "mp4",
		"--output", outTemplate,
		"--no-playlist",
		"--progress",
		"--newline",
		"--concurrent-fragments", "16",
		"--throttled-rate", "100K",
		"--extractor-args", "youtube:player_client=android,web",
	}
	args = append(args, req.URL)

	log.Printf("[%s] yt-dlp cmd: yt-dlp %s", req.JobID, strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, "yt-dlp", args...)
	cmd.Env = append(os.Environ())

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("yt-dlp start: %w", err)
	}

	pctRe := regexp.MustCompile(`(\d+(?:\.\d+)?)\%`)
	var lastLines []string
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		log.Printf("[%s] yt-dlp: %s", req.JobID, line)
		lastLines = append(lastLines, line)
		if len(lastLines) > 20 {
			lastLines = lastLines[1:]
		}
		if m := pctRe.FindStringSubmatch(line); m != nil {
			if pct, err := strconv.ParseFloat(m[1], 64); err == nil {
				progress(int(pct))
			}
		}
	}

	if err := cmd.Wait(); err != nil {
		tail := strings.Join(lastLines, "\n")
		return "", fmt.Errorf("yt-dlp: %w\n%s", err, tail)
	}

	// Find the output file
	entries, err := os.ReadDir(jobDir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".mp4") {
			return filepath.Join(jobDir, e.Name()), nil
		}
	}
	return "", fmt.Errorf("no .mp4 found in %s after download", jobDir)
}

func (p *Processor) chunkUploadFolder() string {
	if p.outputFolder != "" {
		return p.outputFolder
	}
	return p.statusFolder
}

// processStreamingChunked gets direct stream URLs from yt-dlp, then pipes them
// through ffmpeg to produce TS segments in real-time, uploading each chunk as
// ffmpeg finishes writing it — so playback can start before the download ends.
func (p *Processor) processStreamingChunked(ctx context.Context, req *Request, jobDir, statusFileID string) error {
	p.updateStatus(ctx, statusFileID, req.JobID, StatusDownloading, 0, "", "")

	// Resolve direct media URLs for video+audio streams.
	videoURL, audioURL, err := p.resolveStreamURLs(ctx, req)
	if err != nil {
		return fmt.Errorf("resolve stream urls: %w", err)
	}
	audioLog := audioURL
	if len(audioLog) > 60 {
		audioLog = audioLog[:60]
	}
	if audioLog == "" {
		audioLog = "(muxed)"
	}
	log.Printf("[%s] resolved streams: video=%s audio=%s", req.JobID, videoURL[:min(60, len(videoURL))], audioLog)

	chunkSizeBytes := int64(req.ChunkSizeMB) * 1024 * 1024
	pattern := filepath.Join(jobDir, "chunk_%05d.ts")

	var args []string
	if audioURL == "" {
		// Muxed stream: single input, copy all streams.
		args = []string{
			"-y",
			"-i", videoURL,
			"-c", "copy",
			"-f", "segment",
			"-segment_format", "mpegts",
			"-segment_size", strconv.FormatInt(chunkSizeBytes, 10),
			"-reset_timestamps", "1",
			pattern,
		}
	} else {
		// Separate video + audio DASH streams.
		args = []string{
			"-y",
			"-i", videoURL,
			"-i", audioURL,
			"-map", "0:v:0",
			"-map", "1:a:0",
			"-c", "copy",
			"-f", "segment",
			"-segment_format", "mpegts",
			"-segment_size", strconv.FormatInt(chunkSizeBytes, 10),
			"-reset_timestamps", "1",
			pattern,
		}
	}
	log.Printf("[%s] ffmpeg segmenting start: segment_size=%dMB", req.JobID, req.ChunkSizeMB)
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)

	var ffmpegErrBuf strings.Builder
	cmd.Stderr = &ffmpegErrBuf

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("ffmpeg start: %w", err)
	}

	// Poll the job directory and upload completed chunks while ffmpeg runs.
	var uploaded []ChunkRef
	uploadedSet := map[int]bool{}
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	uploadChunks := func(done bool) {
		entries, _ := os.ReadDir(jobDir)
		var chunkPaths []string
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), "chunk_") && strings.HasSuffix(e.Name(), ".ts") {
				chunkPaths = append(chunkPaths, filepath.Join(jobDir, e.Name()))
			}
		}
		sort.Strings(chunkPaths)

		// When still running: upload all-but-last (last may still be written).
		limit := len(chunkPaths)
		if !done && limit > 0 {
			limit--
		}
		for i, path := range chunkPaths[:limit] {
			if uploadedSet[i] {
				continue
			}
			dur := probeChunkDuration(path)
			driveID, err := p.driveClient.UploadFile(ctx, p.chunkUploadFolder(), path, "video/mp2t")
			if err != nil {
				log.Printf("[%s] warn: upload chunk %d: %v", req.JobID, i, err)
				continue
			}
			uploadedSet[i] = true
			uploaded = append(uploaded, ChunkRef{Index: i, DriveFileID: driveID, DurationS: dur})
			log.Printf("[%s] uploaded chunk %d (%.1fs)", req.JobID, i, dur)
			p.updateChunkStatus(ctx, statusFileID, req.JobID, req.ChunkSizeMB, -1, uploaded, false)
		}
	}

	doneCh := make(chan error, 1)
	go func() { doneCh <- cmd.Wait() }()

loop:
	for {
		select {
		case <-ctx.Done():
			cmd.Process.Kill() //nolint:errcheck
			return ctx.Err()
		case ffErr := <-doneCh:
			if ffErr != nil {
				tail := ffmpegErrBuf.String()
				if len(tail) > 500 {
					tail = tail[len(tail)-500:]
				}
				return fmt.Errorf("ffmpeg: %w\n%s", ffErr, tail)
			}
			break loop
		case <-ticker.C:
			uploadChunks(false)
		}
	}

	// Upload any remaining chunks.
	uploadChunks(true)

	p.updateChunkStatus(ctx, statusFileID, req.JobID, req.ChunkSizeMB, len(uploaded), uploaded, true)
	log.Printf("[%s] streaming-chunk done: %d chunks", req.JobID, len(uploaded))
	return nil
}

// resolveStreamURLs calls yt-dlp --get-url for video and audio format strings.
// It tries each player client in order and falls back to looser format selectors on failure.
func (p *Processor) resolveStreamURLs(ctx context.Context, req *Request) (videoURL, audioURL string, err error) {
	videoFmts, audioFmt := ytdlpStreamFormats(req.Quality)

	playerClients := []string{
		"android_vr",
		"web",
		"tv_embedded,web",
		"", // yt-dlp default client selection
	}

	getURL := func(formats []string) (string, error) {
		var lastErr error
		for _, client := range playerClients {
			for _, format := range formats {
					args := []string{"--format", format, "--get-url", "--no-playlist"}
				if client != "" {
					args = append(args, "--extractor-args", "youtube:player_client="+client)
				}
				args = append(args, req.URL)
				cmd := exec.CommandContext(ctx, "yt-dlp", args...)
				out, cmdErr := cmd.Output()
				if cmdErr != nil {
					var exitErr *exec.ExitError
					if errors.As(cmdErr, &exitErr) {
						lastErr = fmt.Errorf("client=%s format=%s: %w\n%s", client, format, cmdErr, exitErr.Stderr)
					} else {
						lastErr = fmt.Errorf("client=%s format=%s: %w", client, format, cmdErr)
					}
					log.Printf("[%s] yt-dlp --get-url: %v", req.JobID, lastErr)
					continue
				}
				u := strings.TrimSpace(string(out))
				if u == "" {
					lastErr = fmt.Errorf("client=%s format=%s: empty output", client, format)
					continue
				}
				log.Printf("[%s] yt-dlp --get-url resolved (client=%s format=%s)", req.JobID, client, format)
				return u, nil
			}
		}
		return "", fmt.Errorf("yt-dlp --get-url: all attempts failed: %w", lastErr)
	}

	videoURL, err = getURL(videoFmts)
	if err != nil {
		return "", "", err
	}
	audioURL, err = getURL([]string{audioFmt})
	if err != nil {
		// Audio stream unavailable — videoURL may be a muxed stream already; signal with empty audioURL.
		log.Printf("[%s] no separate audio stream (%v) — treating video URL as muxed", req.JobID, err)
		audioURL = ""
	}
	return videoURL, audioURL, nil
}

// ytdlpStreamFormats returns video format selectors (preferred first) and the audio selector.
// The video list ends with muxed-format fallbacks for videos with no separate DASH streams.
func ytdlpStreamFormats(quality string) (videoFmts []string, audioFmt string) {
	switch quality {
	case "2160p":
		videoFmts = []string{
			"bestvideo[height<=2160][vcodec^=avc1]",
			"bestvideo[height<=2160]",
			"bestvideo",
			"best[height<=2160][protocol^=m3u8]",
			"best[height<=2160]",
			"best",
		}
	case "1440p":
		videoFmts = []string{
			"bestvideo[height<=1440][vcodec^=avc1]",
			"bestvideo[height<=1440]",
			"bestvideo",
			"best[height<=1440][protocol^=m3u8]",
			"best[height<=1440]",
			"best",
		}
	case "1080p":
		videoFmts = []string{
			"bestvideo[height<=1080][vcodec^=avc1]",
			"bestvideo[height<=1080]",
			"bestvideo",
			"best[height<=1080][protocol^=m3u8]",
			"best[height<=1080]",
			"best",
		}
	case "720p":
		videoFmts = []string{
			"bestvideo[height<=720][vcodec^=avc1]",
			"bestvideo[height<=720]",
			"bestvideo",
			"best[height<=720][protocol^=m3u8]",
			"best[height<=720]",
			"best",
		}
	case "480p":
		videoFmts = []string{"bestvideo[height<=480]", "bestvideo", "best[height<=480][protocol^=m3u8]", "best[height<=480]", "best"}
	case "360p":
		videoFmts = []string{"bestvideo[height<=360]", "bestvideo", "best[height<=360][protocol^=m3u8]", "best[height<=360]", "best"}
	default:
		videoFmts = []string{"bestvideo[vcodec^=avc1]", "bestvideo", "best[protocol^=m3u8]", "best"}
	}
	audioFmt = "bestaudio[acodec^=mp4a]/bestaudio"
	return
}

func (p *Processor) updateChunkStatus(ctx context.Context, fileID, jobID string, chunkSizeMB, totalChunks int, chunks []ChunkRef, done bool) {
	st := StatusChunking
	if done {
		st = StatusDone
	}
	total := totalChunks
	if total < 0 {
		total = 0 // unknown while streaming
	}
	s := &Status{
		JobID:       jobID,
		Status:      st,
		TotalChunks: total,
		ChunkSizeMB: chunkSizeMB,
		Chunks:      chunks,
		UpdatedAt:   now(),
	}
	if fileID == "" {
		p.driveClient.UploadJSON(ctx, p.statusFolder, "status-"+jobID+".json", s) //nolint:errcheck
		return
	}
	if err := p.driveClient.UpdateJSON(ctx, fileID, s); err != nil {
		log.Printf("[%s] warn: update chunk status: %v", jobID, err)
	}
}

func probeChunkDuration(path string) float64 {
	out, err := exec.Command("ffprobe", "-v", "quiet",
		"-show_entries", "format=duration",
		"-of", "csv=p=0", path).Output()
	if err != nil {
		return 0
	}
	d, _ := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	return d
}

func remuxFaststart(ctx context.Context, input string) (string, error) {
	out := strings.TrimSuffix(input, ".mp4") + "_fs.mp4"
	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", input,
		"-c", "copy", "-movflags", "+faststart", out)
	if b, err := cmd.CombinedOutput(); err != nil {
		return input, fmt.Errorf("%w: %s", err, b)
	}
	os.Remove(input)
	return out, nil
}

func ytdlpFormat(quality string) string {
	switch quality {
	case "2160p":
		return "bestvideo[height<=2160][vcodec^=avc1]+bestaudio[acodec^=mp4a]/bestvideo[height<=2160]+bestaudio/best"
	case "1440p":
		return "bestvideo[height<=1440][vcodec^=avc1]+bestaudio[acodec^=mp4a]/bestvideo[height<=1440]+bestaudio/best"
	case "1080p":
		return "bestvideo[height<=1080][vcodec^=avc1]+bestaudio[acodec^=mp4a]/bestvideo[height<=1080]+bestaudio/best"
	case "720p":
		return "bestvideo[height<=720][vcodec^=avc1]+bestaudio[acodec^=mp4a]/bestvideo[height<=720]+bestaudio/best"
	case "480p":
		return "bestvideo[height<=480]+bestaudio/best"
	case "360p":
		return "bestvideo[height<=360]+bestaudio/best"
	case "audio":
		return "bestaudio[ext=m4a]/bestaudio"
	default: // "best"
		return "bestvideo[vcodec^=avc1]+bestaudio[acodec^=mp4a]/bestvideo+bestaudio/best"
	}
}

func now() string { return time.Now().UTC().Format(time.RFC3339) }

func (p *Processor) createStatus(ctx context.Context, jobID, status string, progress int, driveFileID, errMsg string) (string, error) {
	s := &Status{
		JobID:       jobID,
		Status:      status,
		Progress:    progress,
		DriveFileID: driveFileID,
		Error:       errMsg,
		UpdatedAt:   now(),
	}
	return p.driveClient.UploadJSON(ctx, p.statusFolder, "status-"+jobID+".json", s)
}

func (p *Processor) updateStatus(ctx context.Context, fileID, jobID, status string, progress int, driveFileID, errMsg string) {
	s := &Status{
		JobID:       jobID,
		Status:      status,
		Progress:    progress,
		DriveFileID: driveFileID,
		Error:       errMsg,
		UpdatedAt:   now(),
	}
	if fileID == "" {
		// Best-effort create if initial creation failed
		p.driveClient.UploadJSON(ctx, p.statusFolder, "status-"+jobID+".json", s) //nolint:errcheck
		return
	}
	if err := p.driveClient.UpdateJSON(ctx, fileID, s); err != nil {
		log.Printf("[%s] warn: update status: %v", jobID, err)
	}
}

func (p *Processor) fail(ctx context.Context, jobID, msg string) {
	log.Printf("[%s] failed: %s", jobID, msg)
	p.driveClient.UploadJSON(ctx, p.statusFolder, "status-"+jobID+".json", &Status{ //nolint:errcheck
		JobID:     jobID,
		Status:    StatusFailed,
		Error:     msg,
		UpdatedAt: now(),
	})
}
