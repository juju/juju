// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshproxy

import (
	"context"
	"net/http"

	"gopkg.in/tomb.v2"
)

// hijackWorker is the shared worker skeleton for the SSH upgrade handlers.
// Killing it cancels the request contexts of in-flight upgrades, whose
// dying watches close their hijacked connections on apiserver shutdown.
type hijackWorker struct {
	tomb tomb.Tomb
}

// start launches the tomb keeper so Wait can reach dead.
func (h *hijackWorker) start() {
	h.tomb.Go(func() error {
		<-h.tomb.Dying()
		return tomb.ErrDying
	})
}

// requestContext ties the request to the worker's tomb.
func (h *hijackWorker) requestContext(r *http.Request) context.Context {
	return h.tomb.Context(r.Context())
}

// Kill implements worker.Worker.Kill.
func (h *hijackWorker) Kill() {
	h.tomb.Kill(nil)
}

// Wait implements worker.Worker.Wait.
func (h *hijackWorker) Wait() error {
	return h.tomb.Wait()
}
