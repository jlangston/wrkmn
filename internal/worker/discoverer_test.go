package worker

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverCfProjects(t *testing.T) {
	root := t.TempDir()
	// A cf project with a package.json name.
	writeFile(t, filepath.Join(root, "api", "cloudflare.config.ts"), "export default {}")
	writeFile(t, filepath.Join(root, "api", "package.json"), `{"name":"my-api","scripts":{"dev":"cf dev"}}`)
	// A cf project with no package.json — named by directory.
	writeFile(t, filepath.Join(root, "bare", "cloudflare.config.ts"), "export default {}")
	// An unmigrated wrangler project — reported, not a worker.
	writeFile(t, filepath.Join(root, "legacy", "wrangler.toml"), `name = "legacy"`)
	// A plain directory — ignored entirely.
	writeFile(t, filepath.Join(root, "docs", "readme.md"), "hi")

	workers, unmigrated, err := NewDiscoverer().DiscoverWorkers(root)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	names := map[string]*Worker{}
	for _, w := range workers {
		names[w.Name] = w
	}
	if len(workers) != 2 {
		t.Fatalf("workers = %d (%v), want 2", len(workers), names)
	}
	if w := names["my-api"]; w == nil || !w.IsCloudflareWorker() || w.Scripts["dev"] != "cf dev" {
		t.Fatalf("my-api = %+v", names["my-api"])
	}
	if w := names["bare"]; w == nil || !w.IsCloudflareWorker() {
		t.Fatalf("bare = %+v", names["bare"])
	}
	if len(unmigrated) != 1 || filepath.Base(unmigrated[0]) != "legacy" {
		t.Fatalf("unmigrated = %v", unmigrated)
	}
}

func TestDiscoverBuildOutputEnrichment(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "api")
	writeFile(t, filepath.Join(dir, "cloudflare.config.ts"), "export default {}")
	writeFile(t, filepath.Join(dir, ".cloudflare", "output", "v0", "config.json"), `{"name":"deployed-api","account_id":"acc1"}`)

	workers, _, err := NewDiscoverer().DiscoverWorkers(root)
	if err != nil || len(workers) != 1 {
		t.Fatalf("discover: %v (%d workers)", err, len(workers))
	}
	if workers[0].Name != "deployed-api" || workers[0].AccountID != "acc1" {
		t.Fatalf("enrichment: %+v", workers[0])
	}
}
