#!/bin/bash
# Installation script for Caddy Gitea Pages

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
SERVICE_DIR="/etc/systemd/system"
CACHE_DIR="/var/cache/gitea-pages"
LOG_DIR="/var/log/caddy"
USER="caddy"
GROUP="caddy"

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

check_dependencies() {
    log_info "Checking dependencies..."
    
    # Check if Go is installed
    if ! command -v go &> /dev/null; then
        log_error "Go is not installed. Please install Go 1.21 or later."
        exit 1
    fi
    
    # Check Go version
    GO_VERSION=$(go version | grep -oP 'go\K[0-9]+\.[0-9]+')
    if [[ $(echo "$GO_VERSION < 1.21" | bc -l) -eq 1 ]]; then
        log_error "Go version $GO_VERSION is too old. Please install Go 1.21 or later."
        exit 1
    fi
    
    log_success "Go $GO_VERSION found"
}

install_xcaddy() {
    log_info "Installing xcaddy..."
    
    if command -v xcaddy &> /dev/null; then
        log_warning "xcaddy already installed"
        return
    fi
    
    go install github.com/caddyserver/xcaddy/cmd/xcaddy@latest
    
    # Add Go bin to PATH if not already there
    if ! echo $PATH | grep -q "$(go env GOPATH)/bin"; then
        echo 'export PATH=$PATH:$(go env GOPATH)/bin' >> /etc/profile
        export PATH=$PATH:$(go env GOPATH)/bin
    fi
    
    log_success "xcaddy installed"
}

build_caddy() {
    log_info "Building Caddy with Gitea Pages module..."
    
    cd /tmp
    $(go env GOPATH)/bin/xcaddy build --with github.com/rsp2k/caddy-gitea-pages
    
    if [[ ! -f "./caddy" ]]; then
        log_error "Failed to build Caddy"
        exit 1
    fi
    
    log_success "Caddy built successfully"
}

install_caddy() {
    log_info "Installing Caddy binary..."
    
    # Stop existing Caddy service if running
    if systemctl is-active --quiet caddy; then
        log_info "Stopping existing Caddy service..."
        systemctl stop caddy
    fi
    
    # Install binary
    cp /tmp/caddy $INSTALL_DIR/caddy
    chmod +x $INSTALL_DIR/caddy
    
    # Verify installation
    if $INSTALL_DIR/caddy version; then
        log_success "Caddy installed to $INSTALL_DIR/caddy"
    else
        log_error "Failed to install Caddy"
        exit 1
    fi
}

create_user() {
    log_info "Creating caddy user..."
    
    if id "$USER" &>/dev/null; then
        log_warning "User $USER already exists"
        return
    fi
    
    groupadd -r $GROUP
    useradd -r -g $GROUP -s /sbin/nologin -d /var/lib/caddy $USER
    
    log_success "User $USER created"
}

setup_directories() {
    log_info "Setting up directories..."
    
    # Create directories
    mkdir -p $CONFIG_DIR
    mkdir -p $CACHE_DIR
    mkdir -p $LOG_DIR
    mkdir -p /var/lib/caddy
    
    # Set permissions
    chown $USER:$GROUP $CONFIG_DIR
    chown $USER:$GROUP $CACHE_DIR
    chown $USER:$GROUP $LOG_DIR
    chown $USER:$GROUP /var/lib/caddy
    
    # Set proper permissions
    chmod 755 $CONFIG_DIR
    chmod 755 $CACHE_DIR
    chmod 755 $LOG_DIR
    chmod 755 /var/lib/caddy
    
    log_success "Directories created and configured"
}

install_systemd_service() {
    log_info "Installing systemd service..."
    
    cat > $SERVICE_DIR/caddy.service << EOF
[Unit]
Description=Caddy with Gitea Pages
Documentation=https://caddyserver.com/docs/
After=network.target network-online.target
Requires=network-online.target

[Service]
Type=notify
User=$USER
Group=$GROUP
ExecStart=$INSTALL_DIR/caddy run --environ --config $CONFIG_DIR/Caddyfile
ExecReload=$INSTALL_DIR/caddy reload --config $CONFIG_DIR/Caddyfile --force
TimeoutStopSec=5s
LimitNOFILE=1048576
LimitNPROC=1048576
PrivateTmp=true
ProtectSystem=full
AmbientCapabilities=CAP_NET_BIND_SERVICE
Restart=on-abnormal
RestartSec=5s
NoNewPrivileges=true
ProtectHome=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictNamespaces=true
LockPersonality=true
RestrictRealtime=true

[Install]
WantedBy=multi-user.target
EOF
    
    systemctl daemon-reload
    systemctl enable caddy
    
    log_success "Systemd service installed and enabled"
}

