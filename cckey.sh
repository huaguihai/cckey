#!/bin/bash
# cckey - Claude Code API Key Manager
# Version: 0.2.0
# https://github.com/huaguihai/cckey
#
# A lightweight CLI tool for managing multiple Anthropic API keys.
# Designed for headless Linux servers where GUI tools like cc-switch cannot run.

CCKEY_VERSION="0.2.0"
KEYS_DIR="$HOME/.cckey"
KEYS_FILE="$KEYS_DIR/keys.conf"
CURRENT_FILE="$KEYS_DIR/current"
CLAUDE_SETTINGS="$HOME/.claude/settings.json"

mkdir -p "$KEYS_DIR"
touch "$KEYS_FILE"

_cckey_sync_settings() {
    local key="$1" url="$2"
    [ ! -f "$CLAUDE_SETTINGS" ] && return
    if ! command -v jq &>/dev/null; then
        echo "Warning: jq not installed, skipping settings.json sync"
        return
    fi
    local tmp="${CLAUDE_SETTINGS}.tmp"
    jq --arg key "$key" --arg url "$url" '
        .env.ANTHROPIC_AUTH_TOKEN = $key |
        if $url != "" then .env.ANTHROPIC_BASE_URL = $url
        else del(.env.ANTHROPIC_BASE_URL) end
    ' "$CLAUDE_SETTINGS" > "$tmp" && mv "$tmp" "$CLAUDE_SETTINGS"
}

_cckey_list() {
    if [ ! -s "$KEYS_FILE" ]; then
        echo "No keys configured. Use: cckey add <name> <api_key> [base_url]"
        return 1
    fi
    local current=""
    [ -f "$CURRENT_FILE" ] && current=$(cat "$CURRENT_FILE")
    echo "Configured API Keys:"
    echo "-------------------------------------------"
    while IFS='|' read -r name key url; do
        [ -z "$name" ] && continue
        local masked="${key:0:10}...${key: -4}"
        local marker="  "
        [ "$name" = "$current" ] && marker="* "
        if [ -n "$url" ]; then
            echo "${marker}${name}  ${masked}  (${url})"
        else
            echo "${marker}${name}  ${masked}"
        fi
    done < "$KEYS_FILE"
    echo "-------------------------------------------"
    echo "(* = active)"
}

_cckey_add() {
    local name="$1" key="$2" url="$3"
    if [ -z "$name" ] || [ -z "$key" ]; then
        echo "Usage: cckey add <name> <api_key> [base_url]"
        echo "Example: cckey add main sk-ant-api03-xxxxx"
        echo "         cckey add proxy sk-xxxxx https://proxy.example.com"
        return 1
    fi
    # Remove existing entry with the same name
    if grep -q "^${name}|" "$KEYS_FILE" 2>/dev/null; then
        sed -i "/^${name}|/d" "$KEYS_FILE"
        echo "Updated key: $name"
    else
        echo "Added key: $name"
    fi
    echo "${name}|${key}|${url}" >> "$KEYS_FILE"
}

_cckey_rm() {
    local name="$1"
    if [ -z "$name" ]; then
        echo "Usage: cckey rm <name>"
        return 1
    fi
    if grep -q "^${name}|" "$KEYS_FILE" 2>/dev/null; then
        sed -i "/^${name}|/d" "$KEYS_FILE"
        echo "Removed key: $name"
        # Clear current if it was the active one
        [ -f "$CURRENT_FILE" ] && [ "$(cat "$CURRENT_FILE")" = "$name" ] && rm -f "$CURRENT_FILE"
    else
        echo "Key not found: $name"
        return 1
    fi
}

_cckey_use() {
    local name="$1"
    if [ -z "$name" ]; then
        echo "Usage: cckey use <name>"
        return 1
    fi
    local line
    line=$(grep "^${name}|" "$KEYS_FILE" 2>/dev/null)
    if [ -z "$line" ]; then
        echo "Key not found: $name"
        echo "Available keys:"
        _cckey_list
        return 1
    fi
    local key url
    key=$(echo "$line" | cut -d'|' -f2)
    url=$(echo "$line" | cut -d'|' -f3)
    export ANTHROPIC_API_KEY="$key"
    if [ -n "$url" ]; then
        export ANTHROPIC_BASE_URL="$url"
        echo "Switched to: $name (base_url: $url)"
    else
        unset ANTHROPIC_BASE_URL
        echo "Switched to: $name"
    fi
    echo "$name" > "$CURRENT_FILE"
    # Sync to Claude Code settings.json
    _cckey_sync_settings "$key" "$url"
    echo "  -> Claude Code settings.json updated"
}

