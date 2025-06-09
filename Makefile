.PHONY: help build test test-race test-short bench lint fmt vet clean install xcaddy-build docker-build docker-run

# Default target
help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# Build targets
build: ## Build the module
	go build -v ./...

install: ## Install dependencies
	go mod download
	go mod tidy

# Testing targets
test: ## Run all tests
	go test -v ./...

test-race: ## Run tests with race detection
	go test -v -race ./...

test-short: ## Run short tests only
	go test -v -short ./...

test-coverage: ## Run tests with coverage
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	go tool cover -func=coverage.out

bench: ## Run benchmarks
	go test -bench=. -benchmem ./...

# Code quality targets
lint: ## Run linters
	golangci-lint run

fmt: ## Format code
	gofmt -s -w .
	goimports -w .

vet: ## Run go vet
	go vet ./...

# Build targets
xcaddy-build: ## Build Caddy with xcaddy
	xcaddy build --with github.com/rsp2k/caddy-gitea-pages=.

# Docker targets
docker-build: ## Build Docker image
	docker build -t caddy-gitea-pages .

docker-run: ## Run Docker container
	docker run -p 80:80 -p 443:443 -p 2019:2019 caddy-gitea-pages

# Release targets
release-test: ## Test release build
	goreleaser release --snapshot --rm-dist

# Cleanup targets
clean: ## Clean build artifacts
	go clean -testcache
	rm -f coverage.out coverage.html
	rm -f caddy

# Development targets
dev-setup: ## Set up development environment
	go install github.com/caddyserver/xcaddy/cmd/xcaddy@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install golang.org/x/tools/cmd/goimports@latest

# CI targets
ci-test: test-race lint vet ## Run CI test suite locally

# Security targets
security: ## Run security scans
	gosec ./...
	nancy sleuth < go.sum
