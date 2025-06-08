// +build integration

package giteapages

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

// TestGiteaPages_Integration_RealHTTPFlow tests the integration with proper test tags
// This file should only run when explicitly requested with build tags
func TestGiteaPages_Integration_RealHTTPFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	helper := NewTestHelper(t)
	defer helper.Cleanup()

	// Use only local mock servers for integration tests
	repos := map[string]MockRepo{
		"integration/test": {
			Name:          "test",
			FullName:      "integration/test", 
			DefaultBranch: "main",
			Files: map[string]string{
				"index.html": "<h1>Integration Test</h1>",
			},
		},
	}
	
	helper.CreateMockGiteaServer(repos)
	
	gp := helper.SetupGiteaPages(GiteaPagesConfig{
		GiteaURL:      helper.server.URL,
		GiteaToken:    "integration-token",
		CacheTTL:      1 * time.Minute,
		DefaultBranch: "main",
	})

	// Pre-populate cache for predictable test behavior
	helper.CreateCacheEntry("integration/test", "main", map[string]string{
		"index.html": "<h1>Integration Test</h1>",
	})

	// Test the complete flow
	w := helper.MakeHTTPRequest("GET", "/integration/test/", "", nil)
	
	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d. Body: %s", http.StatusOK, w.Code, w.Body.String())
	}
	
	if !contains(w.Body.String(), "Integration Test") {
		t.Errorf("Expected response to contain 'Integration Test', got: %s", w.Body.String())
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && 
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || 
		 indexOf(s, substr) >= 0))
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// TestGiteaPages_Integration_CacheLifecycle tests cache lifecycle
func TestGiteaPages_Integration_CacheLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	helper := NewTestHelper(t)
	defer helper.Cleanup()

	gp := helper.SetupGiteaPages(GiteaPagesConfig{
		GiteaURL:      "https://git.example.com",
		CacheTTL:      100 * time.Millisecond, // Very short for testing
		DefaultBranch: "main",
	})

	repoKey := "test/lifecycle"
	branch := "main"

	// Initially should need update
	if !gp.shouldUpdateCache(repoKey, branch) {
		t.Error("Should need cache update initially")
	}

	// Simulate cache entry
	helper.CreateCacheEntry(repoKey, branch, map[string]string{
		"test.html": "test content",
	})

	// Should not need update immediately after creation
	if gp.shouldUpdateCache(repoKey, branch) {
		t.Error("Should not need cache update immediately after creation")
	}

	// Wait for TTL to expire
	time.Sleep(150 * time.Millisecond)

	// Should need update after TTL expiry
	if !gp.shouldUpdateCache(repoKey, branch) {
		t.Error("Should need cache update after TTL expiry")
	}
}
