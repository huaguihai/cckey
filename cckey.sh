#!/bin/bash
# cckey - Claude Code API Key Manager
# Version: 0.4.0
# https://github.com/huaguihai/cckey
#
# A lightweight CLI tool for managing multiple Anthropic API keys.
# Designed for headless Linux servers where GUI tools like cc-switch cannot run.

CCKEY_VERSION="0.4.0"
KEYS_DIR="$HOME/.cckey"
KEYS_FILE="$KEYS_DIR/keys.conf"
CURRENT_FILE="$KEYS_DIR/current"
CLAUDE_SETTINGS="$HOME/.claude/settings.json"

mkdir -p "$KEYS_DIR" && chmod 700 "$KEYS_DIR"
touch "$KEYS_FILE" && chmod 600 "$KEYS_FILE"

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
        local masked
        if [ ${#key} -gt 16 ]; then
            masked="${key:0:10}...${key: -4}"
        else
            masked="${key:0:4}****"
        fi
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
        sed -i.bak "/^${name}|/d" "$KEYS_FILE" && rm -f "${KEYS_FILE}.bak"
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
        sed -i.bak "/^${name}|/d" "$KEYS_FILE" && rm -f "${KEYS_FILE}.bak"
        echo "Removed key: $name"
        # Clear current if it was the active one
        [ -f "$CURRENT_FILE" ] && [ "$(cat "$CURRENT_FILE")" = "$name" ] && rm -f "$CURRENT_FILE"
    else
        echo "Key not found: $name"
        return 1
    fi
}

_cckey_rename() {
    local old="$1" new="$2"
    if [ -z "$old" ] || [ -z "$new" ]; then
        echo "Usage: cckey rename <old_name> <new_name>"
        return 1
    fi
    if ! grep -q "^${old}|" "$KEYS_FILE" 2>/dev/null; then
        echo "Key not found: $old"
        return 1
    fi
    if grep -q "^${new}|" "$KEYS_FILE" 2>/dev/null; then
        echo "Key name already exists: $new"
        return 1
    fi
    sed -i.bak "s/^${old}|/${new}|/" "$KEYS_FILE" && rm -f "${KEYS_FILE}.bak"
    # Update current if it was the active one
    if [ -f "$CURRENT_FILE" ] && [ "$(cat "$CURRENT_FILE")" = "$old" ]; then
        echo "$new" > "$CURRENT_FILE"
    fi
    echo "Renamed: $old -> $new"
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
    echo "$name" > "$CURRENT_FILE" && chmod 600 "$CURRENT_FILE"
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
        local masked
        if [ ${#ANTHROPIC_API_KEY} -gt 16 ]; then
            masked="${ANTHROPIC_API_KEY:0:10}...${ANTHROPIC_API_KEY: -4}"
        else
            masked="${ANTHROPIC_API_KEY:0:4}****"
        fi
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

_cckey_test() {
    local name="$1" key="" url=""
    if [ -n "$name" ]; then
        local line
        line=$(grep "^${name}|" "$KEYS_FILE" 2>/dev/null)
        if [ -z "$line" ]; then
            echo "Key not found: $name"
            return 1
        fi
        key=$(echo "$line" | cut -d'|' -f2)
        url=$(echo "$line" | cut -d'|' -f3)
    else
        if [ -z "$ANTHROPIC_API_KEY" ]; then
            echo "No active key. Use: cckey test <name>"
            return 1
        fi
        key="$ANTHROPIC_API_KEY"
        url="$ANTHROPIC_BASE_URL"
        [ -f "$CURRENT_FILE" ] && name=$(cat "$CURRENT_FILE") || name="(env)"
    fi
    local api_url="${url:-https://api.anthropic.com}"
    echo "Testing key: $name ..."
    local response http_code
    if command -v curl &>/dev/null; then
        response=$(curl -s -w "\n%{http_code}" -X POST "${api_url}/v1/messages" \
            -H "x-api-key: ${key}" \
            -H "anthropic-version: 2023-06-01" \
            -H "content-type: application/json" \
            -d '{"model":"claude-sonnet-4-20250514","max_tokens":1,"messages":[{"role":"user","content":"hi"}]}' 2>/dev/null)
    elif command -v wget &>/dev/null; then
        response=$(wget -qO- --header="x-api-key: ${key}" \
            --header="anthropic-version: 2023-06-01" \
            --header="content-type: application/json" \
            --post-data='{"model":"claude-sonnet-4-20250514","max_tokens":1,"messages":[{"role":"user","content":"hi"}]}' \
            "${api_url}/v1/messages" 2>/dev/null)
        # wget doesn't easily give status codes, check response content
        if echo "$response" | grep -q '"error"'; then
            local err_msg
            err_msg=$(echo "$response" | grep -o '"message":"[^"]*"' | head -1 | cut -d'"' -f4)
            echo "FAIL: $err_msg"
            return 1
        fi
        echo "OK: Key is valid."
        return 0
    else
        echo "Error: curl or wget is required."
        return 1
    fi
    http_code=$(echo "$response" | tail -1)
    local body
    body=$(echo "$response" | sed '$d')
    case "$http_code" in
        200) echo "OK: Key is valid." ;;
        401) echo "FAIL: Invalid API key (unauthorized)." ; return 1 ;;
        403) echo "FAIL: Access denied (forbidden)." ; return 1 ;;
        429) echo "WARN: Rate limited or quota exceeded." ; return 1 ;;
        *)
            local err_msg
            err_msg=$(echo "$body" | grep -o '"message":"[^"]*"' | head -1 | cut -d'"' -f4)
            echo "FAIL (HTTP $http_code): ${err_msg:-unknown error}"
            return 1
            ;;
    esac
}

