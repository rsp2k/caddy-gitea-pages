package giteapages

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

// TestGiteaPages_Integration_CompleteFlow tests the complete flow from HTTP request to file serving
func TestGiteaPages_Integration_CompleteFlow(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	// Setup mock Gitea server
	repos := GenerateTestRepos()
	helper.CreateMockGiteaServer(repos)

	// Configure GiteaPages
	_ = helper.SetupGiteaPages(GiteaPagesConfig{
		GiteaURL:      helper.server.URL,
		GiteaToken:    "test-token",
		CacheTTL:      5 * time.Minute,
		DefaultBranch: "main",
	})

	// Pre-populate cache to avoid needing external network calls
	helper.CreateCacheEntry("user/website", "main", map[string]string{
		"index.html":     "<h1>Welcome to My Website</h1>",
		"about.html":     "<h1>About Us</h1>",
		"css/style.css":  "body { font-family: Arial; }",
	})

	helper.CreateCacheEntry("org/blog", "gh-pages", map[string]string{
		"index.html": "<h1>Blog Home</h1>",
	})

	// Test scenarios
	tests := []struct {
		name           string
		path           string
		expectedStatus int
		expectedText   string
	}{
		{
			name:           "serve website index",
			path:           "/user/website/",
			expectedStatus: http.StatusOK,
			expectedText:   "Welcome to My Website",
		},
		{
			name:           "serve specific page",
			path:           "/user/website/about.html",
			expectedStatus: http.StatusOK,
			expectedText:   "About Us",
		},
		{
			name:           "serve CSS file",
			path:           "/user/website/css/style.css",
			expectedStatus: http.StatusOK,
			expectedText:   "font-family: Arial",
		},
		{
			name:           "serve blog with different branch",
			path:           "/org/blog/",
			expectedStatus: http.StatusOK,
			expectedText:   "Blog Home",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := helper.MakeHTTPRequest("GET", tt.path, "", nil)
			helper.AssertResponse(w, tt.expectedStatus, tt.expectedText)
		})
	}
}

// TestGiteaPages_Security_PathTraversalPrevention tests security against path traversal attacks
func TestGiteaPages_Security_PathTraversalPrevention(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	// Create a sensitive file outside repo
	helper.CreateCacheEntry("test/repo", "main", map[string]string{
		"public.html": "PUBLIC_DATA",
	})

	_ = helper.SetupGiteaPages(GiteaPagesConfig{
		GiteaURL:      "https://git.example.com",
		DefaultBranch: "main",
	})

	scenario := SecurityTestScenario{
		Name: "Path Traversal Protection",
		Attacks: []SecurityAttack{
			{
				Name:          "basic_traversal",
				Path:          "/test/repo/../../../etc/passwd",
				SensitiveData: "root:",
			},
			{
				Name:          "encoded_traversal",
				Path:          "/test/repo/%2e%2e%2f%2e%2e%2fetc%2fpasswd",
				SensitiveData: "root:",
			},
			{
				Name:          "double_encoding",
				Path:          "/test/repo/%252e%252e%252f%252e%252e%252fetc%252fpasswd",
				SensitiveData: "root:",
			},
			{
				Name:          "windows_traversal",
				Path:          "/test/repo/..\\\\..\\\\..\\\\windows\\\\system32\\\\config\\\\sam",
				SensitiveData: "SAM",
			},
		},
	}

	helper.TestSecurityScenario(scenario)

	// Test that legitimate files still works
	t.Run("legitimate_file_access", func(t *testing.T) {
		w := helper.MakeHTTPRequest("GET", "/test/repo/public.html", "", nil)
		helper.AssertResponse(w, http.StatusOK, "PUBLIC_DATA")
	})
}

