package fronting

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	yt "github.com/kkdai/youtube/v2"
)

const innerTubeBase = "https://www.youtube.com/youtubei/v1"

// YouTubeClient fetches YouTube metadata via the fronting transport.
// GetVideoInfo and GetFormats use kkdai/youtube (pure Go, handles bot-detection).
// Search, Related, Channel use the InnerTube API directly.
type YouTubeClient struct {
	http        *http.Client
	kkdai       yt.Client
	visitorData string
}

func NewYouTubeClient(frontingIP, allowedSNI, _ string) *YouTubeClient {
	hc := NewClient(frontingIP, allowedSNI)
	return &YouTubeClient{
		http:  hc,
		kkdai: yt.Client{HTTPClient: hc},
	}
}

// ── Video metadata & formats (kkdai/youtube) ─────────────────────────────────

// VideoInfo holds extracted metadata for a YouTube video.
type VideoInfo struct {
	VideoID     string `json:"video_id"`
	Title       string `json:"title"`
	ChannelID   string `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	Duration    int    `json:"duration"`
	ViewCount   string `json:"view_count"`
	Description string `json:"description"`
	Thumbnail   string `json:"thumbnail"`
}

// Format describes one available video stream.
type Format struct {
	QualityLabel string `json:"quality_label"`
	MimeType     string `json:"mime_type"`
	Bitrate      int    `json:"bitrate"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	AudioOnly    bool   `json:"audio_only"`
}

func (y *YouTubeClient) GetVideoInfo(ctx context.Context, videoID string) (*VideoInfo, error) {
	video, err := y.kkdai.GetVideoContext(ctx, videoID)
	if err != nil {
		return nil, err
	}
	vi := &VideoInfo{
		VideoID:     videoID,
		Title:       video.Title,
		ChannelID:   video.ChannelID,
		ChannelName: video.Author,
		Duration:    int(video.Duration.Seconds()),
		Description: video.Description,
	}
	if video.Views > 0 {
		vi.ViewCount = fmt.Sprintf("%d", video.Views)
	}
	// Best thumbnail: prefer maxres, fall back to hqdefault
	for _, q := range []string{"maxresdefault", "sddefault", "hqdefault"} {
		vi.Thumbnail = fmt.Sprintf("https://i.ytimg.com/vi/%s/%s.jpg", videoID, q)
		break
	}
	return vi, nil
}

func (y *YouTubeClient) GetFormats(ctx context.Context, videoID string) ([]Format, error) {
	video, err := y.kkdai.GetVideoContext(ctx, videoID)
	if err != nil {
		return nil, err
	}

	seen := map[int]bool{}
	var out []Format
	for _, f := range video.Formats {
		audioOnly := f.Height == 0
		if !audioOnly && seen[f.Height] {
			continue
		}
		if !audioOnly {
			seen[f.Height] = true
		}
		out = append(out, Format{
			QualityLabel: f.QualityLabel,
			MimeType:     f.MimeType,
			Bitrate:      f.Bitrate,
			Width:        f.Width,
			Height:       f.Height,
			AudioOnly:    audioOnly,
		})
	}
	return out, nil
}

// ── Captions ─────────────────────────────────────────────────────────────────

// CaptionSegment is one timed caption line.
type CaptionSegment struct {
	Text       string `json:"text"`
	StartMs    int    `json:"start_ms"`
	DurationMs int    `json:"duration_ms"`
	OffsetText string `json:"offset_text"`
}

