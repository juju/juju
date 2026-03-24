package internal

import (
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
	PrivateAddress []network.SpaceAddress
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
