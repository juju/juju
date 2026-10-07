// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshproxy

import (
	"net"
	"net/http"
	"time"

	"golang.org/x/net/http/httpguts"

	coresshproxy "github.com/juju/juju/core/sshproxy"
	"github.com/juju/juju/internal/errors"
)

// pushTunnelTimeout bounds how long the handler waits for a tunnel
// consumer to claim a pushed connection.
const pushTunnelTimeout = 10 * time.Second

// hijack upgrades the HTTP request to a raw connection. It validates the
// upgrade headers, writes the 101 Switching Protocols response, hijacks
// the connection, preserves any bytes buffered by the server's bufio reader,
// and clears any deadlines so the protocol taking over the connection
// owns its lifecycle.
//
// The returned connection is the caller's responsibility. The HTTP server
// no longer tracks it.
func hijack(w http.ResponseWriter, r *http.Request, token string) (net.Conn, error) {
	// Connection may list multiple tokens (e.g. "keep-alive, Upgrade").
	if !httpguts.HeaderValuesContainsToken(r.Header["Connection"], "Upgrade") || r.Header.Get("Upgrade") != token {
		http.Error(w, "invalid upgrade request", http.StatusBadRequest)
		return nil, errors.Errorf("expected Upgrade: %s", token)
	}
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "connection cannot be upgraded", http.StatusInternalServerError)
		return nil, errors.Errorf("response writer does not support hijacking")
	}

	// Write the 101 response before hijacking, so the headers go out
	// through the server's machinery.
	w.Header().Set("Connection", "Upgrade")
	w.Header().Set("Upgrade", token)
	w.WriteHeader(http.StatusSwitchingProtocols)

	conn, buf, err := hijacker.Hijack()
	if err != nil {
		return nil, errors.Errorf("hijacking connection: %w", err)
	}

	// net/http may have buffered upgraded protocol bytes, so keep them.
	reader := buf.Reader
	if buffered := reader.Buffered(); buffered > 0 {
		prefix, err := reader.Peek(buffered)
		if err != nil {
			_ = conn.Close()
			return nil, errors.Errorf("reading buffered bytes: %w", err)
		}
		conn = coresshproxy.NewPrefixConn(conn, prefix)
	}

	// Clear any read/write deadlines the HTTP server set. The protocol
	// taking over the connection manages its own lifecycle.
	_ = conn.SetDeadline(time.Time{})

	return conn, nil
}
