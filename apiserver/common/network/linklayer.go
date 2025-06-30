// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"github.com/juju/mgo/v3/txn"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
)

// LinkLayerDevice describes a single layer-2 network device.
type LinkLayerDevice interface {
	// MACAddress is the hardware address of the device.
	MACAddress() string

	// Name is the name of the device.
	Name() string

	// ProviderID returns the provider-specific identifier for this device.
	ProviderID() network.Id

	// Type returns the device's type.
	Type() network.LinkLayerDeviceType

	// SetProviderIDOps returns the operations required to set the input
	// provider ID for the link-layer device.
	SetProviderIDOps(id network.Id) ([]txn.Op, error)

	// RemoveOps returns the transaction operations required to remove this
	// device and if required, its provider ID.
	RemoveOps() []txn.Op

	// UpdateOps returns the transaction operations required to update the
	// device so that it reflects the incoming arguments.
	UpdateOps(state.LinkLayerDeviceArgs) []txn.Op

	// AddAddressOps returns transaction operations required
	// to add the input address to the device.
	AddAddressOps(state.LinkLayerDeviceAddress) ([]txn.Op, error)
}

// LinkLayerAddress describes a single layer-3 network address
// assigned to a layer-2 device.
type LinkLayerAddress interface {
	// DeviceName is the name of the device to which this address is assigned.
	DeviceName() string

	// Value returns the actual IP address.
	Value() string

	// Origin indicates the authority that is maintaining this address.
	Origin() network.Origin

	// SetProviderIDOps returns the operations required to set the input
	// provider ID for the address.
	SetProviderIDOps(id network.Id) ([]txn.Op, error)

	// SetOriginOps returns the transaction operations required to change
	// the origin for this address.
	SetOriginOps(origin network.Origin) []txn.Op

	// SetProviderNetIDsOps returns the transaction operations required to ensure
	// that the input provider IDs are set against the address.
	SetProviderNetIDsOps(networkID, subnetID network.Id) []txn.Op

	// RemoveOps returns the transaction operations required to remove this
	// address and if required, its provider ID.
	RemoveOps() []txn.Op

	// UpdateOps returns the transaction operations required to update the device
	// so that it reflects the incoming arguments.
	UpdateOps(state.LinkLayerDeviceAddress) ([]txn.Op, error)
}

// LinkLayerAccessor describes an entity that can
// return link-layer data related to it.
type LinkLayerAccessor interface {
	// AllLinkLayerDevices returns all currently known
	// layer-2 devices for the machine.
	AllLinkLayerDevices() ([]LinkLayerDevice, error)

	// AllDeviceAddresses returns all IP addresses assigned to
	// the machine's link-layer devices
	AllDeviceAddresses() ([]LinkLayerAddress, error)
}

// LinkLayerWriter describes an entity that can have link-layer
// devices added to it.
type LinkLayerWriter interface {
	// AddLinkLayerDeviceOps returns transaction operations for adding the
	// input link-layer device and the supplied addresses to the machine.
	AddLinkLayerDeviceOps(state.LinkLayerDeviceArgs, ...state.LinkLayerDeviceAddress) ([]txn.Op, error)
}

// LinkLayerMachine describes a machine that can return its link-layer data
// and assert that it is alive in preparation for updating such data.
type LinkLayerMachine interface {
	LinkLayerAccessor
	LinkLayerWriter

	// Id returns the ID for the machine.
	Id() string

	// AssertAliveOp returns a transaction operation for asserting
	// that the machine is currently alive.
	AssertAliveOp() txn.Op

	// ModelUUID returns the unique identifier
	// for the model that this machine is in.
	ModelUUID() string
}

// LinkLayerState describes methods required for sanitising and persisting
// link-layer data sourced from a single machine.
type LinkLayerState interface {
	// Machine returns the machine for which link-layer data is being set.
	Machine(string) (LinkLayerMachine, error)

	// ApplyOperation applied the model operation that sets link-layer data.
	ApplyOperation(state.ModelOperation) error
}

// LinkLayerAndSubnetsState describes a persistence indirection that includes
// the ability to update link-layer data and add discovered subnets.
type LinkLayerAndSubnetsState interface {
	LinkLayerState
}
