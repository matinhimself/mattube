package jobs

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
	httpsProxy    string // SOCKS5 proxy for yt-dlp
}

func NewProcessor(dc *drive.Client, outputFolder, statusFolder, downloadDir, httpsProxy string) *Processor {
	return &Processor{
		driveClient:  dc,
		outputFolder: outputFolder,
		statusFolder: statusFolder,
		downloadDir:  downloadDir,
		httpsProxy:   httpsProxy,
	}
}

func (p *Processor) Process(ctx context.Context, req *Request) {
	log.Printf("[%s] starting: %s quality=%s", req.JobID, req.URL, req.Quality)

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
	}
	if p.httpsProxy != "" {
		args = append(args, "--proxy", p.httpsProxy)
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
