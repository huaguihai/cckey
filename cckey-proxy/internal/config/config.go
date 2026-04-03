package config

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

const defaultListen = "127.0.0.1:5001"

// Profile represents a single upstream endpoint configuration.
type Profile struct {
	Name         string            `json:"name"`
	Mode         string            `json:"mode"` // "passthrough" or "translate"
	BaseURL      string            `json:"base_url"`
	APIKey       string            `json:"api_key"`
	ModelMap     map[string]string `json:"model_map,omitempty"`
	DefaultModel string            `json:"default_model,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	Preset       string            `json:"preset,omitempty"` // "openai" enables stream_options
}

// Config is the top-level configuration.
type Config struct {
	Listen        string   `json:"listen"`
	ProxyToken    string   `json:"proxy_token,omitempty"` // if set, incoming requests must carry this token
	ActiveProfile *Profile `json:"active_profile,omitempty"`
}

// ResolveModel maps a Claude model name to the upstream model.
func (p *Profile) ResolveModel(claudeModel string) string {
	if p.ModelMap != nil {
		if mapped := strings.TrimSpace(p.ModelMap[claudeModel]); mapped != "" {
			return mapped
		}
	}
	if strings.TrimSpace(p.DefaultModel) != "" {
		return p.DefaultModel
	}
	return claudeModel
}

// Load reads config from a JSON file path, or from stdin if path is empty.
// It also supports cckey's pipe-delimited keys.conf format.
func Load(path string) (*Config, error) {
	if path == "" {
		// Try stdin if it's not a terminal
		stat, _ := os.Stdin.Stat()
		if stat != nil && (stat.Mode()&os.ModeCharDevice) == 0 {
			return loadFromStdin()
		}
		return nil, errors.New("no config provided: use --config <path> or pipe JSON to stdin")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	// Try JSON first
	trimmed := strings.TrimSpace(string(data))
	if strings.HasPrefix(trimmed, "{") {
		return loadJSON(data)
	}

	// Try cckey pipe-delimited format
	return loadKeysConf(data)
}

func loadFromStdin() (*Config, error) {
	var cfg Config
	if err := json.NewDecoder(os.Stdin).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode stdin JSON: %w", err)
	}
	if cfg.Listen == "" {
		cfg.Listen = defaultListen
	}
	return &cfg, nil
}

func loadJSON(data []byte) (*Config, error) {
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("decode config JSON: %w", err)
	}
	if cfg.Listen == "" {
		cfg.Listen = defaultListen
	}
	return &cfg, nil
}

// loadKeysConf parses cckey's pipe-delimited format:
//
//	name|base_url|api_key|model_map|default_model
//
// Lines starting with # are comments. The first non-comment line with
// a valid entry becomes the active profile in translate mode.
func loadKeysConf(data []byte) (*Config, error) {
	cfg := &Config{Listen: defaultListen}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Check for listen= directive
		if strings.HasPrefix(line, "listen=") {
			cfg.Listen = strings.TrimPrefix(line, "listen=")
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) < 3 {
			continue
		}

		profile := &Profile{
			Name:    strings.TrimSpace(parts[0]),
			Mode:    "translate",
			BaseURL: strings.TrimSpace(parts[1]),
			APIKey:  strings.TrimSpace(parts[2]),
		}

		if len(parts) > 3 && strings.TrimSpace(parts[3]) != "" {
			profile.ModelMap = parseModelMap(strings.TrimSpace(parts[3]))
		}
		if len(parts) > 4 && strings.TrimSpace(parts[4]) != "" {
			profile.DefaultModel = strings.TrimSpace(parts[4])
		}

		// Use first valid entry as active profile
		if cfg.ActiveProfile == nil {
			cfg.ActiveProfile = profile
		}
	}

	if cfg.ActiveProfile == nil {
		return nil, errors.New("no valid profile found in keys.conf")
	}
	return cfg, nil
}

// parseModelMap parses "claude-sonnet=gpt-4o,claude-opus=o1" format.
func parseModelMap(raw string) map[string]string {
	result := make(map[string]string)
	for _, pair := range strings.Split(raw, ",") {
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) == 2 {
			key := strings.TrimSpace(kv[0])
			val := strings.TrimSpace(kv[1])
			if key != "" && val != "" {
				result[key] = val
			}
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
