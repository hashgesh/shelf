package artifactory

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/halilibrahimonus/shelf/internal/config"
)

const testToken = "tok-very-secret"

func testConfig(url string) *config.Config {
	return &config.Config{
		URL: url, Repos: []string{"conan-local"}, Timeout: 5 * time.Second,
		Auth: config.AuthAuto, MetadataSource: config.MetadataAuto, Token: testToken,
	}
}

const sampleResponse = `{
  "results": [
    {
      "repo": "conan-local",
      "path": "_/zlib/1.3.1/_/b8bc5f6a2a4b1e0e0c2a1b7c3d9e8f71/package/a3c1e2f9b6d4c8e7f0a1b2c3d4e5f60718293a4b/0ba8627bd47edc3a501e8f0eb9a79e5e",
      "name": "conan_package.tgz",
      "size": 104857,
      "created": "2026-03-14T09:26:53.589+03:00",
      "created_by": "ci-builder",
      "modified": "2026-03-14T09:26:53.512Z",
      "modified_by": "ci-builder",
      "properties": [
        {"key": "conan.settings.os", "value": "Linux"},
        {"key": "conan.settings.arch", "value": "x86_64"},
        {"key": "conan.options.shared", "value": "False"},
        {"key": "flag.only"}
      ]
    }
  ],
  "range": {"start_pos": 0, "end_pos": 1, "total": 1}
}`

func TestAQLRequestAndDecode(t *testing.T) {
	var gotPath, gotCT, gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotCT, gotAuth = r.URL.Path, r.Header.Get("Content-Type"), r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		_, _ = io.WriteString(w, sampleResponse)
	}))
	defer srv.Close()

	c, err := New(testConfig(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	q := ItemsQuery{Repos: []string{"conan-local"}, Names: []string{"conan_package.tgz"}}
	res, err := c.AQL(context.Background(), q.String())
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/artifactory/api/search/aql" {
		t.Errorf("path = %q", gotPath)
	}
	if gotCT != "text/plain" || gotAuth != "Bearer "+testToken {
		t.Errorf("content-type=%q auth=%q", gotCT, gotAuth)
	}
	if gotBody != q.String() {
		t.Errorf("body = %q", gotBody)
	}

	if len(res.Results) != 1 || res.Range.Total != 1 {
		t.Fatalf("results = %+v", res)
	}
	it := res.Results[0]
	if it.Size != 104857 || it.CreatedBy != "ci-builder" {
		t.Errorf("item = %+v", it)
	}
	if want := time.Date(2026, 3, 14, 6, 26, 53, 589e6, time.UTC); !it.Created.Equal(want) {
		t.Errorf("created = %v, want %v", it.Created, want)
	}
	if v, ok := it.Prop("conan.settings.os"); !ok || v != "Linux" {
		t.Errorf("os prop = %q, %v", v, ok)
	}
	if v, ok := it.Prop("flag.only"); !ok || v != "" {
		t.Errorf("valueless prop = %q, %v", v, ok)
	}
	if _, ok := it.Prop("missing"); ok {
		t.Error("missing prop reported present")
	}
	if !strings.HasSuffix(it.FullPath(), "/0ba8627bd47edc3a501e8f0eb9a79e5e/conan_package.tgz") {
		t.Errorf("FullPath = %q", it.FullPath())
	}
}

func TestAuthSchemes(t *testing.T) {
	basic := func(u, p string) string {
		return "Basic " + base64.StdEncoding.EncodeToString([]byte(u+":"+p))
	}
	tests := []struct {
		name string
		cfg  func(*config.Config)
		want string
	}{
		{"bearer token", func(*config.Config) {}, "Bearer " + testToken},
		{"basic user+token", func(c *config.Config) { c.Auth = config.AuthBasic; c.User = "u" }, basic("u", testToken)},
		{"basic password wins", func(c *config.Config) { c.User = "u"; c.Password = "pw" }, basic("u", "pw")},
		{"anonymous", func(c *config.Config) { c.Token = "" }, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = r.Header.Get("Authorization")
				_, _ = io.WriteString(w, `{"results":[]}`)
			}))
			defer srv.Close()
			cfg := testConfig(srv.URL)
			tt.cfg(cfg)
			c, err := New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := c.AQL(context.Background(), "items.find()"); err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("Authorization = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBaseURL(t *testing.T) {
	tests := map[string]string{
		"https://art.example":                     "https://art.example/artifactory",
		"https://art.example/":                    "https://art.example/artifactory",
		"https://art.example/artifactory/":        "https://art.example/artifactory",
		"http://10.0.0.5:8082/custom/ctx":         "http://10.0.0.5:8082/custom/ctx",
		"https://user:pw@art.example/artifactory": "https://art.example/artifactory",
	}
	for in, want := range tests {
		u, err := BaseURL(in)
		if err != nil || u.String() != want {
			t.Errorf("BaseURL(%q) = %v, %v; want %q", in, u, err, want)
		}
	}
	for _, bad := range []string{"", "art.example", "ftp://x", "https://"} {
		if _, err := BaseURL(bad); !errors.Is(err, config.ErrInvalid) {
			t.Errorf("BaseURL(%q) err = %v, want ErrInvalid", bad, err)
		}
	}
}

