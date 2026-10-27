package control

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestRequestRoundTripIsNewlineTerminatedJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteRequest(&buf, Request{Cmd: CmdLogs, Name: "stretch-ui", Tail: 50}); err != nil {
		t.Fatal(err)
	}
	raw := buf.String()
	if !strings.HasSuffix(raw, "\n") || strings.Count(raw, "\n") != 1 {
		t.Fatalf("request must be exactly one newline-terminated line, got %q", raw)
	}
	if raw != `{"cmd":"logs","name":"stretch-ui","tail":50}`+"\n" {
		t.Fatalf("unexpected wire format: %q", raw)
	}
	got, err := ReadRequest(bufio.NewReader(&buf))
	if err != nil {
		t.Fatal(err)
	}
	if got.Cmd != CmdLogs || got.Name != "stretch-ui" || got.Tail != 50 {
		t.Fatalf("decoded %+v", got)
	}
}

func TestResponseRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	resp := NewResponse(map[string]int{"n": 1})
	if err := WriteResponse(&buf, resp); err != nil {
		t.Fatal(err)
	}
	if buf.String() != `{"ok":true,"result":{"n":1}}`+"\n" {
		t.Fatalf("unexpected wire format: %q", buf.String())
	}
	got, err := ReadResponse(bufio.NewReader(&buf))
	if err != nil {
		t.Fatal(err)
	}
	if !got.OK || string(got.Result) != `{"n":1}` {
		t.Fatalf("decoded %+v", got)
	}

	buf.Reset()
	if err := WriteResponse(&buf, Errorf("boom %d", 7)); err != nil {
		t.Fatal(err)
	}
	if buf.String() != `{"ok":false,"error":"boom 7"}`+"\n" {
		t.Fatalf("unexpected error wire format: %q", buf.String())
	}
}

func TestRequestValidate(t *testing.T) {
	cases := []struct {
		req  Request
		want bool
	}{
		{Request{Cmd: CmdStatus}, true},
		{Request{Cmd: CmdStartAll}, true},
		{Request{Cmd: CmdStopAll}, true},
		{Request{Cmd: CmdStart, Name: "x"}, true},
		{Request{Cmd: CmdStop, Name: "x"}, true},
		{Request{Cmd: CmdRestart, Name: "x"}, true},
		{Request{Cmd: CmdLogs, Name: "x"}, true},
		{Request{Cmd: CmdStart}, false},
		{Request{Cmd: CmdLogs}, false},
		{Request{Cmd: "dance"}, false},
		{Request{}, false},
		{Request{Cmd: CmdLogs, Name: "x", Tail: -1}, false},
	}
	for _, c := range cases {
		err := c.req.Validate()
		if (err == nil) != c.want {
			t.Errorf("Validate(%+v) err=%v, want ok=%v", c.req, err, c.want)
		}
	}
}

func TestReadRequestRejectsGarbage(t *testing.T) {
	if _, err := ReadRequest(bufio.NewReader(strings.NewReader("not json\n"))); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestDefaultTail(t *testing.T) {
	if DefaultTail != 200 {
		t.Fatalf("DefaultTail = %d, want 200", DefaultTail)
	}
	var r Request
	if err := json.Unmarshal([]byte(`{"cmd":"logs","name":"x"}`), &r); err != nil {
		t.Fatal(err)
	}
	if r.TailOrDefault() != DefaultTail {
		t.Fatalf("TailOrDefault = %d", r.TailOrDefault())
	}
}
