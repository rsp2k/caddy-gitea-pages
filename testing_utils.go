// testing_utils.go
// Testing utilities and helpers based on Caddy testing patterns

package giteapages

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

// TestHelper provides utilities for testing caddy-gitea-pages
type TestHelper struct {
	t        *testing.T
	tempDir  string
	server   *httptest.Server
	gp       *GiteaPages
}

// NewTestHelper creates a new test helper instance
func NewTestHelper(t *testing.T) *TestHelper {
	t.Helper()
	
	tempDir := t.TempDir()
	
	return &TestHelper{
		t:       t,
		tempDir: tempDir,
	}
}

// Cleanup cleans up test resources
func (th *TestHelper) Cleanup() {
	if th.server != nil {
		th.server.Close()
	}
}

// CreateMockGiteaServer creates a mock Gitea server for testing
func (th *TestHelper) CreateMockGiteaServer(repos map[string]MockRepo) {
	th.t.Helper()
	
	th.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		th.handleMockGiteaRequest(w, r, repos)
	}))
}

// MockRepo represents a mock repository configuration
type MockRepo struct {
	Name           string
	FullName       string
	DefaultBranch  string
	Files          map[string]string
	Private        bool
	RequireToken   bool
}

func (th *TestHelper) handleMockGiteaRequest(w http.ResponseWriter, r *http.Request, repos map[string]MockRepo) {
	// Handle API requests
	if strings.HasPrefix(r.URL.Path, "/api/v1/repos/") {
		th.handleRepoAPI(w, r, repos)
		return
	}
	
	// Handle archive requests
	if strings.Contains(r.URL.Path, "/archive/") {
		th.handleArchiveRequest(w, r, repos)
		return
	}
	
	http.NotFound(w, r)
}

func (th *TestHelper) handleRepoAPI(w http.ResponseWriter, r *http.Request, repos map[string]MockRepo) {
	// Extract owner/repo from path: /api/v1/repos/owner/repo
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 4 {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	
	owner := parts[3]
	repoName := parts[4]
	repoKey := fmt.Sprintf("%s/%s", owner, repoName)
	
	repo, exists := repos[repoKey]
	if !exists {
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}
	
	// Check authentication for private repos
	if repo.Private || repo.RequireToken {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "token ") {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}
	
	// Return repository info
	giteaRepo := GiteaRepo{
		Name:          repo.Name,
		FullName:      repo.FullName,
		DefaultBranch: repo.DefaultBranch,
		UpdatedAt:     time.Now().Format(time.RFC3339),
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(giteaRepo)
}

func (th *TestHelper) handleArchiveRequest(w http.ResponseWriter, r *http.Request, repos map[string]MockRepo) {
	// Extract repo info from archive path
	// Example: /api/v1/repos/owner/repo/archive/main.tar.gz
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 6 {
		http.Error(w, "Invalid archive path", http.StatusBadRequest)
		return
	}
	
	owner := pathParts[4]
	repoName := pathParts[5]
	// branch := strings.TrimSuffix(pathParts[7], ".tar.gz")
	
	repoKey := fmt.Sprintf("%s/%s", owner, repoName)
	repo, exists := repos[repoKey]
	if !exists {
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}
	
	// Check authentication
	if repo.Private || repo.RequireToken {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "token ") {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}
	
	// Create and serve archive
	archive := th.createTestArchive(repo)
	w.Header().Set("Content-Type", "application/gzip")
	w.Write(archive)
}

// createTestArchive creates a tar.gz archive from mock repo files
func (th *TestHelper) createTestArchive(repo MockRepo) []byte {
	th.t.Helper()
	
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)
	
	// Create archive with repo structure
	repoDir := fmt.Sprintf("%s-%s/", strings.Replace(repo.FullName, "/", "-", -1), repo.DefaultBranch)
	
	for filename, content := range repo.Files {
		fullPath := repoDir + filename
		
		hdr := &tar.Header{
			Name: fullPath,
			Mode: 0644,
			Size: int64(len(content)),
		}
		
		if strings.HasSuffix(filename, "/") {
			hdr.Typeflag = tar.TypeDir
		} else {
			hdr.Typeflag = tar.TypeReg
		}
		
		if err := tw.WriteHeader(hdr); err != nil {
			th.t.Fatal(err)
		}
		
		if hdr.Typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte(content)); err != nil {
				th.t.Fatal(err)
			}
		}
	}
	
	if err := tw.Close(); err != nil {
		th.t.Fatal(err)
	}
	if err := gzw.Close(); err != nil {
		th.t.Fatal(err)
	}
	
	return buf.Bytes()
}

