// Package cfproject holds all cf-CLI-specific knowledge: project detection,
// Build Output reading, dev-server banner parsing, and debug-websocket
// extraction. The cf CLI is in beta; when its surfaces drift, this package is
// the single place to fix. Shapes verified against cf 1.0.0-beta.10 and
// @cloudflare/vite-plugin 2.0.0-beta (see the cutover spec's probe findings).
package cfproject

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// configNames are the cf project config files, in detection order.
var configNames = []string{
	"cloudflare.config.ts",
	"cloudflare.config.mts",
	"cloudflare.config.js",
	"cloudflare.config.mjs",
}

// wranglerNames mark an unmigrated Wrangler project.
var wranglerNames = []string{"wrangler.toml", "wrangler.jsonc", "wrangler.json"}

// Detect reports whether dir is a cf project and returns the config path.
func Detect(dir string) (string, bool) {
	for _, name := range configNames {
		path := filepath.Join(dir, name)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, true
		}
	}
	return "", false
}

// DetectUnmigrated reports a directory that has a Wrangler config but no cf
// config — a project that needs `cf migrate` before wrkmn can manage it.
func DetectUnmigrated(dir string) bool {
	if _, ok := Detect(dir); ok {
		return false
	}
	for _, name := range wranglerNames {
		if info, err := os.Stat(filepath.Join(dir, name)); err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

// BuildOutput is the subset of .cloudflare/output/v0/config.json wrkmn reads.
type BuildOutput struct {
	WorkerName string
	AccountID  string
}

// ReadBuildOutput loads Build Output if present. A missing file returns
// (nil, nil): enrichment is strictly opportunistic. The format is beta, so
// parsing is lenient — unknown fields ignored, several key spellings probed.
func ReadBuildOutput(dir string) (*BuildOutput, error) {
	data, err := os.ReadFile(filepath.Join(dir, ".cloudflare", "output", "v0", "config.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("build output config.json: %w", err)
	}
	str := func(keys ...string) string {
		for _, k := range keys {
			if v, ok := raw[k].(string); ok && v != "" {
				return v
			}
		}
		return ""
	}
	return &BuildOutput{
		WorkerName: str("name", "worker_name", "workerName"),
		AccountID:  str("account_id", "accountId"),
	}, nil
}

// BannerKind identifies a recognized dev-server banner line.
type BannerKind int

const (
	BannerLocal BannerKind = iota + 1 // "Local:" — the serving URL
	BannerDebug                       // "Debug:" — the /__debug URL
)

var (
	ansiRe  = regexp.MustCompile(`\x1b\[[0-9;]*m`)
	localRe = regexp.MustCompile(`^\s*➜\s+Local:\s+(https?://\S+)`)
	debugRe = regexp.MustCompile(`^\s*➜\s+Debug:\s+(https?://\S+)`)
	wsRe    = regexp.MustCompile(`[?&]ws=([^&"'\]\s]+)`)
)

// ParseBannerLine matches the Vite/cf dev startup banner's Local and Debug
// lines. ANSI color codes are stripped before matching. Anchored to the
// banner's real shape (start-of-line optional whitespace + the ➜ arrow) so an
// app log line that merely contains "Local: http://..." mid-run can't be
// mistaken for the banner and clobber the worker's discovered URLs.
func ParseBannerLine(line string) (BannerKind, string, bool) {
	plain := ansiRe.ReplaceAllString(line, "")
	if m := debugRe.FindStringSubmatch(plain); m != nil {
		return BannerDebug, m[1], true
	}
	if m := localRe.FindStringSubmatch(plain); m != nil {
		return BannerLocal, m[1], true
	}
	return 0, "", false
}

// ExtractDebugWS fetches the dev server's /__debug page and extracts the CDP
// websocket for workerName. The page embeds one hosted-DevTools URL per
// worker, each carrying a URL-encoded "ws=host:port/worker-name" parameter.
// With a single entry, the name need not match (single-worker session); with
// several, the entry whose path is "/<workerName>" is required.
func ExtractDebugWS(ctx context.Context, debugURL, workerName string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, debugURL, nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching %s: %w", debugURL, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}

	var endpoints []string
	for _, m := range wsRe.FindAllStringSubmatch(string(body), -1) {
		decoded, err := url.QueryUnescape(m[1])
		if err != nil {
			continue
		}
		endpoints = append(endpoints, decoded)
	}
	if len(endpoints) == 0 {
		return "", fmt.Errorf("%s contains no ws= debug endpoints", debugURL)
	}
	if len(endpoints) == 1 {
		return "ws://" + endpoints[0], nil
	}
	for _, ep := range endpoints {
		if strings.HasSuffix(ep, "/"+workerName) {
			return "ws://" + ep, nil
		}
	}
	return "", fmt.Errorf("%s lists %d debug endpoints but none ends in /%s", debugURL, len(endpoints), workerName)
}
