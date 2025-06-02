// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"
	"time"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	corerelation "github.com/juju/juju/core/relation"
	corestorage "github.com/juju/juju/core/storage"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/life"
)

// These structs represent the persistent block device entity schema in the database.

type modelInfo struct {
	ModelType string `db:"type"`
}

type KeyValue struct {
	Key   string `db:"key"`
	Value string `db:"value"`
}

// applicationID is used to get the ID of an application.
type applicationID struct {
	ID coreapplication.ID `db:"uuid"`
}

// applicationIDAndName is used to get the ID and name of an application.
type applicationIDAndName struct {
	ID   coreapplication.ID `db:"uuid"`
	Name string             `db:"name"`
}

// applicationIDAndLife is used to get the ID and life of an application.
type applicationIDAndLife struct {
	ID     coreapplication.ID `db:"uuid"`
	LifeID life.Life          `db:"life_id"`
}

type applicationChannel struct {
	ApplicationID coreapplication.ID `db:"application_uuid"`
	Track         string             `db:"track"`
	Risk          string             `db:"risk"`
	Branch        string             `db:"branch"`
}

type applicationPlatform struct {
	ApplicationID  coreapplication.ID `db:"application_uuid"`
	OSTypeID       int                `db:"os_id"`
	Channel        string             `db:"channel"`
	ArchitectureID int                `db:"architecture_id"`
}

// applicationName is used to get the name of an application.
type applicationName struct {
	Name string `db:"name"`
}

type applicationDetails struct {
	UUID      coreapplication.ID `db:"uuid"`
	Name      string             `db:"name"`
	CharmUUID corecharm.ID       `db:"charm_uuid"`
	LifeID    life.Life          `db:"life_id"`
	SpaceUUID string             `db:"space_uuid"`
}

type applicationScale struct {
	ApplicationID coreapplication.ID `db:"application_uuid"`
	Scaling       bool               `db:"scaling"`
	Scale         int                `db:"scale"`
	ScaleTarget   int                `db:"scale_target"`
}

type architectureMap struct {
	ID   int    `db:"id"`
	Name string `db:"name"`
}

type unitAgentVersion struct {
	UnitUUID       string `db:"unit_uuid"`
	Version        string `db:"version"`
	ArchitectureID int    `db:"architecture_id"`
}

type unitUUID struct {
	UnitUUID coreunit.UUID `db:"uuid"`
}

type unitName struct {
	Name coreunit.Name `db:"name"`
}

type unitDetails struct {
	UnitUUID                coreunit.UUID      `db:"uuid"`
	CharmUUID               corecharm.ID       `db:"charm_uuid"`
	NetNodeID               string             `db:"net_node_uuid"`
	Name                    coreunit.Name      `db:"name"`
	ApplicationID           coreapplication.ID `db:"application_uuid"`
	LifeID                  life.Life          `db:"life_id"`
	PasswordHash            sql.NullString     `db:"password_hash"`
	PasswordHashAlgorithmID sql.NullInt16      `db:"password_hash_algorithm_id"`
}

type unitAttributes struct {
	UnitUUID    coreunit.UUID  `db:"uuid"`
	Name        coreunit.Name  `db:"name"`
	LifeID      life.Life      `db:"life_id"`
	ResolveMode sql.NullInt16  `db:"resolve_mode_id"`
	ProviderID  sql.NullString `db:"provider_id"`
}

type unitPassword struct {
	UnitUUID                coreunit.UUID `db:"uuid"`
	PasswordHash            string        `db:"password_hash"`
	PasswordHashAlgorithmID int           `db:"password_hash_algorithm_id"`
}

type unitUUIDs []coreunit.UUID

type minimalUnit struct {
	UUID      coreunit.UUID `db:"uuid"`
	NetNodeID string        `db:"net_node_uuid"`
	Name      coreunit.Name `db:"name"`
	LifeID    life.Life     `db:"life_id"`
}

type unitCount struct {
	UnitLifeID        life.Life `db:"unit_life_id"`
	ApplicationLifeID life.Life `db:"app_life_id"`
	Count             int       `db:"count"`
}

type unitStatusInfo struct {
	UnitUUID  coreunit.UUID `db:"unit_uuid"`
	StatusID  int           `db:"status_id"`
	Message   string        `db:"message"`
	Data      []byte        `db:"data"`
	UpdatedAt *time.Time    `db:"updated_at"`
}

type cloudContainer struct {
	UnitUUID   coreunit.UUID `db:"unit_uuid"`
	ProviderID string        `db:"provider_id"`
}

type cloudService struct {
	UUID            string             `db:"uuid"`
	ApplicationUUID coreapplication.ID `db:"application_uuid"`
	NetNodeUUID     string             `db:"net_node_uuid"`
	ProviderID      string             `db:"provider_id"`
}

type cloudServiceDevice struct {
	UUID              string `db:"uuid"`
	Name              string `db:"name"`
	NetNodeID         string `db:"net_node_uuid"`
	DeviceTypeID      int    `db:"device_type_id"`
	VirtualPortTypeID int    `db:"virtual_port_type_id"`
}

type applicationCharmUUID struct {
	CharmUUID corecharm.ID `db:"charm_uuid"`
}

type cloudContainerDevice struct {
	UUID              string `db:"uuid"`
	Name              string `db:"name"`
	NetNodeID         string `db:"net_node_uuid"`
	DeviceTypeID      int    `db:"device_type_id"`
	VirtualPortTypeID int    `db:"virtual_port_type_id"`
}

type k8sPodPort struct {
	Port string `db:"port"`
}

type unitK8sPodPort struct {
	UnitUUID coreunit.UUID `db:"unit_uuid"`
	Port     string        `db:"port"`
}