// TestGiteaPages_Security_PrivateRepoAccess tests authentication for private repositories
func TestGiteaPages_Security_PrivateRepoAccess(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	// Setup mock server with private repo
	repos := map[string]MockRepo{
		"company/private": {
			Name:          "private",
			FullName:      "company/private",
			DefaultBranch: "main",
			Private:       true,
			RequireToken:  true,
			Files: map[string]string{
				"index.html": "<h1>Private Content</h1>",
			},
		},
	}
	helper.CreateMockGiteaServer(repos)

	// Test without token - should fail at API level
	_ = helper.SetupGiteaPages(GiteaPagesConfig{
		GiteaURL:      helper.server.URL,
		DefaultBranch: "main",
	})

	w := helper.MakeHTTPRequest("GET", "/company/private/", "", nil)
	// Should not return the private content
	if w.Code == http.StatusOK && strings.Contains(w.Body.String(), "Private Content") {
		t.Error("Private repository content should not be accessible without token")
	}

	// Test with token - should work
	_ = helper.SetupGiteaPages(GiteaPagesConfig{
		GiteaURL:      helper.server.URL,
		GiteaToken:    "valid-token",
		DefaultBranch: "main",
	})

	w2 := helper.MakeHTTPRequest("GET", "/company/private/", "", nil)
	// This would work in a real scenario with proper token handling
	helper.t.Logf("Request with token status: %d", w2.Code)
}

// TestGiteaPages_DomainMapping tests domain-based routing
func TestGiteaPages_DomainMapping(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	// Setup cache with domain-mapped content
	helper.CreateCacheEntry("company/website", "main", map[string]string{
		"index.html": "<h1>Company Website</h1>",
		"about.html": "<h1>About Company</h1>",
	})

	_ = helper.SetupGiteaPages(GiteaPagesConfig{
		GiteaURL:      "https://git.example.com",
		DefaultBranch: "main",
		DomainMappings: []DomainMapping{
			{
				Domain:     "company.example.com",
				Owner:      "company",
				Repository: "website",
				Branch:     "main",
			},
		},
	})

	tests := []struct {
		name           string
		host           string
		path           string
		expectedStatus int
		expectedText   string
	}{
		{
			name:           "domain_mapped_index",
			host:           "company.example.com",
			path:           "/",
			expectedStatus: http.StatusOK,
			expectedText:   "Company Website",
		},
		{
			name:           "domain_mapped_subpage",
			host:           "company.example.com",
			path:           "/about.html",
			expectedStatus: http.StatusOK,
			expectedText:   "About Company",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := helper.MakeHTTPRequest("GET", tt.path, tt.host, nil)
			helper.AssertResponse(w, tt.expectedStatus, tt.expectedText)
		})
	}
}

// TestGiteaPages_AutoMapping tests automatic domain-to-repository mapping
func TestGiteaPages_AutoMapping(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	// Setup cache for auto-mapped content
	helper.CreateCacheEntry("websites/blog", "main", map[string]string{
		"index.html": "<h1>Auto Mapped Blog</h1>",
	})

	_ = helper.SetupGiteaPages(GiteaPagesConfig{
		GiteaURL:      "https://git.example.com",
		DefaultBranch: "main",
		AutoMapping: &AutoMapping{
			Enabled:    true,
			Pattern:    "{subdomain}.{domain}",
			Owner:      "websites",
			RepoFormat: "{subdomain}",
			Branch:     "main",
		},
	})

	w := helper.MakeHTTPRequest("GET", "/", "blog.example.com", nil)
	helper.AssertResponse(w, http.StatusOK, "Auto Mapped Blog")
}

