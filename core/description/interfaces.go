// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	"time"

	"github.com/juju/version"
	"gopkg.in/juju/names.v2"
)

// HasAnnotations defines the common methods for setting and
// getting annotations for the various entities.
type HasAnnotations interface {
	Annotations() map[string]string
	SetAnnotations(map[string]string)
}

// HasConstraints defines the common methods for setting and
// getting constraints for the various entities.
type HasConstraints interface {
	Constraints() Constraints
	SetConstraints(ConstraintsArgs)
}

// HasStatus defines the common methods for setting and getting status
// entries for the various entities.
type HasStatus interface {
	Status() Status
	SetStatus(StatusArgs)
}

// HasStatusHistory defines the common methods for setting and
// getting historical status entries for the various entities.
type HasStatusHistory interface {
	StatusHistory() []Status
	SetStatusHistory([]StatusArgs)
}

// Model is a database agnostic representation of an existing model.
type Model interface {
	HasAnnotations
	HasConstraints

	Cloud() string
	CloudRegion() string
	CloudCredential() string
	Tag() names.ModelTag
	Owner() names.UserTag
	Config() map[string]interface{}
	LatestToolsVersion() version.Number

	// UpdateConfig overwrites existing config values with those specified.
	UpdateConfig(map[string]interface{})

	// Blocks returns a map of block type to the message associated with that
	// block.
	Blocks() map[string]string

	Users() []User
	AddUser(UserArgs)

	Machines() []Machine
	AddMachine(MachineArgs) Machine

	Applications() []Application
	AddApplication(ApplicationArgs) Application

	Relations() []Relation
	AddRelation(RelationArgs) Relation

	Spaces() []Space
	AddSpace(SpaceArgs) Space

	LinkLayerDevices() []LinkLayerDevice
	AddLinkLayerDevice(LinkLayerDeviceArgs) LinkLayerDevice

	Subnets() []Subnet
	AddSubnet(SubnetArgs) Subnet

	IPAddresses() []IPAddress
	AddIPAddress(IPAddressArgs) IPAddress

	SSHHostKeys() []SSHHostKey
	AddSSHHostKey(SSHHostKeyArgs) SSHHostKey

	CloudImageMetadata() []CloudImageMetadata
	AddCloudImageMetadata(CloudImageMetadataArgs) CloudImageMetadata

	Actions() []Action
	AddAction(ActionArgs) Action

	Sequences() map[string]int
	SetSequence(name string, value int)

	Volumes() []Volume
	AddVolume(VolumeArgs) Volume

	Filesystems() []Filesystem
	AddFilesystem(FilesystemArgs) Filesystem

	Storages() []Storage
	AddStorage(StorageArgs) Storage

	StoragePools() []StoragePool
	AddStoragePool(StoragePoolArgs) StoragePool

	Validate() error
}

// User represents a user of the model. Users are able to connect to, and
// depending on the read only flag, modify the model.
type User interface {
	Name() names.UserTag
	DisplayName() string
	CreatedBy() names.UserTag
	DateCreated() time.Time
	LastConnection() time.Time
	Access() string
}

// Address represents an IP Address of some form.
type Address interface {
	Value() string
	Type() string
	Scope() string
	Origin() string
}

// AgentTools represent the version and related binary file
// that the machine and unit agents are using.
type AgentTools interface {
	Version() version.Binary
	URL() string
	SHA256() string
	Size() int64
}

// Machine represents an existing live machine or container running in the
// model.
type Machine interface {
	HasAnnotations
	HasConstraints
	HasStatus
	HasStatusHistory

	Id() string
	Tag() names.MachineTag
	Nonce() string
	PasswordHash() string
	Placement() string
	Series() string
	ContainerType() string
	Jobs() []string
	SupportedContainers() ([]string, bool)

	Instance() CloudInstance
	SetInstance(CloudInstanceArgs)

	// Life() string -- only transmit alive things?
	ProviderAddresses() []Address
	MachineAddresses() []Address
	SetAddresses(machine []AddressArgs, provider []AddressArgs)

	PreferredPublicAddress() Address
	PreferredPrivateAddress() Address
	SetPreferredAddresses(public AddressArgs, private AddressArgs)

	Tools() AgentTools
	SetTools(AgentToolsArgs)

	Containers() []Machine
	AddContainer(MachineArgs) Machine

	BlockDevices() []BlockDevice
	AddBlockDevice(BlockDeviceArgs) BlockDevice

	OpenedPorts() []OpenedPorts
	AddOpenedPorts(OpenedPortsArgs) OpenedPorts

	// THINKING: Validate() error to make sure the machine has
	// enough stuff set, like tools, and addresses etc.
	Validate() error

	// port docs
}