type unitK8sPodInfo struct {
	ProviderID sql.Null[network.Id] `db:"provider_id"`
	Address    sql.Null[string]     `db:"address"`
}

type ipAddress struct {
	AddressUUID  string `db:"uuid"`
	Value        string `db:"address_value"`
	NetNodeUUID  string `db:"net_node_uuid"`
	ConfigTypeID int    `db:"config_type_id"`
	TypeID       int    `db:"type_id"`
	OriginID     int    `db:"origin_id"`
	ScopeID      int    `db:"scope_id"`
	DeviceID     string `db:"device_uuid"`
}

type spaceAddress struct {
	Value        string         `db:"address_value"`
	ConfigTypeID int            `db:"config_type_id"`
	TypeID       int            `db:"type_id"`
	OriginID     int            `db:"origin_id"`
	ScopeID      int            `db:"scope_id"`
	DeviceID     string         `db:"device_uuid"`
	SpaceUUID    sql.NullString `db:"space_uuid"`
}

// These structs represent the persistent charm schema in the database.

// charmID represents a single charm row from the charm table, that only
// contains the charm ID.
type charmID struct {
	UUID corecharm.ID `db:"uuid"`
}

type charmUUID struct {
	UUID corecharm.ID `db:"charm_uuid"`
}

// charmName is used to pass the name to the query.
type charmName struct {
	Name string `db:"name"`
}

// charmReferenceNameRevisionSource is used to pass the reference name,
// revision and source to the query.
type charmReferenceNameRevisionSource struct {
	ReferenceName string `db:"reference_name"`
	Revision      int    `db:"revision"`
	Source        int    `db:"source_id"`
}

// charmAvailable is used to get the available application.UnitWorkloadStatusType a charm.
type charmAvailable struct {
	Available bool `db:"available"`
}

// charmSubordinate is used to get the subordinate application.UnitWorkloadStatusType a charm.
type charmSubordinate struct {
	Subordinate bool `db:"subordinate"`
}

// charmHash is used to get the hash of a charm.
type charmHash struct {
	HashKindID int    `db:"hash_kind_id"`
	Hash       string `db:"hash"`
}

// setCharmHash is used to set the hash of a charm.
type setCharmHash struct {
	CharmUUID  string `db:"charm_uuid"`
	HashKindID int    `db:"hash_kind_id"`
	Hash       string `db:"hash"`
}

type charmNameAndArchitecture struct {
	Name           string `db:"name"`
	ArchitectureID int    `db:"architecture_id"`
}

type charmState struct {
	ReferenceName   string         `db:"reference_name"`
	Revision        int            `db:"revision"`
	ArchivePath     string         `db:"archive_path"`
	ObjectStoreUUID sql.NullString `db:"object_store_uuid"`
	Available       bool           `db:"available"`
	SourceID        int            `db:"source_id"`
	ArchitectureID  sql.NullInt64  `db:"architecture_id"`
	Version         string         `db:"version"`
}

// setCharmState is used to set the charm.
type setCharmState struct {
	UUID            string         `db:"uuid"`
	ReferenceName   string         `db:"reference_name"`
	Revision        int            `db:"revision"`
	ArchivePath     string         `db:"archive_path"`
	ObjectStoreUUID sql.NullString `db:"object_store_uuid"`
	Available       bool           `db:"available"`
	SourceID        int            `db:"source_id"`
	ArchitectureID  sql.NullInt64  `db:"architecture_id"`
	Version         string         `db:"version"`
	LXDProfile      []byte         `db:"lxd_profile"`
}

// resolveCharmState is used to resolve the charm state. This will make the
// charm available if it is not already.
type resolveCharmState struct {
	ObjectStoreUUID string `db:"object_store_uuid"`
	ArchivePath     string `db:"archive_path"`
	LXDProfile      []byte `db:"lxd_profile"`
}

// charmDownloadInfo is used to get the download info of a charm.
type charmDownloadInfo struct {
	Provenance         string `db:"name"`
	CharmhubIdentifier string `db:"charmhub_identifier"`
	DownloadURL        string `db:"download_url"`
	DownloadSize       int64  `db:"download_size"`
}

// setCharmDownloadInfo is used to set the download info of a charm.
type setCharmDownloadInfo struct {
	CharmUUID          string `db:"charm_uuid"`
	ProvenanceID       int    `db:"provenance_id"`
	CharmhubIdentifier string `db:"charmhub_identifier"`
	DownloadURL        string `db:"download_url"`
	DownloadSize       int64  `db:"download_size"`
}

// charmMetadata is used to get the metadata of a charm.
type charmMetadata struct {
	Name           string `db:"name"`
	Summary        string `db:"summary"`
	Description    string `db:"description"`
	Subordinate    bool   `db:"subordinate"`
	MinJujuVersion string `db:"min_juju_version"`
	Assumes        []byte `db:"assumes"`
	RunAs          string `db:"run_as"`
}

// setCharmMetadata is used to set the metadata of a charm.
// This includes the setting of the LXD profile.
type setCharmMetadata struct {
	CharmUUID      string `db:"charm_uuid"`
	Name           string `db:"name"`
	Summary        string `db:"summary"`
	Description    string `db:"description"`
	Subordinate    bool   `db:"subordinate"`
	MinJujuVersion string `db:"min_juju_version"`
	Assumes        []byte `db:"assumes"`
	RunAsID        int    `db:"run_as_id"`
}

// charmTag is used to get the tags of a charm.
// This is a row based struct that is normalised form of an array of strings.
type charmTag struct {
	CharmUUID string `db:"charm_uuid"`
	Tag       string `db:"value"`
}

