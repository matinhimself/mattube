package fronting

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

const (
	driveAPIBase    = "https://www.googleapis.com/drive/v3"
	driveUploadBase = "https://www.googleapis.com/upload/drive/v3"
)

// DriveClient performs Drive operations via the fronting transport.
type DriveClient struct {
	http *http.Client
	mu   sync.RWMutex
	ts   oauth2.TokenSource
}

// staticTokenSource wraps a fixed access token with no refresh capability.
type staticTokenSource struct{ token string }

func (s *staticTokenSource) Token() (*oauth2.Token, error) {
	if s.token == "" {
		return nil, fmt.Errorf("no Drive token configured")
	}
	return &oauth2.Token{AccessToken: s.token}, nil
}

// PersistingTokenSource wraps a base TokenSource and writes refreshed tokens to disk.
type PersistingTokenSource struct {
	mu   sync.Mutex
	Base oauth2.TokenSource
	File string
	last string
}

func NewPersistingTokenSource(base oauth2.TokenSource, file, lastToken string) *PersistingTokenSource {
	return &PersistingTokenSource{Base: base, File: file, last: lastToken}
}

func (p *PersistingTokenSource) Token() (*oauth2.Token, error) {
	tok, err := p.Base.Token()
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if tok.AccessToken != p.last {
		if b, err := json.MarshalIndent(tok, "", "  "); err == nil {
			os.WriteFile(p.File, b, 0600) //nolint:errcheck
		}
		p.last = tok.AccessToken
	}
	return tok, nil
}

// PersistingDBTokenSource wraps a base TokenSource and writes refreshed tokens to the database.
type PersistingDBTokenSource struct {
	mu   sync.Mutex
	Base oauth2.TokenSource
	db   *sql.DB
	last string
}

func NewPersistingDBTokenSource(base oauth2.TokenSource, database *sql.DB, lastToken string) *PersistingDBTokenSource {
	return &PersistingDBTokenSource{Base: base, db: database, last: lastToken}
}

func (p *PersistingDBTokenSource) Token() (*oauth2.Token, error) {
	tok, err := p.Base.Token()
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if tok.AccessToken != p.last {
		if b, err := json.MarshalIndent(tok, "", "  "); err == nil {
			p.db.Exec( //nolint:errcheck
				`INSERT INTO settings(key,value,updated_at) VALUES('drive_token',?,datetime('now'))
				 ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at=excluded.updated_at`,
				string(b),
			)
		}
		p.last = tok.AccessToken
	}
	return tok, nil
}

func NewDriveClient(frontingIP, allowedSNI, accessToken string) *DriveClient {
	c := NewClient(frontingIP, allowedSNI)
	c.Timeout = 5 * time.Minute
	return &DriveClient{
		http: c,
		ts:   &staticTokenSource{token: accessToken},
	}
}

// NewDriveClientWithSource creates a DriveClient that auto-refreshes tokens via ts.
func NewDriveClientWithSource(frontingIP, allowedSNI string, ts oauth2.TokenSource) *DriveClient {
	c := NewClient(frontingIP, allowedSNI)
	c.Timeout = 5 * time.Minute
	return &DriveClient{
		http: c,
		ts:   ts,
	}
}

// SetTokenSource swaps the token source at runtime (e.g. after OAuth callback).
func (d *DriveClient) SetTokenSource(ts oauth2.TokenSource) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.ts = ts
}

// IsConnected reports whether a valid token is available.
func (d *DriveClient) IsConnected() bool {
	d.mu.RLock()
	ts := d.ts
	d.mu.RUnlock()
	tok, err := ts.Token()
	return err == nil && tok != nil && tok.AccessToken != ""
}

func (d *DriveClient) bearer() string {
	d.mu.RLock()
	ts := d.ts
	d.mu.RUnlock()
	tok, err := ts.Token()
	if err != nil || tok == nil {
		return ""
	}
	return tok.AccessToken
}

// UploadJSON creates a new JSON file in folderID. Returns the Drive file ID.
func (d *DriveClient) UploadJSON(ctx context.Context, folderID, name string, src any) (string, error) {
	body, err := json.Marshal(src)
	if err != nil {
		return "", err
	}

	// Multipart: metadata + media
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	// Part 1: metadata
	metaHeader := textproto.MIMEHeader{}
	metaHeader.Set("Content-Type", "application/json; charset=UTF-8")
	metaPart, _ := mw.CreatePart(metaHeader)
	meta, _ := json.Marshal(map[string]any{"name": name, "parents": []string{folderID}})
	metaPart.Write(meta) //nolint:errcheck

	// Part 2: media
	mediaHeader := textproto.MIMEHeader{}
	mediaHeader.Set("Content-Type", "application/json")
	mediaPart, _ := mw.CreatePart(mediaHeader)
	mediaPart.Write(body) //nolint:errcheck
	mw.Close()

	req, err := http.NewRequestWithContext(ctx, "POST",
		driveUploadBase+"/files?uploadType=multipart&fields=id",
		&buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "multipart/related; boundary="+mw.Boundary())
	SetBearer(req, d.bearer())

	resp, err := d.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("drive upload json: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("drive upload json %d: %s", resp.StatusCode, b)
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.ID, nil
}

// DownloadJSON fetches fileID and unmarshals into dst.
func (d *DriveClient) DownloadJSON(ctx context.Context, fileID string, dst any) error {
	r, _, err := d.Download(ctx, fileID)
	if err != nil {
		return err
	}
	defer r.Close()
	return json.NewDecoder(r).Decode(dst)
}

// UpdateJSON replaces the content of an existing Drive file with src serialised as JSON.
func (d *DriveClient) UpdateJSON(ctx context.Context, fileID string, src any) error {
	b, err := json.Marshal(src)
	if err != nil {
		return err
	}
	u := driveUploadBase + "/files/" + fileID + "?uploadType=media"
	req, err := http.NewRequestWithContext(ctx, "PATCH", u, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	SetBearer(req, d.bearer())
	resp, err := d.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("drive update %d: %s", resp.StatusCode, body)
	}
	return nil
}

// Delete permanently deletes a Drive file.
func (d *DriveClient) Delete(ctx context.Context, fileID string) error {
	u := driveAPIBase + "/files/" + fileID
	req, err := http.NewRequestWithContext(ctx, "DELETE", u, nil)
	if err != nil {
		return err
	}
	SetBearer(req, d.bearer())
	resp, err := d.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 204 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("drive delete %d: %s", resp.StatusCode, body)
	}
	return nil
}

