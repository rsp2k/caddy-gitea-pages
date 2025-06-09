package giteapages

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"go.uber.org/zap"
)

func init() {
	caddy.RegisterModule(GiteaPages{})
	httpcaddyfile.RegisterHandlerDirective("gitea_pages", parseCaddyfile)
	httpcaddyfile.RegisterDirectiveOrder("gitea_pages", httpcaddyfile.Before, "file_server")
}

// GiteaPages implements GitHub Pages functionality for Gitea
type GiteaPages struct {
	// Gitea server configuration
	GiteaURL   string `json:"gitea_url,omitempty"`
	GiteaToken string `json:"gitea_token,omitempty"`

	// Local cache configuration
	CacheDir string        `json:"cache_dir,omitempty"`
	CacheTTL caddy.Duration `json:"cache_ttl,omitempty"`

	// Pages configuration
	DefaultBranch string   `json:"default_branch,omitempty"`
	IndexFiles    []string `json:"index_files,omitempty"`

	// Custom domain mapping
	DomainMappings []DomainMapping `json:"domain_mappings,omitempty"`
	AutoMapping    *AutoMapping    `json:"auto_mapping,omitempty"`

	// Internal fields
	logger *zap.Logger
	cache  *fileCache
}

// DomainMapping represents a custom domain to repository mapping
type DomainMapping struct {
	Domain     string `json:"domain"`
	Owner      string `json:"owner"`
	Repository string `json:"repository"`
	Branch     string `json:"branch,omitempty"`
}

// AutoMapping defines automatic domain-to-repository mapping rules
type AutoMapping struct {
	Enabled    bool   `json:"enabled,omitempty"`
	Pattern    string `json:"pattern,omitempty"`    // e.g., "{domain}" or "{subdomain}.{domain}"
	Owner      string `json:"owner,omitempty"`      // Default owner for auto-mapped repos
	RepoFormat string `json:"repo_format,omitempty"` // e.g., "{domain}" or "{subdomain}"
	Branch     string `json:"branch,omitempty"`     // Override default branch for auto-mapped repos
}

// fileCache manages cached individual files
type fileCache struct {
	mu       sync.RWMutex
	files    map[string]*cacheEntry
	cacheDir string
}

type cacheEntry struct {
	lastUpdate time.Time
	path       string
	etag       string
}

// GiteaRepo represents a repository from Gitea API
type GiteaRepo struct {
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	DefaultBranch string `json:"default_branch"`
	UpdatedAt     string `json:"updated_at"`
}

// GiteaFileInfo represents file information from Gitea API
type GiteaFileInfo struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Size        int64  `json:"size"`
	SHA         string `json:"sha"`
	Type        string `json:"type"`
	DownloadURL string `json:"download_url"`
}

// CaddyModule returns the Caddy module information
func (GiteaPages) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.gitea_pages",
		New: func() caddy.Module { return new(GiteaPages) },
	}
}

// Provision sets up the module
func (gp *GiteaPages) Provision(ctx caddy.Context) error {
	gp.logger = ctx.Logger(gp)

	// Set defaults
	if gp.CacheDir == "" {
		gp.CacheDir = filepath.Join(caddy.AppDataDir(), "gitea_pages_cache")
	}
	if gp.CacheTTL == 0 {
		gp.CacheTTL = caddy.Duration(15 * time.Minute)
	}
	if gp.DefaultBranch == "" {
		gp.DefaultBranch = "main"
	}
	if len(gp.IndexFiles) == 0 {
		gp.IndexFiles = []string{"index.html", "index.htm"}
	}

	// Create cache directory
	if err := os.MkdirAll(gp.CacheDir, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %v", err)
	}

	// Initialize cache
	gp.cache = &fileCache{
		files:    make(map[string]*cacheEntry),
		cacheDir: gp.CacheDir,
	}

	gp.logger.Info("gitea_pages module provisioned",
		zap.String("gitea_url", gp.GiteaURL),
		zap.String("cache_dir", gp.CacheDir),
		zap.Duration("cache_ttl", time.Duration(gp.CacheTTL)))

	return nil
}

