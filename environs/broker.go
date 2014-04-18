// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/tools"
)

// StartInstanceParams holds parameters for the
// InstanceBroker.StartInstance method.
type StartInstanceParams struct {
	// Constraints is a set of constraints on
	// the kind of instance to create.
	Constraints constraints.Value

	// Tools is a list of tools that may be used
	// to start a Juju agent on the machine.
	Tools tools.List

	// MachineConfig describes the machine's configuration.
	MachineConfig *cloudinit.MachineConfig

	// DistributionGroup, if non-nil, is a function
	// that returns a slice of instance.Ids that belong
	// to the same distribution group as the machine
	// being provisioned. The InstanceBroker may use
	// this information to distribute instances for
	// high availability.
	DistributionGroup func() ([]instance.Id, error)
}

// NetworkInfo describes a single network interface available on an
// instance. For providers that support networks, this will be
// available at StartInstance() time.
type NetworkInfo struct {
	// MACAddress is the network interface's hardware MAC address
	// (e.g. "aa:bb:cc:dd:ee:ff").
	MACAddress string

	// CIDR of the network, in 123.45.67.89/24 format.
	CIDR string

	// NetworkName is juju-internal name of the network.
	NetworkName string

	// ProviderId is a provider-specific network id.
	ProviderId instance.NetworkId

	// VLANTag needs to be between 1 and 4094 for VLANs and 0 for
	// normal networks. It's defined by IEEE 802.1Q standard.
	VLANTag int

	// InterfaceName is the OS-specific network device name (e.g.
	// "eth0" or "eth1.42" for a VLAN virtual interface).
	InterfaceName string

	// IsVirtual is true when the interface is a virtual device, as
	// opposed to a physical device.
	IsVirtual bool
}

// TODO(wallyworld) - we want this in the environs/instance package but import loops
// stop that from being possible right now.
type InstanceBroker interface {
	// StartInstance asks for a new instance to be created, associated with
	// the provided config in machineConfig. The given config describes the juju
	// state for the new instance to connect to. The config MachineNonce, which must be
	// unique within an environment, is used by juju to protect against the
	// consequences of multiple instances being started with the same machine
	// id.
	StartInstance(args StartInstanceParams) (instance.Instance, *instance.HardwareCharacteristics, []NetworkInfo, error)

	// StopInstances shuts down the given instances.
	StopInstances([]instance.Instance) error

	// AllInstances returns all instances currently known to the broker.
	AllInstances() ([]instance.Instance, error)
}
