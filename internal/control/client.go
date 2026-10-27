package control

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"time"
)

// ErrNoServer means nothing answered on the socket: no wrkmn is running for
// that project (or it predates the control socket).
var ErrNoServer = errors.New("no running wrkmn")

// dialTimeout bounds the connect; the reply wait is the caller's timeout.
const dialTimeout = 2 * time.Second

// Call sends one request and waits up to timeout for the response.
func Call(path string, req Request, timeout time.Duration) (Response, error) {
	conn, err := net.DialTimeout("unix", path, dialTimeout)
	if err != nil {
		return Response{}, fmt.Errorf("%w; socket %s: %v", ErrNoServer, path, err)
	}
	defer conn.Close()
	deadline := time.Now().Add(timeout)
	if err := conn.SetDeadline(deadline); err != nil {
		return Response{}, err
	}
	if err := WriteRequest(conn, req); err != nil {
		return Response{}, fmt.Errorf("sending request: %w", err)
	}
	resp, err := ReadResponse(bufio.NewReader(conn))
	if err != nil {
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			return Response{}, fmt.Errorf("timed out after %s waiting for wrkmn to answer %s", timeout, req.Cmd)
		}
		return Response{}, fmt.Errorf("reading response: %w", err)
	}
	return resp, nil
}
