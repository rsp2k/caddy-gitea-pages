# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Nothing yet

### Changed
- Nothing yet

### Fixed
- Nothing yet

## [1.0.0] - 2025-06-07

### Added
- Initial release of Caddy Gitea Pages module
- GitHub Pages-like functionality for Gitea repositories
- Custom domain mapping support
- Automatic domain-to-repository mapping
- Explicit domain mappings
- Repository caching with configurable TTL
- Branch-specific serving
- Index file detection and serving
- Authentication support for private repositories
- Comprehensive configuration options
- Path-based routing fallback
- Security features (path traversal protection)
- Performance optimizations (caching, compression)
- Complete documentation and examples
- Docker and Kubernetes deployment examples
- Monitoring and alerting configurations
- Installation and update scripts
- CI/CD workflows
- Contributing guidelines
- Troubleshooting guide

### Security
- Built-in path traversal protection
- Secure token handling via environment variables
- Proper file system permissions
- Input validation and sanitization

### Documentation
- Comprehensive README with usage examples
- Configuration guide with all options
- Troubleshooting guide for common issues
- Deployment examples for various environments
- API integration documentation
- Performance tuning guidelines
- Security best practices

[Unreleased]: https://github.com/rsp2k/caddy-gitea-pages/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/rsp2k/caddy-gitea-pages/releases/tag/v1.0.0