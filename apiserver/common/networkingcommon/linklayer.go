// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networkingcommon

import (
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
)

// LinkLayerDevice describes a single layer-2 network device.
type LinkLayerDevice interface {
	// ID returns the unique identifier for the device.
	ID() string

	// MACAddress is the hardware address of the device.
	MACAddress() string

	// Name is the name of the device.
	Name() string

	// ProviderID returns the provider-specific identifier for this device.
	ProviderID() network.Id

	// SetProviderIDOps returns the operations required to set the input
	// provider ID for the link-layer device.
	SetProviderIDOps(id network.Id) ([]txn.Op, error)

	// ParentID returns the globally unique identifier
	// for this device's parent if it has one.
	ParentID() string

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

	// AllAddresses returns all IP addresses assigned to the machine's
	// link-layer devices
	AllAddresses() ([]LinkLayerAddress, error)
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
}

// MachineLinkLayerOp is a base type for model operations that update
// link-layer data for a single machine/host/container.
type MachineLinkLayerOp struct {
	// machine is the machine for which this operation
	// sets link-layer device information.
	machine LinkLayerMachine

	// incoming is the network interface information supplied for update.
	incoming network.InterfaceInfos

	// processedDevs is the set of hardware IDs that we have
	// processed from the incoming interfaces.
	processedDevs set.Strings

	// processedAddrs is the set of IP addresses that we have processed,
	// keyed by the hardware address of the device they apply to.
	// In theory this allows the same IP address to exist on devices in
	// physically separate networks.
	processedAddrs map[string]set.Strings

	existingDevs  []LinkLayerDevice
	existingAddrs []LinkLayerAddress
}

// NewMachineLinkLayerOp returns a reference that can be embedded in a model
// operation for updating the input machine's link layer data.
func NewMachineLinkLayerOp(machine LinkLayerMachine, incoming network.InterfaceInfos) *MachineLinkLayerOp {
	logger.Debugf("processing link-layer devices for machine %q", machine.Id())

	return &MachineLinkLayerOp{
		machine:        machine,
		incoming:       incoming,
		processedDevs:  set.NewStrings(),
		processedAddrs: make(map[string]set.Strings),
	}
}

// Incoming is a property accessor for the link-layer data we are processing.
func (o *MachineLinkLayerOp) Incoming() network.InterfaceInfos {
	return o.incoming
}

// ExistingDevices is a property accessor for the
// currently known machine link-layer devices.
func (o *MachineLinkLayerOp) ExistingDevices() []LinkLayerDevice {
	return o.existingDevs
}

// ExistingAddresses is a property accessor for the currently
// known addresses assigned to machine link-layer devices.
func (o *MachineLinkLayerOp) ExistingAddresses() []LinkLayerAddress {
	return o.existingAddrs
}

// PopulateExistingDevices retrieves all current
// link-layer devices for the machine.
func (o *MachineLinkLayerOp) PopulateExistingDevices() error {
	var err error
	o.existingDevs, err = o.machine.AllLinkLayerDevices()
	return errors.Trace(err)
}

// PopulateExistingAddresses retrieves all current
// link-layer device addresses for the machine.
func (o *MachineLinkLayerOp) PopulateExistingAddresses() error {
	var err error
	o.existingAddrs, err = o.machine.AllAddresses()
	return errors.Trace(err)
}

// MatchingIncoming returns the first incoming interface that
// matches the input known device, based on hardware address.
// Nil is returned if there is no match.
func (o *MachineLinkLayerOp) MatchingIncoming(dev LinkLayerDevice) *network.InterfaceInfo {
	if matches := o.incoming.GetByHardwareAddress(dev.MACAddress()); len(matches) > 0 {
		return &matches[0]
	}
	return nil
}

// MatchingIncomingAddrs finds all the primary addresses on devices matching
// the input hardware address, and returns them as state args.
// TODO (manadart 2020-07-15): We should investigate making an enhanced
// core/network address type instead of this state type.
// It would embed ProviderAddress and could be obtained directly via a method
// or property of InterfaceInfos.
func (o *MachineLinkLayerOp) MatchingIncomingAddrs(hwAddress string) []state.LinkLayerDeviceAddress {
	return networkAddressStateArgsForHWAddr(o.Incoming(), hwAddress)
}

// DeviceAddresses returns all currently known
// IP addresses assigned to the input device.
func (o *MachineLinkLayerOp) DeviceAddresses(dev LinkLayerDevice) []LinkLayerAddress {
	var addrs []LinkLayerAddress
	for _, addr := range o.existingAddrs {
		if addr.DeviceName() == dev.Name() {
			addrs = append(addrs, addr)
		}
	}
	return addrs
}

// AssertAliveOp returns a transaction operation for asserting that the machine
// for which we are updating link-layer data is alive.
func (o *MachineLinkLayerOp) AssertAliveOp() txn.Op {
	return o.machine.AssertAliveOp()
}

// MarkDevProcessed indicates that the input (known) device was present in the
// incoming data and its updates have been handled by the build step.
func (o *MachineLinkLayerOp) MarkDevProcessed(dev LinkLayerDevice) {
	o.processedDevs.Add(dev.MACAddress())
}

// IsDevProcessed returns a boolean indicating whether the input incoming
// device matches a known device that was marked as processed by the method
// above.
func (o *MachineLinkLayerOp) IsDevProcessed(dev network.InterfaceInfo) bool {
	return o.processedDevs.Contains(dev.MACAddress)
}

// MarkAddrProcessed indicates that the input (known) IP address was present in
// the incoming data for the device with input hardware address.
func (o *MachineLinkLayerOp) MarkAddrProcessed(hwAddr, ipAddr string) {
	if _, ok := o.processedAddrs[hwAddr]; !ok {
		o.processedAddrs[hwAddr] = set.NewStrings(ipAddr)
	} else {
		o.processedAddrs[hwAddr].Add(ipAddr)
	}
}

// IsAddrProcessed returns a boolean indicating whether the input incoming
// device/address pair matches an entry that was marked as processed by the
// method above.
func (o *MachineLinkLayerOp) IsAddrProcessed(hwAddr, ipAddr string) bool {
	if addrs, ok := o.processedAddrs[hwAddr]; ok {
		return addrs.Contains(ipAddr)
	}
	return false
}

// Done (state.ModelOperation) returns the result of running the operation.
func (o *MachineLinkLayerOp) Done(err error) error {
	return err
}
