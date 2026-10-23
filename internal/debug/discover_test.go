package debug

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDiscoverWorkerWSPrimary(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<script>const workerUrls = ["https://devtools.devprod.cloudflare.dev/js_app?debugger=true&ws=localhost%3A9229%2Fapi&domain=api"];</script>`))
	}))
	defer srv.Close()
	debugURL := func() string { return srv.URL }
	ws, err := DiscoverWorkerWS(context.Background(), "api", debugURL, nil, 3*time.Second)
	if err != nil || ws != "ws://localhost:9229/api" {
		t.Fatalf("DiscoverWorkerWS = (%q, %v)", ws, err)
	}
}

func TestDiscoverWorkerWSWaitsForBanner(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<script>const workerUrls = ["https://x/js_app?ws=localhost%3A9229%2Fapi"];</script>`))
	}))
	defer srv.Close()
	// url is set from a goroutine after a delay and polled from DiscoverWorkerWS's
	// loop; guard it with a mutex so -race doesn't flag the legitimate handoff.
	var mu sync.Mutex
	var url string
	go func() {
		time.Sleep(400 * time.Millisecond)
		mu.Lock()
		url = srv.URL
		mu.Unlock()
	}()
	debugURL := func() string {
		mu.Lock()
		defer mu.Unlock()
		return url
	}
	ws, err := DiscoverWorkerWS(context.Background(), "api", debugURL, nil, 3*time.Second)
	if err != nil || ws != "ws://localhost:9229/api" {
		t.Fatalf("late banner not handled: (%q, %v)", ws, err)
	}
}

func TestDiscoverWorkerWSFallbackScan(t *testing.T) {
	// Review Focus 5: a /json listing with a foreign target plus the real one —
	// match strictly by "/<worker>" path.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/json" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `[
			{"webSocketDebuggerUrl":"ws://localhost:9229/some-node-process","title":"node"},
			{"webSocketDebuggerUrl":"ws://localhost:9229/api","title":"Cloudflare Worker"}
		]`)
	}))
	defer srv.Close()
	_, portStr, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	port, _ := strconv.Atoi(portStr)

	ws, err := DiscoverWorkerWS(context.Background(), "api", func() string { return "" }, []int{port}, 3*time.Second)
	if err != nil || ws != "ws://localhost:9229/api" {
		t.Fatalf("fallback = (%q, %v)", ws, err)
	}
	// No match for an unknown worker — must error, not grab the first target.
	if _, err := DiscoverWorkerWS(context.Background(), "ghost", func() string { return "" }, []int{port}, 1*time.Second); err == nil {
		t.Fatal("expected no-match error for unknown worker")
	}
}

func TestDiscoverWorkerWSTimeoutNamesSteps(t *testing.T) {
	// Review Focus 3: nothing ever appears — bounded error naming what failed.
	start := time.Now()
	_, err := DiscoverWorkerWS(context.Background(), "api", func() string { return "" }, nil, 700*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("timeout not bounded")
	}
	if !strings.Contains(err.Error(), "api") || !strings.Contains(err.Error(), "Debug URL") {
		t.Fatalf("error not descriptive: %v", err)
	}
}
