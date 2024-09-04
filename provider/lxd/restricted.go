// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

// A RestrictedServer is a LXD client which can only perform a restricted
// subset of operations, such as getting information about the LXD cluster as a
// whole. These operations do not require one to specify the LXD project.
type RestrictedServer interface {
	// ServerVersion returns the version of the LXD server.
	ServerVersion() string
	// LocalBridgeName returns the name of the local LXD network bridge.
	LocalBridgeName() string
}