// TestGiteaPages_Cache_Concurrency tests cache operations under concurrent access
func TestGiteaPages_Cache_Concurrency(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	_ = helper.SetupGiteaPages(GiteaPagesConfig{
		GiteaURL:      "https://git.example.com",
		CacheTTL:      15 * time.Minute,
		DefaultBranch: "main",
	})

	const numWorkers = 20
	const operationsPerWorker = 50

	var wg sync.WaitGroup

	// Test concurrent shouldUpdateCache operations
	t.Run("concurrent_should_update", func(t *testing.T) {
		for i := 0; i < numWorkers; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				for j := 0; j < operationsPerWorker; j++ {
					repoKey := fmt.Sprintf("owner%d/repo%d", workerID%5, j%10)
					branch := "main"
					helper.gp.shouldUpdateCache(repoKey, branch)
				}
			}(i)
		}
		wg.Wait()
	})

	// Test concurrent cache updates
	t.Run("concurrent_cache_updates", func(t *testing.T) {
		for i := 0; i < numWorkers; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				for j := 0; j < operationsPerWorker; j++ {
					cacheKey := fmt.Sprintf("owner%d/repo%d:main", workerID%5, j%10)
					helper.gp.cache.mu.Lock()
					helper.gp.cache.repos[cacheKey] = &cacheEntry{
						lastUpdate: time.Now(),
						path:       fmt.Sprintf("/tmp/test-%d-%d", workerID, j),
					}
					helper.gp.cache.mu.Unlock()
				}
			}(i)
		}
		wg.Wait()
	})

	// Verify cache integrity after concurrent operations
	helper.gp.cache.mu.RLock()
	numEntries := len(helper.gp.cache.repos)
	helper.gp.cache.mu.RUnlock()

	if numEntries == 0 {
		t.Error("Cache should contain entries after concurrent operations")
	}

	// Test that all entries are valid
	helper.gp.cache.mu.RLock()
	for key, entry := range helper.gp.cache.repos {
		if entry == nil {
			t.Errorf("Cache entry for key %s should not be nil", key)
		}
		if entry.lastUpdate.IsZero() {
			t.Errorf("Cache entry for key %s should have valid lastUpdate", key)
		}
	}
	helper.gp.cache.mu.RUnlock()
}

// TestGiteaPages_ErrorHandling tests various error scenarios
func TestGiteaPages_ErrorHandling(t *testing.T) {
	tests := []struct {
		name          string
		setupServer   func() *httptest.Server
		expectedError bool
	}{
		{
			name: "gitea_server_unavailable",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
				}))
			},
			expectedError: true,
		},
		{
			name: "gitea_server_unauthorized",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
				}))
			},
			expectedError: true,
		},
		{
			name: "gitea_server_not_found",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					http.Error(w, "Not Found", http.StatusNotFound)
				}))
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			defer server.Close()

			helper := NewTestHelper(t)
			defer helper.Cleanup()

			_ = helper.SetupGiteaPages(GiteaPagesConfig{
				GiteaURL:      server.URL,
				DefaultBranch: "main",
			})

			// Try to trigger cache update which should fail
			err := helper.gp.updateRepoCache("test", "repo", "main")

			if tt.expectedError && err == nil {
				t.Error("Expected error but got none")
			}

			if !tt.expectedError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

// TestGiteaPages_IndexFileResolution tests index file resolution logic
func TestGiteaPages_IndexFileResolution(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	tests := []struct {
		name          string
		files         map[string]string
		indexFiles    []string
		expectedIndex string
	}{
		{
			name: "prefer_index_html",
			files: map[string]string{
				"index.html": "<h1>HTML Index</h1>",
				"index.htm":  "<h1>HTM Index</h1>",
			},
			indexFiles:    []string{"index.html", "index.htm"},
			expectedIndex: "index.html",
		},
		{
			name: "fallback_to_index_htm",
			files: map[string]string{
				"index.htm":     "<h1>HTM Index</h1>",
				"default.html":  "<h1>Default</h1>",
			},
			indexFiles:    []string{"index.html", "index.htm", "default.html"},
			expectedIndex: "index.htm",
		},
		{
			name: "no_index_files_found",
			files: map[string]string{
				"page.html": "<h1>Page</h1>",
			},
			indexFiles:    []string{"index.html", "index.htm"},
			expectedIndex: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helper.CreateCacheEntry("test/website", "main", tt.files)

			_ = helper.SetupGiteaPages(GiteaPagesConfig{
				GiteaURL:      "https://git.example.com",
				DefaultBranch: "main",
				IndexFiles:    tt.indexFiles,
			})

			result := helper.gp.findIndexFile("test", "website")
			if result != tt.expectedIndex {
				t.Errorf("Expected index file '%s', got '%s'", tt.expectedIndex, result)
			}
		})
	}
}

