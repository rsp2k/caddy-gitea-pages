package giteapages

import (
	"archive/tar"
	"compress/gzip"
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
	caddy.RegisterModule(GitteaPages{})
	httpcaddyfile.RegisterHandlerDirective("gitea_pages", parseCaddyfile)
	httpcaddyfile.RegisterDirectiveOrder("gitea_pages", httpcaddyfile.Before, "file_server")
}

// GitteaPages implements GitHub Pages functionality for Gitea
type GitteaPages struct {
	// Gitea server configuration
	GitteaURL   string `json:"gitea_url,omitempty"`
	GitteaToken string `json:"gitea_token,omitempty"`

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
	cache  *repoCache
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
	Pattern    string `json:"pattern,omitempty"`     // e.g., "{domain}" or "{subdomain}.{domain}"
	Owner      string `json:"owner,omitempty"`       // Default owner for auto-mapped repos
	RepoFormat string `json:"repo_format,omitempty"` // e.g., "{domain}" or "{subdomain}"
	Branch     string `json:"branch,omitempty"`      // Override default branch for auto-mapped repos
}

// repoCache manages cached repository contents
type repoCache struct {
	mu       sync.RWMutex
	repos    map[string]*cacheEntry
	cacheDir string
}

type cacheEntry struct {
	lastUpdate time.Time
	path       string
}

// GitteaRepo represents a repository from Gitea API
type GitteaRepo struct {
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	DefaultBranch string `json:"default_branch"`
	UpdatedAt     string `json:"updated_at"`
}

// CaddyModule returns the Caddy module information
func (GitteaPages) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.gitea_pages",
		New: func() caddy.Module { return new(GitteaPages) },
	}
}

// Provision sets up the module
func (gp *GitteaPages) Provision(ctx caddy.Context) error {
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
	gp.cache = &repoCache{
		repos:    make(map[string]*cacheEntry),
		cacheDir: gp.CacheDir,
	}

	gp.logger.Info("gitea_pages module provisioned",
		zap.String("gitea_url", gp.GitteaURL),
		zap.String("cache_dir", gp.CacheDir),
		zap.Duration("cache_ttl", time.Duration(gp.CacheTTL)))

	return nil
}

// ServeHTTP handles HTTP requests
func (gp *GitteaPages) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
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
		filePath = gp.findIndexFile(owner, repo)
		if filePath == "" {
			return next.ServeHTTP(w, r)
		}
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
func (gp *GitteaPages) serveFile(w http.ResponseWriter, r *http.Request, owner, repo, filePath, branch string) error {
	repoKey := fmt.Sprintf("%s/%s", owner, repo)

	// Check if we need to update the cache
	if gp.shouldUpdateCache(repoKey, branch) {
		if err := gp.updateRepoCache(owner, repo, branch); err != nil {
			return fmt.Errorf("failed to update cache: %v", err)
		}
	}

	// Get cached repo path
	cacheKey := fmt.Sprintf("%s:%s", repoKey, branch)
	gp.cache.mu.RLock()
	entry, exists := gp.cache.repos[cacheKey]
	gp.cache.mu.RUnlock()

	if !exists {
		return fmt.Errorf("repository not found in cache")
	}

	// Serve the file
	fullPath := filepath.Join(entry.path, filePath)

	// Security check: ensure the file is within the repository directory
	if !strings.HasPrefix(fullPath, entry.path) {
		return fmt.Errorf("invalid file path")
	}

	// Check if file exists
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return fmt.Errorf("file not found")
	}

	http.ServeFile(w, r, fullPath)
	return nil
}

// shouldUpdateCache checks if the cache needs updating
func (gp *GitteaPages) shouldUpdateCache(repoKey, branch string) bool {
	cacheKey := fmt.Sprintf("%s:%s", repoKey, branch)
	gp.cache.mu.RLock()
	entry, exists := gp.cache.repos[cacheKey]
	gp.cache.mu.RUnlock()

	if !exists {
		return true
	}

	return time.Since(entry.lastUpdate) > time.Duration(gp.CacheTTL)
}

// updateRepoCache downloads and caches repository content
func (gp *GitteaPages) updateRepoCache(owner, repo, branch string) error {
	repoKey := fmt.Sprintf("%s/%s", owner, repo)

	// Get repository info from Gitea API
	repoInfo, err := gp.getRepoInfo(owner, repo)
	if err != nil {
		return fmt.Errorf("failed to get repo info: %v", err)
	}

	// Use provided branch, fallback to repo default, then module default
	if branch == "" {
		if repoInfo.DefaultBranch != "" {
			branch = repoInfo.DefaultBranch
		} else {
			branch = gp.DefaultBranch
		}
	}

	// Download repository archive
	archiveURL := fmt.Sprintf("%s/api/v1/repos/%s/%s/archive/%s.tar.gz",
		strings.TrimRight(gp.GitteaURL, "/"), owner, repo, branch)

	cacheKey := fmt.Sprintf("%s:%s", repoKey, branch)
	if err := gp.downloadAndExtractRepo(archiveURL, cacheKey); err != nil {
		return fmt.Errorf("failed to download repo: %v", err)
	}

	// Update cache entry
	gp.cache.mu.Lock()
	gp.cache.repos[cacheKey] = &cacheEntry{
		lastUpdate: time.Now(),
		path:       filepath.Join(gp.cache.cacheDir, cacheKey),
	}
	gp.cache.mu.Unlock()

	gp.logger.Debug("updated repo cache",
		zap.String("repo", repoKey),
		zap.String("branch", branch))

	return nil
}

