// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rafttransport

import (
	"fmt"
	"net"
	"net/http"
)

// Handler is an http.Handler suitable for serving an endpoint that
// upgrades to raft transport connections.
type Handler struct {
	connections chan<- net.Conn
	abort       <-chan struct{}
}

// NewHandler returns a new Handler that sends connections to the
// given connections channel, and stops accepting connections after
// the abort channel is closed.
func NewHandler(
	connections chan<- net.Conn,
	abort <-chan struct{},
) *Handler {
	return &Handler{
		connections: connections,
		abort:       abort,
	}
}

// ServeHTTP is part of the http.Handler interface.
//
// ServeHTTP checks for "raft" upgrade requests, and hijacks
// those connections for use as a raw connection for raft
// communications.
//
// Based on code from https://github.com/CanonicalLtd/raft-http.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Fail immediately if we've been closed.
	select {
	case <-h.abort:
		http.Error(w, "raft transport closed", http.StatusForbidden)
		return
	default:
	}

	if r.Header.Get("Upgrade") != "raft" {
		http.Error(w, "missing or invalid upgrade header", http.StatusBadRequest)
		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "webserver doesn't support hijacking", http.StatusInternalServerError)
		return
	}

	conn, _, err := hijacker.Hijack()
	if err != nil {
		message := fmt.Sprintf("failed to hijack connection: %s", err)
		http.Error(w, message, http.StatusInternalServerError)
		return
	}

	// Write the status line and upgrade header by hand since w.WriteHeader()
	// would fail after Hijack()
	data := []byte("HTTP/1.1 101 Switching Protocols\r\nUpgrade: raft\r\n\r\n")
	if n, err := conn.Write(data); err != nil || n != len(data) {
		conn.Close()
		return
	}

	select {
	case h.connections <- conn:
	case <-r.Context().Done():
		conn.Close()
	}
}
