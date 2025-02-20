// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/resource"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application/architecture"
	domaincharm "github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/ipaddress"
	"github.com/juju/juju/domain/linklayerdevice"
	internalcharm "github.com/juju/juju/internal/charm"
	charmresource "github.com/juju/juju/internal/charm/resource"
)

// AddApplicationArg contains parameters for saving an application to state.
type AddApplicationArg struct {
	// Charm is the charm to add to the application. This is required to
	// be able to add the application.
	Charm domaincharm.Charm
	// CharmDownloadInfo contains the download information for the charm.
	// This information is used to download the charm from the charm store if
	// required.
	CharmDownloadInfo *domaincharm.DownloadInfo
	// Platform contains the platform information for the application. The
	// operating system and architecture.
	Platform Platform
	// Channel contains the channel information for the application. The track,
	// risk and branch of the charm when it was downloaded from the charm store.
	Channel *Channel
	// Resources defines the list of resources to add to an application.
	// They should match all the resources defined in the Charm.
	Resources []AddApplicationResourceArg
	// PendingResources are the uuids of resources added before the
	// application is created.
	PendingResources []resource.UUID
	// Storage defines the list of storage directives to add to an application.
	// The Name values should match the storage defined in the Charm.
	Storage []AddApplicationStorageArg
	// Config contains the configuration for the application, overlaid on top
	// of the charm's default configuration.
	Config map[string]ApplicationConfig
	// Settings contains the settings for the application. This includes the
	// trust setting.
	Settings ApplicationSettings
	// Scale contains the scale information for the application.
	Scale int
	// Status contains the status of the application.
	Status *StatusInfo[WorkloadStatusType]
}

// AddApplicationResourceArg defines the arguments required to add a resource to an application.
type AddApplicationResourceArg struct {
	Name     string
	Revision *int
	Origin   charmresource.Origin
}

// AddApplicationStorageArg defines the arguments required to add storage to an application.
type AddApplicationStorageArg struct {
	Name  string
	Pool  string
	Size  uint64
	Count uint64
}

// Channel represents the channel of a application charm.
// Do not confuse this with a channel that is in the manifest file found
// in the charm package. They represent different concepts, yet hold the
// same data.
type Channel struct {
	Track  string
	Risk   ChannelRisk
	Branch string
}

// ChannelRisk describes the type of risk in a current channel.
type ChannelRisk string

const (
	RiskStable    ChannelRisk = "stable"
	RiskCandidate ChannelRisk = "candidate"
	RiskBeta      ChannelRisk = "beta"
	RiskEdge      ChannelRisk = "edge"
)

// OSType represents the type of an application's OS.
type OSType int

const (
	Ubuntu OSType = iota
)

// Platform contains parameters for an application's platform.
type Platform struct {
	Channel      string
	OSType       OSType
	Architecture Architecture
}

// Architecture represents the architecture of a application charm.
type Architecture = architecture.Architecture

// ScaleState describes the scale status of a k8s application.
type ScaleState struct {
	Scaling     bool
	Scale       int
	ScaleTarget int
}

// CloudService contains parameters for an application's cloud service.
type CloudService struct {
	ProviderID string
	Address    *ServiceAddress
}

// ServiceAddress contains parameters for a cloud service address.
// This may be from a load balancer, or cluster service etc.
type ServiceAddress struct {
	Value       string
	AddressType ipaddress.AddressType
	Scope       ipaddress.Scope
	Origin      ipaddress.Origin
	ConfigType  ipaddress.ConfigType
}

// Origin contains parameters for an application's origin.
type Origin struct {
	ID       string
	Channel  Channel
	Platform Platform
	Revision int
}

const (
	// HashAlgorithmSHA256 is the sha256 hash algorithm.
	// Currently it's the only one.
	HashAlgorithmSHA256 = 0
)

// PasswordInfo contains password parameters.
type PasswordInfo struct {
	PasswordHash  string
	HashAlgorithm int
}

// CloudContainer contains parameters for a unit's cloud container.
type CloudContainer struct {
	ProviderID string
	Address    *ContainerAddress
	Ports      *[]string
}

// ContainerDevice is the placeholder link layer device
// used to tie the cloud container IP address to the container.
type ContainerDevice struct {
	Name              string
	DeviceTypeID      linklayerdevice.DeviceType
	VirtualPortTypeID linklayerdevice.VirtualPortType
}

// ContainerAddress contains parameters for a cloud container address.
// Device is an attribute of address rather than cloud container
// since it's a placeholder used to tie the address to the
// cloud container and is only needed if the address exists.
type ContainerAddress struct {
	Device      ContainerDevice
	Value       string
	AddressType ipaddress.AddressType
	Scope       ipaddress.Scope
	Origin      ipaddress.Origin
	ConfigType  ipaddress.ConfigType
}