func TestHTTPErrors(t *testing.T) {
	tests := []struct {
		status   int
		body     string
		sentinel error
	}{
		{http.StatusUnauthorized, `{"errors":[{"status":401,"message":"Bad credentials"}]}`, ErrAuth},
		{http.StatusForbidden, "forbidden", ErrAuth},
		{http.StatusNotFound, "nope", ErrNotFound},
		{http.StatusBadRequest, "Failed to parse query: items.find(", nil},
		{http.StatusInternalServerError, strings.Repeat("x", 1000), nil},
	}
	for _, tt := range tests {
		t.Run(strconv.Itoa(tt.status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			}))
			defer srv.Close()
			c, _ := New(testConfig(srv.URL))
			_, err := c.AQL(context.Background(), "items.find()")
			var he *HTTPError
			if !errors.As(err, &he) || he.Status != tt.status {
				t.Fatalf("err = %v", err)
			}
			if tt.sentinel != nil && !errors.Is(err, tt.sentinel) {
				t.Errorf("errors.Is(%v, %v) = false", err, tt.sentinel)
			}
			if strings.Contains(err.Error(), testToken) {
				t.Error("token leaked into error message")
			}
			if len(he.Body) > maxErrorSnippet+len("…") {
				t.Errorf("snippet not truncated: %d bytes", len(he.Body))
			}
		})
	}
}

func TestInvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "<html>login page</html>")
	}))
	defer srv.Close()
	c, _ := New(testConfig(srv.URL))
	if _, err := c.AQL(context.Background(), "items.find()"); err == nil || !strings.Contains(err.Error(), "çözümlenemedi") {
		t.Fatalf("err = %v", err)
	}
}

var pageRe = regexp.MustCompile(`(?:\.offset\((\d+)\))?\.limit\((\d+)\)$`)

// pagingServer serves total items, honoring .offset().limit() in the query.
// The item at index splitAt is emitted twice across a page boundary with
// different properties, to exercise merging.
func pagingServer(t *testing.T, total, splitAt int, requests *int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*requests++
		b, _ := io.ReadAll(r.Body)
		m := pageRe.FindStringSubmatch(string(b))
		if m == nil {
			t.Errorf("query without paging: %s", b)
			return
		}
		offset, _ := strconv.Atoi(m[1])
		limit, _ := strconv.Atoi(m[2])

		var rows []Item
		for i := 0; i < total; i++ {
			rows = append(rows, Item{Repo: "r", Path: fmt.Sprintf("p/%03d", i), Name: "conan_package.tgz",
				Properties: []Property{{Key: "idx", Value: strconv.Itoa(i)}}})
			if i == splitAt {
				rows = append(rows, Item{Repo: "r", Path: fmt.Sprintf("p/%03d", i), Name: "conan_package.tgz",
					Properties: []Property{{Key: "extra", Value: "yes"}, {Key: "idx", Value: strconv.Itoa(i)}}})
			}
		}
		end := min(offset+limit, len(rows))
		page := []Item{}
		if offset < len(rows) {
			page = rows[offset:end]
		}
		_ = json.NewEncoder(w).Encode(Result{Results: page})
	}))
}