// OpenedPorts represents a collection of port ranges that are open on a
// particular subnet. OpenedPorts are always associated with a Machine.
type OpenedPorts interface {
	SubnetID() string
	OpenPorts() []PortRange
}

// PortRange represents one or more contiguous ports opened by a particular
// Unit.
type PortRange interface {
	UnitName() string
	FromPort() int
	ToPort() int
	Protocol() string
}

// CloudInstance holds information particular to a machine
// instance in a cloud.
type CloudInstance interface {
	InstanceId() string
	Status() string
	Architecture() string
	Memory() uint64
	RootDisk() uint64
	CpuCores() uint64
	CpuPower() uint64
	Tags() []string
	AvailabilityZone() string
}

// Constraints holds information about particular deployment
// constraints for entities.
type Constraints interface {
	Architecture() string
	Container() string
	CpuCores() uint64
	CpuPower() uint64
	InstanceType() string
	Memory() uint64
	RootDisk() uint64

	Spaces() []string
	Tags() []string

	VirtType() string
}

// Status represents an agent, application, or workload status.
type Status interface {
	Value() string
	Message() string
	Data() map[string]interface{}
	Updated() time.Time
}

// Application represents a deployed charm in a model.
type Application interface {
	HasAnnotations
	HasConstraints
	HasStatus
	HasStatusHistory

	Tag() names.ApplicationTag
	Name() string
	Series() string
	Subordinate() bool
	CharmURL() string
	Channel() string
	CharmModifiedVersion() int
	ForceCharm() bool
	Exposed() bool
	MinUnits() int

	Settings() map[string]interface{}

	Leader() string
	LeadershipSettings() map[string]interface{}

	MetricsCredentials() []byte
	StorageConstraints() map[string]StorageConstraint

	Units() []Unit
	AddUnit(UnitArgs) Unit

	Validate() error
}

// Unit represents an instance of an application in a model.
type Unit interface {
	HasAnnotations
	HasConstraints

	Tag() names.UnitTag
	Name() string
	Machine() names.MachineTag

	PasswordHash() string

	Principal() names.UnitTag
	Subordinates() []names.UnitTag

	MeterStatusCode() string
	MeterStatusInfo() string

	Tools() AgentTools
	SetTools(AgentToolsArgs)

	WorkloadStatus() Status
	SetWorkloadStatus(StatusArgs)

	WorkloadStatusHistory() []Status
	SetWorkloadStatusHistory([]StatusArgs)

	WorkloadVersion() string

	WorkloadVersionHistory() []Status
	SetWorkloadVersionHistory([]StatusArgs)

	AgentStatus() Status
	SetAgentStatus(StatusArgs)

	AgentStatusHistory() []Status
	SetAgentStatusHistory([]StatusArgs)

	AddPayload(PayloadArgs) Payload
	Payloads() []Payload

	Validate() error
}

// Relation represents a relationship between two applications,
// or a peer relation between different instances of an application.
type Relation interface {
	Id() int
	Key() string

	Endpoints() []Endpoint
	AddEndpoint(EndpointArgs) Endpoint
}

// Endpoint represents one end of a relation. A named endpoint provided
// by the charm that is deployed for the application.
type Endpoint interface {
	ApplicationName() string
	Name() string
	// Role, Interface, Optional, Limit, and Scope should all be available
	// through the Charm associated with the Application. There is no real need
	// for this information to be denormalised like this. However, for now,
	// since the import may well take place before the charms have been loaded
	// into the model, we'll send this information over.
	Role() string
	Interface() string
	Optional() bool
	Limit() int
	Scope() string

	// UnitCount returns the number of units the endpoint has settings for.
	UnitCount() int

	Settings(unitName string) map[string]interface{}
	SetUnitSettings(unitName string, settings map[string]interface{})
}