// SetupGiteaPages creates and provisions a GiteaPages instance for testing
func (th *TestHelper) SetupGiteaPages(config GiteaPagesConfig) *GiteaPages {
	th.t.Helper()
	
	gp := &GiteaPages{
		GiteaURL:        config.GiteaURL,
		GiteaToken:      config.GiteaToken,
		CacheDir:        filepath.Join(th.tempDir, "cache"),
		CacheTTL:        caddy.Duration(config.CacheTTL),
		DefaultBranch:   config.DefaultBranch,
		IndexFiles:      config.IndexFiles,
		DomainMappings:  config.DomainMappings,
		AutoMapping:     config.AutoMapping,
	}
	
	if gp.DefaultBranch == "" {
		gp.DefaultBranch = "main"
	}
	
	if len(gp.IndexFiles) == 0 {
		gp.IndexFiles = []string{"index.html", "index.htm"}
	}
	
	if config.CacheTTL == 0 {
		gp.CacheTTL = caddy.Duration(15 * time.Minute)
	}
	
	// Provision the module
	ctx := caddy.Context{}
	if err := gp.Provision(ctx); err != nil {
		th.t.Fatalf("Failed to provision GiteaPages: %v", err)
	}
	
	th.gp = gp
	return gp
}

// GiteaPagesConfig holds configuration for test setup
type GiteaPagesConfig struct {
	GiteaURL       string
	GiteaToken     string
	CacheTTL       time.Duration
	DefaultBranch  string
	IndexFiles     []string
	DomainMappings []DomainMapping
	AutoMapping    *AutoMapping
}

// MakeHTTPRequest creates and executes an HTTP request for testing
func (th *TestHelper) MakeHTTPRequest(method, path, host string, headers map[string]string) *httptest.ResponseRecorder {
	th.t.Helper()
	
	req := httptest.NewRequest(method, path, nil)
	if host != "" {
		req.Host = host
	}
	
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	
	w := httptest.NewRecorder()
	
	// Create mock next handler
	next := caddyhttp.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Not handled by gitea-pages"))
		return nil
	})
	
	err := th.gp.ServeHTTP(w, req, next)
	if err != nil {
		th.t.Logf("ServeHTTP returned error: %v", err)
	}
	
	return w
}

// AssertResponse asserts various properties of an HTTP response
func (th *TestHelper) AssertResponse(w *httptest.ResponseRecorder, expectedStatus int, expectedContains string) {
	th.t.Helper()
	
	if w.Code != expectedStatus {
		th.t.Errorf("Expected status %d, got %d", expectedStatus, w.Code)
	}
	
	if expectedContains != "" && !strings.Contains(w.Body.String(), expectedContains) {
		th.t.Errorf("Expected response to contain '%s', got: %s", expectedContains, w.Body.String())
	}
}

// CreateCacheEntry manually creates a cache entry for testing
func (th *TestHelper) CreateCacheEntry(repoKey, branch string, files map[string]string) {
	th.t.Helper()
	
	cacheKey := fmt.Sprintf("%s:%s", repoKey, branch)
	cachePath := filepath.Join(th.tempDir, "cache", cacheKey)
	
	if err := os.MkdirAll(cachePath, 0755); err != nil {
		th.t.Fatal(err)
	}
	
	for filename, content := range files {
		fullPath := filepath.Join(cachePath, filename)
		dir := filepath.Dir(fullPath)
		
		if err := os.MkdirAll(dir, 0755); err != nil {
			th.t.Fatal(err)
		}
		
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			th.t.Fatal(err)
		}
	}
	
	// Add to cache
	if th.gp != nil && th.gp.cache != nil {
		th.gp.cache.mu.Lock()
		th.gp.cache.repos[cacheKey] = &cacheEntry{
			lastUpdate: time.Now(),
			path:       cachePath,
		}
		th.gp.cache.mu.Unlock()
	}
}

// ParseCaddyfile parses a Caddyfile string for testing
func (th *TestHelper) ParseCaddyfile(caddyfileContent string) *GiteaPages {
	th.t.Helper()
	
	d := caddyfile.NewTestDispenser(caddyfileContent)
	gp := new(GiteaPages)
	
	err := gp.UnmarshalCaddyfile(d)
	if err != nil {
		th.t.Fatalf("Failed to parse Caddyfile: %v", err)
	}
	
	return gp
}

