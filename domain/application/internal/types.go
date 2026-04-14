// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/unit"
)

// ProxySettings contains the proxy settings for a unit context.
type ProxySettings struct {
	HTTP    string
	HTTPS   string
	FTP     string
	NoProxy string
}

// IAASUnitContext contains the IAAS context information required for the
// construction of a context factory for a unit.
type IAASUnitContext struct {
	// LegacyProxySettings contains the proxy settings for a unit context that
	// are set in the model config, using the legacy proxy configuration keys.
	LegacyProxySettings ProxySettings
	// JujuProxySettings contains the proxy settings for a unit context that are
	// set in the model config, using the Juju proxy configuration keys.
	JujuProxySettings ProxySettings
	/// PrivateAddress contains the private address for a unit context.
	PrivateAddress *string
	// OpenedMachinePortRangesByEndpoint contains the opened machine port ranges
	// by endpoint for a unit context.
	OpenedMachinePortRangesByEndpoint map[unit.Name]network.GroupedPortRanges
}

// CAASUnitContext contains the CAAS context information required for the
// construction of a context factory for a unit.
type CAASUnitContext struct {
	// LegacyProxySettings contains the proxy settings for a unit context that
	// are set in the model config, using the legacy proxy configuration keys.
	LegacyProxySettings ProxySettings
	// JujuProxySettings contains the proxy settings for a unit context that are
	// set in the model config, using the Juju proxy configuration keys.
	JujuProxySettings ProxySettings
	// OpenedPortRangesByEndpoint contains the opened port ranges by endpoint
	// for a unit context.
	OpenedPortRangesByEndpoint map[unit.Name]network.GroupedPortRanges
}

// UpdateUnitCharmArg contains information required for changing the charm used
// by a unit.
type UpdateUnitCharmArg struct {
	// UUID is the uuid of the unit to be refreshed.
	UUID unit.UUID

	// CharmUUID is the uuid of the new charm that the unit will use.
	CharmUUID charm.ID

	// UnitStorage are the arguments required to create new storage for this
	// unit once it has changed charms.
	UnitStorage CreateUnitStorageArg

	// MachineUUID is set when this unit is an IAAS unit on a machine.
	MachineUUID *machine.UUID

	// IAASUnitStorage is set when this is an IAAS unit on a machine, they are
	// the arguments required to create new storage for this unit once it has
	// changed charms.
	IAASUnitStorage *CreateIAASUnitStorageArg
}