// AddUnitArg contains parameters for adding a unit to state.
type AddUnitArg struct {
	UnitName coreunit.Name
	UnitStatusArg
}

// InsertUnitArg is used to insert a fully populated unit.
// Used by import and when registering a CAAS unit.
type InsertUnitArg struct {
	UnitName       coreunit.Name
	CloudContainer *CloudContainer
	Password       *PasswordInfo
	UnitStatusArg
}

// RegisterCAASUnitArg contains parameters for introducing
// a k8s unit representing a new pod to the model.
type RegisterCAASUnitArg struct {
	UnitName     coreunit.Name
	PasswordHash string
	ProviderID   string
	Address      *string
	Ports        *[]string
	OrderedScale bool
	OrderedId    int
}

// UnitStatusArg contains parameters for updating a unit status in state.
type UnitStatusArg struct {
	AgentStatus    *StatusInfo[UnitAgentStatusType]
	WorkloadStatus *StatusInfo[WorkloadStatusType]
}

// UpdateCAASUnitParams contains parameters for updating a CAAS unit.
type UpdateCAASUnitParams struct {
	ProviderID           *string
	Address              *string
	Ports                *[]string
	AgentStatus          *StatusInfo[UnitAgentStatusType]
	WorkloadStatus       *StatusInfo[WorkloadStatusType]
	CloudContainerStatus *StatusInfo[CloudContainerStatusType]
}

// CloudContainerParams contains parameters for a unit cloud container.
type CloudContainerParams struct {
	ProviderID    string
	Address       *network.SpaceAddress
	AddressOrigin *network.Origin
	Ports         *[]string
}

// CharmDownloadInfo contains parameters for downloading a charm.
type CharmDownloadInfo struct {
	CharmUUID    charm.ID
	Name         string
	SHA256       string
	DownloadInfo domaincharm.DownloadInfo
}

// ResolveCharmDownload contains parameters for resolving a charm download.
type ResolveCharmDownload struct {
	CharmUUID charm.ID
	SHA256    string
	SHA384    string
	Path      string
	Size      int64
}

// ResolveControllerCharmDownload contains parameters for resolving a charm
// download.
type ResolveControllerCharmDownload struct {
	SHA256 string
	SHA384 string
	Path   string
	Size   int64
}

// ResolvedCharmDownload contains parameters for a resolved charm download.
type ResolvedCharmDownload struct {
	// Actions is the actions that the charm supports.
	// Deprecated: should be filled in by the charm store.
	Actions         domaincharm.Actions
	LXDProfile      []byte
	ObjectStoreUUID objectstore.UUID
	ArchivePath     string
}

// ResolvedControllerCharmDownload contains parameters for a resolved controller
// charm download.
type ResolvedControllerCharmDownload struct {
	Charm           internalcharm.Charm
	ArchivePath     string
	ObjectStoreUUID objectstore.UUID
}

// RevisionUpdaterApplication is responsible for updating the revision of an
// application.
type RevisionUpdaterApplication struct {
	Name         string
	CharmLocator domaincharm.CharmLocator
	Origin       Origin
	NumUnits     int
}

// ApplicationConfig contains the configuration for the application config.
// This will include the charm config type.
type ApplicationConfig struct {
	// Type dictates the type of the config value. The value is derived from
	// the charm config.
	Type  domaincharm.OptionType
	Value any
}

// ApplicationSettings contains the settings for an application.
type ApplicationSettings struct {
	Trust bool
}

