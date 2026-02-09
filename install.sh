#!/bin/bash
# cckey installer

set -e

INSTALL_DIR="$HOME/.cckey"
SCRIPT_URL="https://raw.githubusercontent.com/huaguihai/cckey/main/cckey.sh"
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
    echo "# cckey: Claude Code API Key Manager" >> "$SHELL_RC"
    echo "$SOURCE_LINE" >> "$SHELL_RC"
    echo "Added cckey to $SHELL_RC"
else
    echo "cckey already configured in $SHELL_RC"
fi

echo "Done! Run 'source $SHELL_RC' or open a new terminal to start using cckey."
echo ""
echo "Quick start:"
echo "  cckey add main sk-ant-api03-your-key-here"
echo "  cckey use main"
echo "  cckey help"