func TestSearchItemsPaging(t *testing.T) {
	requests := 0
	srv := pagingServer(t, 7, 2, &requests) // 8 rows, item 2 split across pages 1/2
	defer srv.Close()
	c, _ := New(testConfig(srv.URL), WithPageSize(3))

	items, err := c.SearchItems(context.Background(), ItemsQuery{Repos: []string{"r"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 7 {
		t.Fatalf("got %d items, want 7", len(items))
	}
	if requests != 3 {
		t.Errorf("requests = %d, want 3", requests)
	}
	for i, it := range items {
		if v, _ := it.Prop("idx"); v != strconv.Itoa(i) {
			t.Errorf("item %d has idx %q", i, v)
		}
	}
	if v, ok := items[2].Prop("extra"); !ok || v != "yes" || len(items[2].Properties) != 2 {
		t.Errorf("split item not merged: %+v", items[2].Properties)
	}
}

func TestSearchItemsExactPageMultiple(t *testing.T) {
	requests := 0
	srv := pagingServer(t, 6, -1, &requests)
	defer srv.Close()
	c, _ := New(testConfig(srv.URL), WithPageSize(3))
	items, err := c.SearchItems(context.Background(), ItemsQuery{Repos: []string{"r"}})
	if err != nil || len(items) != 6 || requests != 3 {
		t.Fatalf("items=%d requests=%d err=%v", len(items), requests, err)
	}
}

func TestSearchItemsWithoutPaging(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		_, _ = io.WriteString(w, sampleResponse)
	}))
	defer srv.Close()
	c, _ := New(testConfig(srv.URL), WithPageSize(0))
	items, err := c.SearchItems(context.Background(), ItemsQuery{Repos: []string{"conan-local"}})
	if err != nil || len(items) != 1 {
		t.Fatalf("items=%v err=%v", items, err)
	}
	if strings.Contains(body, "limit(") || strings.Contains(body, "sort(") {
		t.Errorf("unexpected paging clause: %s", body)
	}
}

func TestSearchItemsRequiresRepo(t *testing.T) {
	c, _ := New(testConfig("https://unused.example"))
	if _, err := c.SearchItems(context.Background(), ItemsQuery{}); !errors.Is(err, config.ErrInvalid) {
		t.Fatalf("err = %v", err)
	}
}

func TestTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(500 * time.Millisecond):
		}
	}))
	defer srv.Close()
	cfg := testConfig(srv.URL)
	cfg.Timeout = 50 * time.Millisecond
	c, _ := New(cfg)
	_, err := c.AQL(context.Background(), "items.find()")
	if err == nil || !strings.Contains(err.Error(), "zaman aşımı") {
		t.Fatalf("err = %v", err)
	}
}

