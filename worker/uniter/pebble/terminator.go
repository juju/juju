// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pebble

import (
	"fmt"

	"github.com/canonical/pebble/client"
)

// Terminator is a pebble specific client for terminating pebble services in
// containers.
type Terminator struct {
	clientFunc TerminatorClientFunc
}

// TerminatorClient defines the very small subset of functionality needed from
// Pebble to terminate containers.
type TerminatorClient interface {
	Shutdown(*client.ShutdownOptions) error
}

// TerminatorClientFunc defines the function signature needed to generate a new
// TerminatorClient.
type TerminatorClientFunc func(string) (TerminatorClient, error)

// NewTerminator constructs a new container terminator from the client supplied
// by clientFunc.
func NewTerminator(clientFunc TerminatorClientFunc) *Terminator {
	return &Terminator{
		clientFunc: clientFunc,
	}
}

// ShutdownContainers terminates all the Pebble instances for the set of
// containers supplied. Return the subset of containers that were not able to be
// shutdown and any errors.
func (t *Terminator) ShutdownContainers(containers []string) ([]string, error) {
	i := 0
	for ; i < len(containers); i++ {
		container := containers[i]
		tc, err := t.clientFunc(container)
		if err != nil {
			return containers[i:],
				fmt.Errorf("creating pebble terminator client for container %q: %w", container, err)
		}

		if err := tc.Shutdown(&client.ShutdownOptions{}); err != nil {
			return containers[i:],
				fmt.Errorf("shutting down pebble for container %q: %w", container, err)
		}
	}

	return []string{}, nil
}