create_sample_config() {
    log_info "Creating sample configuration..."
    
    if [[ -f "$CONFIG_DIR/Caddyfile" ]]; then
        log_warning "Caddyfile already exists, creating backup"
        cp "$CONFIG_DIR/Caddyfile" "$CONFIG_DIR/Caddyfile.backup.$(date +%Y%m%d_%H%M%S)"
    fi
    
    cat > $CONFIG_DIR/Caddyfile << 'EOF'
# Caddy Gitea Pages Configuration
# Customize this configuration for your needs

# Example: pages.example.com serves repositories from Gitea
pages.example.com {
    gitea_pages {
        gitea_url https://git.example.com
        gitea_token {env.GITEA_TOKEN}
        
        # Auto-mapping: subdomain.pages.example.com -> user/subdomain repository
        auto_mapping {
            enabled true
            pattern {subdomain}.pages.{domain}
            owner users
            repo_format {subdomain}
        }
        
        cache_ttl 15m
        default_branch main
        index_files index.html index.htm
    }
    
    # Optional: Add security headers
    header {
        X-Content-Type-Options nosniff
        X-Frame-Options DENY
        X-XSS-Protection "1; mode=block"
    }
    
    # Optional: Enable compression
    encode gzip
    
    # Logging
    log {
        output file /var/log/caddy/access.log {
            roll_size 100mb
            roll_keep 5
        }
    }
}

# Health check endpoint
:8080 {
    handle /health {
        respond "OK" 200
    }
}
EOF
    
    chown $USER:$GROUP $CONFIG_DIR/Caddyfile
    chmod 644 $CONFIG_DIR/Caddyfile
    
    log_success "Sample configuration created at $CONFIG_DIR/Caddyfile"
}

create_environment_file() {
    log_info "Creating environment file..."
    
    cat > $CONFIG_DIR/environment << 'EOF'
# Caddy Gitea Pages Environment Variables
# Uncomment and set your values

# Gitea access token (required for private repositories)
# GITEA_TOKEN=your_gitea_token_here

# Cache directory (optional, overrides default)
# GITEA_PAGES_CACHE_DIR=/custom/cache/path

# Log level (optional)
# LOG_LEVEL=INFO
EOF
    
    chown $USER:$GROUP $CONFIG_DIR/environment
    chmod 600 $CONFIG_DIR/environment
    
    log_success "Environment file created at $CONFIG_DIR/environment"
}

print_next_steps() {
    log_success "Installation completed successfully!"
    echo
    echo -e "${BLUE}Next steps:${NC}"
    echo "1. Edit the configuration file: $CONFIG_DIR/Caddyfile"
    echo "2. Set your Gitea token in: $CONFIG_DIR/environment"
    echo "3. Start the service: systemctl start caddy"
    echo "4. Check service status: systemctl status caddy"
    echo "5. View logs: journalctl -u caddy -f"
    echo
    echo -e "${BLUE}Example commands:${NC}"
    echo "# Set Gitea token"
    echo "echo 'GITEA_TOKEN=your_token_here' >> $CONFIG_DIR/environment"
    echo
    echo "# Start service"
    echo "systemctl start caddy"
    echo
    echo "# Test health check"
    echo "curl http://localhost:8080/health"
    echo
    echo -e "${YELLOW}Documentation:${NC} https://github.com/rsp2k/caddy-gitea-pages"
}

# Main installation process
main() {
    log_info "Starting Caddy Gitea Pages installation..."
    
    check_root
    check_dependencies
    install_xcaddy
    build_caddy
    create_user
    setup_directories
    install_caddy
    install_systemd_service
    create_sample_config
    create_environment_file
    
    # Cleanup
    rm -f /tmp/caddy
    
    print_next_steps
}

# Handle command line arguments
case "${1:-}" in
    --help|-h)
        echo "Caddy Gitea Pages Installation Script"
        echo
        echo "Usage: $0 [OPTIONS]"
        echo
        echo "Options:"
        echo "  --help, -h     Show this help message"
        echo "  --uninstall    Remove Caddy Gitea Pages"
        echo
        exit 0
        ;;
    --uninstall)
        log_info "Uninstalling Caddy Gitea Pages..."
        systemctl stop caddy 2>/dev/null || true
        systemctl disable caddy 2>/dev/null || true
        rm -f $SERVICE_DIR/caddy.service
        rm -f $INSTALL_DIR/caddy
        systemctl daemon-reload
        log_success "Caddy Gitea Pages uninstalled"
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