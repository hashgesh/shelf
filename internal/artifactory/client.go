// Package artifactory is a small client for the Artifactory REST API parts
// shelf needs: AQL search, file download and ping.
package artifactory

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/halilibrahimonus/shelf/internal/config"
)

// Response size caps, to fail fast instead of exhausting memory.
const (
	maxAQLBytes      = 512 << 20
	maxDownloadBytes = 4 << 20
	maxErrorSnippet  = 300
)

// DefaultPageSize keeps each AQL request under Artifactory's default
// non-admin result cap (artifactory.search.userQueryLimit = 1000).
const DefaultPageSize = 1000

var (
	// ErrAuth is returned for 401/403 responses.
	ErrAuth = errors.New("kimlik doğrulama başarısız")
	// ErrNotFound is returned for 404 responses.
	ErrNotFound = errors.New("bulunamadı")
)

// HTTPError is a non-2xx response.
type HTTPError struct {
	Method string
	URL    string // without query string or credentials
	Status int
	Body   string // trimmed snippet
}

func (e *HTTPError) Error() string {
	msg := fmt.Sprintf("%s %s: HTTP %d", e.Method, e.URL, e.Status)
	if e.Body != "" {
		msg += ": " + e.Body
	}
	return msg
}

// Unwrap maps status codes to sentinel errors.
func (e *HTTPError) Unwrap() error {
	switch e.Status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return ErrAuth
	case http.StatusNotFound:
		return ErrNotFound
	}
	return nil
}

// Client talks to one Artifactory instance.
type Client struct {
	base      *url.URL // ends with the Artifactory context path, e.g. /artifactory
	http      *http.Client
	authorize func(*http.Request)
	pageSize  int
	userAgent string
}

// Option customizes a Client.
type Option func(*Client)

// WithHTTPClient replaces the HTTP client (tests).
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// WithPageSize sets the AQL page size; 0 disables paging.
func WithPageSize(n int) Option { return func(c *Client) { c.pageSize = n } }

// WithUserAgent sets the User-Agent header.
func WithUserAgent(ua string) Option { return func(c *Client) { c.userAgent = ua } }

// New builds a client from a validated configuration.
func New(cfg *config.Config, opts ...Option) (*Client, error) {
	base, err := BaseURL(cfg.URL)
	if err != nil {
		return nil, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if cfg.InsecureTLS {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // explicit user opt-in
	}
	c := &Client{
		base:      base,
		http:      &http.Client{Transport: transport, Timeout: cfg.Timeout},
		authorize: authorizer(cfg),
		pageSize:  DefaultPageSize,
		userAgent: "shelf",
	}
	for _, o := range opts {
		o(c)
	}
	return c, nil
}

// BaseURL normalizes the configured URL. A bare host gets the default
// "/artifactory" context path; any explicit path is kept as is so custom
// reverse-proxy layouts keep working.
func BaseURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimRight(raw, "/"))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("%w: url geçersiz: %q", config.ErrInvalid, raw)
	}
	u.User = nil
	u.RawQuery, u.Fragment = "", ""
	if u.Path == "" {
		u.Path = "/artifactory"
	}
	return u, nil
}

// authorizer picks the auth scheme:
//   - password set → Basic user:password
//   - auth=basic   → Basic user:token
//   - token set    → Bearer token
//   - otherwise    → anonymous
func authorizer(cfg *config.Config) func(*http.Request) {
	switch {
	case cfg.Password.IsSet():
		user, pass := cfg.User, cfg.Password.Reveal()
		return func(r *http.Request) { r.SetBasicAuth(user, pass) }
	case cfg.Auth == config.AuthBasic && cfg.Token.IsSet():
		user, tok := cfg.User, cfg.Token.Reveal()
		return func(r *http.Request) { r.SetBasicAuth(user, tok) }
	case cfg.Token.IsSet():
		tok := cfg.Token.Reveal()
		return func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+tok) }
	}
	return func(*http.Request) {}
}

// endpoint joins path segments onto the base URL, escaping each element.
func (c *Client) endpoint(segments ...string) string {
	var b strings.Builder
	b.WriteString(strings.TrimRight(c.base.String(), "/"))
	for _, s := range segments {
		for _, part := range strings.Split(s, "/") {
			if part != "" {
				b.WriteByte('/')
				b.WriteString(url.PathEscape(part))
			}
		}
	}
	return b.String()
}