// GetCaptions returns transcript segments for the given video and language code.
// Pass lang="" to use the video's default language (first available track).
func (y *YouTubeClient) GetCaptions(ctx context.Context, videoID, lang string) ([]CaptionSegment, error) {
	video, err := y.kkdai.GetVideoContext(ctx, videoID)
	if err != nil {
		return nil, err
	}
	if len(video.CaptionTracks) == 0 {
		return nil, nil
	}

	// Pick the best matching track.
	track := video.CaptionTracks[0]
	if lang != "" {
		for _, t := range video.CaptionTracks {
			if strings.HasPrefix(t.LanguageCode, lang) {
				track = t
				break
			}
		}
	}

	// Fetch timedtext as JSON3. BaseURL already has fmt=srv3; replace it.
	u := strings.ReplaceAll(track.BaseURL, "fmt=srv3", "fmt=json3")
	if !strings.Contains(u, "fmt=") {
		u += "&fmt=json3"
	}
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := y.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("captions: status %d", resp.StatusCode)
	}

	var j3 struct {
		Events []struct {
			TStartMs    int `json:"tStartMs"`
			DDurationMs int `json:"dDurationMs"`
			Segs        []struct {
				Utf8 string `json:"utf8"`
			} `json:"segs"`
		} `json:"events"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&j3); err != nil {
		return nil, err
	}

	var out []CaptionSegment
	for _, ev := range j3.Events {
		var text strings.Builder
		for _, seg := range ev.Segs {
			text.WriteString(seg.Utf8)
		}
		t := strings.TrimSpace(text.String())
		if t == "" || t == "\n" {
			continue
		}
		out = append(out, CaptionSegment{
			Text:       t,
			StartMs:    ev.TStartMs,
			DurationMs: ev.DDurationMs,
		})
	}
	return out, nil
}

// ── InnerTube (search, related, channel) ─────────────────────────────────────

// clientVariant describes one InnerTube client type for non-player endpoints.
type clientVariant struct {
	name      string
	clientNum string
	apiKey    string
	version   string
	userAgent string
	extra     map[string]any
}

var knownClients = []clientVariant{
	{
		name: "WEB", clientNum: "1",
		apiKey:    "AIzaSyAO_FJ2SlqU8Q4STEHLGCilw_Y9_11qcW8",
		version:   "2.20231219.01.00",
		userAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
	},
	{
		name: "MWEB", clientNum: "2",
		apiKey:    "AIzaSyAO_FJ2SlqU8Q4STEHLGCilw_Y9_11qcW8",
		version:   "2.20231219.01.00",
		userAgent: "Mozilla/5.0 (Linux; Android 11; Pixel 5) AppleWebKit/537.36",
	},
	{
		name: "TVHTML5", clientNum: "7",
		apiKey:    "AIzaSyDCU8hByM-4DrUqRex-fFmPMQ_Rp7GQZTU",
		version:   "7.20231129.0.0",
		userAgent: "Mozilla/5.0 (SMART-TV; LINUX; Tizen 6.0) AppleWebKit/538.1",
	},
}

func (y *YouTubeClient) ensureVisitorData(ctx context.Context) {
	if y.visitorData != "" {
		return
	}
	req, err := http.NewRequestWithContext(ctx, "GET", "https://www.youtube.com/", nil)
	if err != nil {
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	resp, err := y.http.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if i := bytes.Index(body, []byte(`"VISITOR_DATA":"`)); i >= 0 {
		start := i + 16
		if j := bytes.IndexByte(body[start:], '"'); j >= 0 {
			y.visitorData = string(body[start : start+j])
		}
	}
}

