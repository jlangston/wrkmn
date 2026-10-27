package control

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// stubHandler records every request and answers from a table, standing in
// for the TUI model.
type stubHandler struct {
	mu   sync.Mutex
	seen []Request
}

func (s *stubHandler) handle(req Request) Response {
	s.mu.Lock()
	s.seen = append(s.seen, req)
	s.mu.Unlock()
	switch req.Cmd {
	case CmdStatus:
		return NewResponse(StatusResult{Processes: []StatusRow{{Name: "api", Status: "running", Port: 8787, Type: "worker", DevMode: "local", SelectedMode: "development"}}})
	case CmdStart, CmdStop, CmdRestart:
		if req.Name == "ghost" {
			return Errorf("no process named %q", req.Name)
		}
		return NewResponse(ActionResult{Name: req.Name, Status: "running"})
	case CmdStartAll, CmdStopAll:
		return NewResponse(StatusResult{})
	case CmdLogs:
		return NewResponse(LogsResult{Name: req.Name, Lines: []string{"a", "b"}})
	}
	return Errorf("unhandled")
}

func shortSockPath(t *testing.T) string {
	t.Helper()
	// Unix socket paths are limited to ~104 bytes; keep it short.
	dir, err := os.MkdirTemp("", "wc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, "s.sock")
}

func TestServerHandlesEveryCommand(t *testing.T) {
	path := shortSockPath(t)
	stub := &stubHandler{}
	srv := NewServer(path, stub.handle)
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	reqs := []Request{
		{Cmd: CmdStatus},
		{Cmd: CmdStart, Name: "api"},
		{Cmd: CmdStop, Name: "api"},
		{Cmd: CmdRestart, Name: "api"},
		{Cmd: CmdStartAll},
		{Cmd: CmdStopAll},
		{Cmd: CmdLogs, Name: "api", Tail: 2},
	}
	for _, req := range reqs {
		resp, err := Call(path, req, 5*time.Second)
		if err != nil {
			t.Fatalf("%s: %v", req.Cmd, err)
		}
		if !resp.OK {
			t.Fatalf("%s: ok=false err=%s", req.Cmd, resp.Error)
		}
	}
	var status StatusResult
	resp, _ := Call(path, Request{Cmd: CmdStatus}, time.Second)
	if err := json.Unmarshal(resp.Result, &status); err != nil {
		t.Fatal(err)
	}
	if len(status.Processes) != 1 || status.Processes[0].DevMode != "local" {
		t.Fatalf("status result = %+v", status)
	}
	var logs LogsResult
	resp, _ = Call(path, Request{Cmd: CmdLogs, Name: "api"}, time.Second)
	if err := json.Unmarshal(resp.Result, &logs); err != nil {
		t.Fatal(err)
	}
	if len(logs.Lines) != 2 {
		t.Fatalf("logs result = %+v", logs)
	}

	stub.mu.Lock()
	defer stub.mu.Unlock()
	if len(stub.seen) != len(reqs)+2 {
		t.Fatalf("handler saw %d requests, want %d", len(stub.seen), len(reqs)+2)
	}
	if stub.seen[6].Tail != 2 {
		t.Fatalf("tail not forwarded: %+v", stub.seen[6])
	}
}

func TestServerPropagatesHandlerErrorAsOKFalse(t *testing.T) {
	path := shortSockPath(t)
	srv := NewServer(path, (&stubHandler{}).handle)
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	resp, err := Call(path, Request{Cmd: CmdStart, Name: "ghost"}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if resp.OK || resp.Error == "" {
		t.Fatalf("resp = %+v, want ok=false with error", resp)
	}
}

func TestServerRejectsInvalidRequestsWithoutCallingHandler(t *testing.T) {
	path := shortSockPath(t)
	stub := &stubHandler{}
	srv := NewServer(path, stub.handle)
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	// Unknown command.
	resp, err := Call(path, Request{Cmd: "dance"}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if resp.OK {
		t.Fatal("unknown cmd must be rejected")
	}
	// Malformed JSON straight on the wire.
	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("{nope\n")); err != nil {
		t.Fatal(err)
	}
	resp, err = ReadResponse(bufio.NewReader(conn))
	if err != nil {
		t.Fatal(err)
	}
	if resp.OK {
		t.Fatal("malformed request must be rejected")
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if len(stub.seen) != 0 {
		t.Fatalf("handler must not be called for invalid requests, saw %+v", stub.seen)
	}
}

func TestServerRemovesStaleSocketOnStartAndFileOnClose(t *testing.T) {
	path := shortSockPath(t)
	// A dead socket file from a crashed instance.
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	ln.Close()
	if _, err := os.Stat(path); err == nil {
		// Go removes the file on Close for unix listeners; recreate a stale
		// one by hand to simulate a crash.
		t.Fatal("expected Close to remove file; test setup assumption changed")
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	srv := NewServer(path, (&stubHandler{}).handle)
	if err := srv.Start(); err != nil {
		t.Fatalf("Start over stale socket: %v", err)
	}
	if _, err := Call(path, Request{Cmd: CmdStatus}, time.Second); err != nil {
		t.Fatalf("call after stale removal: %v", err)
	}
	if err := srv.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("socket file should be removed on Close, stat err=%v", err)
	}
}

func TestServerRefusesToClobberLiveSocket(t *testing.T) {
	path := shortSockPath(t)
	first := NewServer(path, (&stubHandler{}).handle)
	if err := first.Start(); err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second := NewServer(path, (&stubHandler{}).handle)
	if err := second.Start(); err == nil {
		second.Close()
		t.Fatal("second server must refuse to replace a live socket")
	}
	// The first server is still reachable.
	if _, err := Call(path, Request{Cmd: CmdStatus}, time.Second); err != nil {
		t.Fatalf("first server broken by second Start: %v", err)
	}
}

func TestServerStartCreatesParentDirPrivately(t *testing.T) {
	dir, err := os.MkdirTemp("", "wc")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "sub", "s.sock")
	srv := NewServer(path, (&stubHandler{}).handle)
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	info, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Fatalf("socket dir perm = %o, want 700", perm)
	}
}

func TestCallReportsNoServer(t *testing.T) {
	path := shortSockPath(t)
	_, err := Call(path, Request{Cmd: CmdStatus}, time.Second)
	if !errors.Is(err, ErrNoServer) {
		t.Fatalf("err = %v, want ErrNoServer", err)
	}
}

func TestCallTimesOutOnSilentHandler(t *testing.T) {
	path := shortSockPath(t)
	block := make(chan struct{})
	srv := NewServer(path, func(Request) Response { <-block; return NewResponse(nil) })
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	defer close(block)
	start := time.Now()
	_, err := Call(path, Request{Cmd: CmdStatus}, 200*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("Call did not honour timeout, took %s", time.Since(start))
	}
}