// do sends req and returns the body (capped at limit) for 2xx responses.
func (c *Client) do(req *http.Request, limit int64) ([]byte, error) {
	c.authorize(req)
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, friendlyTransportError(req, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("%s %s: yanıt okunamadı: %w", req.Method, redactURL(req.URL), err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, &HTTPError{
			Method: req.Method,
			URL:    redactURL(req.URL),
			Status: resp.StatusCode,
			Body:   snippet(body),
		}
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("%s %s: yanıt çok büyük (> %d bayt)", req.Method, redactURL(req.URL), limit)
	}
	return body, nil
}

func friendlyTransportError(req *http.Request, err error) error {
	var unknownCA x509.UnknownAuthorityError
	var verifyErr *tls.CertificateVerificationError
	var hostErr x509.HostnameError
	if errors.As(err, &unknownCA) || errors.As(err, &verifyErr) || errors.As(err, &hostErr) {
		return fmt.Errorf("%s: TLS sertifikası doğrulanamadı (self-signed ise insecure_tls: true veya --insecure kullanın): %w",
			redactURL(req.URL), err)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%s: zaman aşımı (timeout ayarını artırmayı deneyin): %w", redactURL(req.URL), err)
	}
	return fmt.Errorf("sunucuya bağlanılamadı: %w", err)
}

func redactURL(u *url.URL) string {
	c := *u
	c.User = nil
	c.RawQuery = ""
	return c.String()
}

func snippet(b []byte) string {
	s := strings.Join(strings.Fields(string(b)), " ")
	if len(s) > maxErrorSnippet {
		s = s[:maxErrorSnippet] + "…"
	}
	return s
}

// RawAQL posts query and returns the unparsed response body.
func (c *Client) RawAQL(ctx context.Context, query string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint("api", "search", "aql"), strings.NewReader(query))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Accept", "application/json")
	return c.do(req, maxAQLBytes)
}

// AQL runs a raw query and decodes the response.
func (c *Client) AQL(ctx context.Context, query string) (*Result, error) {
	body, err := c.RawAQL(ctx, query)
	if err != nil {
		return nil, err
	}
	var res Result
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&res); err != nil {
		return nil, fmt.Errorf("AQL yanıtı çözümlenemedi: %w", err)
	}
	return &res, nil
}

// SearchItems runs q, paging with the client's page size, and returns every
// item. Rows of the same item split across a page boundary are merged.
func (c *Client) SearchItems(ctx context.Context, q ItemsQuery) ([]Item, error) {
	if len(q.Repos) == 0 {
		return nil, fmt.Errorf("%w: en az bir repo gerekli", config.ErrInvalid)
	}
	if c.pageSize <= 0 {
		q.Offset, q.Limit = 0, 0
		res, err := c.AQL(ctx, q.String())
		if err != nil {
			return nil, err
		}
		return mergeItems(nil, res.Results), nil
	}

	var items []Item
	for offset := 0; ; offset += c.pageSize {
		q.Offset, q.Limit = offset, c.pageSize
		res, err := c.AQL(ctx, q.String())
		if err != nil && !q.NoSort && sortUnsupported(err) {
			// shortcut: OSS paging relies on the server's default (id) order, which
			// is stable in practice; mergeItems absorbs duplicates if it is not.
			q.NoSort = true
			res, err = c.AQL(ctx, q.String())
		}
		if err != nil {
			return nil, err
		}
		before := len(items)
		items = mergeItems(items, res.Results)
		// Stop on a short page, or when a full page brought nothing new: a
		// server (or proxy) that ignores offset would otherwise loop forever.
		if len(res.Results) < c.pageSize || len(items) == before {
			return items, nil
		}
	}
}

// sortUnsupported reports the error Artifactory OSS returns for .sort().
func sortUnsupported(err error) bool {
	var he *HTTPError
	return errors.As(err, &he) && he.Status == http.StatusBadRequest &&
		strings.Contains(he.Body, "Sorting is not supported")
}

// mergeItems appends rows to items, folding a row into an existing item when
// both describe the same artifact. This is defensive: should the server ever
// split one item's properties across pages, nothing is lost or duplicated.
func mergeItems(items, rows []Item) []Item {
	index := make(map[string]int, len(items)+len(rows))
	for i, it := range items {
		index[it.Key()] = i
	}
	for _, r := range rows {
		if i, ok := index[r.Key()]; ok {
			items[i].Properties = appendNewProps(items[i].Properties, r.Properties)
			continue
		}
		index[r.Key()] = len(items)
		items = append(items, r)
	}
	return items
}

func appendNewProps(dst, src []Property) []Property {
	seen := make(map[Property]bool, len(dst))
	for _, p := range dst {
		seen[p] = true
	}
	for _, p := range src {
		if !seen[p] {
			seen[p] = true
			dst = append(dst, p)
		}
	}
	return dst
}

// Download fetches a (small) file from repo at path.
func (c *Client) Download(ctx context.Context, repo, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint(repo, path), nil)
	if err != nil {
		return nil, err
	}
	return c.do(req, maxDownloadBytes)
}

// Ping checks connectivity. The endpoint may allow anonymous access, so it
// does not prove that the credentials are valid; use AQL for that.
func (c *Client) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint("api", "system", "ping"), nil)
	if err != nil {
		return err
	}
	_, err = c.do(req, 1<<10)
	return err
}
