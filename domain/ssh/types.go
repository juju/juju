// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"time"

	"github.com/juju/juju/core/network"
)

// SSHConnRequest describes a one-shot reverse tunnel request for a machine.
type SSHConnRequest struct {
	TunnelID            string
	MachineID           string
	Expires             time.Time
	Username            string
	Password            string
	ControllerAddresses network.SpaceAddresses
	UnitPort            int
	EphemeralPublicKey  []byte
}
