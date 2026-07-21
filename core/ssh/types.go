// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

// ReverseTunnelUser is the username used when machine agents
// connect to the controller to establish a reverse tunnel.
const ReverseTunnelUser = "juju-reverse-tunnel"

// PublicKey represents a single public ssh key for a user within a model.
type PublicKey struct {
	// Fingerprint is the calculated fingerprint of the ssh key.
	Fingerprint string

	// Key is the raw public key.
	Key string
}