// GetLineNumberInPullRequestFile finds line numbers in pull request files for testing
// This is a mock implementation for testing purposes
func (th *TestHelper) GetLineNumberInPullRequestFile(owner, repo string, pullNumber int, path, content string) (int, error) {
	th.t.Helper()
	
	// For testing purposes, return a mock line number based on content
	// In a real implementation, this would parse the PR diff and find the actual line number
	lines := strings.Split(content, "\n")
	
	// Return the middle line number for consistency in tests
	if len(lines) > 0 {
		return (len(lines) / 2) + 1, nil
	}
	
	// Default to line 10 for empty content
	return 10, nil
}

// TestSecurityScenario tests various security attack scenarios
func (th *TestHelper) TestSecurityScenario(scenario SecurityTestScenario) {
	th.t.Helper()
	
	for _, attack := range scenario.Attacks {
		th.t.Run(attack.Name, func(t *testing.T) {
			w := th.MakeHTTPRequest("GET", attack.Path, attack.Host, attack.Headers)
			
			// Should either return error status or not contain sensitive data
			if w.Code == http.StatusOK && strings.Contains(w.Body.String(), attack.SensitiveData) {
				t.Errorf("Security vulnerability: attack '%s' succeeded and returned sensitive data", attack.Name)
			}
		})
	}
}

// SecurityTestScenario defines a security testing scenario
type SecurityTestScenario struct {
	Name    string
	Attacks []SecurityAttack
}

// SecurityAttack defines a security attack test case
type SecurityAttack struct {
	Name          string
	Path          string
	Host          string
	Headers       map[string]string
	SensitiveData string
}

// BenchmarkHelper provides utilities for performance testing
type BenchmarkHelper struct {
	b       *testing.B
	tempDir string
}

// NewBenchmarkHelper creates a new benchmark helper
func NewBenchmarkHelper(b *testing.B) *BenchmarkHelper {
	return &BenchmarkHelper{
		b:       b,
		tempDir: b.TempDir(),
	}
}

// SetupBenchmarkData creates test data for benchmarking
func (bh *BenchmarkHelper) SetupBenchmarkData(fileCount int, fileSize int) *GiteaPages {
	bh.b.Helper()
	
	// Create test files
	files := make(map[string]string)
	content := strings.Repeat("x", fileSize)
	
	for i := 0; i < fileCount; i++ {
		filename := fmt.Sprintf("file%d.html", i)
		files[filename] = content
	}
	
	// Setup cache structure
	repoPath := filepath.Join(bh.tempDir, "bench/repo:main")
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		bh.b.Fatal(err)
	}
	
	for filename, fileContent := range files {
		fullPath := filepath.Join(repoPath, filename)
		if err := os.WriteFile(fullPath, []byte(fileContent), 0644); err != nil {
			bh.b.Fatal(err)
		}
	}
	
	gp := &GiteaPages{
		cache: &repoCache{
			repos: map[string]*cacheEntry{
				"bench/repo:main": {
					lastUpdate: time.Now(),
					path:       repoPath,
				},
			},
		},
	}
	
	return gp
}

// GenerateTestRepos creates a set of test repositories for comprehensive testing
func GenerateTestRepos() map[string]MockRepo {
	return map[string]MockRepo{
		"user/website": {
			Name:          "website",
			FullName:      "user/website",
			DefaultBranch: "main",
			Files: map[string]string{
				"index.html":          "<h1>Welcome to My Website</h1>",
				"about.html":          "<h1>About Us</h1>",
				"contact.html":        "<h1>Contact</h1>",
				"css/style.css":       "body { font-family: Arial; }",
				"js/script.js":        "console.log('Hello World');",
				"images/logo.png":     "PNG_DATA_HERE",
				"assets/data.json":    `{"name": "test"}`,
			},
		},
		"org/blog": {
			Name:          "blog",
			FullName:      "org/blog",
			DefaultBranch: "gh-pages",
			Files: map[string]string{
				"index.html":         "<h1>Blog Home</h1>",
				"posts/post1.html":   "<h1>First Post</h1>",
				"posts/post2.html":   "<h1>Second Post</h1>",
				"feed.xml":           "<rss></rss>",
			},
		},
		"company/private": {
			Name:          "private",
			FullName:      "company/private",
			DefaultBranch: "main",
			Private:       true,
			RequireToken:  true,
			Files: map[string]string{
				"index.html":  "<h1>Private Site</h1>",
				"secret.html": "<h1>Secret Content</h1>",
			},
		},
	}
}
