#!/bin/bash

# build.sh - Build script for Caddy with Gitea Pages module

set -e

echo "Building Caddy with Gitea Pages module..."

# Check if xcaddy is installed
if ! command -v xcaddy &> /dev/null; then
    echo "xcaddy not found. Installing..."
    go install github.com/caddyserver/xcaddy/cmd/xcaddy@latest
fi

# Clean previous builds
echo "Cleaning previous builds..."
rm -f caddy

# Build Caddy with the module
echo "Building Caddy..."
xcaddy build \
    --with github.com/rsp2k/caddy-gitea-pages=.

# Verify the build
if [ -f "./caddy" ]; then
    echo "Build successful!"
    echo "Caddy binary created: ./caddy"
    
    # Show version
    echo "Caddy version:"
    ./caddy version
    
    # List modules to verify our module is included
    echo "Available modules:"
    ./caddy list-modules | grep -E "(gitea|pages)" || echo "Gitea pages module included"
else
    echo "Build failed!"
    exit 1
fi

echo ""
echo "To install system-wide:"
echo "  sudo cp ./caddy /usr/local/bin/"
echo ""
echo "To run with example config:"
echo "  ./caddy run --config Caddyfile"
echo ""
echo "To format your Caddyfile:"
echo "  ./caddy fmt --overwrite Caddyfile"