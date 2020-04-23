// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import "github.com/juju/errors"

// Locus represents a graph for a given set of link layer devices along with
// their associated ip addresses (if any).
//
// Extracting information about machine or provider addresses will then be
// possible.
//
//                              +---------------+
//                              |               |
//                              |               |
//                              |     LOCUS     |
//                              |               |
//                              |               |
//                              +-------+-------+
//                                      |
//                           +----------+--------+
//                           |                   |
//                   +-------+------+     +------+-------+
//                   |              |     |              |
//                   |              |     |              |
//                   |  LINK LAYER  |     |  LINK LAYER  |
//                   |    DEVICE    |     |    DEVICE    |
//                   |              |     |              |
//                   |              |     |              |
//                   +-------+------+     +--------------+
//                           |
//         +-----------------+
//         |                 |
// +-------+-------+ +-------+-------+
// |               | |               |
// |               | |               |
// |  IP ADDRESS   | |  IP ADDRESS   |
// |               | |               |
// |               | |               |
// +---------------+ +---------------+
//
// The name locus comes from the idea of a particular position or place where
// something occurs. We can then use one locus to compare to another to get
// the situation of divergence.
type Locus struct {
	machines  InterfaceInfos
	providers InterfaceInfos
}

// NewLocus creates a new Locus to work on.
func NewLocus() *Locus {
	return &Locus{}
}

// Add a new InterfaceInfo to the locus.
// It expects the InterfaceInfo.Addresses to be homogenous to the origin.
func (l *Locus) Add(origin Origin, info InterfaceInfo) error {
	switch origin {
	case OriginMachine:
		l.machines = append(l.machines, info)
	case OriginProvider:
		l.providers = append(l.providers, info)
	default:
		return errors.Errorf("unexpected origin: %q", origin)
	}
	return nil
}

// MachineAddresses returns the MachineAddresses for a set of interfaces.
func (l *Locus) MachineAddresses() []ProviderAddresses {
	return providerAddresses(l.machines)
}

// ProviderAddresses returns the ProviderAddresses for a set of interfaces.
func (l *Locus) ProviderAddresses() []ProviderAddresses {
	return providerAddresses(l.providers)
}

func providerAddresses(interfaces InterfaceInfos) []ProviderAddresses {
	var result []ProviderAddresses
	for _, info := range interfaces {
		result = append(result, info.Addresses)
	}
	return result
}

// LinkLayerDevice describes the link-layer network device for a machine.
type LinkLayerDevice interface {
	// Addresses returns all IP addresses assigned to the device.
	Addresses() ([]LinkLayerDeviceAddress, error)
	// ProviderID returns the provider-specific device ID, if set.
	ProviderID() Id
}

// LinkLayerDeviceAddress represents the state of an IP address assigned to a
// link-layer network device on a machine.
type LinkLayerDeviceAddress interface {
	// Origin represents the authoritative source of the ipAddress.
	// It is expected that either the provider gave us this address or the
	// machine gave us this address.
	// Giving us this information allows us to reason about when a ipAddress is
	// in use.
	Origin() Origin

	// Value returns the value of this IP address.
	Value() string
}

// NewLocusFromLinkLayerDevices creates a new Locus from a given set of
// LinkLayerDevices.
func NewLocusFromLinkLayerDevices(devices []LinkLayerDevice) (*Locus, error) {
	locus := NewLocus()
	for _, linkLayerDevice := range devices {
		addresses, err := linkLayerDevice.Addresses()
		if err != nil {
			return nil, errors.Trace(err)
		}

		origin, providerAddresses, err := getProviderAddresses(addresses)
		if err != nil {
			return nil, errors.Trace(err)
		}

		locus.Add(origin, InterfaceInfo{
			ProviderId: linkLayerDevice.ProviderID(),
			Addresses:  providerAddresses,
		})
	}

	return locus, nil
}

// getProviderAddresses iterates over a slice of LinkLayerDevice address, to
// return a homogeneous set of ProviderAddresses.
// If the underlying LinkLayerDevice addresses aren't of the same type, then
// an error is returned stating that.
// As we don't know what the origin will be from the slice we have to inspect
// that and return that depending on that as well.
func getProviderAddresses(addresses []LinkLayerDeviceAddress) (Origin, ProviderAddresses, error) {
	if len(addresses) == 0 {
		return OriginProvider, nil, nil
	}

	// To ensure homogeneous we can use the first address.
	origin := addresses[0].Origin()
	result := make(ProviderAddresses, len(addresses))
	for i, address := range addresses {
		if origin != address.Origin() {
			return OriginUnknown, nil, errors.Errorf("expected homogeneous link-layer device addresses")
		}
		result[i] = NewProviderAddress(address.Value())
	}

	return origin, result, nil
}
