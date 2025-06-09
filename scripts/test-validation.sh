#!/bin/bash
# test-validation.sh - Quick test validation script

set -e

echo "🧪 Caddy Gitea Pages - Test Validation"
echo "======================================"

# Check if we're in the right directory
if [ ! -f "giteapages.go" ]; then
    echo "❌ Error: Must be run from the caddy-gitea-pages root directory"
    exit 1
fi

echo "✅ Checking Go environment..."
go version

echo "✅ Cleaning module cache..."
go clean -modcache -cache
go mod tidy

echo "✅ Verifying dependencies..."
go mod verify

echo "✅ Running go vet..."
go vet ./...

echo "✅ Running basic tests..."
go test -v -short ./...

echo "✅ Running tests with race detection..."
go test -v -race -short ./...

echo "✅ Running benchmarks (quick)..."
go test -bench=. -benchtime=1s -short ./...

echo "✅ Checking test coverage..."
go test -coverprofile=coverage.out ./...
COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}')
echo "📊 Total test coverage: $COVERAGE"

# Check if coverage is reasonable (>70%)
COVERAGE_NUM=$(echo $COVERAGE | sed 's/%//')
if (( $(echo "$COVERAGE_NUM >= 70" | bc -l) )); then
    echo "✅ Coverage is good (>70%)"
else
    echo "⚠️  Coverage could be improved (<70%)"
fi

echo ""
echo "🎉 All basic tests completed successfully!"
echo "🔧 Ready for CI/CD pipeline"
echo ""
echo "To run the full test suite:"
echo "  make test           # Full test suite"
echo "  make test-coverage  # With coverage report"
echo "  make bench          # Performance benchmarks"