// ListByPrefix lists files in folderID whose names start with prefix.
func (d *DriveClient) ListByPrefix(ctx context.Context, folderID, prefix string) ([]struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}, error) {
	q := fmt.Sprintf("'%s' in parents and name contains '%s' and trashed=false", folderID, prefix)
	u := driveAPIBase + "/files?q=" + urlEncode(q) + "&fields=files(id,name)"

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	SetBearer(req, d.bearer())

	resp, err := d.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Files []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"files"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Files, nil
}

// Download returns a ReadCloser for a Drive file's content, following redirects and
// handling Google's virus-scan confirmation page for large files.
func (d *DriveClient) Download(ctx context.Context, fileID string) (io.ReadCloser, int64, error) {
	// Try authenticated API endpoint first
	apiURL := fmt.Sprintf("%s/files/%s?alt=media", driveAPIBase, fileID)
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, 0, err
	}
	SetBearer(req, d.bearer())

	resp, err := d.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("drive download api: %w", err)
	}

	// If auth fails fall back to public download URL
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		resp.Body.Close()
		return d.downloadPublic(ctx, fileID)
	}

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, 0, fmt.Errorf("drive download %d: %s", resp.StatusCode, b)
	}

	return resp.Body, resp.ContentLength, nil
}

func (d *DriveClient) downloadPublic(ctx context.Context, fileID string) (io.ReadCloser, int64, error) {
	u := "https://drive.google.com/uc?id=" + fileID + "&export=download"
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, 0, err
	}

	resp, err := d.http.Do(req)
	if err != nil {
		return nil, 0, err
	}

	ct := resp.Header.Get("Content-Type")
	// If we get HTML it's likely the virus-scan confirmation page
	if strings.Contains(ct, "text/html") {
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, 0, err
		}
		confirmURL, err := extractConfirmURL(string(body), fileID)
		if err != nil {
			return nil, 0, fmt.Errorf("drive confirm page: %w", err)
		}
		req2, _ := http.NewRequestWithContext(ctx, "GET", confirmURL, nil)
		resp2, err := d.http.Do(req2)
		if err != nil {
			return nil, 0, err
		}
		return resp2.Body, resp2.ContentLength, nil
	}

	return resp.Body, resp.ContentLength, nil
}

// extractConfirmURL parses the virus-scan HTML form or legacy confirm token.
// Ported from telegram-crawler/client/drive_fronting.py _extract_confirm_url.
func extractConfirmURL(html, fileID string) (string, error) {
	// New form-based confirmation: <form action="..." method="GET"> with hidden inputs
	actionRe := regexp.MustCompile(`<form[^>]+action="([^"]+)"`)
	hiddenRe := regexp.MustCompile(`<input[^>]+name="([^"]+)"[^>]+value="([^"]+)"`)

	if m := actionRe.FindStringSubmatch(html); m != nil {
		action := strings.ReplaceAll(m[1], "&amp;", "&")
		params := map[string]string{}
		for _, hm := range hiddenRe.FindAllStringSubmatch(html, -1) {
			params[hm[1]] = strings.ReplaceAll(hm[2], "&amp;", "&")
		}
		var sb strings.Builder
		sb.WriteString(action)
		first := true
		for k, v := range params {
			if first {
				sb.WriteString("?")
				first = false
			} else {
				sb.WriteString("&")
			}
			sb.WriteString(k + "=" + v)
		}
		return sb.String(), nil
	}

	// Legacy: &confirm=<token>
	legacyRe := regexp.MustCompile(`confirm=([0-9A-Za-z_-]+)`)
	if m := legacyRe.FindStringSubmatch(html); m != nil {
		return "https://drive.google.com/uc?id=" + fileID + "&export=download&confirm=" + m[1], nil
	}

	return "", fmt.Errorf("no confirmation URL found in page")
}

func urlEncode(s string) string {
	var b strings.Builder
	for _, c := range s {
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9',
			c == '-', c == '_', c == '.', c == '~':
			b.WriteRune(c)
		default:
			b.WriteString(fmt.Sprintf("%%%02X", c))
		}
	}
	return b.String()
}
