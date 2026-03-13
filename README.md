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

### Auto-failover

Automatically detect invalid or exhausted keys and switch to the next available one:

```bash
# Enable auto-failover
cckey failover on

# Disable auto-failover
cckey failover off

# Check status
cckey failover
```

When enabled, cckey checks the current key validity at shell startup. If the key returns 401, 403, or 429, it automatically rotates through available keys until a working one is found.

### Self-update

```bash
cckey update
```

### Other commands

```bash
# List all keys (* marks the active one)
cckey list

# Show active key
cckey current

# Remove a key
cckey rm old-key

# Show version
cckey version
```

Tab completion is supported for both Bash and Zsh. Key names auto-complete for `use`, `rm`, and `test` commands.

## How It Works

- Keys are stored in `~/.cckey/keys.conf` (permission `600`)
- The `~/.cckey/` directory is protected with permission `700`
- Switching sets `ANTHROPIC_API_KEY` (and `ANTHROPIC_BASE_URL` if configured) in the current shell
- Switching also syncs `ANTHROPIC_AUTH_TOKEN` and `ANTHROPIC_BASE_URL` to `~/.claude/settings.json` for Claude Code
- `cckey next` rotates through keys in order, wrapping around to the first
- Requires `jq` for reading/writing `settings.json`

## Supported Shells

- Bash
- Zsh

## Requirements

- Linux / macOS
- Bash 4.0+ or Zsh 5.0+
- [jq](https://jqlang.github.io/jq/) (for Claude Code settings sync)
- `curl` or `wget` (for `test`, `update`, and `failover` features)

## Roadmap

- [x] Auto-failover: detect quota exhaustion and automatically switch to the next key
- [ ] Multi-app support: manage keys for Codex CLI, Gemini CLI, OpenCode, etc.
- [ ] Usage tracking: record and display per-key usage statistics
- [ ] Encrypted storage: encrypt keys at rest with a master password

## License

MIT