// setCharmTag is used to set the tags of a charm.
// This includes the setting of the index.
// This is a row based struct that is normalised form of an array of strings.
type setCharmTag struct {
	CharmUUID string `db:"charm_uuid"`
	Tag       string `db:"value"`
	Index     int    `db:"array_index"`
}

// charmCategory is used to get the categories of a charm.
// This is a row based struct that is normalised form of an array of strings.
type charmCategory struct {
	CharmUUID string `db:"charm_uuid"`
	Category  string `db:"value"`
}

// setCharmCategory is used to set the categories of a charm.
// This includes the setting of the index.
// This is a row based struct that is normalised form of an array of strings.
type setCharmCategory struct {
	CharmUUID string `db:"charm_uuid"`
	Category  string `db:"value"`
	Index     int    `db:"array_index"`
}

// charmTerm is used to get the terms of a charm.
// This is a row based struct that is normalised form of an array of strings.
type charmTerm struct {
	CharmUUID string `db:"charm_uuid"`
	Term      string `db:"value"`
}

// setCharmTerm is used to set the terms of a charm.
// This includes the setting of the index.
// This is a row based struct that is normalised form of an array of strings.
type setCharmTerm struct {
	CharmUUID string `db:"charm_uuid"`
	Term      string `db:"value"`
	Index     int    `db:"array_index"`
}

// charmRelation is used to get the relations of a charm.
type charmRelation struct {
	CharmUUID string `db:"charm_uuid"`
	Name      string `db:"name"`
	Role      string `db:"role"`
	Interface string `db:"interface"`
	Optional  bool   `db:"optional"`
	Capacity  int    `db:"capacity"`
	Scope     string `db:"scope"`
}

// charmRelationName represents is used to fetch relation of a charm when only
// the name is required
type charmRelationName struct {
	UUID string `db:"uuid"`
	Name string `db:"name"`
}

// setCharmRelation is used to set the relations of a charm.
type setCharmRelation struct {
	UUID      string `db:"uuid"`
	CharmUUID string `db:"charm_uuid"`
	Name      string `db:"name"`
	RoleID    int    `db:"role_id"`
	Interface string `db:"interface"`
	Optional  bool   `db:"optional"`
	Capacity  int    `db:"capacity"`
	ScopeID   int    `db:"scope_id"`
}

// relationInfo represents metadata and configuration details for an existing
// relation within an application.
type relationInfo struct {
	ApplicationName string `db:"application_name"`
	CharmUUID       string `db:"charm_uuid"`
	Name            string `db:"name"`
	Role            string `db:"role"`
	Interface       string `db:"interface"`
	Optional        bool   `db:"optional"`
	Capacity        int    `db:"capacity"`
	Scope           string `db:"scope"`
	Count           int    `db:"count"`
}

// charmExtraBinding is used to get the extra bindings of a charm.
type charmExtraBinding struct {
	UUID      string `db:"uuid"`
	CharmUUID string `db:"charm_uuid"`
	Name      string `db:"name"`
}

// setCharmExtraBinding is used to set the extra bindings of a charm.
type setCharmExtraBinding struct {
	UUID      string `db:"uuid"`
	CharmUUID string `db:"charm_uuid"`
	Name      string `db:"name"`
}

// charmStorage is used to get the storage of a charm.
// This is a row based struct that is normalised form of an array of strings
// for the property field.
type charmStorage struct {
	CharmUUID   string `db:"charm_uuid"`
	Name        string `db:"name"`
	Description string `db:"description"`
	Kind        string `db:"kind"`
	Shared      bool   `db:"shared"`
	ReadOnly    bool   `db:"read_only"`
	CountMin    int    `db:"count_min"`
	CountMax    int    `db:"count_max"`
	MinimumSize uint64 `db:"minimum_size_mib"`
	Location    string `db:"location"`
	Property    string `db:"property"`
}

// setCharmStorage is used to set the storage of a charm.
type setCharmStorage struct {
	CharmUUID   string `db:"charm_uuid"`
	Name        string `db:"name"`
	Description string `db:"description"`
	KindID      int    `db:"storage_kind_id"`
	Shared      bool   `db:"shared"`
	ReadOnly    bool   `db:"read_only"`
	CountMin    int    `db:"count_min"`
	CountMax    int    `db:"count_max"`
	MinimumSize uint64 `db:"minimum_size_mib"`
	Location    string `db:"location"`
}

// setCharmStorageProperty is used to set the storage property of a charm.
type setCharmStorageProperty struct {
	CharmUUID string `db:"charm_uuid"`
	Name      string `db:"charm_storage_name"`
	Index     int    `db:"array_index"`
	Value     string `db:"value"`
}

// charmDevice is used to get the devices of a charm.
type charmDevice struct {
	CharmUUID   string `db:"charm_uuid"`
	Key         string `db:"key"`
	Name        string `db:"name"`
	Description string `db:"description"`
	DeviceType  string `db:"device_type"`
	CountMin    int64  `db:"count_min"`
	CountMax    int64  `db:"count_max"`
}

// setCharmDevice is used to set the devices of a charm.
type setCharmDevice struct {
	CharmUUID   string `db:"charm_uuid"`
	Key         string `db:"key"`
	Name        string `db:"name"`
	Description string `db:"description"`
	DeviceType  string `db:"device_type"`
	CountMin    int64  `db:"count_min"`
	CountMax    int64  `db:"count_max"`
}

// charmResource is used to get the resources of a charm.
type charmResource struct {
	CharmUUID   string `db:"charm_uuid"`
	Name        string `db:"name"`
	Kind        string `db:"kind"`
	Path        string `db:"path"`
	Description string `db:"description"`
}

