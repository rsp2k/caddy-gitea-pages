# Contributing to Caddy Gitea Pages

Thank you for your interest in contributing to the Caddy Gitea Pages module! This document provides guidelines for contributing to the project.

## Ways to Contribute

- **Bug Reports**: Help us identify and fix issues
- **Feature Requests**: Suggest new functionality
- **Code Contributions**: Submit bug fixes and new features
- **Documentation**: Improve documentation and examples
- **Testing**: Help test the module in different environments

## Getting Started

### Prerequisites

- Go 1.21 or later
- Git
- xcaddy for building Caddy with custom modules
- Access to a Gitea instance for testing

### Setting Up Development Environment

1. **Fork the repository**
   ```bash
   # Fork on GitHub, then clone your fork
   git clone https://github.com/YOUR_USERNAME/caddy-gitea-pages.git
   cd caddy-gitea-pages
   ```

2. **Install dependencies**
   ```bash
   go mod tidy
   ```

3. **Install xcaddy**
   ```bash
   go install github.com/caddyserver/xcaddy/cmd/xcaddy@latest
   ```

4. **Build and test**
   ```bash
   chmod +x build.sh
   ./build.sh
   ```

## Development Workflow

### Making Changes

1. **Create a new branch**
   ```bash
   git checkout -b feature/your-feature-name
   # or
   git checkout -b fix/issue-description
   ```

2. **Make your changes**
   - Follow Go best practices
   - Add tests for new functionality
   - Update documentation as needed

3. **Test your changes**
   ```bash
   # Build the module
   ./build.sh
   
   # Test with a simple Caddyfile
   ./caddy run --config examples/Caddyfile
   ```

4. **Commit your changes**
   ```bash
   git add .
   git commit -m "feat: add new feature" # or "fix: resolve issue"
   ```

### Commit Message Convention

We follow conventional commits for clear commit history:

- `feat:` for new features
- `fix:` for bug fixes
- `docs:` for documentation changes
- `test:` for adding tests
- `refactor:` for code refactoring
- `chore:` for maintenance tasks

Examples:
- `feat: add webhook support for cache invalidation`
- `fix: resolve path traversal security issue`
- `docs: update configuration examples`

## Code Guidelines

### Go Style

- Follow standard Go formatting (use `gofmt`)
- Use meaningful variable and function names
- Add comments for exported functions and types
- Handle errors appropriately
- Follow Go best practices for concurrency

### Security Considerations

- Validate all user inputs
- Prevent path traversal attacks
- Secure handling of authentication tokens
- Follow principle of least privilege

### Performance

- Efficient caching strategies
- Minimize API calls to Gitea
- Proper resource cleanup
- Consider memory usage for large repositories

## Testing

### Manual Testing

1. **Set up test environment**
   - Local Gitea instance or access to a test instance
   - Test repositories with static content
   - Different domain configurations

2. **Test scenarios to verify**
   - Path-based routing (`/owner/repo/file`)
   - Explicit domain mappings
   - Auto-mapping patterns
   - Cache functionality
   - Error handling (404s, API failures)
   - Security (path traversal attempts)

### Automated Testing

We welcome contributions to add automated tests:

- Unit tests for individual functions
- Integration tests with mock Gitea API
- End-to-end tests with real Gitea instance

## Documentation

### README Updates

When adding new features, update:
- Configuration options table
- Usage examples
- Troubleshooting section if applicable

### Code Documentation

- Add GoDoc comments for exported types and functions
- Include usage examples in comments
- Document configuration options

### Examples

- Add new Caddyfile examples for new features
- Update existing examples when behavior changes
- Include real-world use cases

## Submitting Changes

### Pull Request Process

1. **Ensure your branch is up to date**
   ```bash
   git fetch origin
   git rebase origin/main
   ```

2. **Run final tests**
   ```bash
   go mod tidy
   ./build.sh
   # Test your changes
   ```

3. **Push your branch**
   ```bash
   git push origin feature/your-feature-name
   ```

4. **Create pull request**
   - Use a clear title describing the change
   - Reference any related issues
   - Provide detailed description of changes
   - Include testing steps

### Pull Request Template

```markdown
## Description
Brief description of changes

## Related Issues
Fixes #issue_number

## Changes Made
- [ ] Feature/fix implemented
- [ ] Tests added/updated
- [ ] Documentation updated
- [ ] Examples updated

## Testing
Steps to test the changes:
1. ...
2. ...

## Screenshots (if applicable)

## Checklist
- [ ] Code follows project style guidelines
- [ ] Self-review completed
- [ ] Documentation updated
- [ ] No breaking changes (or clearly documented)
```

## Code Review Process

1. **Automated checks**: Ensure all CI checks pass
2. **Peer review**: At least one maintainer will review
3. **Testing**: Changes will be tested in various environments
4. **Approval**: Once approved, changes will be merged

## Release Process

### Versioning

We follow semantic versioning (SemVer):
- **MAJOR**: Incompatible API changes
- **MINOR**: New functionality (backward compatible)
- **PATCH**: Bug fixes (backward compatible)

### Release Notes

Each release includes:
- Summary of changes
- New features
- Bug fixes
- Breaking changes (if any)
- Migration guide (if needed)

## Community Guidelines

### Code of Conduct

- Be respectful and inclusive
- Focus on constructive feedback
- Help others learn and grow
- Follow GitHub's community guidelines

### Getting Help

- **Issues**: Use GitHub issues for bugs and feature requests
- **Discussions**: Use GitHub discussions for questions
- **Documentation**: Check README and examples first

### Recognition

Contributors are recognized through:
- GitHub contributors list
- Release notes acknowledgments
- Community mentions

## Development Tips

### Debugging

1. **Enable debug logging**
   ```caddyfile
   {
       debug
   }
   ```

2. **Use structured logging**
   ```go
   gp.logger.Debug("debug message",
       zap.String("key", "value"),
       zap.Error(err))
   ```

3. **Test with curl**
   ```bash
   curl -v http://localhost:8080/owner/repo/
   ```

### Common Issues

- **Module not loading**: Check `go.mod` and build process
- **Cache issues**: Clear cache directory during development
- **API errors**: Verify Gitea URL and token
- **Domain mapping**: Check host header in requests

### Useful Resources

- [Caddy Module Development](https://caddyserver.com/docs/extending-caddy)
- [Go Documentation](https://golang.org/doc/)
- [Gitea API Documentation](https://docs.gitea.io/en-us/api-usage/)

Thank you for contributing to Caddy Gitea Pages!