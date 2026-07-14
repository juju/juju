// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import gossh "golang.org/x/crypto/ssh"

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

// EphemeralKeysUpdater adds and removes ephemeral SSH keys from the machine's
// authorized_keys file. It is consumed by the sshsession worker, which injects
// an ephemeral key for the lifetime of a reverse SSH tunnel.
type EphemeralKeysUpdater interface {
	// AddEphemeralKey adds an ephemeral key to the authorized_keys file,
	// tagged with the supplied comment for later removal.
	AddEphemeralKey(key gossh.PublicKey, comment string) error
	// RemoveEphemeralKey removes a previously added ephemeral key.
	RemoveEphemeralKey(key gossh.PublicKey) error
}
