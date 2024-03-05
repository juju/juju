// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pinger

import (
	"context"

	"github.com/juju/worker/v4"
)

// Pinger describes a resource that can be pinged and stopped.
type Pinger interface {
	worker.Worker
	Ping()
}

// API serves pinger-specific API methods.
type API struct {
	pinger Pinger
}

// NewAPI builds a new facade for the given backend.
func NewAPI(pinger Pinger) *API {
	return &API{
		pinger: pinger,
	}
}

// Ping is used by the client heartbeat monitor and resets.
func (a API) Ping(ctx context.Context) {
	a.pinger.Ping()
}

// Stop stops the pinger.
func (a API) Stop(ctx context.Context) error {
	a.pinger.Kill()
	return a.pinger.Wait()
}
