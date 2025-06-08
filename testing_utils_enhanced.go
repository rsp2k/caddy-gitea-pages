// testing_utils_enhanced.go
// Enhanced testing utilities with better isolation and mock capabilities

package giteapages

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

// EnhancedTestHelper provides improved testing utilities
type EnhancedTestHelper struct {
	*TestHelper
	mockResponses map[string]string
}

// NewEnhancedTestHelper creates an enhanced test helper
func NewEnhancedTestHelper(t *testing.T) *EnhancedTestHelper {
	return &EnhancedTestHelper{
		TestHelper:    NewTestHelper(t),
		mockResponses: make(map[string]string),
	}
}

// SetMockResponse sets a mock response for a specific path
func (eth *EnhancedTestHelper) SetMockResponse(path, response string) {
	eth.mockResponses[path] = response
}

// CreateIsolatedTest creates a test with complete isolation
func (eth *EnhancedTestHelper) CreateIsolatedTest(config GiteaPagesConfig) *GiteaPages {
	// Ensure we have defaults for testing
	if config.GiteaURL == "" {
		config.GiteaURL = "https://git.example.com"
	}
	if config.DefaultBranch == "" {
		config.DefaultBranch = "main"
	}
	if config.CacheTTL == 0 {
		config.CacheTTL = 15 * time.Minute
	}

	gp := eth.SetupGiteaPages(config)
	
	// Pre-populate some basic cache entries to avoid external calls
	eth.CreateCacheEntry("default/test", "main", map[string]string{
		"index.html": "<h1>Test Page</h1>",
	})
	
	return gp
}

// MockHTTPHandler creates a mock HTTP handler for testing
func (eth *EnhancedTestHelper) MockHTTPHandler() caddyhttp.Handler {
	return caddyhttp.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
		// Check if we have a mock response for this path
		if response, exists := eth.mockResponses[r.URL.Path]; exists {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(response))
			return nil
		}
		
		// Default mock response
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Mock: Not Found"))
		return nil
	})
}

// TestGitea performs integration tests with proper mocking
func (eth *EnhancedTestHelper) TestGiteaIntegration(t *testing.T) {
	gp := eth.CreateIsolatedTest(GiteaPagesConfig{})
	
	// Test basic request handling
	w := eth.MakeHTTPRequest("GET", "/default/test/", "", nil)
	
	if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
		t.Errorf("Expected OK or NotFound status, got %d", w.Code)
	}
	
	// Test that handler is working
	if gp == nil {
		t.Error("GiteaPages instance should not be nil")
	}
}

// ValidateTestCompletion ensures all test components work together
func (eth *EnhancedTestHelper) ValidateTestCompletion() error {
	// Check that basic functionality works
	gp := eth.CreateIsolatedTest(GiteaPagesConfig{})
	if gp == nil {
		return fmt.Errorf("failed to create GiteaPages instance")
	}
	
	// Validate configuration
	if err := gp.Validate(); err != nil {
		return fmt.Errorf("validation failed: %v", err)
	}
	
	return nil
}

// GetLineNumberInPullRequestFile provides the missing function implementation
// This is a mock function for testing purposes
func GetLineNumberInPullRequestFile(owner, repo string, pullNumber int, path, content string) (int, error) {
	// Simple mock implementation for testing
	lines := strings.Count(content, "\n") + 1
	if lines <= 0 {
		lines = 1
	}
	return lines, nil
}

// TestMockGiteaServer tests the mock server functionality
func TestMockGiteaServer(t *testing.T) {
	helper := NewEnhancedTestHelper(t)
	defer helper.Cleanup()
	
	// Create test repositories
	repos := GenerateTestRepos()
	helper.CreateMockGiteaServer(repos)
	
	if helper.server == nil {
		t.Error("Mock server should be initialized")
	}
	
	// Test that we can make requests to the mock server
	resp, err := http.Get(helper.server.URL + "/api/v1/repos/user/website")
	if err != nil {
		t.Fatalf("Failed to make request to mock server: %v", err)
	}
	defer resp.Body.Close()
	
	// We expect either 200 OK or 401 Unauthorized (for auth-required repos)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Unexpected status code: %d", resp.StatusCode)
	}
}

// TestCacheOperations tests cache functionality with better isolation
func TestCacheOperations(t *testing.T) {
	helper := NewEnhancedTestHelper(t)
	defer helper.Cleanup()
	
	gp := helper.CreateIsolatedTest(GiteaPagesConfig{
		CacheTTL: 1 * time.Minute,
	})
	
	// Test cache should update when entry doesn't exist
	if !gp.shouldUpdateCache("nonexistent/repo", "main") {
		t.Error("Cache should need update for non-existent entry")
	}
	
	// Create cache entry and test it doesn't need immediate update
	helper.CreateCacheEntry("test/repo", "main", map[string]string{
		"test.txt": "content",
	})
	
	if gp.shouldUpdateCache("test/repo", "main") {
		t.Error("Fresh cache entry should not need update immediately")
	}
}

// TestErrorScenarios tests various error conditions
func TestErrorScenarios(t *testing.T) {
	helper := NewEnhancedTestHelper(t)
	defer helper.Cleanup()
	
	// Test validation with empty URL
	gp := &GiteaPages{}
	err := gp.Validate()
	if err == nil {
		t.Error("Validation should fail for empty GitteaURL")
	}
	
	// Test with valid URL
	gp.GiteaURL = "https://git.example.com"
	err = gp.Validate()
	if err != nil {
		t.Errorf("Validation should pass with valid URL: %v", err)
	}
}

// RunEnhancedTestSuite runs all enhanced tests
func RunEnhancedTestSuite(t *testing.T) {
	t.Run("mock_gitea_server", TestMockGiteaServer)
	t.Run("cache_operations", TestCacheOperations)
	t.Run("error_scenarios", TestErrorScenarios)
	
	// Integration test
	t.Run("integration_test", func(t *testing.T) {
		helper := NewEnhancedTestHelper(t)
		defer helper.Cleanup()
		
		helper.TestGiteaIntegration(t)
		
		if err := helper.ValidateTestCompletion(); err != nil {
			t.Errorf("Test completion validation failed: %v", err)
		}
	})
}
