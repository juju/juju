// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/collections/set"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/resource"
	corestorage "github.com/juju/juju/core/storage"
	coreunit "github.com/juju/juju/core/unit"
	domaincharm "github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/domain/ipaddress"
	"github.com/juju/juju/domain/life"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/status"
	domainstorage "github.com/juju/juju/domain/storage"
	internalcharm "github.com/juju/juju/internal/charm"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/storage"
)

// BaseAddApplicationArg contains parameters for saving an application to state
// that are common to all application types (IAAS and CAAS).
type BaseAddApplicationArg struct {
	// Charm is the charm to add to the application. This is required to
	// be able to add the application.
	Charm domaincharm.Charm
	// CharmDownloadInfo contains the download information for the charm.
	// This information is used to download the charm from the charm store if
	// required.
	CharmDownloadInfo *domaincharm.DownloadInfo
	// Platform contains the platform information for the application. The
	// operating system and architecture.
	Platform deployment.Platform
	// Channel contains the channel information for the application. The track,
	// risk and branch of the charm when it was downloaded from the charm store.
	Channel *deployment.Channel
	// Resources defines the list of resources to add to an application.
	// They should match all the resources defined in the Charm.
	Resources []AddApplicationResourceArg
	// PendingResources are the uuids of resources added before the
	// application is created.
	PendingResources []resource.UUID
	// Storage defines the list of storage directives to add to an application.
	// The Name values should match the storage defined in the Charm.
	Storage []ApplicationStorageArg
	// Config contains the configuration for the application, overlaid on top
	// of the charm's default configuration.
	Config map[string]ApplicationConfig
	// Settings contains the settings for the application. This includes the
	// trust setting.
	Settings ApplicationSettings
	// Status contains the status of the application.
	Status *status.StatusInfo[status.WorkloadStatusType]
	// StoragePoolKind holds a mapping of the kind of storage supported
	// by the named storage pool / provider type.
	StoragePoolKind map[string]storage.StorageKind
	// EndpointBindings is a map to bind application endpoint by name to a
	// specific space. The default space is referenced by an empty key, if any.
	EndpointBindings map[string]network.SpaceName
	// Devices contains the device constraints for the application.
	Devices map[string]devices.Constraints
	// IsController indicates whether the application is a controller
	// application.
	IsController bool
}

// AddIAASApplicationArg contains parameters for saving an IAAS application to
// state.
// This is split out to allow for specific IAAS application properties to be
// added *just* for IAAS applications.
type AddIAASApplicationArg struct {
	BaseAddApplicationArg
}

// AddCAASApplicationArg contains parameters for saving a CAAS application to
// state.
// This is split out to allow for specific CAAS application properties to be
// added *just* for CAAS applications.
type AddCAASApplicationArg struct {
	BaseAddApplicationArg
	// Scale contains the scale information for the application.
	Scale int
}

// AddApplicationResourceArg defines the arguments required to add a resource to
// an application.
type AddApplicationResourceArg struct {
	Name     string
	Revision *int
	Origin   charmresource.Origin
}

// ApplicationStorageArg describes details of storage for an application.
type ApplicationStorageArg struct {
	Name           corestorage.Name
	PoolNameOrType string
	Size           uint64
	Count          uint64
}

// CharmOrigin represents the origin of a charm.
type CharmOrigin struct {
	Name               string
	Source             domaincharm.CharmSource
	Channel            *deployment.Channel
	Platform           deployment.Platform
	Revision           int
	Hash               string
	CharmhubIdentifier string
}

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
	Device      CloudServiceDevice
	Value       string
	AddressType ipaddress.AddressType
	Scope       ipaddress.Scope
	Origin      ipaddress.Origin
	ConfigType  ipaddress.ConfigType
}

// CloudServiceDevice is the placeholder link layer device
// used to tie the cloud service IP address to the application.
type CloudServiceDevice struct {
	Name              string
	DeviceTypeID      domainnetwork.DeviceType
	VirtualPortTypeID domainnetwork.VirtualPortType
}

// Origin contains parameters for an application's origin.
type Origin struct {
	ID       string
	Channel  deployment.Channel
	Platform deployment.Platform
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
	DeviceTypeID      domainnetwork.DeviceType
	VirtualPortTypeID domainnetwork.VirtualPortType
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
	UnitStatusArg
	Constraints constraints.Constraints
	Placement   deployment.Placement
}

// AddIAASUnitArg contains parameters for adding a IAAS unit to state.
type AddIAASUnitArg struct {
	AddUnitArg
	Platform deployment.Platform
	Nonce    *string
}

// UnitStorageDirectiveArg describes the arguments required for making storage
// directives on a unit.
type UnitStorageDirectiveArg struct {
	// Count is the number of storage instances to create from this directive.
	Count uint64

	// Size is the desired size of the storage directive in MiB.
	Size uint64

	// StorageName is the name of the storage associated with the charm storage
	// definition.
	StorageName string

	// StoragePoolUUID is the UUID of the storage pool to use when creating
	// storage instances from this directive. This value is optional and if nil
	// it is expected that [UnitStorageDirective.StorageType] is set.
	StoragePoolUUID *domainstorage.StoragePoolUUID

	// Storage provider defines the unique name of the provider to use when
	// provisioning storage from this directive.
	StorageProvider *string
}

// InsertUnitArg is used to insert a fully populated unit.
// Used by import and when registering a CAAS unit.
type InsertUnitArg struct {
	UnitName          coreunit.Name
	CloudContainer    *CloudContainer
	Password          *PasswordInfo
	Constraints       constraints.Constraints
	Placement         deployment.Placement
	Storage           []ApplicationStorageArg
	StorageDirectives []UnitStorageDirectiveArg
	StoragePoolKind   map[string]storage.StorageKind
	UnitStatusArg
}

