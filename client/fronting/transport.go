package fronting

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// Transport implements http.RoundTripper with SNI domain-fronting.
// It connects TCP to FrontingIP:443, presents AllowedSNI in the TLS handshake,
// but sends the original request Host header inside the encrypted tunnel.
// DPI sees AllowedSNI (an allowed domain); the CDN routes based on Host.
type Transport struct {
	FrontingIP string // e.g. "216.239.38.120"
	AllowedSNI string // e.g. "www.google.com"
}

func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Dial the fronting IP directly
	conn, err := net.Dial("tcp", t.FrontingIP+":443")
	if err != nil {
		return nil, fmt.Errorf("fronting dial %s: %w", t.FrontingIP, err)
	}

	// TLS with spoofed SNI; skip cert verification (CDN cert won't match Host)
	tlsConn := tls.Client(conn, &tls.Config{
		ServerName:         t.AllowedSNI,
		InsecureSkipVerify: true, //nolint:gosec
	})
	if err := tlsConn.Handshake(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("fronting tls handshake: %w", err)
	}

	// Write the HTTP request with the real Host header intact
	if err := req.Write(tlsConn); err != nil {
		tlsConn.Close()
		return nil, fmt.Errorf("fronting write request: %w", err)
	}

	resp, err := http.ReadResponse(bufio.NewReader(tlsConn), req)
	if err != nil {
		tlsConn.Close()
		return nil, fmt.Errorf("fronting read response: %w", err)
	}

	// Wrap body to close the TLS connection when done
	resp.Body = &connCloser{ReadCloser: resp.Body, conn: tlsConn}
	return resp, nil
}

type connCloser struct {
	io.ReadCloser
	conn net.Conn
}

func (c *connCloser) Close() error {
	err := c.ReadCloser.Close()
	c.conn.Close()
	return err
}

// Client returns an *http.Client that follows redirects via the fronting transport.
// maxRedirects=0 means use Go default (10).
func NewClient(frontingIP, allowedSNI string) *http.Client {
	t := &Transport{FrontingIP: frontingIP, AllowedSNI: allowedSNI}
	return &http.Client{
		Transport: t,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			return nil
		},
	}
}

// NewRequest builds an *http.Request for a fronted target.
// targetURL must use the real hostname (e.g. "https://drive.google.com/...").
// The Transport will connect to frontingIP but preserve this Host.
func NewRequest(method, targetURL string, body io.Reader) (*http.Request, error) {
	u, err := url.Parse(targetURL)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(method, targetURL, body)
	if err != nil {
		return nil, err
	}
	req.Host = u.Host // ensure Host header matches real target
	return req, nil
}

// SetBearer attaches an Authorization: Bearer header.
func SetBearer(req *http.Request, token string) {
	req.Header.Set("Authorization", "Bearer "+token)
}

// SetJSON sets Content-Type and writes JSON body.
func SetJSON(req *http.Request, body []byte) {
	req.Header.Set("Content-Type", "application/json")
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))
}

// HostHeader returns the Host part of a URL string.
func HostHeader(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Host
}

// IsGoogleHost returns true if the host is a Google-owned domain.
func IsGoogleHost(host string) bool {
	googleDomains := []string{
		"google.com", "googleapis.com", "googlevideo.com",
		"ggpht.com", "ytimg.com", "youtube.com",
	}
	host = strings.ToLower(host)
	for _, d := range googleDomains {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}