_cckey_next() {
    if [ ! -s "$KEYS_FILE" ]; then
        echo "No keys configured."
        return 1
    fi
    local current="" found_current=0 first_name=""
    [ -f "$CURRENT_FILE" ] && current=$(cat "$CURRENT_FILE")
    while IFS='|' read -r name key url; do
        [ -z "$name" ] && continue
        [ -z "$first_name" ] && first_name="$name"
        if [ "$found_current" -eq 1 ]; then
            _cckey_use "$name"
            return 0
        fi
        [ "$name" = "$current" ] && found_current=1
    done < "$KEYS_FILE"
    # Wrap around to the first key
    _cckey_use "$first_name"
}

_cckey_current() {
    if [ -f "$CURRENT_FILE" ] && [ -n "$(cat "$CURRENT_FILE")" ]; then
        local name
        name=$(cat "$CURRENT_FILE")
        local masked="${ANTHROPIC_API_KEY:0:10}...${ANTHROPIC_API_KEY: -4}"
        echo "Current: $name ($masked)"
        [ -n "$ANTHROPIC_BASE_URL" ] && echo "Base URL: $ANTHROPIC_BASE_URL"
    else
        echo "No key is active. Use: cckey use <name>"
    fi
}

_cckey_import() {
    if [ ! -f "$CLAUDE_SETTINGS" ]; then
        echo "Claude Code settings not found at $CLAUDE_SETTINGS"
        return 1
    fi
    if ! command -v jq &>/dev/null; then
        echo "Error: jq is required for import"
        return 1
    fi
    local key url name
    key=$(jq -r '.env.ANTHROPIC_AUTH_TOKEN // empty' "$CLAUDE_SETTINGS" 2>/dev/null)
    if [ -z "$key" ]; then
        echo "No ANTHROPIC_AUTH_TOKEN found in settings.json"
        return 1
    fi
    url=$(jq -r '.env.ANTHROPIC_BASE_URL // empty' "$CLAUDE_SETTINGS" 2>/dev/null)
    name="${1:-default}"
    # Check if this key already exists
    if grep -q "|${key}|" "$KEYS_FILE" 2>/dev/null; then
        echo "This key is already in cckey."
        return 0
    fi
    _cckey_add "$name" "$key" "$url"
    echo "$name" > "$CURRENT_FILE"
    export ANTHROPIC_API_KEY="$key"
    [ -n "$url" ] && export ANTHROPIC_BASE_URL="$url"
    echo "Imported as active key: $name"
}

cckey() {
    local cmd="${1:-help}"
    shift 2>/dev/null
    case "$cmd" in
        list|ls)      _cckey_list ;;
        add)          _cckey_add "$@" ;;
        rm|remove)    _cckey_rm "$@" ;;
        use|switch)   _cckey_use "$@" ;;
        next|n)       _cckey_next ;;
        current)      _cckey_current ;;
        import)       _cckey_import "$@" ;;
        version|--version|-v) echo "cckey v${CCKEY_VERSION}" ;;
        help|--help|-h|*)
            echo "cckey v${CCKEY_VERSION} - Claude Code API Key Manager"
            echo ""
            echo "Commands:"
            echo "  cckey add <name> <key> [base_url]  Add or update a key"
            echo "  cckey use <name>                   Switch to a key"
            echo "  cckey next                         Switch to next key (rotate)"
            echo "  cckey list                         List all keys"
            echo "  cckey current                      Show active key"
            echo "  cckey rm <name>                    Remove a key"
            echo "  cckey import [name]                Import key from Claude Code settings"
            echo "  cckey version                      Show version"
            echo "  cckey help                         Show this help"
            ;;
    esac
}
