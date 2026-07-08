// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"time"

	"github.com/juju/juju/core/network"
)

// SSHConnRequest describes a one-shot reverse tunnel request for a machine in
// a model.
type SSHConnRequest struct {
	// TunnelID is the unique identifier for the SSH connection request to enable the tunneler
	// to route this reverse connection to the correct client connection.
	TunnelID string
	// MachineName is the name of the machine that the SSH connection is being requested for.
	MachineName string
	// Expires is the time at which the SSH connection request expires.
	Expires time.Time
	// SSHUsername contains the reverse tunnel username to use for the SSH connection.
	SSHUsername string
	// SSHPassword contains the reverse tunnel JWT, and is not actually
	// a plaintext password.
	SSHPassword string
	// ControllerAddresses contains the controller addresses to use for the SSH connection.
	ControllerAddresses network.SpaceAddresses
	// UnitPort contains the port of the unit that the SSH connection is being requested for.
	UnitPort int
	// EphemeralPublicKey contains the ephemeral public key to use for the SSH connection.
	EphemeralPublicKey []byte
}