// Space represents a network space, which is a named collection of subnets.
type Space interface {
	Name() string
	Public() bool
	ProviderID() string
}

// LinkLayerDevice represents a link layer device.
type LinkLayerDevice interface {
	Name() string
	MTU() uint
	ProviderID() string
	MachineID() string
	Type() string
	MACAddress() string
	IsAutoStart() bool
	IsUp() bool
	ParentName() string
}

// Subnet represents a network subnet.
type Subnet interface {
	ProviderId() string
	CIDR() string
	VLANTag() int
	AvailabilityZone() string
	SpaceName() string
	AllocatableIPHigh() string
	AllocatableIPLow() string
}

// IPAddress represents an IP address.
type IPAddress interface {
	ProviderID() string
	DeviceName() string
	MachineID() string
	SubnetCIDR() string
	ConfigMethod() string
	Value() string
	DNSServers() []string
	DNSSearchDomains() []string
	GatewayAddress() string
}

// SSHHostKey represents an ssh host key.
type SSHHostKey interface {
	MachineID() string
	Keys() []string
}

// CloudImageMetadata represents an IP cloudimagemetadata.
type CloudImageMetadata interface {
	Stream() string
	Region() string
	Version() string
	Series() string
	Arch() string
	VirtType() string
	RootStorageType() string
	RootStorageSize() (uint64, bool)
	DateCreated() int64
	Source() string
	Priority() int
	ImageId() string
}

// Action represents an IP action.
type Action interface {
	Id() string
	Receiver() string
	Name() string
	Parameters() map[string]interface{}
	Enqueued() time.Time
	Started() time.Time
	Completed() time.Time
	Results() map[string]interface{}
	Status() string
	Message() string
}

// Volume represents a volume (disk, logical volume, etc.) in the model.
type Volume interface {
	HasStatus
	HasStatusHistory

	Tag() names.VolumeTag
	Storage() names.StorageTag

	Binding() (names.Tag, error)

	Provisioned() bool

	Size() uint64
	Pool() string

	HardwareID() string
	VolumeID() string
	Persistent() bool

	Attachments() []VolumeAttachment
	AddAttachment(VolumeAttachmentArgs) VolumeAttachment
}

// VolumeAttachment represents a volume attached to a machine.
type VolumeAttachment interface {
	Machine() names.MachineTag
	Provisioned() bool
	ReadOnly() bool
	DeviceName() string
	DeviceLink() string
	BusAddress() string
}

// Filesystem represents a filesystem in the model.
type Filesystem interface {
	HasStatus
	HasStatusHistory

	Tag() names.FilesystemTag
	Volume() names.VolumeTag
	Storage() names.StorageTag
	Binding() (names.Tag, error)

	Provisioned() bool

	Size() uint64
	Pool() string

	FilesystemID() string

	Attachments() []FilesystemAttachment
	AddAttachment(FilesystemAttachmentArgs) FilesystemAttachment
}

// FilesystemAttachment represents a filesystem attached to a machine.
type FilesystemAttachment interface {
	Machine() names.MachineTag
	Provisioned() bool
	MountPoint() string
	ReadOnly() bool
}

// Storage represents the state of a unit or application-wide storage instance
// in the model.
type Storage interface {
	Tag() names.StorageTag
	Kind() string
	// Owner returns the tag of the application or unit that owns this storage
	// instance.
	Owner() (names.Tag, error)
	Name() string

	Attachments() []names.UnitTag

	Validate() error
}

// StoragePool represents a named storage pool and its settings.
type StoragePool interface {
	Name() string
	Provider() string
	Attributes() map[string]interface{}
}

// StorageConstraint repressents the user-specified constraints for
// provisioning storage instances for an application unit.
type StorageConstraint interface {
	// Pool is the name of the storage pool from which to provision the
	// storage instances.
	Pool() string
	// Size is the required size of the storage instances, in MiB.
	Size() uint64
	// Count is the required number of storage instances.
	Count() uint64
}
