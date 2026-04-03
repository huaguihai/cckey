package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/huaguihai/cckey/cckey-proxy/internal/config"
	"github.com/huaguihai/cckey/cckey-proxy/internal/proxy"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "serve":
		if err := runServe(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "health":
		if err := runHealth(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "version":
		fmt.Printf("cckey-proxy %s\n", version)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage: cckey-proxy <command>\n\nCommands:\n  serve    Run the proxy server\n  health   Check health endpoint\n  version  Print version\n\nFlags:\n  --config <path>  Path to JSON config file\n")
}

func runServe() error {
	configPath := ""
	for i, arg := range os.Args {
		if arg == "--config" && i+1 < len(os.Args) {
			configPath = os.Args[i+1]
		}
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	return proxy.ListenAndServe(cfg)
}

func runHealth() error {
	configPath := ""
	for i, arg := range os.Args {
		if arg == "--config" && i+1 < len(os.Args) {
			configPath = os.Args[i+1]
		}
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	url := fmt.Sprintf("http://%s/health", cfg.Listen)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned status %d", resp.StatusCode)
	}
	fmt.Println("ok")
	return nil
}