// setCharmResource is used to set the resources of a charm.
type setCharmResource struct {
	CharmUUID   string `db:"charm_uuid"`
	Name        string `db:"name"`
	KindID      int    `db:"kind_id"`
	Path        string `db:"path"`
	Description string `db:"description"`
}

// charmContainer is used to get the containers of a charm.
// This is a row based struct that is normalised form of an array of strings
// for the storage and location field.
type charmContainer struct {
	CharmUUID string `db:"charm_uuid"`
	Key       string `db:"key"`
	Resource  string `db:"resource"`
	Uid       int    `db:"uid"`
	Gid       int    `db:"gid"`
	Storage   string `db:"storage"`
	Location  string `db:"location"`
}

// setCharmContainer is used to set the containers of a charm.
type setCharmContainer struct {
	CharmUUID string `db:"charm_uuid"`
	Key       string `db:"key"`
	Resource  string `db:"resource"`
	Uid       int    `db:"uid"`
	Gid       int    `db:"gid"`
}

// setCharmMount is used to set the mounts of a charm.
// This includes the setting of the index.
// This is a row based struct that is normalised form of an array of strings.
type setCharmMount struct {
	CharmUUID string `db:"charm_uuid"`
	Key       string `db:"charm_container_key"`
	Index     int    `db:"array_index"`
	Storage   string `db:"storage"`
	Location  string `db:"location"`
}

// charmManifest is used to get the manifest of a charm.
// This is a row based struct that is normalised form of an array of strings
// for the all the fields.
type charmManifest struct {
	CharmUUID    string `db:"charm_uuid"`
	Index        int    `db:"array_index"`
	NestedIndex  int    `db:"nested_array_index"`
	Track        string `db:"track"`
	Risk         string `db:"risk"`
	Branch       string `db:"branch"`
	OS           string `db:"os"`
	Architecture string `db:"architecture"`
}

// setCharmManifest is used to set the manifest of a charm.
// This includes the setting of the index.
type setCharmManifest struct {
	CharmUUID      string `db:"charm_uuid"`
	Index          int    `db:"array_index"`
	NestedIndex    int    `db:"nested_array_index"`
	Track          string `db:"track"`
	Risk           string `db:"risk"`
	Branch         string `db:"branch"`
	OSID           int    `db:"os_id"`
	ArchitectureID int    `db:"architecture_id"`
}

// charmLXDProfile is used to get the LXD profile of a charm.
type charmLXDProfile struct {
	UUID       string `db:"uuid"`
	LXDProfile []byte `db:"lxd_profile"`
	Revision   int    `db:"revision"`
}

// charmConfig is used to get the config of a charm.
// This is a row based struct that is normalised form of a map of config.
type charmConfig struct {
	CharmUUID    string  `db:"charm_uuid"`
	Key          string  `db:"key"`
	Type         string  `db:"type"`
	DefaultValue *string `db:"default_value"`
	Description  string  `db:"description"`
}

// setCharmConfig is used to set the config of a charm.
type setCharmConfig struct {
	CharmUUID    string  `db:"charm_uuid"`
	Key          string  `db:"key"`
	TypeID       int     `db:"type_id"`
	DefaultValue *string `db:"default_value"`
	Description  string  `db:"description"`
}

// charmAction is used to get the actions of a charm.
// This is a row based struct that is normalised form of a map of actions.
type charmAction struct {
	CharmUUID      string `db:"charm_uuid"`
	Key            string `db:"key"`
	Description    string `db:"description"`
	Parallel       bool   `db:"parallel"`
	ExecutionGroup string `db:"execution_group"`
	Params         []byte `db:"params"`
}

// setCharmAction is used to set the actions of a charm.
type setCharmAction struct {
	CharmUUID      string `db:"charm_uuid"`
	Key            string `db:"key"`
	Description    string `db:"description"`
	Parallel       bool   `db:"parallel"`
	ExecutionGroup string `db:"execution_group"`
	Params         []byte `db:"params"`
}

// charmArchivePath is used to get the archive path of a charm.
type charmArchivePath struct {
	ArchivePath string `db:"archive_path"`
}

// charmArchivePathAndHash is used to get the archive path and hash of a charm.
type charmArchivePathAndHash struct {
	ArchivePath string `db:"archive_path"`
	Hash        string `db:"hash"`
}

// charmArchiveHash is used to get the hash of a charm.
type charmArchiveHash struct {
	Available bool   `db:"available"`
	Hash      string `db:"hash"`
}

type countResult struct {
	Count int `db:"count"`
}

// charmLocator is used to get the locator of a charm. The locator is purely
// to reconstruct the charm URL.
type charmLocator struct {
	ReferenceName  string        `db:"reference_name"`
	Revision       int           `db:"revision"`
	SourceID       int           `db:"source_id"`
	ArchitectureID sql.NullInt64 `db:"architecture_id"`
}

type applicationCharmDownloadInfo struct {
	CharmUUID          string `db:"charm_uuid"`
	Name               string `db:"name"`
	Available          bool   `db:"available"`
	Hash               string `db:"hash"`
	Provenance         string `db:"provenance"`
	CharmhubIdentifier string `db:"charmhub_identifier"`
	DownloadURL        string `db:"download_url"`
	DownloadSize       int64  `db:"download_size"`
	SourceID           int    `db:"source_id"`
}

type resourceToAdd struct {
	UUID      string       `db:"uuid"`
	CharmUUID corecharm.ID `db:"charm_uuid"`
	Name      string       `db:"charm_resource_name"`
	Revision  *int         `db:"revision"`
	Origin    string       `db:"origin_type_name"`
	State     string       `db:"state_name"`
	CreatedAt time.Time    `db:"created_at"`
}

