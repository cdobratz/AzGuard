#!/bin/bash
# azguard install script
# Usage: curl -sSL https://azguard.dev/install.sh | bash

set -e

VERSION="latest"
INSTALL_DIR="/usr/local/bin"
REPO="cdobratz/AzGuard"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Detect OS
detect_os() {
    case "$(uname -s)" in
        Linux*)     echo "linux";;
        Darwin*)     echo "darwin";;
        CYGWIN*)     echo "windows";;
        MINGW*)      echo "windows";;
        MSYS*)       echo "windows";;
        *)           echo "linux";;
    esac
}

# Detect architecture
detect_arch() {
    case "$(uname -m)" in
        x86_64)     echo "amd64";;
        aarch64)    echo "arm64";;
        arm64)       echo "arm64";;
        *)           echo "amd64";;
    esac
}

# Get latest version
get_latest_version() {
    if command -v curl &> /dev/null; then
        curl -sL https://api.github.com/repos/${REPO}/releases/latest 2>/dev/null | grep -o '"tag_name":.*' | cut -d'"' -f4 | sed 's/v//'
    else
        echo "latest"
    fi
}

# Download and install
install() {
    local os=$1
    local arch=$2
    local version=$3
    
    if [ "$version" = "latest" ] || [ -z "$version" ]; then
        version=$(get_latest_version)
    fi
    
    local filename="azguard_${version}_${os}_${arch}"
    local ext="zip"
    if [ "$os" = "linux" ]; then
        ext="tar.gz"
    fi
    
    local url="https://github.com/${REPO}/releases/download/v${version}/${filename}.${ext}"
    
    log_info "Downloading azguard v${version} for ${os}/${arch}..."
    
    local tmp_dir=$(mktemp -d)
    local download_file="${tmp_dir}/azguard.${ext}"
    
    # Download
    if command -v curl &> /dev/null; then
        curl -sL -o "$download_file" "$url"
    elif command -v wget &> /dev/null; then
        wget -q -O "$download_file" "$url"
    else
        log_error "curl or wget required"
        exit 1
    fi
    
    # Extract
    log_info "Extracting..."
    cd "$tmp_dir"
    
    if [ "$ext" = "zip" ]; then
        unzip -q -o azguard.zip
    else
        tar -xzf azguard.tar.gz
    fi
    
    # Install
    log_info "Installing to ${INSTALL_DIR}..."
    
    if [ -w "$INSTALL_DIR" ]; then
        cp azguard "$INSTALL_DIR/"
    else
        log_warn "Need sudo to install to ${INSTALL_DIR}"
        sudo cp azguard "$INSTALL_DIR/"
    fi
    
    # Cleanup
    rm -rf "$tmp_dir"
    
    log_info "Installed successfully!"
    
    # Verify
    if command -v azguard &> /dev/null; then
        echo ""
        echo "Run 'azguard --help' to get started"
        azguard --version 2>/dev/null || azguard status --help | head -3
    fi
}

# Main
main() {
    echo "🛡️  azguard installer"
    echo "===================="
    echo ""
    
    local os=$(detect_os)
    local arch=$(detect_arch)
    
    log_info "Detected: ${os}/${arch}"
    
    install "$os" "$arch" "$VERSION"
}

main "$@"
