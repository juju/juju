// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container

import (
	"github.com/juju/juju/worker/uniter/pebble"
)

// Terminator is responsible for providing a helper interface to the
// uniter for ensuring the charm processes started with the containers under
// management are shutdown cleanly.
type Terminator interface {
	// ShutdownContainers takes a set of containers to shutdown cleanly that are
	// being managed  by this Uniter. Returns the subset of containers that were
	// not able to be successfully shutdown and any errors occurred.
	ShutdownContainers([]string) ([]string, error)
}

// noopTerminator is a noop implementation of the Terminator interface.
type noopTerminator struct{}

// NewTerminator returns a terminator that is capable of shutting down
// containers that are running under pebble or older style Juju container
// implementations.
func NewTerminator(isPebble bool) Terminator {
	// If it's not pebble then do nothing because there is nothing to do.
	if !isPebble {
		return &noopTerminator{}
	}

	return pebble.NewTerminator(func(container string) (pebble.TerminatorClient, error) {
		return pebble.ClientForContainer(container)
	})
}

// ShutdownContainers implements the Terminator interface.
func (_ *noopTerminator) ShutdownContainers(_ []string) ([]string, error) {
	return []string{}, nil
}
