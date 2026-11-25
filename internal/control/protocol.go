package control

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Commands. One JSON request per connection, one JSON response back, both
// newline-terminated.
const (
	CmdStatus   = "status"
	CmdStart    = "start"
	CmdStop     = "stop"
	CmdRestart  = "restart"
	CmdStartAll = "start-all"
	CmdStopAll  = "stop-all"
	CmdLogs     = "logs"
)

// DefaultTail is the number of log lines `logs` returns when tail is unset.
const DefaultTail = 200

// maxLine bounds a single request/response line on the wire.
const maxLine = 4 << 20

// Request is the wire form of one control command.
type Request struct {
	Cmd  string `json:"cmd"`
	Name string `json:"name,omitempty"`
	Tail int    `json:"tail,omitempty"`
}

// Response is the wire form of a reply. Result is left as raw JSON so the
// client can decode it into the command-specific type.
type Response struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// StatusRow mirrors one row of the TUI's process table.
type StatusRow struct {
	Name          string `json:"name"`
	Status        string `json:"status"`
	Port          int    `json:"port"`
	UptimeSeconds int64  `json:"uptime_seconds"`
	Path          string `json:"path"`
	Type          string `json:"type"`
	// Manual mirrors `autostart: false`: the row only starts on explicit
	// request; a stopped manual row is deliberately off, not broken.
	Manual bool `json:"manual,omitempty"`
	// Worker-only fields (type == "worker").
	DevMode      string `json:"dev_mode,omitempty"`
	SelectedMode string `json:"selected_mode,omitempty"`
	DevServerURL string `json:"dev_server_url,omitempty"`
	PID          int    `json:"pid,omitempty"`
}

// StatusResult is the result of status, start-all and stop-all.
type StatusResult struct {
	Processes []StatusRow `json:"processes"`
}

// ActionResult is the result of start, stop and restart.
type ActionResult struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

// LogsResult is the result of logs.
type LogsResult struct {
	Name  string   `json:"name"`
	Lines []string `json:"lines"`
}

func needsName(cmd string) bool {
	switch cmd {
	case CmdStart, CmdStop, CmdRestart, CmdLogs:
		return true
	}
	return false
}

// Validate rejects unknown commands and missing arguments before a request
// reaches the TUI.
func (r Request) Validate() error {
	switch r.Cmd {
	case CmdStatus, CmdStart, CmdStop, CmdRestart, CmdStartAll, CmdStopAll, CmdLogs:
	case "":
		return errors.New("missing cmd")
	default:
		return fmt.Errorf("unknown cmd %q", r.Cmd)
	}
	if needsName(r.Cmd) && r.Name == "" {
		return fmt.Errorf("%s requires a name", r.Cmd)
	}
	if r.Tail < 0 {
		return fmt.Errorf("tail must be >= 0, got %d", r.Tail)
	}
	return nil
}

// TailOrDefault returns the requested tail length, or DefaultTail.
func (r Request) TailOrDefault() int {
	if r.Tail <= 0 {
		return DefaultTail
	}
	return r.Tail
}

// NewResponse wraps a successful result.
func NewResponse(result any) Response {
	resp := Response{OK: true}
	if result == nil {
		return resp
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return Errorf("encoding result: %v", err)
	}
	resp.Result = raw
	return resp
}

// Errorf builds a failure response.
func Errorf(format string, args ...any) Response {
	return Response{OK: false, Error: fmt.Sprintf(format, args...)}
}

func writeLine(w io.Writer, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = w.Write(append(data, '\n'))
	return err
}

func readLine(r *bufio.Reader, v any) error {
	line, err := r.ReadBytes('\n')
	if err != nil && (err != io.EOF || len(line) == 0) {
		return err
	}
	if len(line) > maxLine {
		return fmt.Errorf("line exceeds %d bytes", maxLine)
	}
	return json.Unmarshal(line, v)
}

// WriteRequest writes one newline-terminated request.
func WriteRequest(w io.Writer, req Request) error { return writeLine(w, req) }

// ReadRequest reads one newline-terminated request.
func ReadRequest(r *bufio.Reader) (Request, error) {
	var req Request
	err := readLine(r, &req)
	return req, err
}

// WriteResponse writes one newline-terminated response.
func WriteResponse(w io.Writer, resp Response) error { return writeLine(w, resp) }

// ReadResponse reads one newline-terminated response.
func ReadResponse(r *bufio.Reader) (Response, error) {
	var resp Response
	err := readLine(r, &resp)
	return resp, err
}
