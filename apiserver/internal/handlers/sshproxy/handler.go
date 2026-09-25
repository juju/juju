// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshproxy

import (
	"net"
	"net/http"
	"time"

	"golang.org/x/net/http/httpguts"

	"github.com/juju/juju/internal/errors"
)

// pushTunnelTimeout bounds how long the handler waits for a tunnel
// consumer to claim a pushed connection.
const pushTunnelTimeout = 10 * time.Second

// hijack upgrades the HTTP request to a raw connection. It validates the
// upgrade headers, writes the 101 Switching Protocols response, hijacks
// the connection, drains any bytes buffered by the server's bufio reader,
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

	// Drain any bytes the server buffered beyond the request head before
	// handing the connection to the next protocol.
	reader := buf.Reader
	if buffered := reader.Buffered(); buffered > 0 {
		if _, err := reader.Discard(buffered); err != nil {
			_ = conn.Close()
			return nil, errors.Errorf("draining buffered bytes: %w", err)
		}
	}

	// Clear any read/write deadlines the HTTP server set. The protocol
	// taking over the connection manages its own lifecycle.
	_ = conn.SetDeadline(time.Time{})

	return conn, nil
}
