# cckey

Lightweight CLI tool for managing multiple Anthropic API keys for [Claude Code](https://docs.anthropic.com/en/docs/claude-code).

Designed for headless servers and terminals where GUI tools like [cc-switch](https://github.com/farion1231/cc-switch) cannot run.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/huaguihai/cckey/main/install.sh | bash
source ~/.bashrc
```

Or manually:

```bash
mkdir -p ~/.cckey
curl -fsSL https://raw.githubusercontent.com/huaguihai/cckey/main/cckey.sh -o ~/.cckey/cckey.sh
echo 'source "$HOME/.cckey/cckey.sh"' >> ~/.bashrc
source ~/.bashrc
```

## Usage

### Import existing key

If you already have a key configured in Claude Code (`~/.claude/settings.json`), import it directly:

```bash
cckey import mykey
```

This reads `ANTHROPIC_AUTH_TOKEN` and `ANTHROPIC_BASE_URL` from your current Claude Code settings and adds them to cckey.

### Add keys

```bash
# Anthropic official API
cckey add main sk-ant-api03-xxxxx

# Third-party proxy with custom base URL
cckey add proxy sk-ant-zzzzz https://proxy.example.com
```

### Switch keys

```bash
# Switch to a specific key
cckey use main

# Rotate to next key (when quota runs out)
cckey next
```

Switching automatically updates `~/.claude/settings.json`, so the next Claude Code session will use the new key immediately.

### Test a key

```bash
# Test the active key
cckey test

# Test a specific key
cckey test main
```

### Smart rotation

Three rotation strategies to choose from:

```bash
# 1. Failover (default) — auto-switch on API errors (401/403/429)
cckey rotate failover

# 2. Timer — rotate every N hours
cckey rotate timer 4

# 3. Counter — rotate every N shell sessions
cckey rotate counter 10

# Disable rotation
cckey rotate off

# Check current strategy
cckey rotate status
```

| Strategy | Trigger | Best for |
|----------|---------|----------|
| `failover` | API returns 401/403/429 | Error recovery, quota exhaustion |
| `timer` | Time interval elapsed | Even usage distribution over time |
| `counter` | Shell session count reached | Even usage distribution by sessions |

The `failover` strategy checks the key at shell startup and rotates through available keys until a working one is found. The `timer` and `counter` strategies silently rotate to the next key when the threshold is reached.

### Other commands

```bash
# List all keys (* marks the active one)
cckey list

# Show active key
cckey current

# Rename a key
cckey rename old-name new-name

# Remove a key
cckey rm old-key

# Self-update to latest version
cckey update

# Show version
cckey version
```

Tab completion is supported for both Bash and Zsh. Commands, key names, and rotation strategies auto-complete.

## How It Works

- Keys are stored in `~/.cckey/keys.conf` (permission `600`)
- The `~/.cckey/` directory is protected with permission `700`
- Switching sets `ANTHROPIC_API_KEY` (and `ANTHROPIC_BASE_URL` if configured) in the current shell
- Switching also syncs `ANTHROPIC_AUTH_TOKEN` and `ANTHROPIC_BASE_URL` to `~/.claude/settings.json` for Claude Code
- `cckey next` rotates through keys in order, wrapping around to the first
- Rotation config is stored in `~/.cckey/rotate.conf`
- Requires `jq` for reading/writing `settings.json`

## Supported Shells

- Bash
- Zsh

## Platform Support

| Platform | Status | Notes |
|----------|--------|-------|
| Linux | Fully supported | |
| macOS | Fully supported | |
| Windows (WSL) | Fully supported | [Install WSL](https://learn.microsoft.com/en-us/windows/wsl/install), then use same as Linux |
| Windows (PowerShell) | Not supported | Use WSL instead |

## Requirements

- Linux / macOS / WSL
- Bash 4.0+ or Zsh 5.0+
- [jq](https://jqlang.github.io/jq/) (for Claude Code settings sync)
- `curl` or `wget` (for `test`, `update`, and `failover` features)

## Roadmap

- [x] Auto-failover: detect quota exhaustion and automatically switch to the next key
- [x] Smart rotation: timer-based and session-count-based key rotation
- [ ] Multi-app support: manage keys for Codex CLI, Gemini CLI, OpenCode, etc.
- [ ] Usage tracking: record and display per-key usage statistics
- [ ] Encrypted storage: encrypt keys at rest with a master password
- [ ] Native Windows PowerShell support

## License

MIT
