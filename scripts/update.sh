#!/bin/bash
# Update script for Caddy Gitea Pages

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/caddy"
BACKUP_DIR="/var/backups/caddy"

# Functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

check_root() {
    if [[ $EUID -ne 0 ]]; then
        log_error "This script must be run as root"
        exit 1
    fi
}

check_installation() {
    if [[ ! -f "$INSTALL_DIR/caddy" ]]; then
        log_error "Caddy not found at $INSTALL_DIR/caddy"
        log_info "Please run the installation script first"
        exit 1
    fi
    
    if ! $INSTALL_DIR/caddy list-modules | grep -q gitea_pages; then
        log_error "Caddy Gitea Pages module not found"
        log_info "This might not be a Caddy Gitea Pages installation"
        exit 1
    fi
}

get_current_version() {
    CURRENT_VERSION=$($INSTALL_DIR/caddy version | head -n1)
    log_info "Current version: $CURRENT_VERSION"
}

backup_current() {
    log_info "Creating backup..."
    
    mkdir -p $BACKUP_DIR
    BACKUP_FILE="$BACKUP_DIR/caddy-$(date +%Y%m%d_%H%M%S)"
    
    cp $INSTALL_DIR/caddy $BACKUP_FILE
    
    log_success "Current binary backed up to $BACKUP_FILE"
}

build_new_version() {
    log_info "Building new version..."
    
    cd /tmp
    
    # Check if xcaddy is available
    if ! command -v xcaddy &> /dev/null; then
        if [[ -f "$(go env GOPATH)/bin/xcaddy" ]]; then
            XCADDY="$(go env GOPATH)/bin/xcaddy"
        else
            log_error "xcaddy not found. Installing..."
            go install github.com/caddyserver/xcaddy/cmd/xcaddy@latest
            XCADDY="$(go env GOPATH)/bin/xcaddy"
        fi
    else
        XCADDY="xcaddy"
    fi
    
    # Build with latest module version
    $XCADDY build --with github.com/rsp2k/caddy-gitea-pages@latest
    
    if [[ ! -f "./caddy" ]]; then
        log_error "Failed to build new version"
        exit 1
    fi
    
    log_success "New version built successfully"
}

test_new_version() {
    log_info "Testing new version..."
    
    # Basic version check
    if ! /tmp/caddy version &> /dev/null; then
        log_error "New version fails basic version check"
        exit 1
    fi
    
    # Module check
    if ! /tmp/caddy list-modules | grep -q gitea_pages; then
        log_error "Gitea Pages module not found in new version"
        exit 1
    fi
    
    # Configuration validation
    if [[ -f "$CONFIG_DIR/Caddyfile" ]]; then
        if ! /tmp/caddy validate --config "$CONFIG_DIR/Caddyfile" &> /dev/null; then
            log_error "New version fails configuration validation"
            log_warning "Your Caddyfile may need updates for the new version"
            exit 1
        fi
    fi
    
    log_success "New version passes tests"
}

install_new_version() {
    log_info "Installing new version..."
    
    # Stop service
    if systemctl is-active --quiet caddy; then
        log_info "Stopping Caddy service..."
        systemctl stop caddy
        NEED_RESTART=true
    else
        NEED_RESTART=false
    fi
    
    # Install new binary
    cp /tmp/caddy $INSTALL_DIR/caddy
    chmod +x $INSTALL_DIR/caddy
    
    # Start service if it was running
    if [[ "$NEED_RESTART" == "true" ]]; then
        log_info "Starting Caddy service..."
        systemctl start caddy
        
        # Wait a moment and check if it started successfully
        sleep 2
        if systemctl is-active --quiet caddy; then
            log_success "Service started successfully"
        else
            log_error "Service failed to start. Check logs: journalctl -u caddy"
            log_info "You may need to restore from backup: cp $BACKUP_FILE $INSTALL_DIR/caddy"
            exit 1
        fi
    fi
    
    log_success "New version installed"
}

get_new_version() {
    NEW_VERSION=$($INSTALL_DIR/caddy version | head -n1)
    log_success "Updated to: $NEW_VERSION"
}

cleanup() {
    rm -f /tmp/caddy
}

print_summary() {
    echo
    log_success "Update completed successfully!"
    echo
    echo -e "${BLUE}Summary:${NC}"
    echo "Previous version: $CURRENT_VERSION"
    echo "Current version:  $NEW_VERSION"
    echo "Backup location:  $BACKUP_FILE"
    echo
    echo -e "${BLUE}Useful commands:${NC}"
    echo "Check service status: systemctl status caddy"
    echo "View logs:           journalctl -u caddy -f"
    echo "Test configuration:  caddy validate --config $CONFIG_DIR/Caddyfile"
    echo
    if [[ "$NEED_RESTART" == "true" ]]; then
        echo -e "${GREEN}Service is running${NC}"
    else
        echo -e "${YELLOW}Service was not running - start with: systemctl start caddy${NC}"
    fi
}

# Rollback function
rollback() {
    log_info "Rolling back to previous version..."
    
    if [[ ! -f "$BACKUP_FILE" ]]; then
        log_error "Backup file not found: $BACKUP_FILE"
        exit 1
    fi
    
    systemctl stop caddy 2>/dev/null || true
    cp "$BACKUP_FILE" $INSTALL_DIR/caddy
    chmod +x $INSTALL_DIR/caddy
    systemctl start caddy
    
    log_success "Rollback completed"
}

# Main update process
main() {
    log_info "Starting Caddy Gitea Pages update..."
    
    check_root
    check_installation
    get_current_version
    backup_current
    build_new_version
    test_new_version
    install_new_version
    get_new_version
    cleanup
    
    print_summary
}

# Handle command line arguments
case "${1:-}" in
    --help|-h)
        echo "Caddy Gitea Pages Update Script"
        echo
        echo "Usage: $0 [OPTIONS]"
        echo
        echo "Options:"
        echo "  --help, -h     Show this help message"
        echo "  --rollback     Rollback to previous version"
        echo "  --check        Check for updates without installing"
        echo
        exit 0
        ;;
    --rollback)
        check_root
        # Find the most recent backup
        BACKUP_FILE=$(ls -t $BACKUP_DIR/caddy-* 2>/dev/null | head -n1)
        if [[ -z "$BACKUP_FILE" ]]; then
            log_error "No backup files found in $BACKUP_DIR"
            exit 1
        fi
        rollback
        exit 0
        ;;
    --check)
        log_info "Checking for updates..."
        check_installation
        get_current_version
        
        # This is a simplified check - in a real scenario you might
        # compare with GitHub releases or tags
        log_info "To update, run: $0"
        exit 0
        ;;
    "")
        main
        ;;
    *)
        log_error "Unknown option: $1"
        echo "Use --help for usage information"
        exit 1
        ;;
esac