_cckey_update() {
    local url="https://raw.githubusercontent.com/huaguihai/cckey/main/cckey.sh"
    local tmp_file="${KEYS_DIR}/cckey.sh.tmp"
    local target="${KEYS_DIR}/cckey.sh"
    echo "Checking for updates..."
    if command -v curl &>/dev/null; then
        curl -fsSL "$url" -o "$tmp_file" 2>/dev/null
    elif command -v wget &>/dev/null; then
        wget -qO "$tmp_file" "$url" 2>/dev/null
    else
        echo "Error: curl or wget is required."
        return 1
    fi
    if [ ! -s "$tmp_file" ]; then
        echo "Error: failed to download update."
        rm -f "$tmp_file"
        return 1
    fi
    local new_version
    new_version=$(grep '^CCKEY_VERSION=' "$tmp_file" | head -1 | cut -d'"' -f2)
    if [ "$new_version" = "$CCKEY_VERSION" ]; then
        echo "Already up to date (v${CCKEY_VERSION})."
        rm -f "$tmp_file"
        return 0
    fi
    mv "$tmp_file" "$target"
    echo "Updated: v${CCKEY_VERSION} -> v${new_version}"
    echo "Run 'source ~/.bashrc' or 'source ~/.zshrc' to apply."
}

_cckey_rotate_conf="${KEYS_DIR}/rotate.conf"

_cckey_rotate_read() {
    local mode="off" timer_hours="4" counter_max="10"
    if [ -f "$_cckey_rotate_conf" ]; then
        eval "$(grep -E '^(mode|timer_hours|counter_max)=' "$_cckey_rotate_conf" 2>/dev/null)"
    fi
    echo "$mode|$timer_hours|$counter_max"
}

_cckey_rotate_write() {
    local mode="$1" timer_hours="$2" counter_max="$3"
    cat > "$_cckey_rotate_conf" <<CONF
mode=$mode
timer_hours=$timer_hours
counter_max=$counter_max
CONF
    chmod 600 "$_cckey_rotate_conf"
}

