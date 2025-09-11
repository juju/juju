// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	coremachine "github.com/juju/juju/core/machine"
	domainnetwork "github.com/juju/juju/domain/network"
)

// MachineIdentifiers represents the set of available identifiers that can be
// used to reference a machine by in the model.
type MachineIdentifiers struct {
	UUID        coremachine.UUID
	Name        coremachine.Name
	NetNodeUUID domainnetwork.NetNodeUUID
}