// ServeHTTP handles HTTP requests
func (gp *GiteaPages) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	// Try to resolve the request using custom domain mapping
	owner, repo, filePath, branch := gp.resolveDomainMapping(r)

	if owner == "" || repo == "" {
		// Fallback to path-based routing if no domain mapping found
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) < 2 {
			return next.ServeHTTP(w, r)
		}

		owner = parts[0]
		repo = parts[1]
		filePath = strings.Join(parts[2:], "/")
	}

	// If no file path specified, look for index files
	if filePath == "" {
		foundIndex, err := gp.findIndexFile(owner, repo, branch)
		if err != nil || foundIndex == "" {
			return next.ServeHTTP(w, r)
		}
		filePath = foundIndex
	}

	// Use custom branch if specified, otherwise use default
	if branch == "" {
		branch = gp.DefaultBranch
	}

	// Serve the file from cache or fetch from Gitea
	if err := gp.serveFile(w, r, owner, repo, filePath, branch); err != nil {
		gp.logger.Error("failed to serve file",
			zap.String("owner", owner),
			zap.String("repo", repo),
			zap.String("file", filePath),
			zap.String("branch", branch),
			zap.Error(err))
		return next.ServeHTTP(w, r)
	}

	return nil
}

// serveFile serves a file from the repository
func (gp *GiteaPages) serveFile(w http.ResponseWriter, r *http.Request, owner, repo, filePath, branch string) error {
	fileKey := fmt.Sprintf("%s/%s:%s:%s", owner, repo, branch, filePath)

	// Check if we need to update the cache
	if gp.shouldUpdateCache(fileKey) {
		if err := gp.updateFileCache(owner, repo, filePath, branch); err != nil {
			return fmt.Errorf("failed to update cache: %v", err)
		}
	}

	// Get cached file path
	gp.cache.mu.RLock()
	entry, exists := gp.cache.files[fileKey]
	gp.cache.mu.RUnlock()

	if !exists {
		return fmt.Errorf("file not found in cache")
	}

	// Security check: ensure the file is within the cache directory
	if !strings.HasPrefix(entry.path, gp.cache.cacheDir) {
		return fmt.Errorf("invalid file path")
	}

	// Check if file exists
	if _, err := os.Stat(entry.path); os.IsNotExist(err) {
		return fmt.Errorf("file not found")
	}

	http.ServeFile(w, r, entry.path)
	return nil
}

// shouldUpdateCache checks if the cache needs updating
func (gp *GiteaPages) shouldUpdateCache(fileKey string) bool {
	gp.cache.mu.RLock()
	entry, exists := gp.cache.files[fileKey]
	gp.cache.mu.RUnlock()

	if !exists {
		return true
	}

	return time.Since(entry.lastUpdate) > time.Duration(gp.CacheTTL)
}

// updateFileCache downloads and caches an individual file
func (gp *GiteaPages) updateFileCache(owner, repo, filePath, branch string) error {
	// Get file info from Gitea API
	fileInfo, err := gp.getFileInfo(owner, repo, filePath, branch)
	if err != nil {
		return fmt.Errorf("failed to get file info: %v", err)
	}

	fileKey := fmt.Sprintf("%s/%s:%s:%s", owner, repo, branch, filePath)
	
	// Create cache subdirectory for this repo/branch
	cacheSubDir := filepath.Join(gp.cache.cacheDir, owner, repo, branch)
	if err := os.MkdirAll(cacheSubDir, 0755); err != nil {
		return fmt.Errorf("failed to create cache subdirectory: %v", err)
	}

	// Download the file
	cachedFilePath := filepath.Join(cacheSubDir, filepath.Base(filePath))
	if err := gp.downloadFile(fileInfo.DownloadURL, cachedFilePath); err != nil {
		return fmt.Errorf("failed to download file: %v", err)
	}

	// Update cache entry
	gp.cache.mu.Lock()
	gp.cache.files[fileKey] = &cacheEntry{
		lastUpdate: time.Now(),
		path:       cachedFilePath,
		etag:       fileInfo.SHA,
	}
	gp.cache.mu.Unlock()

	gp.logger.Debug("updated file cache",
		zap.String("file_key", fileKey),
		zap.String("path", cachedFilePath))

	return nil
}

