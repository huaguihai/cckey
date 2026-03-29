# cckey

Lightweight CLI tool for managing multiple AI API keys for [Claude Code](https://docs.anthropic.com/en/docs/claude-code), [Codex CLI](https://github.com/openai/codex), and [Gemini CLI](https://github.com/google-gemini/gemini-cli).

Designed for headless servers and terminals where GUI tools cannot run.

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
# Claude Code (Anthropic official API)
cckey add main sk-ant-api03-xxxxx

# Claude Code with third-party proxy
cckey add proxy sk-ant-zzzzz https://proxy.example.com

# Codex CLI (OpenAI)
cckey add gpt4 sk-openai-xxxxx --type codex

# Codex CLI with custom base URL
cckey add gpt4-proxy sk-openai-xxxxx https://api.openai.com --type codex

# Gemini CLI
cckey add gem1 AIzaSy-xxxxx --type gemini
```

When adding a Claude key with a base URL, cckey automatically queries `/v1/models` to detect supported models. The best model (opus > sonnet > haiku) is displayed when listing keys.

### Model management

```bash
# Show supported models for all keys
cckey models

# Show supported models for a specific key
cckey models main

# Scan all keys to fetch/refresh supported models
cckey scan
```

### Switch keys

```bash
# Switch to a specific key
cckey use main

# Rotate to next key of the same type (when quota runs out)
cckey next
```

Switching a Claude key automatically updates `~/.claude/settings.json`, so the next Claude Code session uses the new key immediately.

Each key type sets the appropriate environment variables:

| Type | Environment Variables |
|------|-----------------------|
| `claude` | `ANTHROPIC_API_KEY`, `ANTHROPIC_BASE_URL` (+ `settings.json` sync) |
| `codex` | `OPENAI_API_KEY`, `OPENAI_BASE_URL` |
| `gemini` | `GEMINI_API_KEY` |

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

> Note: `cckey next` and smart rotation only cycle through keys of the same type as the currently active key.

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

- Keys are stored in `~/.cckey/keys.conf` (permission `600`) as `name|key|url|type|models` records
- The `~/.cckey/` directory is protected with permission `700`
- Switching sets the appropriate environment variables for the key type in the current shell
- Switching a `claude` key also syncs `ANTHROPIC_AUTH_TOKEN` and `ANTHROPIC_BASE_URL` to `~/.claude/settings.json`
- Switching also syncs the active key and best supported model to `~/.claude-to-im/config.env` and restarts the bridge (if running)
- `cckey next` rotates through keys of the same type in order, wrapping around to the first
- Rotation config is stored in `~/.cckey/rotate.conf`
- Requires `jq` for reading/writing Claude Code `settings.json`

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
- [x] Multi-app support: Claude Code, Codex CLI, Gemini CLI
- [ ] Usage tracking: record and display per-key usage statistics
- [ ] Encrypted storage: encrypt keys at rest with a master password
- [ ] Native Windows PowerShell support

## License

MIT
