// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

const (
	// WellKnownUUIDNamespace is Juju's namespace for deterministic UUIDs.
	WellKnownUUIDNamespace = "96bb15e6-8b85-448b-9fce-ede1a1700e64"

	// SSHServerHostKeyWellKnownName is the deterministic name used for the
	// controller jump host key row.
	SSHServerHostKeyWellKnownName = "juju.ssh.server.host.key"

	// SSHServerHostKeyUUID is the well-known UUID of the controller jump host
	// key row.
	SSHServerHostKeyUUID = "dcbfd8fd-773d-52e3-8d64-fbd27483b072"

	// SSHKeyEncodingTypeOpenSSHID identifies PEM-wrapped OpenSSH private keys.
	SSHKeyEncodingTypeOpenSSHID = 0
	// SSHKeyEncodingTypeOpenSSH is the stored key encoding for current SSH host
	// keys.
	SSHKeyEncodingTypeOpenSSH = "openssh"
)