type storagePool struct {
	UUID string `db:"uuid"`
	Name string `db:"name"`
}

type storageToAdd struct {
	ApplicationUUID string       `db:"application_uuid"`
	CharmUUID       corecharm.ID `db:"charm_uuid"`
	StorageName     string       `db:"storage_name"`
	StoragePoolUUID *string      `db:"storage_pool_uuid"`
	StorageType     *string      `db:"storage_type"`
	Size            uint         `db:"size_mib"`
	Count           uint         `db:"count"`
}

type linkResourceApplication struct {
	ResourceUUID    string `db:"resource_uuid"`
	ApplicationUUID string `db:"application_uuid"`
}

type revisionUpdaterApplication struct {
	UUID                   string        `db:"uuid"`
	Name                   string        `db:"name"`
	ReferenceName          string        `db:"reference_name"`
	Revision               int           `db:"revision"`
	CharmArchitectureID    sql.NullInt64 `db:"charm_architecture_id"`
	ChannelTrack           string        `db:"channel_track"`
	ChannelRisk            string        `db:"channel_risk"`
	ChannelBranch          string        `db:"channel_branch"`
	PlatformOSID           sql.NullInt64 `db:"platform_os_id"`
	PlatformChannel        string        `db:"platform_channel"`
	PlatformArchitectureID sql.NullInt64 `db:"platform_architecture_id"`
	CharmhubIdentifier     string        `db:"charmhub_identifier"`
}

type revisionUpdaterApplicationNumUnits struct {
	UUID     string `db:"uuid"`
	NumUnits int    `db:"num_units"`
}

type applicationConfig struct {
	Key   string `db:"key"`
	Value any    `db:"value"`
	Type  string `db:"type"`
}

type setApplicationConfig struct {
	ApplicationUUID coreapplication.ID `db:"application_uuid"`
	Key             string             `db:"key"`
	Value           any                `db:"value"`
	TypeID          int                `db:"type_id"`
}

type applicationSettings struct {
	Trust bool `db:"trust"`
}

type setApplicationSettings struct {
	ApplicationUUID coreapplication.ID `db:"application_uuid"`
	Trust           bool               `db:"trust"`
}

type applicationConfigHash struct {
	ApplicationUUID coreapplication.ID `db:"application_uuid"`
	SHA256          string             `db:"sha256"`
}

type applicationStatus struct {
	ApplicationUUID string     `db:"application_uuid"`
	StatusID        int        `db:"status_id"`
	Message         string     `db:"message"`
	Data            []byte     `db:"data"`
	UpdatedAt       *time.Time `db:"updated_at"`
}

// applicationConstraint represents a single returned row when joining the
// constraint table with the constraint_space, constraint_tag and
// constraint_zone.
type applicationConstraint struct {
	ApplicationUUID  string         `db:"application_uuid"`
	Arch             sql.NullString `db:"arch"`
	CPUCores         sql.NullInt64  `db:"cpu_cores"`
	CPUPower         sql.NullInt64  `db:"cpu_power"`
	Mem              sql.NullInt64  `db:"mem"`
	RootDisk         sql.NullInt64  `db:"root_disk"`
	RootDiskSource   sql.NullString `db:"root_disk_source"`
	InstanceRole     sql.NullString `db:"instance_role"`
	InstanceType     sql.NullString `db:"instance_type"`
	ContainerType    sql.NullString `db:"container_type"`
	VirtType         sql.NullString `db:"virt_type"`
	AllocatePublicIP sql.NullBool   `db:"allocate_public_ip"`
	ImageID          sql.NullString `db:"image_id"`
	SpaceName        sql.NullString `db:"space_name"`
	SpaceExclude     sql.NullBool   `db:"space_exclude"`
	Tag              sql.NullString `db:"tag"`
	Zone             sql.NullString `db:"zone"`
}

type applicationConstraints []applicationConstraint

type setApplicationConstraint struct {
	ApplicationUUID string `db:"application_uuid"`
	ConstraintUUID  string `db:"constraint_uuid"`
}

type setApplicationEndpointBinding struct {
	UUID          corerelation.EndpointUUID `db:"uuid"`
	ApplicationID coreapplication.ID        `db:"application_uuid"`
	RelationUUID  string                    `db:"charm_relation_uuid"`
	Space         sql.Null[string]          `db:"space_uuid"`
}

type setApplicationExtraEndpointBinding struct {
	ApplicationID coreapplication.ID `db:"application_uuid"`
	RelationUUID  string             `db:"charm_extra_binding_uuid"`
	Space         sql.Null[string]   `db:"space_uuid"`
}

type updateApplicationEndpointBinding struct {
	ApplicationID coreapplication.ID `db:"application_uuid"`
	RelationUUID  string             `db:"charm_relation_uuid"`
	Space         sql.Null[string]   `db:"space_uuid"`
}

type updateApplicationExtraEndpointBinding struct {
	ApplicationID coreapplication.ID `db:"application_uuid"`
	RelationUUID  string             `db:"charm_extra_binding_uuid"`
	Space         sql.Null[string]   `db:"space_uuid"`
}

type setConstraint struct {
	UUID             string  `db:"uuid"`
	Arch             *string `db:"arch"`
	CPUCores         *uint64 `db:"cpu_cores"`
	CPUPower         *uint64 `db:"cpu_power"`
	Mem              *uint64 `db:"mem"`
	RootDisk         *uint64 `db:"root_disk"`
	RootDiskSource   *string `db:"root_disk_source"`
	InstanceRole     *string `db:"instance_role"`
	InstanceType     *string `db:"instance_type"`
	ContainerTypeID  *uint64 `db:"container_type_id"`
	VirtType         *string `db:"virt_type"`
	AllocatePublicIP *bool   `db:"allocate_public_ip"`
	ImageID          *string `db:"image_id"`
}