func TestTLS(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"results":[]}`)
	}))
	defer srv.Close()

	c, _ := New(testConfig(srv.URL))
	_, err := c.AQL(context.Background(), "items.find()")
	if err == nil || !strings.Contains(err.Error(), "insecure_tls") {
		t.Fatalf("self-signed cert: err = %v, want hint about insecure_tls", err)
	}

	cfg := testConfig(srv.URL)
	cfg.InsecureTLS = true
	c, _ = New(cfg)
	if _, err := c.AQL(context.Background(), "items.find()"); err != nil {
		t.Fatalf("insecure: %v", err)
	}
}

func TestDownload(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		if strings.HasSuffix(r.URL.Path, "big.bin") {
			_, _ = w.Write(make([]byte, maxDownloadBytes+1))
			return
		}
		_, _ = io.WriteString(w, "[settings]\nos=Linux\n")
	}))
	defer srv.Close()
	c, _ := New(testConfig(srv.URL + "/artifactory"))

	b, err := c.Download(context.Background(), "conan-local", "_/zlib/1.3.1/_/rrev/package/pid/prev/conaninfo.txt")
	if err != nil || !strings.Contains(string(b), "os=Linux") {
		t.Fatalf("Download = %q, %v", b, err)
	}
	if gotPath != "/artifactory/conan-local/_/zlib/1.3.1/_/rrev/package/pid/prev/conaninfo.txt" {
		t.Errorf("path = %q", gotPath)
	}

	if _, err := c.Download(context.Background(), "conan-local", "dir with space/a#b.txt"); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/artifactory/conan-local/dir%20with%20space/a%23b.txt" {
		t.Errorf("escaped path = %q", gotPath)
	}

	if _, err := c.Download(context.Background(), "r", "big.bin"); err == nil || !strings.Contains(err.Error(), "çok büyük") {
		t.Errorf("oversized download err = %v", err)
	}
}

func TestPing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/artifactory/api/system/ping" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = io.WriteString(w, "OK")
	}))
	defer srv.Close()
	c, _ := New(testConfig(srv.URL))
	if err := c.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestItemsQueryString(t *testing.T) {
	tests := []struct {
		name string
		q    ItemsQuery
		want string
	}{
		{
			"single repo and name",
			ItemsQuery{Repos: []string{"conan-local"}, Names: []string{"conan_package.tgz"}, Include: []string{"repo", "path"}},
			`items.find({"name":"conan_package.tgz","repo":"conan-local"}).include("repo","path")`,
		},
		{
			"multiple repos",
			ItemsQuery{Repos: []string{"a", "b"}, Names: []string{"conan_package.tgz"}, Include: []string{"name"}},
			`items.find({"$or":[{"repo":"a"},{"repo":"b"}],"name":"conan_package.tgz"}).include("name")`,
		},
		{
			"multiple repos and names",
			ItemsQuery{Repos: []string{"a", "b"}, Names: []string{"x", "y"}, Include: []string{"name"}},
			`items.find({"$and":[{"$or":[{"repo":"a"},{"repo":"b"}]},{"$or":[{"name":"x"},{"name":"y"}]}]}).include("name")`,
		},
		{
			"single repo multiple names",
			ItemsQuery{Repos: []string{"a"}, Names: []string{"x", "y"}, Include: []string{"name"}},
			`items.find({"$or":[{"name":"x"},{"name":"y"}],"repo":"a"}).include("name")`,
		},
		{
			"paging",
			ItemsQuery{Repos: []string{"a"}, Include: []string{"name"}, Offset: 10, Limit: 5},
			`items.find({"repo":"a"}).include("name").sort({"$asc":["repo","path","name"]}).offset(10).limit(5)`,
		},
		{
			"paging without sort",
			ItemsQuery{Repos: []string{"a"}, Include: []string{"name"}, Offset: 10, Limit: 5, NoSort: true},
			`items.find({"repo":"a"}).include("name").offset(10).limit(5)`,
		},
		{
			"quotes are escaped",
			ItemsQuery{Repos: []string{`evil"})`}, Include: []string{"name"}},
			`items.find({"repo":"evil\"})"}).include("name")`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.q.String(); got != tt.want {
				t.Errorf("\n got %s\nwant %s", got, tt.want)
			}
		})
	}
	def := ItemsQuery{Repos: []string{"a"}}.String()
	if !strings.Contains(def, `"property.*"`) || !strings.Contains(def, `"created_by"`) {
		t.Errorf("default include missing fields: %s", def)
	}
}

func TestSearchItemsRetriesWithoutSortOnOSS(t *testing.T) {
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		queries = append(queries, string(b))
		if strings.Contains(string(b), "sort(") {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"errors":[{"status":400,"message":"Sorting is not supported by AQL in the open source version\n"}]}`)
			return
		}
		_, _ = io.WriteString(w, sampleResponse)
	}))
	defer srv.Close()
	// Page size 1 with a 1-item response forces a second page.
	c, _ := New(testConfig(srv.URL), WithPageSize(1))
	items, err := c.SearchItems(context.Background(), ItemsQuery{Repos: []string{"conan-local"}})
	if err != nil || len(items) != 1 {
		t.Fatalf("items=%v err=%v", items, err)
	}
	// Rejected sorted page, then page 1 and page 2 without sort.
	if len(queries) != 3 || strings.Contains(queries[1], "sort(") || strings.Contains(queries[2], "sort(") ||
		!strings.Contains(queries[2], ".offset(1)") {
		t.Errorf("queries = %q", queries)
	}
}

func TestSearchItemsStopsWhenOffsetIgnored(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		// Always the same full page, whatever the offset.
		page := Result{Results: []Item{
			{Repo: "r", Path: "a", Name: "conan_package.tgz"},
			{Repo: "r", Path: "b", Name: "conan_package.tgz"},
		}}
		_ = json.NewEncoder(w).Encode(page)
	}))
	defer srv.Close()
	c, _ := New(testConfig(srv.URL), WithPageSize(2))
	items, err := c.SearchItems(context.Background(), ItemsQuery{Repos: []string{"r"}})
	if err != nil || len(items) != 2 {
		t.Fatalf("items=%d err=%v", len(items), err)
	}
	if requests != 2 {
		t.Errorf("requests = %d, want 2", requests)
	}
}
