#!/bin/bash
# cckey installer — installs cckey.sh + optional cckey-proxy binary

set -e

INSTALL_DIR="$HOME/.cckey"
SCRIPT_URL="https://raw.githubusercontent.com/huaguihai/cckey/main/cckey.sh"
PROXY_REPO="huaguihai/cckey"
SHELL_RC=""

# Detect shell config file
if [ -n "$ZSH_VERSION" ] || [ "$(basename "$SHELL")" = "zsh" ]; then
    SHELL_RC="$HOME/.zshrc"
elif [ -n "$BASH_VERSION" ] || [ "$(basename "$SHELL")" = "bash" ]; then
    SHELL_RC="$HOME/.bashrc"
else
    SHELL_RC="$HOME/.profile"
fi

echo "Installing cckey..."

# Create directory and download script
mkdir -p "$INSTALL_DIR"
if command -v curl &>/dev/null; then
    curl -fsSL "$SCRIPT_URL" -o "$INSTALL_DIR/cckey.sh"
elif command -v wget &>/dev/null; then
    wget -qO "$INSTALL_DIR/cckey.sh" "$SCRIPT_URL"
else
    echo "Error: curl or wget is required."
    exit 1
fi

# Add source line to shell config if not already present
SOURCE_LINE='source "$HOME/.cckey/cckey.sh"'
if ! grep -qF ".cckey/cckey.sh" "$SHELL_RC" 2>/dev/null; then
    echo "" >> "$SHELL_RC"
    echo "# cckey: AI CLI API Key Manager" >> "$SHELL_RC"
    echo "$SOURCE_LINE" >> "$SHELL_RC"
    echo "Added cckey to $SHELL_RC"
else
    echo "cckey already configured in $SHELL_RC"
fi

# Try to install cckey-proxy binary
install_proxy() {
    local os arch
    os="$(uname -s | tr '[:upper:]' '[:lower:]')"
    arch="$(uname -m)"
    case "$arch" in
        x86_64|amd64) arch="amd64" ;;
        aarch64|arm64) arch="arm64" ;;
        *) echo "Unsupported architecture: $arch"; return 1 ;;
    esac

    local binary_name="cckey-proxy-${os}-${arch}"
    local download_url="https://github.com/${PROXY_REPO}/releases/latest/download/${binary_name}"
    local target="$INSTALL_DIR/cckey-proxy"

    echo "Downloading cckey-proxy (${os}/${arch})..."
    if command -v curl &>/dev/null; then
        if curl -fsSL "$download_url" -o "$target" 2>/dev/null; then
            chmod +x "$target"
            echo "cckey-proxy installed at $target"
            return 0
        fi
    elif command -v wget &>/dev/null; then
        if wget -qO "$target" "$download_url" 2>/dev/null; then
            chmod +x "$target"
            echo "cckey-proxy installed at $target"
            return 0
        fi
    fi
    echo "Note: cckey-proxy binary not found in releases."
    echo "  Build from source: cd cckey-proxy && go build -o ~/.cckey/cckey-proxy ."
    return 1
}

echo ""
install_proxy || true

echo ""
echo "Done! Run 'source $SHELL_RC' or open a new terminal to start using cckey."
echo ""
echo "Quick start:"
echo "  cckey add main sk-ant-api03-your-key-here"
echo "  cckey use main"
echo "  cckey help"
echo ""
echo "Proxy (optional):"
echo "  cckey proxy start    # Start local proxy for non-Anthropic upstreams"
echo "  cckey doctor         # Full diagnostics"