type containerTypeID struct {
	ID uint64 `db:"id"`
}

type containerTypeVal struct {
	Value string `db:"value"`
}

type setConstraintTag struct {
	ConstraintUUID string `db:"constraint_uuid"`
	Tag            string `db:"tag"`
}

type setConstraintSpace struct {
	ConstraintUUID string `db:"constraint_uuid"`
	Space          string `db:"space"`
	Exclude        bool   `db:"exclude"`
}

type setConstraintZone struct {
	ConstraintUUID string `db:"constraint_uuid"`
	Zone           string `db:"zone"`
}

type setDefaultSpace struct {
	UUID  coreapplication.ID `db:"uuid"`
	Space network.SpaceName  `db:"space"`
}

type applicationUUID struct {
	ApplicationUUID string `db:"application_uuid"`
}

type unitConstraintUUID struct {
	UnitUUID string `db:"unit_uuid"`
}

type constraintUUID struct {
	ConstraintUUID string `db:"constraint_uuid"`
}

type space struct {
	UUID string            `db:"uuid"`
	Name network.SpaceName `db:"name"`
}

// These structs are only needed to check existence before adding constraints.
type spaceName struct {
	Name string `db:"name"`
}

type spaceUUID struct {
	UUID string `db:"uuid"`
}

type storageInstance struct {
	StorageUUID      corestorage.UUID `db:"uuid"`
	StorageID        corestorage.ID   `db:"storage_id"`
	CharmUUID        corecharm.ID     `db:"charm_uuid"`
	StorageName      corestorage.Name `db:"storage_name"`
	LifeID           life.Life        `db:"life_id"`
	StoragePoolUUID  *string          `db:"storage_pool_uuid"`
	StorageType      *string          `db:"storage_type"`
	RequestedSizeMIB uint64           `db:"requested_size_mib"`
}

type storageUnit struct {
	StorageUUID corestorage.UUID `db:"storage_instance_uuid"`
	UnitUUID    coreunit.UUID    `db:"unit_uuid"`
}

type unitCharmStorage struct {
	UnitUUID    coreunit.UUID    `db:"uuid"`
	StorageName corestorage.Name `db:"name"`
}

type appCharmStorage struct {
	ApplicationUUID coreapplication.ID `db:"uuid"`
	StorageName     corestorage.Name   `db:"name"`
}

type storageCount struct {
	StorageUUID corestorage.UUID `db:"uuid"`
	StorageName corestorage.Name `db:"storage_name"`
	UnitUUID    coreunit.UUID    `db:"unit_uuid"`
	Count       uint64           `db:"count"`
}

type storageAttachment struct {
	StorageUUID corestorage.UUID `db:"storage_instance_uuid"`
	UnitUUID    coreunit.UUID    `db:"unit_uuid"`
	LifeID      life.Life        `db:"life_id"`
}

type filesystemUUID struct {
	UUID       corestorage.FilesystemUUID `db:"uuid"`
	AttachedTo *string                    `db:"net_node_uuid"`
}

type volumeUUID struct {
	UUID       corestorage.VolumeUUID `db:"uuid"`
	AttachedTo *string                `db:"net_node_uuid"`
}

type filesystem struct {
	UUID         corestorage.FilesystemUUID `db:"uuid"`
	LifeID       life.Life                  `db:"life_id"`
	FilesystemID string                     `db:"filesystem_id"`
	ProviderID   string                     `db:"provider_id"`
	SizeMIB      uint64                     `db:"size_mib"`
}

type filesystemStatus struct {
	FilesystemUUID string     `db:"filesystem_uuid"`
	StatusID       int        `db:"status_id"`
	Message        string     `db:"message"`
	UpdatedAt      *time.Time `db:"updated_at"`
}

type storageInstanceFilesystem struct {
	StorageUUID    corestorage.UUID           `db:"storage_instance_uuid"`
	FilesystemUUID corestorage.FilesystemUUID `db:"storage_filesystem_uuid"`
}

type volume struct {
	UUID         corestorage.VolumeUUID `db:"uuid"`
	LifeID       life.Life              `db:"life_id"`
	VolumeID     string                 `db:"volume_id"`
	ProviderID   string                 `db:"provider_id"`
	SizeMIB      uint64                 `db:"size_mib"`
	Name         string                 `db:"name"`
	HardwareIDID string                 `db:"hardware_id"`
	WWN          string                 `db:"wwn"`
	Persistent   bool                   `db:"persistent"`
}

type volumeStatus struct {
	VolumeUUID string     `db:"volume_uuid"`
	StatusID   int        `db:"status_id"`
	Message    string     `db:"message"`
	UpdatedAt  *time.Time `db:"updated_at"`
}

type storageInstanceVolume struct {
	StorageUUID corestorage.UUID       `db:"storage_instance_uuid"`
	VolumeUUID  corestorage.VolumeUUID `db:"storage_volume_uuid"`
}

type filesystemAttachment struct {
	UUID           corestorage.FilesystemAttachmentUUID `db:"uuid"`
	NetNodeUUID    string                               `db:"net_node_uuid"`
	FilesystemUUID corestorage.FilesystemUUID           `db:"storage_filesystem_uuid"`
	LifeID         life.Life                            `db:"life_id"`
	MountPoint     string                               `db:"mount_point"`
	ReadOnly       bool                                 `db:"read_only"`
}