func (y *YouTubeClient) post(ctx context.Context, endpoint string, body map[string]any) (map[string]any, error) {
	y.ensureVisitorData(ctx)
	var lastErr error
	for _, c := range knownClients {
		result, err := y.postWith(ctx, endpoint, body, c)
		if err == nil {
			return result, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("innertube %s: all clients failed: %w", endpoint, lastErr)
}

func (y *YouTubeClient) postWith(ctx context.Context, endpoint string, body map[string]any, c clientVariant) (map[string]any, error) {
	clientCtx := map[string]any{
		"clientName":    c.name,
		"clientVersion": c.version,
		"hl":            "en",
		"gl":            "US",
	}
	if y.visitorData != "" {
		clientCtx["visitorData"] = y.visitorData
	}
	for k, v := range c.extra {
		clientCtx[k] = v
	}

	payload := make(map[string]any, len(body)+1)
	for k, v := range body {
		payload[k] = v
	}
	payload["context"] = map[string]any{"client": clientCtx}

	b, _ := json.Marshal(payload)
	u := innerTubeBase + "/" + endpoint + "?key=" + c.apiKey + "&prettyPrint=false"
	req, err := http.NewRequestWithContext(ctx, "POST", u, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-YouTube-Client-Name", c.clientNum)
	req.Header.Set("X-YouTube-Client-Version", c.version)
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Origin", "https://www.youtube.com")
	if y.visitorData != "" {
		req.Header.Set("X-Goog-Visitor-Id", y.visitorData)
	}

	resp, err := y.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		rb, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("client %s status %d: %s", c.name, resp.StatusCode, rb)
	}
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

// ── Search ────────────────────────────────────────────────────────────────────

// SearchResult is one item in a YouTube search or related-videos response.
type SearchResult struct {
	VideoID     string `json:"video_id"`
	Title       string `json:"title"`
	ChannelID   string `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	Duration    string `json:"duration"`
	ViewCount   string `json:"view_count"`
	Thumbnail   string `json:"thumbnail"`
}

func (y *YouTubeClient) Search(ctx context.Context, query string, n int) ([]SearchResult, error) {
	data, err := y.post(ctx, "search", map[string]any{"query": query})
	if err != nil {
		return nil, err
	}
	var results []SearchResult
	// Try twoColumn layout (WEB client) first, fall back to direct sectionListRenderer.
	contents := walkPath(data, "contents", "twoColumnSearchResultsRenderer",
		"primaryContents", "sectionListRenderer", "contents")
	if contents == nil {
		contents = walkPath(data, "contents", "sectionListRenderer", "contents")
	}
	for _, section := range asList(contents) {
		items := walkPath(section, "itemSectionRenderer", "contents")
		for _, item := range asList(items) {
			if vr, ok := item.(map[string]any)["videoRenderer"].(map[string]any); ok {
				sr := SearchResult{
					VideoID:     str(vr["videoId"]),
					Title:       runText(vr["title"]),
					ChannelID:   str(walkPath(vr, "ownerText", "runs", 0, "navigationEndpoint", "browseEndpoint", "browseId")),
					ChannelName: runText(vr["ownerText"]),
					Duration:    str(walkPath(vr, "lengthText", "simpleText")),
					ViewCount:   str(walkPath(vr, "viewCountText", "simpleText")),
				}
				if thumbs, ok := vr["thumbnail"].(map[string]any); ok {
					if list := asList(thumbs["thumbnails"]); len(list) > 0 {
						sr.Thumbnail = str(asMap(list[0])["url"])
					}
				}
				if sr.VideoID != "" {
					results = append(results, sr)
					if len(results) >= n {
						return results, nil
					}
				}
			}
		}
	}
	return results, nil
}

// ── Related videos ────────────────────────────────────────────────────────────

func (y *YouTubeClient) GetRelatedVideos(ctx context.Context, videoID string) ([]SearchResult, error) {
	data, err := y.post(ctx, "next", map[string]any{"videoId": videoID})
	if err != nil {
		return nil, err
	}
	secondary := walkPath(data, "contents", "twoColumnWatchNextResults",
		"secondaryResults", "secondaryResults", "results")
	var results []SearchResult
	for _, item := range asList(secondary) {
		vr, ok := asMap(item)["compactVideoRenderer"].(map[string]any)
		if !ok {
			continue
		}
		sr := SearchResult{
			VideoID:     str(vr["videoId"]),
			Title:       runText(vr["title"]),
			ChannelName: runText(vr["longBylineText"]),
			Duration:    str(walkPath(vr, "lengthText", "simpleText")),
			ViewCount:   str(walkPath(vr, "viewCountText", "simpleText")),
		}
		if sr.ChannelName == "" {
			sr.ChannelName = runText(vr["shortBylineText"])
		}
		if thumbs, ok := vr["thumbnail"].(map[string]any); ok {
			if list := asList(thumbs["thumbnails"]); len(list) > 0 {
				sr.Thumbnail = str(asMap(list[len(list)-1])["url"])
			}
		}
		if sr.VideoID != "" {
			results = append(results, sr)
		}
	}
	return results, nil
}

// ── Channel ───────────────────────────────────────────────────────────────────

// ChannelInfo holds metadata for a YouTube channel.
type ChannelInfo struct {
	ChannelID   string `json:"channel_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Avatar      string `json:"avatar"`
	Subscribers string `json:"subscribers"`
}

func (y *YouTubeClient) GetChannel(ctx context.Context, channelID string) (*ChannelInfo, error) {
	data, err := y.post(ctx, "browse", map[string]any{"browseId": channelID})
	if err != nil {
		return nil, err
	}
	ci := &ChannelInfo{ChannelID: channelID}
	if meta, ok := data["metadata"].(map[string]any); ok {
		if cm, ok := meta["channelMetadataRenderer"].(map[string]any); ok {
			ci.Name = str(cm["title"])
			ci.Description = str(cm["description"])
			if list := asList(walkPath(cm, "avatar", "thumbnails")); len(list) > 0 {
				ci.Avatar = str(asMap(list[len(list)-1])["url"])
			}
		}
	}
	return ci, nil
}

func (y *YouTubeClient) GetChannelVideos(ctx context.Context, channelID string) ([]SearchResult, error) {
	data, err := y.post(ctx, "browse", map[string]any{
		"browseId": channelID,
		"params":   "EgZ2aWRlb3PyBgQKAjoA",
	})
	if err != nil {
		return nil, err
	}
	var results []SearchResult
	tabs := walkPath(data, "contents", "twoColumnBrowseResultsRenderer", "tabs")
	for _, tab := range asList(tabs) {
		for _, item := range asList(walkPath(tab, "tabRenderer", "content", "richGridRenderer", "contents")) {
			vi, ok := asMap(walkPath(item, "richItemRenderer", "content"))["videoRenderer"].(map[string]any)
			if !ok {
				continue
			}
			sr := SearchResult{
				VideoID:  str(vi["videoId"]),
				Title:    runText(vi["title"]),
				Duration: str(walkPath(vi, "lengthText", "simpleText")),
			}
			if thumbs, ok := vi["thumbnail"].(map[string]any); ok {
				if list := asList(thumbs["thumbnails"]); len(list) > 0 {
					sr.Thumbnail = str(asMap(list[0])["url"])
				}
			}
			if sr.VideoID != "" {
				results = append(results, sr)
			}
		}
	}
	return results, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func str(v any) string {
	s, _ := v.(string)
	return s
}

func runText(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	if runs, ok := m["runs"].([]any); ok && len(runs) > 0 {
		return str(asMap(runs[0])["text"])
	}
	return str(m["simpleText"])
}

func asList(v any) []any {
	l, _ := v.([]any)
	return l
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func walkPath(v any, keys ...any) any {
	cur := v
	for _, k := range keys {
		if cur == nil {
			return nil
		}
		switch key := k.(type) {
		case string:
			m, ok := cur.(map[string]any)
			if !ok {
				return nil
			}
			cur = m[key]
		case int:
			l, ok := cur.([]any)
			if !ok || key >= len(l) {
				return nil
			}
			cur = l[key]
		}
	}
	return cur
}

// strings import kept for InnerTube search parsing
var _ = strings.Contains
