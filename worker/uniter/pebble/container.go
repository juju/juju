// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pebble

import (
	"fmt"
	"path"

	"github.com/canonical/pebble/client"
)

const (
	// pebbleSocketPathPrefix is the path prefix to apply to the pebble socket
	// for containers.
	pebbleSocketPathPrefix = "/charm/containers"

	// pebbleSocketName is the name of the socket file used for Pebble.
	pebbleSocketName = "pebble.socket"
)

// ClientForContainer constructs a new pebble client for the specified container
// name.
func ClientForContainer(container string) (*client.Client, error) {
	sockPath := SocketPathForContainer(container)
	config := &client.Config{
		Socket: sockPath,
	}

	client, err := client.New(config)
	if err != nil {
		return client, fmt.Errorf("creating pebble client for container %q at socket path %q: %w",
			container,
			sockPath,
			err)
	}

	return client, nil
}

// SocketPathForContainer generates the path to the Pebble socket for a given
// container name.
func SocketPathForContainer(container string) string {
	return path.Join(pebbleSocketPathPrefix, container, pebbleSocketName)
}
