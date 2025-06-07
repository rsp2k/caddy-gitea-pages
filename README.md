# Caddy Gitea Pages Module

A Caddy module that implements GitHub Pages-like functionality for self-hosted Gitea installations. This module allows you to serve static websites directly from Gitea repositories, similar to how GitHub Pages works.

## Features

- **Static Site Hosting**: Serve static websites directly from Gitea repositories
- **Custom Domain Mapping**: Map custom domains to specific repositories
- **Automatic Domain Resolution**: Auto-map domains to repositories using configurable patterns
- **Branch Support**: Serve from specific branches per domain or use repository defaults
- **Automatic Caching**: Built-in caching system to improve performance
- **Index File Detection**: Automatically serves index.html or custom index files
- **Token Authentication**: Support for private repositories via Gitea tokens
- **Flexible Configuration**: Multiple hosting patterns supported

## Installation

### Option 1: Using xcaddy (Recommended)

First, install xcaddy if you haven't already:
```bash
go install github.com/caddyserver/xcaddy/cmd/xcaddy@latest
```

Then build Caddy with the module:
```bash
xcaddy build --with github.com/rsp2k/caddy-gitea-pages
```

### Option 2: Manual Build

1. Install xcaddy:
   ```bash
   go install github.com/caddyserver/xcaddy/cmd/xcaddy@latest
   ```

2. Clone this repository:
   ```bash
   git clone https://github.com/rsp2k/caddy-gitea-pages.git
   cd caddy-gitea-pages
   ```

3. Build Caddy with the module:
   ```bash
   go mod tidy
   xcaddy build --with github.com/rsp2k/caddy-gitea-pages=.
   ```

## Configuration

### Basic Caddyfile Configuration

```caddyfile
pages.example.com {
    gitea_pages {
        gitea_url https://git.example.com
        gitea_token your_gitea_access_token_here
        cache_ttl 15m
    }
}
```

You can also use environment variables for the token:
```caddyfile
pages.example.com {
    gitea_pages {
        gitea_url https://git.example.com
        gitea_token {env.GITEA_TOKEN}
        cache_ttl 15m
    }
}
```

### Configuration Options

| Option | Description | Default |
|--------|-------------|---------|
| `gitea_url` | URL of your Gitea instance | Required |
| `gitea_token` | Gitea access token for API access | Optional |
| `cache_dir` | Directory for caching repositories | `$CADDY_DATA/gitea_pages_cache` |
| `cache_ttl` | Cache time-to-live | `15m` |
| `default_branch` | Default branch to serve | `main` |
| `index_files` | Index files to look for | `index.html index.htm` |
| `domain_mapping` | Explicit domain to repository mapping | None |
| `auto_mapping` | Automatic domain mapping configuration | Disabled |

### Domain Mapping Configuration

#### Explicit Domain Mappings

```caddyfile
gitea_pages {
    domain_mapping example.com owner repository branch
    domain_mapping blog.example.com user blog-repo main
}
```

#### Auto Mapping

```caddyfile
gitea_pages {
    auto_mapping {
        enabled true
        pattern {subdomain}.{domain}
        owner myorg
        repo_format {subdomain}
        branch main
    }
}
```

**Auto Mapping Options:**

| Option | Description | Example |
|--------|-------------|---------|
| `enabled` | Enable auto mapping | `true` |
| `pattern` | Domain pattern to match | `{subdomain}.{domain}`, `{domain}`, `{user}.pages.{domain}` |
| `owner` | Default repository owner | `myorg` |
| `repo_format` | Repository name format | `{subdomain}`, `{domain}`, `{input}` |
| `branch` | Branch override for auto-mapped repos | `main` |

### Token Configuration

You have two options for configuring the Gitea token:

#### Option 1: Direct in Caddyfile
```caddyfile
gitea_pages {
    gitea_url https://git.example.com
    gitea_token your_actual_token_here
}
```

#### Option 2: Environment Variable (Recommended)
```bash
export GITEA_TOKEN=your_gitea_access_token_here
```

```caddyfile
gitea_pages {
    gitea_url https://git.example.com
    gitea_token {env.GITEA_TOKEN}
}
```

## Usage Patterns

### Pattern 1: Path-based Routing (Traditional)

```
https://pages.example.com/owner/repo/
```

Repository: `owner/repo`  
Serves: Files from the repository's default branch

### Pattern 2: Explicit Domain Mapping

```caddyfile
blog.example.com {
    gitea_pages {
        gitea_url https://git.example.com
        gitea_token your_token_here
        domain_mapping blog.example.com johndoe personal-blog
    }
}
```

**Result**: `blog.example.com` serves from `johndoe/personal-blog` repository

### Pattern 3: Subdomain Auto-mapping

```caddyfile
*.example.com {
    gitea_pages {
        gitea_url https://git.example.com
        gitea_token your_token_here
        auto_mapping {
            enabled true
            pattern {subdomain}.{domain}
            owner websites
            repo_format {subdomain}
        }
    }
}
```

