// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package leaseconsumer

import "net/http"

// Handler is an http.Handler suitable for serving lease consuming connections.
type Handler struct {
	abort <-chan struct{}
}

// NewHandler returns a new Handler that sends connections to the
// given connections channel, and stops accepting connections after
// the abort channel is closed.
func NewHandler(
	abort <-chan struct{},
) *Handler {
	return &Handler{
		abort: abort,
	}
}

// ServeHTTP is part of the http.Handler interface.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Fail immediately if we've been closed.
	select {
	case <-h.abort:
		http.Error(w, "lease consumer closed", http.StatusGone)
		return
	default:
	}
}
