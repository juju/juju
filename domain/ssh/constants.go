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

	// SSHKeyAlgorithmTypeRSAID identifies RSA SSH private keys.
	SSHKeyAlgorithmTypeRSAID = 0
	// SSHKeyAlgorithmTypeRSA is the stored algorithm for RSA SSH private keys.
	SSHKeyAlgorithmTypeRSA = "ssh-rsa"

	// SSHKeyAlgorithmTypeECDSA256ID identifies P-256 ECDSA SSH private keys.
	SSHKeyAlgorithmTypeECDSA256ID = 1
	// SSHKeyAlgorithmTypeECDSA256 is the stored algorithm for P-256 ECDSA SSH
	// private keys.
	SSHKeyAlgorithmTypeECDSA256 = "ecdsa-sha2-nistp256"

	// SSHKeyAlgorithmTypeED25519ID identifies ED25519 SSH private keys.
	SSHKeyAlgorithmTypeED25519ID = 2
	// SSHKeyAlgorithmTypeED25519 is the stored algorithm for ED25519 SSH private
	// keys.
	SSHKeyAlgorithmTypeED25519 = "ssh-ed25519"
)