type volumeAttachment struct {
	UUID        corestorage.VolumeAttachmentUUID `db:"uuid"`
	NetNodeUUID string                           `db:"net_node_uuid"`
	VolumeUUID  corestorage.VolumeUUID           `db:"storage_volume_uuid"`
	LifeID      life.Life                        `db:"life_id"`
	ReadOnly    bool                             `db:"read_only"`
}

type setUnitConstraint struct {
	UnitUUID       string `db:"unit_uuid"`
	ConstraintUUID string `db:"constraint_uuid"`
}

// dbConstraint represents a single row within the v_model_constraint view.
type dbConstraint struct {
	Arch             sql.NullString `db:"arch"`
	CPUCores         sql.NullInt64  `db:"cpu_cores"`
	CPUPower         sql.NullInt64  `db:"cpu_power"`
	Mem              sql.NullInt64  `db:"mem"`
	RootDisk         sql.NullInt64  `db:"root_disk"`
	RootDiskSource   sql.NullString `db:"root_disk_source"`
	InstanceRole     sql.NullString `db:"instance_role"`
	InstanceType     sql.NullString `db:"instance_type"`
	ContainerType    sql.NullString `db:"container_type"`
	VirtType         sql.NullString `db:"virt_type"`
	AllocatePublicIP sql.NullBool   `db:"allocate_public_ip"`
	ImageID          sql.NullString `db:"image_id"`
}

func (c dbConstraint) toValue(
	tags []dbConstraintTag,
	spaces []dbConstraintSpace,
	zones []dbConstraintZone,
) (constraints.Constraints, error) {
	rval := constraints.Constraints{}
	if c.Arch.Valid {
		rval.Arch = &c.Arch.String
	}
	if c.CPUCores.Valid {
		rval.CpuCores = ptr(uint64(c.CPUCores.Int64))
	}
	if c.CPUPower.Valid {
		rval.CpuPower = ptr(uint64(c.CPUPower.Int64))
	}
	if c.Mem.Valid {
		rval.Mem = ptr(uint64(c.Mem.Int64))
	}
	if c.RootDisk.Valid {
		rval.RootDisk = ptr(uint64(c.RootDisk.Int64))
	}
	if c.RootDiskSource.Valid {
		rval.RootDiskSource = &c.RootDiskSource.String
	}
	if c.InstanceRole.Valid {
		rval.InstanceRole = &c.InstanceRole.String
	}
	if c.InstanceType.Valid {
		rval.InstanceType = &c.InstanceType.String
	}
	if c.VirtType.Valid {
		rval.VirtType = &c.VirtType.String
	}
	// We only set allocate public ip when it is true and not nil. The reason
	// for this is no matter what the dqlite driver will always return false
	// out of the database even when the value is NULL.
	if c.AllocatePublicIP.Valid {
		rval.AllocatePublicIP = &c.AllocatePublicIP.Bool
	}
	if c.ImageID.Valid {
		rval.ImageID = &c.ImageID.String
	}
	if c.ContainerType.Valid {
		containerType := instance.ContainerType(c.ContainerType.String)
		rval.Container = &containerType
	}

	consTags := make([]string, 0, len(tags))
	for _, tag := range tags {
		consTags = append(consTags, tag.Tag)
	}
	// Only set constraint tags if there are tags in the database value.
	if len(consTags) != 0 {
		rval.Tags = &consTags
	}

	consSpaces := make([]constraints.SpaceConstraint, 0, len(spaces))
	for _, space := range spaces {
		consSpaces = append(consSpaces, constraints.SpaceConstraint{
			SpaceName: space.Space,
			Exclude:   space.Exclude,
		})
	}
	// Only set constraint spaces if there are spaces in the database value.
	if len(consSpaces) != 0 {
		rval.Spaces = &consSpaces
	}

	consZones := make([]string, 0, len(zones))
	for _, zone := range zones {
		consZones = append(consZones, zone.Zone)
	}
	// Only set constraint zones if there are zones in the database value.
	if len(consZones) != 0 {
		rval.Zones = &consZones
	}

	return rval, nil
}

// dbConstraintTag represents a row from either the constraint_tag table or
// v_model_constraint_tag view.
type dbConstraintTag struct {
	ConstraintUUID string `db:"constraint_uuid"`
	Tag            string `db:"tag"`
}

// dbConstraintSpace represents a row from either the constraint_space table or
// v_model_constraint_space view.
type dbConstraintSpace struct {
	ConstraintUUID string `db:"constraint_uuid"`
	Space          string `db:"space"`
	Exclude        bool   `db:"exclude"`
}

// dbConstraintZone represents a row from either the constraint_zone table or
// v_model_constraint_zone view.
type dbConstraintZone struct {
	ConstraintUUID string `db:"constraint_uuid"`
	Zone           string `db:"zone"`
}

// dbUUID represents a UUID.
type dbUUID struct {
	UUID string `db:"uuid"`
}

type applicationPlatformAndChannel struct {
	PlatformOSID           sql.NullInt64  `db:"platform_os_id"`
	PlatformChannel        string         `db:"platform_channel"`
	PlatformArchitectureID sql.NullInt64  `db:"platform_architecture_id"`
	ChannelTrack           string         `db:"channel_track"`
	ChannelRisk            sql.NullString `db:"channel_risk"`
	ChannelBranch          string         `db:"channel_branch"`
}

type applicationOrigin struct {
	ReferenceName      string         `db:"reference_name"`
	SourceID           int            `db:"source_id"`
	Revision           sql.NullInt64  `db:"revision"`
	CharmhubIdentifier sql.NullString `db:"charmhub_identifier"`
	Hash               sql.NullString `db:"hash"`
}