**Results**:
- `blog.example.com` → `websites/blog` repository
- `docs.example.com` → `websites/docs` repository
- `api.example.com` → `websites/api` repository

### Pattern 4: User Pages

```caddyfile
*.pages.example.com {
    gitea_pages {
        gitea_url https://git.example.com
        gitea_token your_token_here
        auto_mapping {
            enabled true
            pattern {user}.pages.{domain}
            repo_format {domain}
        }
    }
}
```

**Results**:
- `john.pages.example.com` → `john/john.pages.example.com` repository
- `jane.pages.example.com` → `jane/jane.pages.example.com` repository

### Pattern 5: Direct Domain Mapping

```caddyfile
example.com {
    gitea_pages {
        gitea_url https://git.example.com
        gitea_token your_token_here
        auto_mapping {
            enabled true
            pattern {domain}
            owner mainsite
            repo_format {domain}
        }
    }
}
```

**Result**: `example.com` → `mainsite/example.com` repository

### Pattern 6: Mixed Configuration

```caddyfile
*.example.com {
    gitea_pages {
        gitea_url https://git.example.com
        gitea_token your_token_here
        
        # Explicit mappings (highest priority)
        domain_mapping blog.example.com johndoe personal-blog
        domain_mapping api.example.com backend api-docs
        
        # Auto-mapping for other subdomains
        auto_mapping {
            enabled true
            pattern {subdomain}.{domain}
            owner sites
            repo_format {subdomain}-site
        }
    }
}
```

**Priority Order**:
1. Explicit domain mappings
2. Auto-mapping patterns
3. Fallback to path-based routing

## Domain Mapping Examples

### GitHub Pages Style User/Organization Sites

```caddyfile
# User pages: username.github.io equivalent
*.pages.example.com {
    gitea_pages {
        gitea_url https://git.example.com
        gitea_token your_token_here
        auto_mapping {
            enabled true
            pattern {user}.pages.{domain}
            repo_format {user}.pages.{domain}
        }
    }
}
```

### Project Pages with Custom Domains

```caddyfile
# Multiple project sites
*.projects.example.com {
    gitea_pages {
        gitea_url https://git.example.com
        gitea_token your_token_here
        auto_mapping {
            enabled true
            pattern {project}.projects.{domain}
            owner myorg
            repo_format {project}
            branch gh-pages
        }
    }
}
```

### Corporate Website Setup

```caddyfile
# Main corporate sites
*.example.com {
    gitea_pages {
        gitea_url https://git.example.com
        gitea_token your_token_here
        
        # Main website
        domain_mapping example.com corporate main-website
        
        # Specific sections
        domain_mapping blog.example.com marketing blog
        domain_mapping docs.example.com engineering documentation
        domain_mapping support.example.com customer-success help-center
        
        # Auto-mapping for other subdomains
        auto_mapping {
            enabled true
            pattern {subdomain}.{domain}
            owner websites
            repo_format {subdomain}
        }
    }
}
```

## API Integration

The module uses Gitea's REST API to:

- Fetch repository information
- Download repository archives
- Check for updates

### Required API Permissions

For public repositories: No token required  
For private repositories: Token with repository read access

### Creating a Gitea Token

1. Go to your Gitea instance
2. Navigate to Settings → Applications
3. Generate a new token with repository permissions
4. Use the token in your Caddyfile configuration

## Caching Strategy

The module implements a two-level caching system:

1. **Repository Cache**: Downloads and extracts repository contents locally
2. **TTL-based Updates**: Checks for updates based on configured cache TTL

### Cache Management

- Cache is stored in the configured `cache_dir`
- Repositories are re-downloaded when cache expires
- Cache survives Caddy restarts

## Security Considerations

- **Token Security**: Use environment variables for tokens in production
- **Repository Access**: Module respects Gitea's repository permissions
- **Path Traversal**: Built-in protection against directory traversal attacks
- **Rate Limiting**: Consider implementing rate limiting for high-traffic sites

## Troubleshooting

### Common Issues

1. **Repository Not Found**
   - Check repository name and owner
   - Verify token permissions for private repos

2. **Files Not Updating**
   - Check cache TTL configuration
   - Clear cache directory if needed

3. **API Connection Issues**
   - Verify Gitea URL is accessible
   - Check token validity

### Debug Logging

Enable debug logging in your Caddyfile:

```caddyfile
{
    debug
}

pages.example.com {
    gitea_pages {
        gitea_url https://git.example.com
        gitea_token your_token_here
    }
    
    log {
        level DEBUG
    }
}
```

## Performance Optimization

- Set appropriate cache TTL based on update frequency
- Use a fast storage backend for cache directory
- Consider using a CDN for static assets
- Monitor repository sizes to avoid excessive downloads

## Contributing

1. Fork the repository
2. Create a feature branch
3. Add tests for new functionality
4. Submit a pull request

## License

This project is licensed under the Apache 2.0 License - see the LICENSE file for details.

## Support

- GitHub Issues: Report bugs and feature requests
- Documentation: Check the wiki for additional examples
- Community: Join discussions in the issues section