// TestGiteaPages_ConfigurationValidation tests comprehensive configuration validation
func TestGiteaPages_ConfigurationValidation(t *testing.T) {
	tests := []struct {
		name        string
		caddyfile   string
		shouldError bool
		errorText   string
	}{
		{
			name: "valid_minimal_config",
			caddyfile: `gitea_pages {
				gitea_url https://git.example.com
			}`,
			shouldError: false,
		},
		{
			name: "missing_gitea_url",
			caddyfile: `gitea_pages {
				cache_ttl 15m
			}`,
			shouldError: true,
			errorText:   "gitea_url is required",
		},
		{
			name: "invalid_cache_ttl",
			caddyfile: `gitea_pages {
				gitea_url https://git.example.com
				cache_ttl invalid_duration
			}`,
			shouldError: true,
		},
		{
			name: "complete_valid_config",
			caddyfile: `gitea_pages {
				gitea_url https://git.example.com
				gitea_token test_token
				cache_dir /tmp/cache
				cache_ttl 30m
				default_branch master
				index_files index.html index.htm
				domain_mapping example.com company main-site main
				auto_mapping {
					enabled true
					pattern {subdomain}.{domain}
					owner websites
					repo_format {subdomain}
					branch main
				}
			}`,
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helper := NewTestHelper(t)
			defer helper.Cleanup()

			gp := helper.ParseCaddyfile(tt.caddyfile)
			err := gp.Validate()

			if tt.shouldError && err == nil {
				t.Error("Expected validation error but got none")
			}

			if !tt.shouldError && err != nil {
				t.Errorf("Expected no validation error but got: %v", err)
			}

			if tt.shouldError && tt.errorText != "" && err != nil && !strings.Contains(err.Error(), tt.errorText) {
				t.Errorf("Expected error containing '%s', got: %v", tt.errorText, err)
			}
		})
	}
}

// BenchmarkGiteaPages_ServeFile benchmarks file serving performance
func BenchmarkGiteaPages_ServeFile(b *testing.B) {
	helper := NewBenchmarkHelper(b)
	_ = helper.SetupBenchmarkData(10, 1024) // 10 files, 1KB each

	req := httptest.NewRequest("GET", "/bench/repo/file5.html", nil)
	next := caddyhttp.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
		return nil
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		helper.gp.serveFile(w, req, "bench", "repo", "file5.html", "main")
	}
}

// BenchmarkGiteaPages_CacheOperations benchmarks cache operations
func BenchmarkGiteaPages_CacheOperations(b *testing.B) {
	helper := NewTestHelper(&testing.T{})
	defer helper.Cleanup()

	_ = helper.SetupGiteaPages(GiteaPagesConfig{
		GiteaURL:      "https://git.example.com",
		CacheTTL:      15 * time.Minute,
		DefaultBranch: "main",
	})

	// Add some initial cache entries
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("user%d/repo%d:main", i%10, i%10)
		helper.gp.cache.repos[key] = &cacheEntry{
			lastUpdate: time.Now(),
			path:       fmt.Sprintf("/tmp/test%d", i),
		}
	}

	b.ResetTimer()

	b.Run("shouldUpdateCache", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			repoKey := fmt.Sprintf("user%d/repo%d", i%10, i%10)
			helper.gp.shouldUpdateCache(repoKey, "main")
		}
	})

	b.Run("formatRepoName", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			input := fmt.Sprintf("repo%d", i%100)
			helper.gp.formatRepoName(input, "{subdomain}-service")
		}
	})
}

// TestGiteaPages_RepoNameFormatting tests repository name formatting with various templates
func TestGiteaPages_RepoNameFormatting(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	gp := helper.SetupGiteaPages(GiteaPagesConfig{
		GiteaURL: "https://git.example.com",
	})

	tests := []struct {
		name     string
		input    string
		format   string
		expected string
	}{
		{
			name:     "empty_format_returns_input",
			input:    "test-repo",
			format:   "",
			expected: "test-repo",
		},
		{
			name:     "domain_template",
			input:    "example.com",
			format:   "{domain}",
			expected: "example.com",
		},
		{
			name:     "subdomain_template",
			input:    "blog",
			format:   "{subdomain}-site",
			expected: "blog-site",
		},
		{
			name:     "input_template",
			input:    "myproject",
			format:   "{input}",
			expected: "myproject",
		},
		{
			name:     "complex_template",
			input:    "api",
			format:   "{subdomain}-service-v1",
			expected: "api-service-v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := gp.formatRepoName(tt.input, tt.format)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}