type exportApplication struct {
	UUID                 coreapplication.ID `db:"uuid"`
	Name                 string             `db:"name"`
	CharmUUID            corecharm.ID       `db:"charm_uuid"`
	Life                 life.Life          `db:"life_id"`
	Subordinate          bool               `db:"subordinate"`
	CharmModifiedVersion int                `db:"charm_modified_version"`
	CharmUpgradeOnError  bool               `db:"charm_upgrade_on_error"`
	CharmReferenceName   string             `db:"reference_name"`
	CharmSourceID        int                `db:"source_id"`
	CharmRevision        int                `db:"revision"`
	CharmArchitectureID  sql.NullInt64      `db:"architecture_id"`
	K8sServiceProviderID sql.NullString     `db:"k8s_provider_id"`
	EndpointBindings     map[string]string
}

// peerEndpoint represents a structure for defining a peer application endpoint
// with a UUID and a name.
type peerEndpoint struct {
	// UUID is the unique identifier of the peer endpoint.
	UUID corerelation.EndpointUUID `db:"uuid"`
	// Name is the human-readable name of the peer endpoint.
	Name string `db:"name"`
}

type exportUnit struct {
	UUID      coreunit.UUID `db:"uuid"`
	Name      coreunit.Name `db:"name"`
	Machine   machine.Name  `db:"machine_name"`
	Principal coreunit.Name `db:"principal_name"`
}

type setExposedSpace struct {
	ApplicationUUID string `db:"application_uuid"`
	EndpointName    string `db:"endpoint"`
	SpaceUUID       string `db:"space_uuid"`
}

type setExposedCIDR struct {
	ApplicationUUID string `db:"application_uuid"`
	EndpointName    string `db:"endpoint"`
	CIDR            string `db:"cidr"`
}

type endpointCIDRsSpaces struct {
	Name      sql.NullString `db:"name"`
	CIDR      string         `db:"cidr"`
	SpaceUUID string         `db:"space_uuid"`
}

// spaces is a type used to pass a slice of space UUIDs to a query using `IN`
// and sqlair.
type spaces []string

// endpointNames is a type used to pass a slice of endpoint names to a query
// using `IN` and sqlair.
type endpointNames []string

type deviceConstraint struct {
	Name           string         `db:"name"`
	Type           string         `db:"type"`
	Count          int            `db:"count"`
	AttributeKey   sql.NullString `db:"key"`
	AttributeValue sql.NullString `db:"value"`
}

type setDeviceConstraint struct {
	UUID            string `db:"uuid"`
	ApplicationUUID string `db:"application_uuid"`
	Name            string `db:"name"`
	Type            string `db:"type"`
	Count           int    `db:"count"`
}

type setDeviceConstraintAttribute struct {
	DeviceConstraintUUID string `db:"device_constraint_uuid"`
	AttributeKey         string `db:"key"`
	AttributeValue       string `db:"value"`
}

type createMachine struct {
	MachineUUID machine.UUID `db:"uuid"`
	NetNodeUUID string       `db:"net_node_uuid"`
	Name        machine.Name `db:"name"`
	LifeID      life.Life    `db:"life_id"`
}

type machineNameWithNetNode struct {
	Name        machine.Name `db:"name"`
	NetNodeUUID string       `db:"net_node_uuid"`
}

type machineNameWithMachineUUID struct {
	Name machine.Name `db:"name"`
	UUID machine.UUID `db:"uuid"`
}

type netNodeUUID struct {
	NetNodeUUID string `db:"uuid"`
}

type unitNetNodeUUID struct {
	NetNodeUUID string `db:"net_node_uuid"`
}

type machinePlacement struct {
	MachineUUID machine.UUID `db:"machine_uuid"`
	ScopeID     int          `db:"scope_id"`
	Directive   string       `db:"directive"`
}

type machineParent struct {
	ParentUUID  machine.UUID `db:"parent_uuid"`
	MachineUUID machine.UUID `db:"machine_uuid"`
}

type getApplicationEndpoint struct {
	SpaceUUID    sql.Null[string] `db:"space_uuid"`
	EndpointName string           `db:"name"`
}

type unitWorkloadVersion struct {
	UnitUUID coreunit.UUID `db:"unit_uuid"`
	Version  string        `db:"version"`
}

type applicationWorkloadVersion struct {
	ApplicationUUID coreapplication.ID `db:"application_uuid"`
	Version         string             `db:"version"`
}

type getPrincipal struct {
	PrincipalUnitName   coreunit.Name `db:"principal_unit_name"`
	SubordinateUnitName coreunit.Name `db:"subordinate_unit_name"`
}

type getUnitMachineName struct {
	UnitName    coreunit.Name `db:"unit_name"`
	MachineName machine.Name  `db:"name"`
}

type getUnitMachineUUID struct {
	UnitName    coreunit.Name `db:"unit_name"`
	MachineUUID machine.UUID  `db:"uuid"`
}

type lifeID struct {
	LifeID life.Life `db:"life_id"`
}

type getCharmUpgradeOnError struct {
	CharmUpgradeOnError bool   `db:"charm_upgrade_on_error"`
	Name                string `db:"name"`
}

type setStatusInfo struct {
	StatusID int        `db:"status_id"`
	Message  string     `db:"message"`
	Data     []byte     `db:"data"`
	Updated  *time.Time `db:"updated_at"`
}

type setMachineStatus struct {
	StatusID    int          `db:"status_id"`
	Message     string       `db:"message"`
	Data        []byte       `db:"data"`
	Updated     *time.Time   `db:"updated_at"`
	MachineUUID machine.UUID `db:"machine_uuid"`
}

type machineInstanceUUID struct {
	MachineUUID machine.UUID `db:"machine_uuid"`
}
