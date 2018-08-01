// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package gomaasapi

import "github.com/juju/collections/set"

const (
	// Capability constants.
	NetworksManagement      = "networks-management"
	StaticIPAddresses       = "static-ipaddresses"
	IPv6DeploymentUbuntu    = "ipv6-deployment-ubuntu"
	DevicesManagement       = "devices-management"
	StorageDeploymentUbuntu = "storage-deployment-ubuntu"
	NetworkDeploymentUbuntu = "network-deployment-ubuntu"
)

// Controller represents an API connection to a MAAS Controller. Since the API
// is restful, there is no long held connection to the API server, but instead
// HTTP calls are made and JSON response structures parsed.
type Controller interface {

	// Capabilities returns a set of capabilities as defined by the string
	// constants.
	Capabilities() set.Strings

	BootResources() ([]BootResource, error)

	// Fabrics returns the list of Fabrics defined in the MAAS controller.
	Fabrics() ([]Fabric, error)

	// Spaces returns the list of Spaces defined in the MAAS controller.
	Spaces() ([]Space, error)

	// StaticRoutes returns the list of StaticRoutes defined in the MAAS controller.
	StaticRoutes() ([]StaticRoute, error)

	// Zones lists all the zones known to the MAAS controller.
	Zones() ([]Zone, error)

	// Machines returns a list of machines that match the params.
	Machines(MachinesArgs) ([]Machine, error)

	// AllocateMachine will attempt to allocate a machine to the user.
	// If successful, the allocated machine is returned.
	AllocateMachine(AllocateMachineArgs) (Machine, ConstraintMatches, error)

	// ReleaseMachines will stop the specified machines, and release them
	// from the user making them available to be allocated again.
	ReleaseMachines(ReleaseMachinesArgs) error

	// Devices returns a list of devices that match the params.
	Devices(DevicesArgs) ([]Device, error)

	// CreateDevice creates and returns a new Device.
	CreateDevice(CreateDeviceArgs) (Device, error)

	// Files returns all the files that match the specified prefix.
	Files(prefix string) ([]File, error)

	// Return a single file by its filename.
	GetFile(filename string) (File, error)

	// AddFile adds or replaces the content of the specified filename.
	// If or when the MAAS api is able to return metadata about a single
	// file without sending the content of the file, we can return a File
	// instance here too.
	AddFile(AddFileArgs) error

	// Returns the DNS Domain Managed By MAAS
	Domains() ([]Domain, error)
}

// File represents a file stored in the MAAS controller.
type File interface {
	// Filename is the name of the file. No path, just the filename.
	Filename() string

	// AnonymousURL is a URL that can be used to retrieve the conents of the
	// file without credentials.
	AnonymousURL() string

	// Delete removes the file from the MAAS controller.
	Delete() error

	// ReadAll returns the content of the file.
	ReadAll() ([]byte, error)
}

// Fabric represents a set of interconnected VLANs that are capable of mutual
// communication. A fabric can be thought of as a logical grouping in which
// VLANs can be considered unique.
//
// For example, a distributed network may have a fabric in London containing
// VLAN 100, while a separate fabric in San Francisco may contain a VLAN 100,
// whose attached subnets are completely different and unrelated.
type Fabric interface {
	ID() int
	Name() string
	ClassType() string

	VLANs() []VLAN
}

// VLAN represents an instance of a Virtual LAN. VLANs are a common way to
// create logically separate networks using the same physical infrastructure.
//
// Managed switches can assign VLANs to each port in either a “tagged” or an
// “untagged” manner. A VLAN is said to be “untagged” on a particular port when
// it is the default VLAN for that port, and requires no special configuration
// in order to access.
//
// “Tagged” VLANs (traditionally used by network administrators in order to
// aggregate multiple networks over inter-switch “trunk” lines) can also be used
// with nodes in MAAS. That is, if a switch port is configured such that
// “tagged” VLAN frames can be sent and received by a MAAS node, that MAAS node
// can be configured to automatically bring up VLAN interfaces, so that the
// deployed node can make use of them.
//
// A “Default VLAN” is created for every Fabric, to which every new VLAN-aware
// object in the fabric will be associated to by default (unless otherwise
// specified).
type VLAN interface {
	ID() int
	Name() string
	Fabric() string

	// VID is the VLAN ID. eth0.10 -> VID = 10.
	VID() int
	// MTU (maximum transmission unit) is the largest size packet or frame,
	// specified in octets (eight-bit bytes), that can be sent.
	MTU() int
	DHCP() bool

	PrimaryRack() string
	SecondaryRack() string
}

// Zone represents a physical zone that a Machine is in. The meaning of a
// physical zone is up to you: it could identify e.g. a server rack, a network,
// or a data centre. Users can then allocate nodes from specific physical zones,
// to suit their redundancy or performance requirements.
type Zone interface {
	Name() string
	Description() string
}

type Domain interface {
	// The name of the Domain
	Name() string
}

// BootResource is the bomb... find something to say here.
type BootResource interface {
	ID() int
	Name() string
	Type() string
	Architecture() string
	SubArchitectures() set.Strings
	KernelFlavor() string
}

// Device represents some form of device in MAAS.
type Device interface {
	// TODO: add domain
	SystemID() string
	Hostname() string
	FQDN() string
	IPAddresses() []string
	Zone() Zone

	// Parent returns the SystemID of the Parent. Most often this will be a
	// Machine.
	Parent() string

	// Owner is the username of the user that created the device.
	Owner() string

	// InterfaceSet returns all the interfaces for the Device.
	InterfaceSet() []Interface

	// CreateInterface will create a physical interface for this machine.
	CreateInterface(CreateInterfaceArgs) (Interface, error)

	// Delete will remove this Device.
	Delete() error
}