// InsertIAASUnitArg contains parameters for inserting an IAAS unit.
type InsertIAASUnitArg struct {
	InsertUnitArg
	Platform deployment.Platform
	Nonce    *string
}

// RegisterCAASUnitParams contains parameters for introducing
// a k8s unit representing a new pod to the model.
type RegisterCAASUnitParams struct {
	ApplicationName string
	ProviderID      string
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
	// TODO(storage) - this needs to be wired through to the register CAAS unit workflow.
	// ObservedAttachedVolumeIDs is the filesystem attachments observed to be attached
	// by the infrastructure, used to map existing attachments.
	ObservedAttachedVolumeIDs []string
}

// UnitStatusArg contains parameters for updating a unit status in state.
type UnitStatusArg struct {
	AgentStatus    *status.StatusInfo[status.UnitAgentStatusType]
	WorkloadStatus *status.StatusInfo[status.WorkloadStatusType]
}

type SubordinateUnitArg struct {
	UnitStatusArg
	SubordinateAppID  application.ID
	PrincipalUnitName coreunit.Name
}

// UpdateCAASUnitParams contains parameters for updating a CAAS unit.
type UpdateCAASUnitParams struct {
	ProviderID     *string
	Address        *string
	Ports          *[]string
	AgentStatus    *status.StatusInfo[status.UnitAgentStatusType]
	WorkloadStatus *status.StatusInfo[status.WorkloadStatusType]
	K8sPodStatus   *status.StatusInfo[status.K8sPodStatusType]
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

// UpdateApplicationSettingsArg is the argument used to update an application's
// settings
type UpdateApplicationSettingsArg struct {
	Trust *bool
}

// ExposedEndpoint encapsulates the expose-related details of a particular
// application endpoint with respect to the sources (CIDRs or space IDs) that
// should be able to access the ports opened by the application charm for an
// endpoint.
type ExposedEndpoint struct {
	// A list of spaces that should be able to reach the opened ports
	// for an exposed application's endpoint.
	ExposeToSpaceIDs set.Strings
	// A list of CIDRs that should be able to reach the opened ports
	// for an exposed application's endpoint.
	ExposeToCIDRs set.Strings
}

// ExportApplication contains parameters for exporting an application.
type ExportApplication struct {
	UUID                 application.ID
	Name                 string
	CharmUUID            charm.ID
	Life                 life.Life
	Subordinate          bool
	CharmModifiedVersion int
	CharmUpgradeOnError  bool
	CharmLocator         domaincharm.CharmLocator
	K8sServiceProviderID *string
	EndpointBindings     map[string]network.SpaceUUID
}

// ExportUnit contains parameters for exporting a unit.
type ExportUnit struct {
	UUID      coreunit.UUID
	Name      coreunit.Name
	Machine   machine.Name
	Principal coreunit.Name
}

// ImportUnitArg is used to import a unit.
type ImportUnitArg struct {
	UnitName        coreunit.Name
	CloudContainer  *CloudContainer
	Password        *PasswordInfo
	Constraints     constraints.Constraints
	Machine         machine.Name
	Storage         []ApplicationStorageArg
	StoragePoolKind map[string]storage.StorageKind
	// Principal contains the name of the units principal unit. If the unit is
	// not a subordinate, this field is empty.
	Principal coreunit.Name
	UnitStatusArg
}

// UnitAttributes contains parameters for exporting a unit.
type UnitAttributes struct {
	Life        life.Life
	ProviderID  string
	ResolveMode string
}

// K8sPodInfo contains information about a unit's k8s pod.
type K8sPodInfo struct {
	ProviderID network.Id
	Address    string
	Ports      []string
}

// InsertApplicationArgs contains arguments for importing an application to the
// model.
type InsertApplicationArgs struct {
	// Charm is the charm to add to the application. This is required to
	// be able to add the application.
	Charm domaincharm.Charm
	// Platform contains the platform information for the application. The
	// operating system and architecture.
	Platform deployment.Platform
	// Channel contains the channel information for the application. The track,
	// risk and branch of the charm when it was downloaded from the charm store.
	Channel *deployment.Channel
	// Resources defines the list of resources to add to an application.
	// They should match all the resources defined in the Charm.
	Resources []AddApplicationResourceArg
	// Config contains the configuration for the application, overlaid on top
	// of the charm's default configuration.
	Config map[string]ApplicationConfig
	// Settings contains the settings for the application. This includes the
	// trust setting.
	Settings ApplicationSettings
	// Scale contains the scale information for the application.
	Scale int
	// StoragePoolKind holds a mapping of the kind of storage supported
	// by the named storage pool / provider type.
	StoragePoolKind map[string]storage.StorageKind
	// EndpointBindings is a map to bind application endpoint by name to a
	// specific space. The default space is referenced by an empty key, if any.
	EndpointBindings map[string]network.SpaceName
	// PeerRelations is a map of peer relation endpoint to relation id.
	PeerRelations map[string]int
}

// UpdateCharmParams contains the parameters for updating
// an application's charm and storage.
type UpdateCharmParams struct {
	// Charm is the new charm to use for the application. New units
	// will be started with this charm, and existing units will be
	// upgraded to use it.
	Charm internalcharm.Charm

	// Storage contains the storage directives to add or update when
	// upgrading the charm.
	//
	// Any existing storage instances for the named stores will be
	// unaffected; the storage directives will only be used for
	// provisioning new storage instances.
	Storage map[string]storage.Directive

	// CharmUpgradeOnError indicates whether the charm must be upgraded
	// even when on error.
	CharmUpgradeOnError bool

	// EndpointBindings is an operator-defined map of endpoint names to
	// space names that should be merged with any existing bindings.
	EndpointBindings map[string]network.SpaceName
}
