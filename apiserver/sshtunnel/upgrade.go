// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshtunnel

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/juju/juju/internal/errors"
)

// PerformUpgrade writes req over conn, reads the response head, and returns
// the connection on a 101 Switching Protocols response. Any bytes the
// response reader buffered past the head (e.g. the server's SSH banner) are
// prepended to the returned connection so they are not lost.
//
// It is the client-side counterpart to the server-side hijack helper and is
// shared by the machine agent's tunnel dial and any other upgrade client.
func PerformUpgrade(req *http.Request, conn net.Conn) (net.Conn, error) {
	if err := req.Write(conn); err != nil {
		return nil, errors.Errorf("writing upgrade request: %w", err)
	}

	// Read the response head with a bounded reader. The connection is handed
	// over raw afterwards, so any bytes the reader buffered past the response
	// head must be preserved.
	reader := bufio.NewReaderSize(conn, 4096)
	resp, err := http.ReadResponse(reader, req)
	if err != nil {
		return nil, errors.Errorf("reading upgrade response: %w", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		body := make([]byte, 1024)
		n, _ := io.ReadFull(io.LimitReader(resp.Body, 1024), body)
		_ = resp.Body.Close()
		return nil, errors.Errorf("upgrade rejected: %s: %s", resp.Status, strings.TrimSpace(string(body[:n])))
	}
	_ = resp.Body.Close()

	// If the reader buffered bytes past the response head, prepend them to
	// the returned connection.
	if buffered := reader.Buffered(); buffered > 0 {
		prefix, err := reader.Peek(buffered)
		if err != nil {
			return nil, errors.Errorf("reading buffered bytes: %w", err)
		}
		return &prefixConn{Conn: conn, prefix: append([]byte(nil), prefix...)}, nil
	}
	return conn, nil
}

// prefixConn wraps a net.Conn, serving a prefix of already-read bytes before
// reading from the underlying connection. This preserves bytes a bufio.Reader
// consumed past an HTTP response head.
type prefixConn struct {
	net.Conn
	prefix []byte
}

// Read first drains the prefix, then reads from the underlying connection.
func (c *prefixConn) Read(p []byte) (int, error) {
	if len(c.prefix) > 0 {
		n := copy(p, c.prefix)
		c.prefix = c.prefix[n:]
		return n, nil
	}
	return c.Conn.Read(p)
}

// CloseWrite delegates to the underlying connection if it supports half-close,
// so a tunnel pipe can signal EOF per direction through the wrapper.
func (c *prefixConn) CloseWrite() error {
	if hc, ok := c.Conn.(interface{ CloseWrite() error }); ok {
		return hc.CloseWrite()
	}
	return nil
}
