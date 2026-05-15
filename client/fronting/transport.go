package fronting

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Transport is an http.RoundTripper that performs SNI domain-fronting.
// Every TCP connection is opened to FrontingIP:443; the TLS handshake
// advertises AllowedSNI; ALPN negotiates "h2" when possible so the
// underlying http.Transport multiplexes many concurrent requests over
// a single TLS connection per upstream host.
//
// The wrapped *http.Transport handles keep-alive, connection pooling,
// and HTTP/2 framing for us — we only override the dial.
type Transport struct {
	FrontingIP string // e.g. "216.239.38.120"
	AllowedSNI string // e.g. "www.google.com"

	once  sync.Once
	inner *http.Transport
	dials atomic.Int64
}

func (t *Transport) lazyInit() {
	t.once.Do(func() {
		t.inner = &http.Transport{
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          64,
			MaxIdleConnsPerHost:   16,
			MaxConnsPerHost:       32,
			IdleConnTimeout:       90 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			DialTLSContext:        t.dialTLS,
		}
	})
}

func (t *Transport) dialTLS(ctx context.Context, network, addr string) (net.Conn, error) {
	n := t.dials.Add(1)
	log.Printf("[fronting] dial#%d ip=%s sni=%s for=%s", n, t.FrontingIP, t.AllowedSNI, addr)

	d := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(t.FrontingIP, "443"))
	if err != nil {
		return nil, fmt.Errorf("fronting dial %s: %w", t.FrontingIP, err)
	}

	tlsConn := tls.Client(conn, &tls.Config{
		ServerName:         t.AllowedSNI,
		InsecureSkipVerify: true, //nolint:gosec // CDN cert won't match Host
		NextProtos:         []string{"h2", "http/1.1"},
	})
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		conn.Close()
		return nil, fmt.Errorf("fronting tls handshake: %w", err)
	}
	cs := tlsConn.ConnectionState()
	log.Printf("[fronting] dial#%d tls ok alpn=%q cipher=0x%04x", n, cs.NegotiatedProtocol, cs.CipherSuite)
	return tlsConn, nil
}

func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.lazyInit()
	log.Printf("[fronting] → %s %s host=%s", req.Method, req.URL.RequestURI(), req.Host)
	resp, err := t.inner.RoundTrip(req)
	if err != nil {
		log.Printf("[fronting] error host=%s: %v", req.Host, err)
		return nil, err
	}
	log.Printf("[fronting] ← %d %s host=%s proto=%s", resp.StatusCode, resp.Status, req.Host, resp.Proto)
	return resp, nil
}

// CloseIdle releases pooled connections (useful for tests / shutdown).
func (t *Transport) CloseIdle() {
	if t.inner != nil {
		t.inner.CloseIdleConnections()
	}
}

// NewClient returns an *http.Client backed by the fronting transport.
// Connections are pooled and (when the upstream supports it) HTTP/2
// multiplexed automatically — callers can fire many concurrent
// requests cheaply.
func NewClient(frontingIP, allowedSNI string) *http.Client {
	t := &Transport{FrontingIP: frontingIP, AllowedSNI: allowedSNI}
	t.lazyInit()
	return &http.Client{
		Transport: t,
		Timeout:   60 * time.Second,
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
// The Transport will dial frontingIP but preserve this Host.
func NewRequest(method, targetURL string, body io.Reader) (*http.Request, error) {
	u, err := url.Parse(targetURL)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(method, targetURL, body)
	if err != nil {
		return nil, err
	}
	req.Host = u.Host
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
		"ggpht.com", "ytimg.com", "youtube.com", "googleusercontent.com",
	}
	host = strings.ToLower(host)
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	for _, d := range googleDomains {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}