// Constraints represents the application constraints.
// All fields of this struct are taken from core/constraints except for the
// spaces, which are modeled with SpaceConstraint to take into account the
// negative constraint (excluded field in the db).
type Constraints struct {
	// Arch, if not nil or empty, indicates that a machine must run the named
	// architecture.
	Arch *string

	// Container, if not nil, indicates that a machine must be the specified container type.
	Container *instance.ContainerType

	// CpuCores, if not nil, indicates that a machine must have at least that
	// number of effective cores available.
	CpuCores *uint64

	// CpuPower, if not nil, indicates that a machine must have at least that
	// amount of CPU power available, where 100 CpuPower is considered to be
	// equivalent to 1 Amazon ECU (or, roughly, a single 2007-era Xeon).
	CpuPower *uint64

	// Mem, if not nil, indicates that a machine must have at least that many
	// megabytes of RAM.
	Mem *uint64

	// RootDisk, if not nil, indicates that a machine must have at least
	// that many megabytes of disk space available in the root disk. In
	// providers where the root disk is configurable at instance startup
	// time, an instance with the specified amount of disk space in the OS
	// disk might be requested.
	RootDisk *uint64

	// RootDiskSource, if specified, determines what storage the root
	// disk should be allocated from. This will be provider specific -
	// in the case of vSphere it identifies the datastore the root
	// disk file should be created in.
	RootDiskSource *string

	// Tags, if not nil, indicates tags that the machine must have applied to it.
	// An empty list is treated the same as a nil (unspecified) list, except an
	// empty list will override any default tags, where a nil list will not.
	Tags *[]string

	// InstanceRole, if not nil, indicates that the specified role/profile for
	// the given cloud should be used. Only valid for clouds which support
	// instance roles. Currently only for AWS with instance-profiles
	InstanceRole *string

	// InstanceType, if not nil, indicates that the specified cloud instance type
	// be used. Only valid for clouds which support instance types.
	InstanceType *string

	// Spaces, if not nil, holds a list of juju network spaces that
	// should be available (or not) on the machine.
	Spaces *[]SpaceConstraint

	// VirtType, if not nil or empty, indicates that a machine must run the named
	// virtual type. Only valid for clouds with multi-hypervisor support.
	VirtType *string

	// Zones, if not nil, holds a list of availability zones limiting where
	// the machine can be located.
	Zones *[]string

	// AllocatePublicIP, if nil or true, signals that machines should be
	// created with a public IP address instead of a cloud local one.
	// The default behaviour if the value is not specified is to allocate
	// a public IP so that public cloud behaviour works out of the box.
	AllocatePublicIP *bool

	// ImageID, if not nil, indicates that a machine must use the specified
	// image. This is provider specific, and for the moment is only
	// implemented on MAAS clouds.
	ImageID *string
}

// SpaceConstraint represents a single space constraint for an application.
type SpaceConstraint struct {
	// Excluded indicates that this space should not be available to the
	// machine.
	Exclude bool

	// SpaceName is the name of the space.
	SpaceName string
}

// FromCoreConstraints is responsible for converting a [constraints.Value] to a
// [Constraints] object.
func FromCoreConstraints(coreCons constraints.Value) Constraints {
	rval := Constraints{
		Arch:             coreCons.Arch,
		Container:        coreCons.Container,
		CpuCores:         coreCons.CpuCores,
		CpuPower:         coreCons.CpuPower,
		Mem:              coreCons.Mem,
		RootDisk:         coreCons.RootDisk,
		RootDiskSource:   coreCons.RootDiskSource,
		Tags:             coreCons.Tags,
		InstanceRole:     coreCons.InstanceRole,
		InstanceType:     coreCons.InstanceType,
		VirtType:         coreCons.VirtType,
		Zones:            coreCons.Zones,
		AllocatePublicIP: coreCons.AllocatePublicIP,
		ImageID:          coreCons.ImageID,
	}

	if coreCons.Spaces == nil {
		return rval
	}

	spaces := make([]SpaceConstraint, 0, len(*coreCons.Spaces))
	// Set included spaces
	for _, incSpace := range coreCons.IncludeSpaces() {
		spaces = append(spaces, SpaceConstraint{
			SpaceName: incSpace,
			Exclude:   false,
		})
	}

	// Set excluded spaces
	for _, exSpace := range coreCons.ExcludeSpaces() {
		spaces = append(spaces, SpaceConstraint{
			SpaceName: exSpace,
			Exclude:   true,
		})
	}
	rval.Spaces = &spaces

	return rval
}

// ToCoreConstraints is responsible for converting a [Constraints] value to a
// [constraints.Value].
func ToCoreConstraints(cons Constraints) constraints.Value {
	rval := constraints.Value{
		Arch:             cons.Arch,
		Container:        cons.Container,
		CpuCores:         cons.CpuCores,
		CpuPower:         cons.CpuPower,
		Mem:              cons.Mem,
		RootDisk:         cons.RootDisk,
		RootDiskSource:   cons.RootDiskSource,
		Tags:             cons.Tags,
		InstanceRole:     cons.InstanceRole,
		InstanceType:     cons.InstanceType,
		VirtType:         cons.VirtType,
		Zones:            cons.Zones,
		AllocatePublicIP: cons.AllocatePublicIP,
		ImageID:          cons.ImageID,
	}

	if cons.Spaces == nil {
		return rval
	}

	for _, space := range *cons.Spaces {
		rval.AddSpace(space.SpaceName, space.Exclude)
	}

	return rval
}
