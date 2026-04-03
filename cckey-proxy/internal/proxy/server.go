package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"path"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/huaguihai/cckey/cckey-proxy/internal/config"
)

// Cached http.Client — reused across all requests.
var httpClient = &http.Client{Timeout: 5 * time.Minute}

// Cached reverse proxies per base URL.
var (
	reverseProxies   = make(map[string]*httputil.ReverseProxy)
	reverseProxiesMu sync.RWMutex
)

// ListenAndServe starts the proxy server with graceful shutdown.
func ListenAndServe(cfg *config.Config) error {
	if cfg.ActiveProfile == nil {
		return errors.New("no active profile configured")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		handleRequest(w, r, cfg)
	})

	server := &http.Server{
		Addr:              cfg.Listen,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Graceful shutdown on SIGTERM/SIGINT
	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-done
		log.Println("shutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			log.Printf("shutdown error: %v", err)
		}
	}()

	log.Printf("cckey-proxy listening on %s (mode=%s, upstream=%s)",
		cfg.Listen, cfg.ActiveProfile.Mode, cfg.ActiveProfile.BaseURL)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func handleRequest(w http.ResponseWriter, r *http.Request, cfg *config.Config) {
	if r.URL.Path == "/health" {
		handleHealth(w, cfg)
		return
	}

	profile := cfg.ActiveProfile

	switch {
	case profile.Mode == "passthrough":
		servePassthrough(w, r, profile)
	case r.URL.Path == "/v1/messages/count_tokens":
		handleTranslatedCountTokens(w, r, profile)
	case r.URL.Path == "/v1/messages":
		handleTranslatedMessages(w, r, cfg, profile)
	case r.URL.Path == "/v1/models":
		handleTranslatedModels(w, profile)
	default:
		writeAnthropicError(w, http.StatusNotFound,
			fmt.Sprintf("path %s is not supported for translate profiles", r.URL.Path))
	}
}

func handleHealth(w http.ResponseWriter, cfg *config.Config) {
	payload := map[string]any{
		"ok":     true,
		"listen": cfg.Listen,
	}
	if p := cfg.ActiveProfile; p != nil {
		payload["active_profile"] = p.Name
		payload["mode"] = p.Mode
		payload["upstream"] = p.BaseURL
	}
	writeJSON(w, http.StatusOK, payload)
}

func servePassthrough(w http.ResponseWriter, r *http.Request, profile *config.Profile) {
	target, err := url.Parse(profile.BaseURL)
	if err != nil {
		writeAnthropicError(w, http.StatusBadGateway, fmt.Sprintf("invalid upstream URL: %v", err))
		return
	}

	rp := getCachedReverseProxy(profile.BaseURL, target)

	// Clone the request with context propagation
	outReq := r.Clone(r.Context())
	outReq.Host = target.Host
	outReq.URL.Path = joinPath(target.Path, r.URL.Path)
	outReq.URL.RawPath = outReq.URL.Path
	if profile.APIKey != "" {
		outReq.Header.Set("x-api-key", profile.APIKey)
		outReq.Header.Set("Authorization", "Bearer "+profile.APIKey)
	}
	for key, value := range profile.Headers {
		outReq.Header.Set(key, value)
	}

	rp.ServeHTTP(w, outReq)
}

func getCachedReverseProxy(baseURL string, target *url.URL) *httputil.ReverseProxy {
	reverseProxiesMu.RLock()
	rp, ok := reverseProxies[baseURL]
	reverseProxiesMu.RUnlock()
	if ok {
		return rp
	}

	reverseProxiesMu.Lock()
	defer reverseProxiesMu.Unlock()

	// Double-check after acquiring write lock
	if rp, ok := reverseProxies[baseURL]; ok {
		return rp
	}

	rp = httputil.NewSingleHostReverseProxy(target)
	rp.Transport = httpClient.Transport
	rp.ErrorHandler = func(writer http.ResponseWriter, _ *http.Request, proxyErr error) {
		writeAnthropicError(writer, http.StatusBadGateway, proxyErr.Error())
	}
	reverseProxies[baseURL] = rp
	return rp
}

func joinPath(basePath string, requestPath string) string {
	if basePath == "" || basePath == "/" {
		return requestPath
	}
	return path.Join(basePath, requestPath)
}

func joinURL(base string, suffix string) string {
	return strings.TrimRight(base, "/") + suffix
}

func writeAnthropicError(w http.ResponseWriter, status int, message string) {
	payload := map[string]any{
		"type": "error",
		"error": map[string]any{
			"type":    "api_error",
			"message": message,
		},
	}
	writeJSON(w, status, payload)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

func decodeBodyMap(body io.Reader) (map[string]any, error) {
	var payload map[string]any
	if err := json.NewDecoder(body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func readErrorMessage(body []byte) string {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err == nil {
		if text := digString(payload, "error", "message"); text != "" {
			return text
		}
		if text, ok := payload["error"].(string); ok && text != "" {
			return text
		}
		if text := digString(payload, "message"); text != "" {
			return text
		}
	}
	message := strings.TrimSpace(string(body))
	if message == "" {
		return "upstream request failed"
	}
	return message
}

func digString(payload map[string]any, path ...string) string {
	current := any(payload)
	for _, key := range path {
		obj, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current, ok = obj[key]
		if !ok {
			return ""
		}
	}
	if text, ok := current.(string); ok {
		return text
	}
	return ""
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func requirePOST(r *http.Request) error {
	if r.Method != http.MethodPost {
		return errors.New("only POST is supported on this endpoint")
	}
	return nil
}