_cckey_rotate() {
    local action="${1:-status}"
    local conf
    conf=$(_cckey_rotate_read)
    local cur_mode cur_timer cur_counter
    cur_mode=$(echo "$conf" | cut -d'|' -f1)
    cur_timer=$(echo "$conf" | cut -d'|' -f2)
    cur_counter=$(echo "$conf" | cut -d'|' -f3)
    case "$action" in
        failover)
            _cckey_rotate_write "failover" "$cur_timer" "$cur_counter"
            echo "Rotate mode: failover (auto-switch on API errors)"
            ;;
        timer)
            local hours="${2:-$cur_timer}"
            if ! echo "$hours" | grep -qE '^[0-9]+\.?[0-9]*$'; then
                echo "Usage: cckey rotate timer <hours>"
                return 1
            fi
            _cckey_rotate_write "timer" "$hours" "$cur_counter"
            # Record current time as last switch
            date +%s > "${KEYS_DIR}/last_switch"
            echo "Rotate mode: timer (every ${hours}h)"
            ;;
        counter)
            local max="${2:-$cur_counter}"
            if ! echo "$max" | grep -qE '^[0-9]+$'; then
                echo "Usage: cckey rotate counter <max_sessions>"
                return 1
            fi
            _cckey_rotate_write "counter" "$cur_timer" "$max"
            echo "0" > "${KEYS_DIR}/session_count"
            echo "Rotate mode: counter (every ${max} sessions)"
            ;;
        off)
            _cckey_rotate_write "off" "$cur_timer" "$cur_counter"
            echo "Rotation disabled."
            ;;
        status)
            echo "Rotate mode: $cur_mode"
            case "$cur_mode" in
                failover) echo "  Strategy: auto-switch on 401/403/429 errors" ;;
                timer)
                    echo "  Interval: every ${cur_timer}h"
                    if [ -f "${KEYS_DIR}/last_switch" ]; then
                        local last_switch now elapsed remaining
                        last_switch=$(cat "${KEYS_DIR}/last_switch")
                        now=$(date +%s)
                        elapsed=$(( (now - last_switch) / 3600 ))
                        remaining=$(echo "$cur_timer $elapsed" | awk '{r=$1-$2; if(r<0) r=0; printf "%.1f", r}')
                        echo "  Last switch: ${elapsed}h ago, next in ~${remaining}h"
                    fi
                    ;;
                counter)
                    local current_count=0
                    [ -f "${KEYS_DIR}/session_count" ] && current_count=$(cat "${KEYS_DIR}/session_count")
                    echo "  Threshold: every ${cur_counter} sessions"
                    echo "  Current count: ${current_count}/${cur_counter}"
                    ;;
                off) echo "  No automatic rotation." ;;
            esac
            ;;
        *)
            echo "Usage: cckey rotate <failover|timer|counter|off|status>"
            echo ""
            echo "Strategies:"
            echo "  cckey rotate failover              Auto-switch on API errors (default)"
            echo "  cckey rotate timer <hours>          Rotate every N hours"
            echo "  cckey rotate counter <sessions>     Rotate every N sessions"
            echo "  cckey rotate off                    Disable rotation"
            echo "  cckey rotate status                 Show current strategy"
            return 1
            ;;
    esac
}

_cckey_check_rotate() {
    [ ! -f "$_cckey_rotate_conf" ] && return
    local conf
    conf=$(_cckey_rotate_read)
    local mode timer_hours counter_max
    mode=$(echo "$conf" | cut -d'|' -f1)
    timer_hours=$(echo "$conf" | cut -d'|' -f2)
    counter_max=$(echo "$conf" | cut -d'|' -f3)
    [ "$mode" = "off" ] && return
    [ ! -s "$KEYS_FILE" ] && return
    local key_count
    key_count=$(grep -c '.' "$KEYS_FILE" 2>/dev/null)
    [ "$key_count" -le 1 ] && return

    case "$mode" in
        failover)
            [ -z "$ANTHROPIC_API_KEY" ] && return
            command -v curl &>/dev/null || return
            local api_url="${ANTHROPIC_BASE_URL:-https://api.anthropic.com}"
            local http_code
            http_code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 10 -X POST "${api_url}/v1/messages" \
                -H "x-api-key: ${ANTHROPIC_API_KEY}" \
                -H "anthropic-version: 2023-06-01" \
                -H "content-type: application/json" \
                -d '{"model":"claude-sonnet-4-20250514","max_tokens":1,"messages":[{"role":"user","content":"hi"}]}' 2>/dev/null)
            if [ "$http_code" = "401" ] || [ "$http_code" = "403" ] || [ "$http_code" = "429" ]; then
                local current_name=""
                [ -f "$CURRENT_FILE" ] && current_name=$(cat "$CURRENT_FILE")
                echo "[cckey] Key '$current_name' failed (HTTP $http_code), switching to next..."
                local tried=0
                while [ "$tried" -lt "$key_count" ]; do
                    _cckey_next
                    tried=$((tried + 1))
                    api_url="${ANTHROPIC_BASE_URL:-https://api.anthropic.com}"
                    http_code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 10 -X POST "${api_url}/v1/messages" \
                        -H "x-api-key: ${ANTHROPIC_API_KEY}" \
                        -H "anthropic-version: 2023-06-01" \
                        -H "content-type: application/json" \
                        -d '{"model":"claude-sonnet-4-20250514","max_tokens":1,"messages":[{"role":"user","content":"hi"}]}' 2>/dev/null)
                    if [ "$http_code" = "200" ]; then
                        echo "[cckey] Failover successful."
                        return 0
                    fi
                done
                echo "[cckey] Warning: all keys failed."
                return 1
            fi
            ;;
        timer)
            local last_switch=0 now
            [ -f "${KEYS_DIR}/last_switch" ] && last_switch=$(cat "${KEYS_DIR}/last_switch")
            now=$(date +%s)
            local interval_sec
            interval_sec=$(echo "$timer_hours" | awk '{printf "%d", $1 * 3600}')
            if [ $((now - last_switch)) -ge "$interval_sec" ]; then
                echo "[cckey] Timer expired (${timer_hours}h), rotating key..."
                _cckey_next
                echo "$now" > "${KEYS_DIR}/last_switch"
            fi
            ;;
        counter)
            local current_count=0
            [ -f "${KEYS_DIR}/session_count" ] && current_count=$(cat "${KEYS_DIR}/session_count")
            current_count=$((current_count + 1))
            if [ "$current_count" -ge "$counter_max" ]; then
                echo "[cckey] Session limit reached (${counter_max}), rotating key..."
                _cckey_next
                echo "0" > "${KEYS_DIR}/session_count"
            else
                echo "$current_count" > "${KEYS_DIR}/session_count"
            fi
            ;;
    esac
}