// getFileInfo fetches file information from Gitea API
func (gp *GiteaPages) getFileInfo(owner, repo, filePath, branch string) (*GiteaFileInfo, error) {
	url := fmt.Sprintf("%s/api/v1/repos/%s/%s/contents/%s?ref=%s",
		strings.TrimRight(gp.GiteaURL, "/"), owner, repo, filePath, branch)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	if gp.GiteaToken != "" {
		req.Header.Set("Authorization", "token "+gp.GiteaToken)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitea API returned status %d", resp.StatusCode)
	}

	var fileInfo GiteaFileInfo
	if err := json.NewDecoder(resp.Body).Decode(&fileInfo); err != nil {
		return nil, err
	}

	return &fileInfo, nil
}

// downloadFile downloads a file from the given URL to the specified path
func (gp *GiteaPages) downloadFile(url, filePath string) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	if gp.GiteaToken != "" {
		req.Header.Set("Authorization", "token "+gp.GiteaToken)
	}

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download file: status %d", resp.StatusCode)
	}

	// Create the file
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %v", filePath, err)
	}
	defer file.Close()

	// Copy the response body to the file
	if _, err := io.Copy(file, resp.Body); err != nil {
		return fmt.Errorf("failed to write file %s: %v", filePath, err)
	}

	return nil
}

// findIndexFile looks for index files in the repository
func (gp *GiteaPages) findIndexFile(owner, repo, branch string) (string, error) {
	if branch == "" {
		branch = gp.DefaultBranch
	}

	for _, indexFile := range gp.IndexFiles {
		_, err := gp.getFileInfo(owner, repo, indexFile, branch)
		if err == nil {
			return indexFile, nil
		}
	}

	return "", nil
}

// resolveDomainMapping resolves a request to owner/repo based on domain mappings
func (gp *GiteaPages) resolveDomainMapping(r *http.Request) (owner, repo, filePath, branch string) {
	host := r.Host

	// Remove port if present
	if colonIndex := strings.Index(host, ":"); colonIndex != -1 {
		host = host[:colonIndex]
	}

	filePath = strings.Trim(r.URL.Path, "/")

	// Check explicit domain mappings first
	for _, mapping := range gp.DomainMappings {
		if mapping.Domain == host {
			return mapping.Owner, mapping.Repository, filePath, mapping.Branch
		}
	}

	// Check auto-mapping if enabled
	if gp.AutoMapping != nil && gp.AutoMapping.Enabled {
		return gp.resolveAutoMapping(host, filePath)
	}

	return "", "", "", ""
}

// resolveAutoMapping handles automatic domain-to-repository mapping
func (gp *GiteaPages) resolveAutoMapping(host, filePath string) (owner, repo, newFilePath, branch string) {
	if gp.AutoMapping == nil {
		return "", "", "", ""
	}

	owner = gp.AutoMapping.Owner
	branch = gp.AutoMapping.Branch
	newFilePath = filePath

	// Parse the domain based on the pattern
	switch gp.AutoMapping.Pattern {
	case "{domain}":
		// Direct domain mapping: example.com -> example.com repo
		repo = gp.formatRepoName(host, gp.AutoMapping.RepoFormat)

	case "{subdomain}.{domain}":
		// Subdomain mapping: blog.example.com -> blog repo
		parts := strings.Split(host, ".")
		if len(parts) >= 2 {
			subdomain := parts[0]
			repo = gp.formatRepoName(subdomain, gp.AutoMapping.RepoFormat)
		}

	case "{user}.pages.{domain}":
		// User pages: john.pages.example.com -> john/john.pages.example.com repo
		parts := strings.Split(host, ".")
		if len(parts) >= 3 && parts[1] == "pages" {
			username := parts[0]
			owner = username
			repo = gp.formatRepoName(host, gp.AutoMapping.RepoFormat)
		}

	default:
		// Custom pattern - basic template replacement
		pattern := gp.AutoMapping.Pattern
		if strings.Contains(pattern, "{domain}") {
			pattern = strings.ReplaceAll(pattern, "{domain}", host)
		}
		if strings.Contains(pattern, "{subdomain}") {
			parts := strings.Split(host, ".")
			if len(parts) > 0 {
				pattern = strings.ReplaceAll(pattern, "{subdomain}", parts[0])
			}
		}
		repo = pattern
	}

	// Validate that we have both owner and repo
	if owner == "" || repo == "" {
		return "", "", "", ""
	}

	return owner, repo, newFilePath, branch
}

