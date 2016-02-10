// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"time"

	"github.com/juju/names"

	"github.com/juju/juju/version"
)

// Model is a database agnostic representation of an existing model.
type Model interface {
	Tag() names.ModelTag
	Owner() names.UserTag
	Config() map[string]interface{}
	LatestToolsVersion() version.Number

	Users() []User
	AddUser(UserArgs)

	Machines() []Machine
	AddMachine(MachineArgs) Machine

	Services() []Service
	AddService(ServiceArgs) Service

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
	ReadOnly() bool
}

// Address represents an IP Address of some form.
type Address interface {
	Value() string
	Type() string
	NetworkName() string
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

	Status() Status
	SetStatus(StatusArgs)

	// StatusHistory() []Status

	// TODO:
	// status history
	// Storage

	NetworkPorts() []NetworkPorts
	AddNetworkPorts(NetworkPortsArgs) NetworkPorts

	// THINKING: Validate() error to make sure the machine has
	// enough stuff set, like tools, and addresses etc.
	Validate() error

	// status
	// constraints
	// requested networks
	// annotations
	// reboot doc
	// block devices
	// network interfaces
	// port docs
	// machine filesystems
}

type NetworkPorts interface {
	NetworkName() string
	OpenPorts() []PortRange
}

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

// Status represents an agent, service, or workload status.
type Status interface {
	Value() string
	Message() string
	Data() map[string]interface{}
	Updated() time.Time
}

// Service represents a deployed charm in a model.
type Service interface {
	Tag() names.ServiceTag
	Name() string
	Series() string
	Subordinate() bool
	CharmURL() string
	ForceCharm() bool
	Exposed() bool
	MinUnits() int

	Settings() map[string]interface{}
	SettingsRefCount() int
	LeadershipSettings() map[string]interface{}

	Status() Status
	SetStatus(StatusArgs)

	Units() []Unit
	AddUnit(UnitArgs) Unit

	Validate() error
}

// Unit represents an instance of a service in a model.
type Unit interface {
	Tag() names.UnitTag
	Name() string
	Machine() names.MachineTag

	PasswordHash() string

	Principal() names.UnitTag
	Subordinates() []names.UnitTag

	// TODO: opened ports
	// TODO: meter status
	// TODO: storage

	Tools() AgentTools
	SetTools(AgentToolsArgs)

	WorkloadStatus() Status
	SetWorkloadStatus(StatusArgs)

	AgentStatus() Status
	SetAgentStatus(StatusArgs)

	Validate() error
}