// getRepoInfo fetches repository information from Gitea API
func (gp *GitteaPages) getRepoInfo(owner, repo string) (*GitteaRepo, error) {
	url := fmt.Sprintf("%s/api/v1/repos/%s/%s",
		strings.TrimRight(gp.GitteaURL, "/"), owner, repo)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	if gp.GitteaToken != "" {
		req.Header.Set("Authorization", "token "+gp.GitteaToken)
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

	var repoInfo GitteaRepo
	if err := json.NewDecoder(resp.Body).Decode(&repoInfo); err != nil {
		return nil, err
	}

	return &repoInfo, nil
}

// downloadAndExtractRepo downloads and extracts repository archive
func (gp *GitteaPages) downloadAndExtractRepo(archiveURL, cacheKey string) error {
	// Create request
	req, err := http.NewRequest("GET", archiveURL, nil)
	if err != nil {
		return err
	}

	if gp.GitteaToken != "" {
		req.Header.Set("Authorization", "token "+gp.GitteaToken)
	}

	// Download archive
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download archive: status %d", resp.StatusCode)
	}

	// Extract archive to cache directory
	extractPath := filepath.Join(gp.cache.cacheDir, cacheKey)
	if err := os.RemoveAll(extractPath); err != nil {
		return err
	}
	if err := os.MkdirAll(extractPath, 0755); err != nil {
		return err
	}

	// Extract tar.gz archive
	gzr, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %v", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar header: %v", err)
		}

		// Skip the top-level directory from the archive
		pathParts := strings.Split(header.Name, "/")
		if len(pathParts) > 1 {
			relativePath := strings.Join(pathParts[1:], "/")
			targetPath := filepath.Join(extractPath, relativePath)

			// Security check: ensure the file is within the extract directory
			if !strings.HasPrefix(targetPath, extractPath) {
				continue
			}

			switch header.Typeflag {
			case tar.TypeDir:
				if err := os.MkdirAll(targetPath, os.FileMode(header.Mode)); err != nil {
					return fmt.Errorf("failed to create directory %s: %v", targetPath, err)
				}
			case tar.TypeReg:
				// Create parent directories if they don't exist
				if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
					return fmt.Errorf("failed to create parent directory for %s: %v", targetPath, err)
				}

				file, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY, os.FileMode(header.Mode))
				if err != nil {
					return fmt.Errorf("failed to create file %s: %v", targetPath, err)
				}

				if _, err := io.Copy(file, tr); err != nil {
					file.Close()
					return fmt.Errorf("failed to extract file %s: %v", targetPath, err)
				}
				file.Close()
			}
		}
	}

	gp.logger.Debug("extracted repository archive",
		zap.String("cache_key", cacheKey),
		zap.String("path", extractPath))

	return nil
}

// findIndexFile looks for index files in the repository
func (gp *GitteaPages) findIndexFile(owner, repo string) string {
	// Try with default branch first
	branch := gp.DefaultBranch
	cacheKey := fmt.Sprintf("%s/%s:%s", owner, repo, branch)

	gp.cache.mu.RLock()
	entry, exists := gp.cache.repos[cacheKey]
	gp.cache.mu.RUnlock()

	if !exists {
		return ""
	}

	for _, indexFile := range gp.IndexFiles {
		fullPath := filepath.Join(entry.path, indexFile)
		if _, err := os.Stat(fullPath); err == nil {
			return indexFile
		}
	}

	return ""
}

// resolveDomainMapping resolves a request to owner/repo based on domain mappings
func (gp *GitteaPages) resolveDomainMapping(r *http.Request) (owner, repo, filePath, branch string) {
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
func (gp *GitteaPages) resolveAutoMapping(host, filePath string) (owner, repo, newFilePath, branch string) {
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
func (gp *GitteaPages) formatRepoName(input, format string) string {
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
func (gp *GitteaPages) Validate() error {
	if gp.GitteaURL == "" {
		return fmt.Errorf("gitea_url is required")
	}
	return nil
}

// UnmarshalCaddyfile implements caddyfile.Unmarshaler.
func (gp *GitteaPages) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	for d.Next() {
		for d.NextBlock(0) {
			switch d.Val() {
			case "gitea_url":
				if !d.Args(&gp.GitteaURL) {
					return d.ArgErr()
				}
			case "gitea_token":
				if !d.Args(&gp.GitteaToken) {
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
	var gp GitteaPages
	err := gp.UnmarshalCaddyfile(h.Dispenser)
	if err != nil {
		return nil, err
	}
	return &gp, nil
}

// Interface guards
var (
	_ caddy.Provisioner              = (*GitteaPages)(nil)
	_ caddy.Validator                = (*GitteaPages)(nil)
	_ caddyhttp.MiddlewareHandler    = (*GitteaPages)(nil)
	_ caddyfile.Unmarshaler          = (*GitteaPages)(nil)
)
