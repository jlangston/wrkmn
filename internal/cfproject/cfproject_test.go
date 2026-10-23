package cfproject

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDetect(t *testing.T) {
	dir := t.TempDir()
	if _, ok := Detect(dir); ok {
		t.Fatal("empty dir detected as cf project")
	}
	writeFile(t, dir, "cloudflare.config.ts", "export default {}")
	path, ok := Detect(dir)
	if !ok || path != filepath.Join(dir, "cloudflare.config.ts") {
		t.Fatalf("Detect = (%q, %v)", path, ok)
	}
}

func TestDetectVariants(t *testing.T) {
	for _, name := range []string{"cloudflare.config.js", "cloudflare.config.mjs", "cloudflare.config.mts"} {
		dir := t.TempDir()
		writeFile(t, dir, name, "export default {}")
		if _, ok := Detect(dir); !ok {
			t.Fatalf("%s not detected", name)
		}
	}
}

func TestDetectUnmigrated(t *testing.T) {
	dir := t.TempDir()
	if DetectUnmigrated(dir) {
		t.Fatal("empty dir reported unmigrated")
	}
	writeFile(t, dir, "wrangler.toml", "name = \"x\"")
	if !DetectUnmigrated(dir) {
		t.Fatal("wrangler.toml-only dir not reported unmigrated")
	}
	writeFile(t, dir, "cloudflare.config.ts", "export default {}")
	if DetectUnmigrated(dir) {
		t.Fatal("migrated dir still reported unmigrated")
	}
}

func TestReadBuildOutput(t *testing.T) {
	dir := t.TempDir()
	bo, err := ReadBuildOutput(dir)
	if bo != nil || err != nil {
		t.Fatalf("missing build output should be (nil, nil), got (%v, %v)", bo, err)
	}
	outDir := filepath.Join(dir, ".cloudflare", "output", "v0")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, outDir, "config.json", `{"name":"api","account_id":"abc123","unknown_field":[1,2]}`)
	bo, err = ReadBuildOutput(dir)
	if err != nil || bo == nil || bo.WorkerName != "api" || bo.AccountID != "abc123" {
		t.Fatalf("ReadBuildOutput = (%+v, %v)", bo, err)
	}
}

func TestParseBannerLine(t *testing.T) {
	cases := []struct {
		line string
		kind BannerKind
		url  string
		ok   bool
	}{
		{"  ➜  Local:   http://localhost:5173/", BannerLocal, "http://localhost:5173/", true},
		{"  ➜  Debug:   http://localhost:5173/__debug", BannerDebug, "http://localhost:5173/__debug", true},
		// Review Focus 1: ANSI-colored banner (green arrow, bold label, cyan URL)
		{"\x1b[32m  ➜\x1b[39m  \x1b[1mLocal\x1b[22m:   \x1b[36mhttp://localhost:5174/\x1b[39m", BannerLocal, "http://localhost:5174/", true},
		{"\x1b[32m  ➜\x1b[39m  Debug:   \x1b[36mhttp://localhost:5174/__debug\x1b[39m", BannerDebug, "http://localhost:5174/__debug", true},
		{"  ➜  Network: use --host to expose", 0, "", false},
		{"VITE v8.3.2 ready in 3433 ms", 0, "", false},
		{"", 0, "", false},
	}
	for _, c := range cases {
		kind, url, ok := ParseBannerLine(c.line)
		if kind != c.kind || url != c.url || ok != c.ok {
			t.Fatalf("ParseBannerLine(%q) = (%v, %q, %v), want (%v, %q, %v)", c.line, kind, url, ok, c.kind, c.url, c.ok)
		}
	}
}

// debugHTML mirrors the real /__debug payload captured in the 2026-10-01 probe.
func debugHTML(entries ...string) string {
	list := ""
	for i, e := range entries {
		if i > 0 {
			list += ","
		}
		list += `"https://devtools.devprod.cloudflare.dev/js_app?theme=systemPreferred&debugger=true&ws=` + e + `&domain=x"`
	}
	return `<script>const workerUrls = [` + list + `];</script>`
}

func TestExtractDebugWS(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(debugHTML("localhost%3A9229%2Fcf-probe")))
	}))
	defer srv.Close()
	ws, err := ExtractDebugWS(context.Background(), srv.URL, "cf-probe")
	if err != nil || ws != "ws://localhost:9229/cf-probe" {
		t.Fatalf("ExtractDebugWS = (%q, %v)", ws, err)
	}
}

func TestExtractDebugWSMultiWorker(t *testing.T) {
	// Review Focus 2: multiple workers in one dev session — pick by name.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(debugHTML("localhost%3A9229%2Fstretch-api", "localhost%3A9229%2Fstretch-render")))
	}))
	defer srv.Close()
	ws, err := ExtractDebugWS(context.Background(), srv.URL, "stretch-render")
	if err != nil || ws != "ws://localhost:9229/stretch-render" {
		t.Fatalf("multi-worker pick = (%q, %v)", ws, err)
	}
	if _, err := ExtractDebugWS(context.Background(), srv.URL, "no-such-worker"); err == nil {
		t.Fatal("expected error when no entry matches the worker name")
	}
}

func TestExtractDebugWSNoEntries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html>nothing here</html>"))
	}))
	defer srv.Close()
	if _, err := ExtractDebugWS(context.Background(), srv.URL, "x"); err == nil {
		t.Fatal("expected error for page without ws entries")
	}
}
