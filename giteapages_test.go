package giteapages

import (
	"testing"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
)

func TestGiteaPages_CaddyModule(t *testing.T) {
	gp := new(GitteaPages)
	moduleInfo := gp.CaddyModule()
	
	if moduleInfo.ID != "http.handlers.gitea_pages" {
		t.Errorf("Expected module ID 'http.handlers.gitea_pages', got '%s'", moduleInfo.ID)
	}
	
	if moduleInfo.New == nil {
		t.Error("Expected New function to be set")
	}
}

func TestGiteaPages_Provision(t *testing.T) {
	gp := &GitteaPages{
		GitteaURL: "https://git.example.com",
	}
	
	ctx := caddy.Context{}
	err := gp.Provision(ctx)
	
	if err != nil {
		t.Fatalf("Provision failed: %v", err)
	}
	
	// Test defaults are set
	if gp.CacheDir == "" {
		t.Error("Expected CacheDir to be set")
	}
	
	if gp.CacheTTL == 0 {
		t.Error("Expected CacheTTL to be set")
	}
	
	if gp.DefaultBranch != "main" {
		t.Errorf("Expected DefaultBranch to be 'main', got '%s'", gp.DefaultBranch)
	}
	
	if len(gp.IndexFiles) == 0 {
		t.Error("Expected IndexFiles to be set")
	}
}

func TestGiteaPages_Validate(t *testing.T) {
	tests := []struct {
		name      string
		giteaURL  string
		expectErr bool
	}{
		{
			name:      "valid URL",
			giteaURL:  "https://git.example.com",
			expectErr: false,
		},
		{
			name:      "empty URL",
			giteaURL:  "",
			expectErr: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gp := &GitteaPages{
				GitteaURL: tt.giteaURL,
			}
			
			err := gp.Validate()
			
			if tt.expectErr && err == nil {
				t.Error("Expected an error but got none")
			}
			
			if !tt.expectErr && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

func TestGiteaPages_UnmarshalCaddyfile(t *testing.T) {
	input := `gitea_pages {
		gitea_url https://git.example.com
		gitea_token test_token
		cache_dir /tmp/cache
		cache_ttl 30m
		default_branch main
		index_files index.html index.htm
	}`
	
	d := caddyfile.NewTestDispenser(input)
	gp := new(GitteaPages)
	
	err := gp.UnmarshalCaddyfile(d)
	if err != nil {
		t.Fatalf("UnmarshalCaddyfile failed: %v", err)
	}
	
	if gp.GitteaURL != "https://git.example.com" {
		t.Errorf("Expected GitteaURL 'https://git.example.com', got '%s'", gp.GitteaURL)
	}
	
	if gp.GitteaToken != "test_token" {
		t.Errorf("Expected GitteaToken 'test_token', got '%s'", gp.GitteaToken)
	}
	
	if gp.CacheDir != "/tmp/cache" {
		t.Errorf("Expected CacheDir '/tmp/cache', got '%s'", gp.CacheDir)
	}
	
	if time.Duration(gp.CacheTTL) != 30*time.Minute {
		t.Errorf("Expected CacheTTL 30m, got %v", time.Duration(gp.CacheTTL))
	}
	
	if gp.DefaultBranch != "main" {
		t.Errorf("Expected DefaultBranch 'main', got '%s'", gp.DefaultBranch)
	}
	
	expectedIndexFiles := []string{"index.html", "index.htm"}
	if len(gp.IndexFiles) != len(expectedIndexFiles) {
		t.Errorf("Expected %d index files, got %d", len(expectedIndexFiles), len(gp.IndexFiles))
	}
	
	for i, expected := range expectedIndexFiles {
		if i >= len(gp.IndexFiles) || gp.IndexFiles[i] != expected {
			t.Errorf("Expected index file '%s' at position %d, got '%s'", expected, i, gp.IndexFiles[i])
		}
	}
}

func TestDomainMapping(t *testing.T) {
	mapping := DomainMapping{
		Domain:     "example.com",
		Owner:      "user",
		Repository: "repo",
		Branch:     "main",
	}
	
	if mapping.Domain != "example.com" {
		t.Errorf("Expected Domain 'example.com', got '%s'", mapping.Domain)
	}
	
	if mapping.Owner != "user" {
		t.Errorf("Expected Owner 'user', got '%s'", mapping.Owner)
	}
	
	if mapping.Repository != "repo" {
		t.Errorf("Expected Repository 'repo', got '%s'", mapping.Repository)
	}
	
	if mapping.Branch != "main" {
		t.Errorf("Expected Branch 'main', got '%s'", mapping.Branch)
	}
}

func TestAutoMapping(t *testing.T) {
	autoMapping := AutoMapping{
		Enabled:    true,
		Pattern:    "{subdomain}.{domain}",
		Owner:      "websites",
		RepoFormat: "{subdomain}",
		Branch:     "main",
	}
	
	if !autoMapping.Enabled {
		t.Error("Expected Enabled to be true")
	}
	
	if autoMapping.Pattern != "{subdomain}.{domain}" {
		t.Errorf("Expected Pattern '{subdomain}.{domain}', got '%s'", autoMapping.Pattern)
	}
	
	if autoMapping.Owner != "websites" {
		t.Errorf("Expected Owner 'websites', got '%s'", autoMapping.Owner)
	}
	
	if autoMapping.RepoFormat != "{subdomain}" {
		t.Errorf("Expected RepoFormat '{subdomain}', got '%s'", autoMapping.RepoFormat)
	}
	
	if autoMapping.Branch != "main" {
		t.Errorf("Expected Branch 'main', got '%s'", autoMapping.Branch)
	}
}

func TestFormatRepoName(t *testing.T) {
	gp := new(GitteaPages)
	
	tests := []struct {
		name     string
		input    string
		format   string
		expected string
	}{
		{
			name:     "empty format returns input",
			input:    "test",
			format:   "",
			expected: "test",
		},
		{
			name:     "domain template",
			input:    "example.com",
			format:   "{domain}",
			expected: "example.com",
		},
		{
			name:     "subdomain template",
			input:    "blog",
			format:   "{subdomain}-site",
			expected: "blog-site",
		},
		{
			name:     "input template",
			input:    "myrepo",
			format:   "{input}",
			expected: "myrepo",
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

func TestShouldUpdateCache(t *testing.T) {
	gp := &GitteaPages{
		CacheTTL: caddy.Duration(15 * time.Minute),
		cache: &repoCache{
			repos: make(map[string]*cacheEntry),
		},
	}
	
	repoKey := "owner/repo"
	branch := "main"
	
	// Should update when entry doesn't exist
	if !gp.shouldUpdateCache(repoKey, branch) {
		t.Error("Expected shouldUpdateCache to return true for non-existent entry")
	}
	
	// Add a fresh entry
	cacheKey := repoKey + ":" + branch
	gp.cache.repos[cacheKey] = &cacheEntry{
		lastUpdate: time.Now(),
		path:       "/tmp/test",
	}
	
	// Should not update fresh entry
	if gp.shouldUpdateCache(repoKey, branch) {
		t.Error("Expected shouldUpdateCache to return false for fresh entry")
	}
	
	// Add an old entry
	gp.cache.repos[cacheKey] = &cacheEntry{
		lastUpdate: time.Now().Add(-30 * time.Minute),
		path:       "/tmp/test",
	}
	
	// Should update old entry
	if !gp.shouldUpdateCache(repoKey, branch) {
		t.Error("Expected shouldUpdateCache to return true for old entry")
	}
}
