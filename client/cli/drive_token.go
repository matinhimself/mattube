package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const driveScope = "https://www.googleapis.com/auth/drive.file"

// GetDriveToken runs a local OAuth2 callback flow for Google Drive.
// credentialsFile: path to OAuth client credentials JSON (downloaded from GCP console).
// tokenOutFile: where to write the resulting token JSON (used as DRIVE_ACCESS_TOKEN source).
func GetDriveToken(credentialsFile, tokenOutFile string) {
	if credentialsFile == "" {
		credentialsFile = "credentials.json"
	}
	if tokenOutFile == "" {
		tokenOutFile = "drive_token.json"
	}

	creds, err := os.ReadFile(credentialsFile)
	if err != nil {
		fatalf("read credentials: %v\n\nDownload OAuth 2.0 client credentials from:\nhttps://console.cloud.google.com/apis/credentials\nChoose 'Desktop app' type and save as credentials.json", err)
	}

	cfg, err := google.ConfigFromJSON(creds, driveScope)
	if err != nil {
		fatalf("parse credentials: %v", err)
	}

	// Start local callback server on a random port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fatalf("start callback server: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	cfg.RedirectURL = fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	srv := &http.Server{}
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			errCh <- fmt.Errorf("no code in callback: %s", r.URL.RawQuery)
			http.Error(w, "no code received", http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, "<html><body><h2>Authorization successful!</h2><p>You can close this tab.</p></body></html>")
		codeCh <- code
	})
	srv.Handler = mux

	go func() {
		if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	authURL := cfg.AuthCodeURL("state", oauth2.AccessTypeOffline)
	fmt.Println("Opening browser for Google Drive authorization...")
	fmt.Println()
	fmt.Println("If the browser doesn't open, visit this URL manually:")
	fmt.Println()
	fmt.Println(" ", authURL)
	fmt.Println()
	openBrowser(authURL)

	go func() {
		fmt.Println("Or paste the redirect URL (or just the code) here for remote connections:")
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			if c := extractAuthCode(scanner.Text()); c != "" {
				codeCh <- c
			}
		}
	}()

	fmt.Println("Waiting for authorization callback...")

	var code string
	select {
	case code = <-codeCh:
	case err := <-errCh:
		fatalf("callback error: %v", err)
	case <-time.After(5 * time.Minute):
		fatalf("timed out waiting for authorization")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	srv.Shutdown(ctx) //nolint:errcheck

	token, err := cfg.Exchange(ctx, code)
	if err != nil {
		fatalf("exchange code: %v", err)
	}

	// Write full token JSON (includes refresh_token for auto-renewal)
	tokenJSON, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		fatalf("marshal token: %v", err)
	}
	if err := os.WriteFile(tokenOutFile, tokenJSON, 0600); err != nil {
		fatalf("write token file: %v", err)
	}

	fmt.Printf("\nToken saved to: %s\n", tokenOutFile)
	fmt.Printf("Access token:   %s\n\n", token.AccessToken)
	fmt.Println("Set this in your environment:")
	fmt.Printf("  export DRIVE_ACCESS_TOKEN=%s\n\n", token.AccessToken)
	fmt.Println("Or load from file automatically — see LoadTokenFromFile().")
}

// LoadTokenFromFile reads a saved token JSON and returns a valid access token,
// refreshing it if expired. Uses the credentials file for refresh.
func LoadTokenFromFile(credentialsFile, tokenFile string) (string, error) {
	if credentialsFile == "" {
		credentialsFile = "credentials.json"
	}
	if tokenFile == "" {
		tokenFile = "drive_token.json"
	}

	creds, err := os.ReadFile(credentialsFile)
	if err != nil {
		return "", fmt.Errorf("read credentials: %w", err)
	}
	cfg, err := google.ConfigFromJSON(creds, driveScope)
	if err != nil {
		return "", fmt.Errorf("parse credentials: %w", err)
	}

	tokenJSON, err := os.ReadFile(tokenFile)
	if err != nil {
		return "", fmt.Errorf("read token file: %w (run 'get-drive-token' first)", err)
	}
	var token oauth2.Token
	if err := json.Unmarshal(tokenJSON, &token); err != nil {
		return "", fmt.Errorf("parse token file: %w", err)
	}

	// Use oauth2 TokenSource for auto-refresh
	ts := cfg.TokenSource(context.Background(), &token)
	fresh, err := ts.Token()
	if err != nil {
		return "", fmt.Errorf("refresh token: %w", err)
	}

	// Persist refreshed token if it changed
	if fresh.AccessToken != token.AccessToken {
		if b, err := json.MarshalIndent(fresh, "", "  "); err == nil {
			os.WriteFile(tokenFile, b, 0600) //nolint:errcheck
		}
	}

	return fresh.AccessToken, nil
}

// PrintTokenFromFile prints a fresh access token to stdout.
func PrintTokenFromFile(credentialsFile, tokenFile string) {
	token, err := LoadTokenFromFile(credentialsFile, tokenFile)
	if err != nil {
		fatalf("%v", err)
	}
	fmt.Println(token)
}

// extractAuthCode pulls the `code` query parameter out of a pasted redirect URL,
// or returns the input as-is if it looks like a raw code.
func extractAuthCode(input string) string {
	s := strings.TrimSpace(input)
	if s == "" {
		return ""
	}
	if u, err := url.Parse(s); err == nil && u.Scheme != "" {
		if c := u.Query().Get("code"); c != "" {
			return c
		}
	}
	if i := strings.Index(s, "code="); i >= 0 {
		rest := s[i+len("code="):]
		if j := strings.IndexAny(rest, "& "); j >= 0 {
			rest = rest[:j]
		}
		if v, err := url.QueryUnescape(rest); err == nil {
			return v
		}
		return rest
	}
	return s
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return
	}
	cmd.Start() //nolint:errcheck
}
