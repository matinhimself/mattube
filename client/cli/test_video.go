package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/matinhimself/mattube/client/fronting"
)

// TestVideo fetches and pretty-prints all available data for a YouTube video ID,
// using the same YouTubeClient code the app uses.
func TestVideo(frontingIP, allowedSNI, videoID string) {
	if frontingIP == "" || allowedSNI == "" || videoID == "" {
		fatalf("usage: test-video <fronting-ip> <allowed-sni> <video-id>\nexample: test-video 216.239.38.120 www.google.com 6RVGfO45_XM")
	}

	yt := fronting.NewYouTubeClient(frontingIP, allowedSNI, os.Getenv("YOUTUBE_API_KEY"))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fmt.Printf("Testing video: %s\n", videoID)
	fmt.Println(strings.Repeat("─", 60))

	// Fetch info, formats, related, and captions in parallel
	type infoResult struct {
		info     *fronting.VideoInfo
		formats  []fronting.Format
		related  []fronting.SearchResult
		captions []fronting.CaptionSegment
		err      [4]error
	}
	res := &infoResult{}
	var wg sync.WaitGroup

	wg.Add(4)
	go func() {
		defer wg.Done()
		res.info, res.err[0] = yt.GetVideoInfo(ctx, videoID)
	}()
	go func() {
		defer wg.Done()
		res.formats, res.err[1] = yt.GetFormats(ctx, videoID)
	}()
	go func() {
		defer wg.Done()
		res.related, res.err[2] = yt.GetRelatedVideos(ctx, videoID)
	}()
	go func() {
		defer wg.Done()
		res.captions, res.err[3] = yt.GetCaptions(ctx, videoID, "")
	}()
	wg.Wait()

	// ── Metadata ────────────────────────────────────────────────────
	fmt.Println("\n▶  METADATA")
	if res.err[0] != nil {
		fmt.Printf("   ERROR: %v\n", res.err[0])
	} else if res.info != nil {
		i := res.info
		fmt.Printf("   Title:    %s\n", i.Title)
		fmt.Printf("   Channel:  %s  (id: %s)\n", i.ChannelName, i.ChannelID)
		fmt.Printf("   Duration: %s\n", fmtDur(i.Duration))
		fmt.Printf("   Views:    %s\n", i.ViewCount)
		fmt.Printf("   Thumb:    %s\n", i.Thumbnail)
		if i.Description != "" {
			desc := i.Description
			lines := strings.Split(desc, "\n")
			preview := strings.Join(lines[:min(5, len(lines))], "\n   ")
			if len(lines) > 5 {
				preview += fmt.Sprintf("\n   … (%d more lines)", len(lines)-5)
			}
			fmt.Printf("   Desc:\n   %s\n", preview)
		}
	}

	// ── Formats ─────────────────────────────────────────────────────
	fmt.Println("\n▶  AVAILABLE FORMATS")
	if res.err[1] != nil {
		fmt.Printf("   ERROR: %v\n", res.err[1])
	} else if len(res.formats) == 0 {
		fmt.Println("   (streamingData not returned — region-locked or sign-in required)")
		fmt.Println("   yt-dlp will still work with its own auth/cookie mechanism.")
	} else {
		fmt.Printf("   %-10s  %-8s  %-10s  %s\n", "QUALITY", "SIZE", "BITRATE", "MIME")
		fmt.Println("   " + strings.Repeat("─", 55))
		for _, f := range res.formats {
			size := ""
			if f.Width > 0 {
				size = fmt.Sprintf("%dx%d", f.Width, f.Height)
			}
			bitrate := ""
			if f.Bitrate > 0 {
				bitrate = fmt.Sprintf("%d kbps", f.Bitrate/1000)
			}
			label := f.QualityLabel
			if f.AudioOnly {
				label = "(audio)"
			}
			fmt.Printf("   %-10s  %-8s  %-10s  %s\n", label, size, bitrate, shortMime(f.MimeType))
		}
	}

	// ── Related videos ───────────────────────────────────────────────
	fmt.Println("\n▶  RELATED VIDEOS")
	if res.err[2] != nil {
		fmt.Printf("   ERROR: %v\n", res.err[2])
	} else if len(res.related) == 0 {
		fmt.Println("   (none returned)")
	} else {
		for i, r := range res.related {
			if i >= 10 {
				fmt.Printf("   … and %d more\n", len(res.related)-10)
				break
			}
			dur := ""
			if r.Duration != "" {
				dur = "  [" + r.Duration + "]"
			}
			fmt.Printf("   %2d. %s%s\n       %-40s  %s\n",
				i+1, truncate(r.Title, 50), dur, r.ChannelName, r.VideoID)
		}
	}

	// ── Captions ─────────────────────────────────────────────────
	fmt.Println("\n▶  CAPTIONS")
	if res.err[3] != nil {
		fmt.Printf("   ERROR: %v\n", res.err[3])
	} else if len(res.captions) == 0 {
		fmt.Println("   (none available)")
	} else {
		fmt.Printf("   %d segments — first 5:\n", len(res.captions))
		for i, seg := range res.captions {
			if i >= 5 {
				fmt.Printf("   … and %d more\n", len(res.captions)-5)
				break
			}
			fmt.Printf("   [%s]  %s\n", fmtMs(seg.StartMs), truncate(seg.Text, 70))
		}
	}

	fmt.Println()
}

func fmtMs(ms int) string {
	s := ms / 1000
	h, m, sec := s/3600, (s%3600)/60, s%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, sec)
	}
	return fmt.Sprintf("%d:%02d", m, sec)
}

func fmtDur(s int) string {
	if s == 0 {
		return "—"
	}
	h, m, sec := s/3600, (s%3600)/60, s%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, sec)
	}
	return fmt.Sprintf("%d:%02d", m, sec)
}

func shortMime(m string) string {
	// "video/mp4; codecs=\"avc1.640028\"" → "video/mp4 avc1"
	parts := strings.SplitN(m, ";", 2)
	result := strings.TrimSpace(parts[0])
	if len(parts) > 1 {
		codec := parts[1]
		if i := strings.Index(codec, "\""); i >= 0 {
			codec = codec[i+1:]
			if j := strings.Index(codec, "\""); j >= 0 {
				codec = codec[:j]
			}
			// shorten avc1.640028 → avc1
			if dot := strings.Index(codec, "."); dot >= 0 {
				codec = codec[:dot]
			}
			result += " " + codec
		}
	}
	return result
}

func truncate(s string, n int) string {
	if len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
