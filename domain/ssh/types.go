// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"time"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/errors"
)

// ValidateSSHServerPort checks that port is a valid TCP port for the
// controller SSH jump server to listen on. An error satisfying
// [coreerrors.NotValid] is returned if it is not.
func ValidateSSHServerPort(port int) error {
	if port <= 0 || port > 65535 {
		return errors.Errorf("port %d out of range 1-65535", port).Add(coreerrors.NotValid)
	}
	return nil
}

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
	// ControllerAddresses contains the controller addresses to use for the SSH connection.
	ControllerAddresses network.SpaceAddresses
	// UnitPort holds the port that the unit worker will forward traffic to on the machine.
	// If this is 0, it defaults to port 22.
	UnitPort int
	// EphemeralPublicKey contains the ephemeral public key to use for the SSH connection.
	EphemeralPublicKey []byte
}