// formatRepoName formats the repository name based on the format string
func (gp *GiteaPages) formatRepoName(input, format string) string {
	if format == "" {
		return input
	}

	// Simple template replacement
	result := format
	result = strings.ReplaceAll(result, "{domain}", input)
	result = strings.ReplaceAll(result, "{subdomain}", input)
	result = strings.ReplaceAll(result, "{input}", input)

	return result
}

// Validate validates the module configuration
func (gp *GiteaPages) Validate() error {
	if gp.GiteaURL == "" {
		return fmt.Errorf("gitea_url is required")
	}
	return nil
}

// UnmarshalCaddyfile implements caddyfile.Unmarshaler.
func (gp *GiteaPages) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	for d.Next() {
		for d.NextBlock(0) {
			switch d.Val() {
			case "gitea_url":
				if !d.Args(&gp.GiteaURL) {
					return d.ArgErr()
				}
			case "gitea_token":
				if !d.Args(&gp.GiteaToken) {
					return d.ArgErr()
				}
			case "cache_dir":
				if !d.Args(&gp.CacheDir) {
					return d.ArgErr()
				}
			case "cache_ttl":
				var ttl string
				if !d.Args(&ttl) {
					return d.ArgErr()
				}
				duration, err := time.ParseDuration(ttl)
				if err != nil {
					return d.Errf("invalid cache_ttl: %v", err)
				}
				gp.CacheTTL = caddy.Duration(duration)
			case "default_branch":
				if !d.Args(&gp.DefaultBranch) {
					return d.ArgErr()
				}
			case "index_files":
				gp.IndexFiles = d.RemainingArgs()
				if len(gp.IndexFiles) == 0 {
					return d.ArgErr()
				}
			case "domain_mapping":
				args := d.RemainingArgs()
				if len(args) < 3 {
					return d.ArgErr()
				}
				mapping := DomainMapping{
					Domain:     args[0],
					Owner:      args[1],
					Repository: args[2],
				}
				if len(args) > 3 {
					mapping.Branch = args[3]
				}
				gp.DomainMappings = append(gp.DomainMappings, mapping)
			case "auto_mapping":
				if gp.AutoMapping == nil {
					gp.AutoMapping = &AutoMapping{}
				}
				for d.NextBlock(1) {
					switch d.Val() {
					case "enabled":
						var enabled string
						if !d.Args(&enabled) {
							return d.ArgErr()
						}
						gp.AutoMapping.Enabled = enabled == "true"
					case "pattern":
						if !d.Args(&gp.AutoMapping.Pattern) {
							return d.ArgErr()
						}
					case "owner":
						if !d.Args(&gp.AutoMapping.Owner) {
							return d.ArgErr()
						}
					case "repo_format":
						if !d.Args(&gp.AutoMapping.RepoFormat) {
							return d.ArgErr()
						}
					case "branch":
						if !d.Args(&gp.AutoMapping.Branch) {
							return d.ArgErr()
						}
					default:
						return d.Errf("unknown auto_mapping subdirective: %s", d.Val())
					}
				}
			default:
				return d.Errf("unknown subdirective: %s", d.Val())
			}
		}
	}

	return nil
}

// parseCaddyfile parses the Caddyfile configuration
func parseCaddyfile(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	var gp GiteaPages
	err := gp.UnmarshalCaddyfile(h.Dispenser)
	if err != nil {
		return nil, err
	}
	return &gp, nil
}

// Interface guards
var (
	_ caddy.Provisioner                = (*GiteaPages)(nil)
	_ caddy.Validator                  = (*GiteaPages)(nil)
	_ caddyhttp.MiddlewareHandler      = (*GiteaPages)(nil)
	_ caddyfile.Unmarshaler            = (*GiteaPages)(nil)
)