# Run rotation check on source
_cckey_check_rotate

cckey() {
    local cmd="${1:-help}"
    shift 2>/dev/null
    case "$cmd" in
        list|ls)      _cckey_list ;;
        add)          _cckey_add "$@" ;;
        rm|remove)    _cckey_rm "$@" ;;
        rename)       _cckey_rename "$@" ;;
        use|switch)   _cckey_use "$@" ;;
        next|n)       _cckey_next ;;
        current)      _cckey_current ;;
        import)       _cckey_import "$@" ;;
        test)         _cckey_test "$@" ;;
        rotate)       _cckey_rotate "$@" ;;
        failover)     # backward compat
            case "$1" in
                on)  _cckey_rotate failover ;;
                off) _cckey_rotate off ;;
                *)   _cckey_rotate status ;;
            esac
            ;;
        update)       _cckey_update ;;
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
            echo "  cckey rename <old> <new>           Rename a key"
            echo "  cckey import [name]                Import key from Claude Code settings"
            echo "  cckey test [name]                  Test if a key is valid"
            echo "  cckey rotate <strategy> [value]     Smart rotation (failover|timer|counter|off|status)"
            echo "  cckey update                       Update cckey to latest version"
            echo "  cckey version                      Show version"
            echo "  cckey help                         Show this help"
            ;;
    esac
}

# Tab completion
_cckey_key_names() {
    [ -f "$KEYS_FILE" ] && cut -d'|' -f1 "$KEYS_FILE"
}

if [ -n "$BASH_VERSION" ]; then
    _cckey_bash_complete() {
        local cur="${COMP_WORDS[COMP_CWORD]}"
        local prev="${COMP_WORDS[COMP_CWORD-1]}"
        if [ "$COMP_CWORD" -eq 1 ]; then
            COMPREPLY=($(compgen -W "add use switch rm remove rename next list ls current import test rotate update version help" -- "$cur"))
        elif [ "$COMP_CWORD" -eq 2 ]; then
            case "$prev" in
                use|switch|rm|remove|test|rename)
                    COMPREPLY=($(compgen -W "$(_cckey_key_names)" -- "$cur"))
                    ;;
                rotate)
                    COMPREPLY=($(compgen -W "failover timer counter off status" -- "$cur"))
                    ;;
            esac
        fi
    }
    complete -F _cckey_bash_complete cckey
elif [ -n "$ZSH_VERSION" ]; then
    _cckey_zsh_complete() {
        local -a subcmds=('add:Add or update a key' 'use:Switch to a key' 'switch:Switch to a key'
            'rm:Remove a key' 'remove:Remove a key' 'rename:Rename a key' 'next:Switch to next key'
            'list:List all keys' 'ls:List all keys' 'current:Show active key'
            'import:Import key from Claude Code settings' 'test:Test if a key is valid'
            'rotate:Smart key rotation strategy' 'update:Update cckey to latest version'
            'version:Show version' 'help:Show help')
        if (( CURRENT == 2 )); then
            _describe 'command' subcmds
        elif (( CURRENT == 3 )); then
            case "${words[2]}" in
                use|switch|rm|remove|test|rename)
                    local -a keys=(${(f)"$(_cckey_key_names)"})
                    _describe 'key name' keys
                    ;;
                rotate)
                    local -a strategies=('failover:Auto-switch on API errors' 'timer:Rotate every N hours'
                        'counter:Rotate every N sessions' 'off:Disable rotation' 'status:Show current strategy')
                    _describe 'strategy' strategies
                    ;;
            esac
        fi
    }
    compdef _cckey_zsh_complete cckey 2>/dev/null
fi
