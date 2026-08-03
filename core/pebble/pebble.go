// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pebble

const (
	// DefaultPebbleDir is the default directory Pebble considers when starting
	// up. It is the root of the default Pebble instance.
	DefaultPebbleDir = "/var/lib/pebble/default"

	// DefaultPebbleSocket is the default Pebble Unix socket path, located
	// inside DefaultPebbleDir.
	DefaultPebbleSocket = DefaultPebbleDir + "/.pebble.socket"

	// ContainerAgentService is the Pebble service name for the
	// containeragent process. It must match the service name defined in the
	// containeragent's Pebble layer (see
	// cmd/containeragent/initialize/command.go).
	ContainerAgentService = "container-agent"
)