// Machine represents a physical machine.
type Machine interface {
	OwnerDataHolder

	SystemID() string
	Hostname() string
	FQDN() string
	Tags() []string

	OperatingSystem() string
	DistroSeries() string
	Architecture() string
	Memory() int
	CPUCount() int

	IPAddresses() []string
	PowerState() string

	// Devices returns a list of devices that match the params and have
	// this Machine as the parent.
	Devices(DevicesArgs) ([]Device, error)

	// Consider bundling the status values into a single struct.
	// but need to check for consistent representation if exposed on other
	// entities.

	StatusName() string
	StatusMessage() string

	// BootInterface returns the interface that was used to boot the Machine.
	BootInterface() Interface
	// InterfaceSet returns all the interfaces for the Machine.
	InterfaceSet() []Interface
	// Interface returns the interface for the machine that matches the id
	// specified. If there is no match, nil is returned.
	Interface(id int) Interface

	// PhysicalBlockDevices returns all the physical block devices on the machine.
	PhysicalBlockDevices() []BlockDevice
	// PhysicalBlockDevice returns the physical block device for the machine
	// that matches the id specified. If there is no match, nil is returned.
	PhysicalBlockDevice(id int) BlockDevice

	// BlockDevices returns all the physical and virtual block devices on the machine.
	BlockDevices() []BlockDevice
	// BlockDevice returns the block device for the machine that matches the
	// id specified. If there is no match, nil is returned.
	BlockDevice(id int) BlockDevice

	Zone() Zone

	// Start the machine and install the operating system specified in the args.
	Start(StartArgs) error

	// CreateDevice creates a new Device with this Machine as the parent.
	// The device will have one interface that is linked to the specified subnet.
	CreateDevice(CreateMachineDeviceArgs) (Device, error)
}

// Space is a name for a collection of Subnets.
type Space interface {
	ID() int
	Name() string
	Subnets() []Subnet
}

// Subnet refers to an IP range on a VLAN.
type Subnet interface {
	ID() int
	Name() string
	Space() string
	VLAN() VLAN

	Gateway() string
	CIDR() string
	// dns_mode

	// DNSServers is a list of ip addresses of the DNS servers for the subnet.
	// This list may be empty.
	DNSServers() []string
}

// StaticRoute defines an explicit route that users have requested to be added
// for a given subnet.
type StaticRoute interface {
	// Source is the subnet that should have the route configured. (Machines
	// inside Source should use GatewayIP to reach Destination addresses.)
	Source() Subnet
	// Destination is the subnet that a machine wants to send packets to. We
	// want to configure a route to that subnet via GatewayIP.
	Destination() Subnet
	// GatewayIP is the IPAddress to direct traffic to.
	GatewayIP() string
	// Metric is the routing metric that determines whether this route will
	// take precedence over similar routes (there may be a route for 10/8, but
	// also a more concrete route for 10.0/16 that should take precedence if it
	// applies.) Metric should be a non-negative integer.
	Metric() int
}

// Interface represents a physical or virtual network interface on a Machine.
type Interface interface {
	ID() int
	Name() string
	// The parents of an interface are the names of interfaces that must exist
	// for this interface  to exist. For example a parent of "eth0.100" would be
	// "eth0". Parents may be empty.
	Parents() []string
	// The children interfaces are the names of those that are dependent on this
	// interface existing. Children may be empty.
	Children() []string
	Type() string
	Enabled() bool
	Tags() []string

	VLAN() VLAN
	Links() []Link

	MACAddress() string
	EffectiveMTU() int

	// Params is a JSON field, and defaults to an empty string, but is almost
	// always a JSON object in practice. Gleefully ignoring it until we need it.

	// Update the name, mac address or VLAN.
	Update(UpdateInterfaceArgs) error

	// Delete this interface.
	Delete() error

	// LinkSubnet will attempt to make this interface available on the specified
	// Subnet.
	LinkSubnet(LinkSubnetArgs) error

	// UnlinkSubnet will remove the Link to the subnet, and release the IP
	// address associated if there is one.
	UnlinkSubnet(Subnet) error
}

// Link represents a network link between an Interface and a Subnet.
type Link interface {
	ID() int
	Mode() string
	Subnet() Subnet
	// IPAddress returns the address if one has been assigned.
	// If unavailble, the address will be empty.
	IPAddress() string
}

// FileSystem represents a formatted filesystem mounted at a location.
type FileSystem interface {
	// Type is the format type, e.g. "ext4".
	Type() string

	MountPoint() string
	Label() string
	UUID() string
}

// Partition represents a partition of a block device. It may be mounted
// as a filesystem.
type Partition interface {
	ID() int
	Path() string
	// FileSystem may be nil if not mounted.
	FileSystem() FileSystem
	UUID() string
	// UsedFor is a human readable string.
	UsedFor() string
	// Size is the number of bytes in the partition.
	Size() uint64
}

// BlockDevice represents an entire block device on the machine.
type BlockDevice interface {
	ID() int
	Name() string
	Model() string
	IDPath() string
	Path() string
	UsedFor() string
	Tags() []string

	BlockSize() uint64
	UsedSize() uint64
	Size() uint64

	Partitions() []Partition

	// There are some other attributes for block devices, but we can
	// expose them on an as needed basis.
}

// OwnerDataHolder represents any MAAS object that can store key/value
// data.
type OwnerDataHolder interface {
	// OwnerData returns a copy of the key/value data stored for this
	// object.
	OwnerData() map[string]string

	// SetOwnerData updates the key/value data stored for this object
	// with the values passed in. Existing keys that aren't specified
	// in the map passed in will be left in place; to clear a key set
	// its value to "". All owner data is cleared when the object is
	// released.
	SetOwnerData(map[string]string) error
}
