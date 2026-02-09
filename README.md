# cckey

Lightweight CLI tool for managing multiple Anthropic API keys for [Claude Code](https://docs.anthropic.com/en/docs/claude-code).

Designed for headless Linux servers where GUI tools like [cc-switch](https://github.com/farion1231/cc-switch) cannot run.

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

```bash
# Add keys
cckey add main sk-ant-api03-xxxxx
cckey add backup sk-ant-api03-yyyyy
cckey add proxy sk-ant-zzzzz https://proxy.example.com

# Switch to a key
cckey use main

# Rotate to next key (when quota runs out)
cckey next

# List all keys
cckey list

# Show active key
cckey current

# Remove a key
cckey rm old-key
```

## How It Works

- Keys are stored in `~/.cckey/keys.conf`
- Switching sets `ANTHROPIC_API_KEY` (and `ANTHROPIC_BASE_URL` if configured) in the current shell
- `cckey next` rotates through keys in order, wrapping around to the first

## Supported Shells

- Bash
- Zsh

## Requirements

- Linux / macOS
- Bash 4.0+ or Zsh 5.0+

## License

MIT
