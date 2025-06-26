// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/tools"
)

// MachineContainersParams holds the arguments for making a SetSupportedContainers
// API call.
type MachineContainersParams struct {
	Params []MachineContainers `json:"params"`
}

// MachineContainers holds the arguments for making an SetSupportedContainers call
// on a given machine.
type MachineContainers struct {
	MachineTag     string                   `json:"machine-tag"`
	ContainerTypes []instance.ContainerType `json:"container-types"`
}

// MachineContainerResults holds the results from making the call to SupportedContainers
// on a given machine.
type MachineContainerResults struct {
	Results []MachineContainerResult `json:"results"`
}

// MachineContainerResult holds the result of making the call to SupportedContainers
// on a given machine.
type MachineContainerResult struct {
	Error          *Error                   `json:"error,omitempty"`
	ContainerTypes []instance.ContainerType `json:"container-types"`
	Determined     bool                     `json:"determined"`
}

// WatchContainer identifies a single container type within a machine.
type WatchContainer struct {
	MachineTag    string `json:"machine-tag"`
	ContainerType string `json:"container-type"`
}

// WatchContainers holds the arguments for making a WatchContainers
// API call.
type WatchContainers struct {
	Params []WatchContainer `json:"params"`
}

// CharmURL identifies a single charm URL.
type CharmURL struct {
	URL string `json:"url"`
}

// CharmURLs identifies multiple charm URLs.
type CharmURLs struct {
	URLs []CharmURL `json:"urls"`
}

// StringsResult holds the result of an API call that returns a slice
// of strings or an error.
type StringsResult struct {
	Error  *Error   `json:"error,omitempty"`
	Result []string `json:"result,omitempty"`
}

// StringsResults holds the bulk operation result of an API call
// that returns a slice of strings or an error.
type StringsResults struct {
	Results []StringsResult `json:"results"`
}

// StringResult holds a string or an error.
type StringResult struct {
	Error  *Error `json:"error,omitempty"`
	Result string `json:"result"`
}

// StringResults holds the bulk operation result of an API call
// that returns a string or an error.
type StringResults struct {
	Results []StringResult `json:"results"`
}

// MapResult holds a generic map or an error.
type MapResult struct {
	Result map[string]interface{} `json:"result"`
	Error  *Error                 `json:"error,omitempty"`
}

// MapResults holds the bulk operation result of an API call
// that returns a map or an error.
type MapResults struct {
	Results []MapResult `json:"results"`
}

// ModelResult holds the result of an API call returning a name and UUID
// for a model.
type ModelResult struct {
	Error *Error `json:"error,omitempty"`
	Name  string `json:"name"`
	UUID  string `json:"uuid"`
	Type  string `json:"type"`
}

// ModelCreateArgs holds the arguments that are necessary to create
// a model.
type ModelCreateArgs struct {
	// Name is the name for the new model.
	Name string `json:"name"`

	// Qualifier disambiguates the name of the model.
	Qualifier string `json:"qualifier"`

	// Config defines the model config, which includes the name of the
	// model. A model UUID is allocated by the API server during the
	// creation of the model.
	Config map[string]interface{} `json:"config,omitempty"`

	// CloudTag is the tag of the cloud to create the model in.
	// If this is empty, the model will be created in the same
	// cloud as the controller model.
	CloudTag string `json:"cloud-tag,omitempty"`

	// CloudRegion is the name of the cloud region to create the
	// model in. If the cloud does not support regions, this must
	// be empty. If this is empty, and CloudTag is empty, the model
	// will be created in the same region as the controller model.
	CloudRegion string `json:"region,omitempty"`

	// CloudCredentialTag is the tag of the cloud credential to use
	// for managing the model's resources. If the cloud does not
	// require credentials, this may be empty. If this is empty,
	// and the owner is the controller owner, the same credential
	// used for the controller model will be used.
	CloudCredentialTag string `json:"credential,omitempty"`
}

// Model holds the result of an API call returning a name and UUID
// for a model and the tag of the server in which it is running.
type Model struct {
	Name      string `json:"name"`
	Qualifier string `json:"qualifier"`
	UUID      string `json:"uuid"`
	Type      string `json:"type"`
}

// UserModel holds information about a model and the last
// time the model was accessed for a particular user.
type UserModel struct {
	Model          `json:"model"`
	LastConnection *time.Time `json:"last-connection"`
}

// UserModelList holds information about a list of models
// for a particular user.
type UserModelList struct {
	UserModels []UserModel `json:"user-models"`
}

// ResolvedModeResult holds a resolved mode or an error.
type ResolvedModeResult struct {
	Error *Error       `json:"error,omitempty"`
	Mode  ResolvedMode `json:"mode"`
}

// ResolvedModeResults holds the bulk operation result of an API call
// that returns a resolved mode or an error.
type ResolvedModeResults struct {
	Results []ResolvedModeResult `json:"results"`
}

// StringBoolResult holds the result of an API call that returns a
// string and a boolean.
type StringBoolResult struct {
	Error  *Error `json:"error,omitempty"`
	Result string `json:"result"`
	Ok     bool   `json:"ok"`
}

// StringBoolResults holds multiple results with a string and a bool
// each.
type StringBoolResults struct {
	Results []StringBoolResult `json:"results"`
}

// BoolResult holds the result of an API call that returns a
// a boolean or an error.
type BoolResult struct {
	Error  *Error `json:"error,omitempty"`
	Result bool   `json:"result"`
}

// BoolResults holds multiple results with BoolResult each.
type BoolResults struct {
	Results []BoolResult `json:"results"`
}

// IntResults holds multiple results with an int in each.
type IntResults struct {
	// Results holds a list of results for calls that return an int or error.
	Results []IntResult `json:"results"`
}

// IntResult holds the result of an API call that returns a
// int or an error.
type IntResult struct {
	// Error holds the error (if any) of this call.
	Error *Error `json:"error,omitempty"`
	// Result holds the integer result of the call (if Error is nil).
	Result int `json:"result"`
}

// Settings holds relation settings names and values.
type Settings map[string]string

// SettingsResult holds a relation settings map or an error.
type SettingsResult struct {
	Error    *Error   `json:"error,omitempty"`
	Settings Settings `json:"settings"`
}

// SettingsResults holds the result of an API calls that
// returns settings for multiple relations.
type SettingsResults struct {
	Results []SettingsResult `json:"results"`
}

// ConfigSettings holds unit, application or charm configuration settings
// with string keys and arbitrary values.
type ConfigSettings map[string]interface{}

// ConfigSettingsResult holds a configuration map or an error.
type ConfigSettingsResult struct {
	Error    *Error         `json:"error,omitempty"`
	Settings ConfigSettings `json:"settings"`
}

// ConfigSettingsResults holds multiple configuration maps or errors.
type ConfigSettingsResults struct {
	Results []ConfigSettingsResult `json:"results"`
}

// UnitStateResult holds a unit's state map or an error.
type UnitStateResult struct {
	Error *Error `json:"error,omitempty"`
	// Charm state set by the unit via hook tool.
	CharmState map[string]string `json:"charm-state,omitempty"`
	// Uniter internal state for this unit.
	UniterState string `json:"uniter-state,omitempty"`
	// RelationState is a internal relation state for this unit.
	RelationState map[int]string `json:"relation-state,omitempty"`
	// StorageState is a internal storage state for this unit.
	StorageState string `json:"storage-state,omitempty"`
	// SecretState is internal secret state for this unit.
	SecretState string `json:"secret-state,omitempty"`
}

// UnitStateResults holds multiple unit state maps or errors.
type UnitStateResults struct {
	Results []UnitStateResult `json:"results"`
}

// SetUnitStateArgs holds multiple SetUnitStateArg objects to be persisted by the controller.
type SetUnitStateArgs struct {
	Args []SetUnitStateArg `json:"args"`
}

// SetUnitStateArg holds a unit tag and pointers to data persisted from
// or via the uniter. State is a map with KV-pairs that represent a
// local charm state to be persisted by the controller.  The other fields
// represent uniter internal data.
//
// Each field with omitempty is optional, setting it will cause the field
// to be evaluated for changes to the persisted data.  A pointer to nil or
// empty data will cause the persisted data to be deleted.
type SetUnitStateArg struct {
	Tag           string             `json:"tag"`
	CharmState    *map[string]string `json:"charm-state,omitempty"`
	UniterState   *string            `json:"uniter-state,omitempty"`
	RelationState *map[int]string    `json:"relation-state,omitempty"`
	StorageState  *string            `json:"storage-state,omitempty"`
	SecretState   *string            `json:"secret-state,omitempty"`
}

// CommitHookChangesArgs serves as a container for CommitHookChangesArg objects
// to be processed by the controller.
type CommitHookChangesArgs struct {
	Args []CommitHookChangesArg `json:"args"`
}

// CommitHookChangesArg holds a unit tag and a list of optional uniter API
// call payloads that are to be executed transactionally.
type CommitHookChangesArg struct {
	Tag string `json:"tag"`

	UpdateNetworkInfo    bool                   `json:"update-network-info"`
	RelationUnitSettings []RelationUnitSettings `json:"relation-unit-settings,omitempty"`
	OpenPorts            []EntityPortRange      `json:"open-ports,omitempty"`
	ClosePorts           []EntityPortRange      `json:"close-ports,omitempty"`
	SetUnitState         *SetUnitStateArg       `json:"unit-state,omitempty"`
	AddStorage           []StorageAddParams     `json:"add-storage,omitempty"`
	SecretCreates        []CreateSecretArg      `json:"secret-creates,omitempty"`
	TrackLatest          []string               `json:"secret-track-latest,omitempty"`
	SecretUpdates        []UpdateSecretArg      `json:"secret-updates,omitempty"`
	SecretGrants         []GrantRevokeSecretArg `json:"secret-grants,omitempty"`
	SecretRevokes        []GrantRevokeSecretArg `json:"secret-revokes,omitempty"`
	SecretDeletes        []DeleteSecretArg      `json:"secret-deletes,omitempty"`
}

// ModelConfig holds a model configuration.
type ModelConfig map[string]interface{}

// ControllerConfig holds a controller configuration.
type ControllerConfig map[string]interface{}

// ModelConfigResult holds model configuration.
type ModelConfigResult struct {
	Config ModelConfig `json:"config"`
}

// ControllerConfigResult holds controller configuration.
type ControllerConfigResult struct {
	Config ControllerConfig `json:"config"`
}

// ControllerAPIInfoResult holds controller api address details.
type ControllerAPIInfoResult struct {
	Addresses []string `json:"addresses"`
	CACert    string   `json:"cacert"`
	Error     *Error   `json:"error,omitempty"`
}

// ControllerAPIInfoResults holds controller api address details results.
type ControllerAPIInfoResults struct {
	Results []ControllerAPIInfoResult `json:"results"`
}

// RelationUnit holds a relation and a unit tag.
type RelationUnit struct {
	Relation string `json:"relation"`
	Unit     string `json:"unit"`
}

// RelationUnits holds the parameters for API calls expecting a pair
// of relation and unit tags.
type RelationUnits struct {
	RelationUnits []RelationUnit `json:"relation-units"`
}

// RelationIds holds multiple relation ids.
type RelationIds struct {
	RelationIds []int `json:"relation-ids"`
}

// RelationUnitPair holds a relation tag, a local and remote unit tags.
type RelationUnitPair struct {
	Relation   string `json:"relation"`
	LocalUnit  string `json:"local-unit"`
	RemoteUnit string `json:"remote-unit"`
}

// RelationUnitPairs holds the parameters for API calls expecting
// multiple sets of a relation tag, a local and remote unit tags.
type RelationUnitPairs struct {
	RelationUnitPairs []RelationUnitPair `json:"relation-unit-pairs"`
}

// RelationUnitSettings holds a relation tag, a unit tag and local
// unit and app-level settings.
// TODO(juju3) - remove
type RelationUnitSettings struct {
	Relation            string   `json:"relation"`
	Unit                string   `json:"unit"`
	Settings            Settings `json:"settings"`
	ApplicationSettings Settings `json:"application-settings"`
}

// RelationUnitsSettings holds the arguments for making a EnterScope
// or UpdateRelationSettings API calls.
// TODO(juju3) - remove
type RelationUnitsSettings struct {
	RelationUnits []RelationUnitSettings `json:"relation-units"`
}

// RelatedApplicationDetails holds information about
// an application related to a unit.
type RelatedApplicationDetails struct {
	ModelUUID       string `json:"model-uuid"`
	ApplicationName string `json:"application"`
}

// RelationResults holds the result of an API call that returns
// information about multiple relations.
type RelationResults struct {
	Results []RelationResult `json:"results"`
}

// Endpoint holds an application-relation pair.
type Endpoint struct {
	ApplicationName string        `json:"application-name"`
	Relation        CharmRelation `json:"relation"`
}

// NewCharmRelation creates a new local CharmRelation structure from  the
// charm.Relation structure. NOTE: when we update the database to not store a
// charm.Relation directly in the database, this method should take the state
// structure type.
func NewCharmRelation(cr charm.Relation) CharmRelation {
	return CharmRelation{
		Name:      cr.Name,
		Role:      string(cr.Role),
		Interface: cr.Interface,
		Optional:  cr.Optional,
		Limit:     cr.Limit,
		Scope:     string(cr.Scope),
	}
}

// RelationResult returns information about a single relation,
// or an error.
type RelationResult struct {
	Error            *Error     `json:"error,omitempty"`
	Life             life.Value `json:"life"`
	Suspended        bool       `json:"bool,omitempty"`
	Id               int        `json:"id"`
	Key              string     `json:"key"`
	Endpoint         Endpoint   `json:"endpoint"`
	OtherApplication string     `json:"other-application,omitempty"`
}

// RelationResultsV2 holds the result of an API call that returns
// information about multiple relations.
type RelationResultsV2 struct {
	Results []RelationResultV2 `json:"results"`
}

// RelationResultV2 returns information about a single relation,
// or an error.
type RelationResultV2 struct {
	Error            *Error                    `json:"error,omitempty"`
	Life             life.Value                `json:"life"`
	Suspended        bool                      `json:"bool,omitempty"`
	Id               int                       `json:"id"`
	Key              string                    `json:"key"`
	Endpoint         Endpoint                  `json:"endpoint"`
	OtherApplication RelatedApplicationDetails `json:"other-application,omitempty"`
}

// EntityCharmURL holds an entity's tag and a charm URL.
type EntityCharmURL struct {
	Tag      string `json:"tag"`
	CharmURL string `json:"charm-url"`
}

// EntitiesCharmURL holds the parameters for making a SetCharmURL API
// call.
type EntitiesCharmURL struct {
	Entities []EntityCharmURL `json:"entities"`
}

// EntityWorkloadVersion holds the workload version for an entity.
type EntityWorkloadVersion struct {
	Tag             string `json:"tag"`
	WorkloadVersion string `json:"workload-version"`
}

// EntityWorkloadVersions holds the parameters for setting the
// workload version for a set of entities.
type EntityWorkloadVersions struct {
	Entities []EntityWorkloadVersion `json:"entities"`
}

// BytesResult holds the result of an API call that returns a slice
// of bytes.
type BytesResult struct {
	Result []byte `json:"result"`
}

// LifeResult holds the life status of a single entity, or an error
// indicating why it is not available.
type LifeResult struct {
	Life  life.Value `json:"life"`
	Error *Error     `json:"error,omitempty"`
}

// LifeResults holds the life or error status of multiple entities.
type LifeResults struct {
	Results []LifeResult `json:"results"`
}

// InstanceInfo holds information about an instance. Instances are
// typically virtual machines hosted by a cloud provider but may also
// be a container.
//
// The InstanceInfo struct contains three categories of information:
//   - interal data, as the machine's tag and the tags of any attached
//     storage volumes
//   - naming and other provider-specific information, including the
//     instance id and display name
//   - configuration information, including its attached storage volumes,
//     charm profiles and networking
type InstanceInfo struct {
	Tag             string                            `json:"tag"`
	InstanceId      instance.Id                       `json:"instance-id"`
	DisplayName     string                            `json:"display-name"`
	Nonce           string                            `json:"nonce"`
	Characteristics *instance.HardwareCharacteristics `json:"characteristics"`
	Volumes         []Volume                          `json:"volumes"`
	// VolumeAttachments is a mapping from volume tag to
	// volume attachment info.
	VolumeAttachments map[string]VolumeAttachmentInfo `json:"volume-attachments"`

	NetworkConfig []NetworkConfig `json:"network-config"`
	CharmProfiles []string        `json:"charm-profiles"`
}

// InstancesInfo holds the parameters for making a SetInstanceInfo
// call for multiple machines.
type InstancesInfo struct {
	Machines []InstanceInfo `json:"machines"`
}

// EntityStatus holds the status of an entity.
type EntityStatus struct {
	Status status.Status          `json:"status"`
	Info   string                 `json:"info"`
	Data   map[string]interface{} `json:"data,omitempty"`
	Since  *time.Time             `json:"since"`
}

// EntityStatusArgs holds parameters for setting the status of a single entity.
type EntityStatusArgs struct {
	Tag    string                 `json:"tag"`
	Status string                 `json:"status"`
	Info   string                 `json:"info"`
	Data   map[string]interface{} `json:"data"`
}

// SetStatus holds the parameters for making a SetStatus/UpdateStatus call.
type SetStatus struct {
	Entities []EntityStatusArgs `json:"entities"`
}

// ConstraintsResult holds machine constraints or an error.
type ConstraintsResult struct {
	Error       *Error            `json:"error,omitempty"`
	Constraints constraints.Value `json:"constraints"`
}

// ConstraintsResults holds multiple constraints results.
type ConstraintsResults struct {
	Results []ConstraintsResult `json:"results"`
}

// AgentGetEntitiesResults holds the results of a
// agent.API.GetEntities call.
type AgentGetEntitiesResults struct {
	Entities []AgentGetEntitiesResult `json:"entities"`
}

// AgentGetEntitiesResult holds the results of a
// machineagent.API.GetEntities call for a single entity.
type AgentGetEntitiesResult struct {
	Life          life.Value             `json:"life"`
	Jobs          []model.MachineJob     `json:"jobs"`
	ContainerType instance.ContainerType `json:"container-type"`
	Error         *Error                 `json:"error,omitempty"`
}

// VersionResult holds the version and possibly error for a given
// DesiredVersion() API call.
type VersionResult struct {
	Version *semversion.Number `json:"version,omitempty"`
	Error   *Error             `json:"error,omitempty"`
}

// VersionResults is a list of versions for the requested entities.
type VersionResults struct {
	Results []VersionResult `json:"results"`
}

// ToolsResult holds the tools and possibly error for a given
// Tools() API call.
type ToolsResult struct {
	ToolsList tools.List `json:"tools"`
	Error     *Error     `json:"error,omitempty"`
}

// ToolsResults is a list of tools for various requested agents.
type ToolsResults struct {
	Results []ToolsResult `json:"results"`
}

// Version holds a specific binary version.
type Version struct {
	Version semversion.Binary `json:"version"`
}

// EntityVersion specifies the tools version to be set for an entity
// with the given tag.
// version.Binary directly.
type EntityVersion struct {
	Tag   string   `json:"tag"`
	Tools *Version `json:"tools"`
}

// EntitiesVersion specifies what tools are being run for
// multiple entities.
type EntitiesVersion struct {
	AgentTools []EntityVersion `json:"agent-tools"`
}

// NotifyWatchResult holds a NotifyWatcher id and an error (if any).
type NotifyWatchResult struct {
	NotifyWatcherId string
	Error           *Error `json:"error,omitempty"`
}

// NotifyWatchResults holds the results for any API call which ends up
// returning a list of NotifyWatchers
type NotifyWatchResults struct {
	Results []NotifyWatchResult `json:"results"`
}

// StringsWatchResult holds a StringsWatcher id, changes and an error
// (if any).
type StringsWatchResult struct {
	StringsWatcherId string   `json:"watcher-id"`
	Changes          []string `json:"changes,omitempty"`
	Error            *Error   `json:"error,omitempty"`
}

// StringsWatchResults holds the results for any API call which ends up
// returning a list of StringsWatchers.
type StringsWatchResults struct {
	Results []StringsWatchResult `json:"results"`
}

// EntitiesWatchResult holds a EntitiesWatcher id, changes and an error
// (if any).
type EntitiesWatchResult struct {
	// Note legacy serialization tag.
	EntitiesWatcherId string   `json:"watcher-id"`
	Changes           []string `json:"changes,omitempty"`
	Error             *Error   `json:"error,omitempty"`
}

// EntitiesWatchResults holds the results for any API call which ends up
// returning a list of EntitiesWatchers.
type EntitiesWatchResults struct {
	Results []EntitiesWatchResult `json:"results"`
}

// UnitSettings specifies the version of some unit's settings in some relation.
type UnitSettings struct {
	Version int64 `json:"version"`
}

// RelationUnitsChange describes the membership and settings of; or changes to;
// some relation scope.
type RelationUnitsChange struct {

	// Changed holds a set of units that are known to be in scope, and the
	// latest known settings version for each.
	Changed map[string]UnitSettings `json:"changed"`

	// Changed holds the versions of each application data for applications related
	// to this unit.
	AppChanged map[string]int64 `json:"app-changed,omitempty"`

	// Departed holds a set of units that have previously been reported to
	// be in scope, but which no longer are.
	Departed []string `json:"departed,omitempty"`
}

// RelationUnitsWatchResult holds a RelationUnitsWatcher id, baseline state
// (in the Changes field), and an error (if any).
type RelationUnitsWatchResult struct {
	RelationUnitsWatcherId string              `json:"watcher-id"`
	Changes                RelationUnitsChange `json:"changes"`
	Error                  *Error              `json:"error,omitempty"`
}

// RelationUnitsWatchResults holds the results for any API call which ends up
// returning a list of RelationUnitsWatchers.
type RelationUnitsWatchResults struct {
	Results []RelationUnitsWatchResult `json:"results"`
}

// RelationUnitStatus holds details about scope
// and suspended status for a relation unit.
type RelationUnitStatus struct {
	RelationTag string `json:"relation-tag"`
	InScope     bool   `json:"in-scope"`
	Suspended   bool   `json:"suspended"`
}

// RelationUnitStatusResult holds details about scope and status for
// relation units, and an error.
type RelationUnitStatusResult struct {
	RelationResults []RelationUnitStatus `json:"results"`
	Error           *Error               `json:"error,omitempty"`
}

// RelationUnitStatusResults holds the results of a
// uniter RelationStatus API call.
type RelationUnitStatusResults struct {
	Results []RelationUnitStatusResult `json:"results"`
}

// RelationApplications holds a set of pairs of relation & application
// tags.
type RelationApplications struct {
	RelationApplications []RelationApplication `json:"relation-applications"`
}

// RelationApplication holds one (relation, application) pair.
type RelationApplication struct {
	Relation    string `json:"relation"`
	Application string `json:"application"`
}

// MachineStorageIdsWatchResult holds a MachineStorageIdsWatcher id,
// changes and an error (if any).
type MachineStorageIdsWatchResult struct {
	MachineStorageIdsWatcherId string             `json:"watcher-id"`
	Changes                    []MachineStorageId `json:"changes"`
	Error                      *Error             `json:"error,omitempty"`
}

// MachineStorageIdsWatchResults holds the results for any API call which ends
// up returning a list of MachineStorageIdsWatchers.
type MachineStorageIdsWatchResults struct {
	Results []MachineStorageIdsWatchResult `json:"results"`
}

// CharmsResponse is the server response to charm upload or GET requests.
type CharmsResponse struct {
	Error string `json:"error,omitempty"`

	// ErrorCode holds the code associated with the error.
	// Ideally, Error would hold an Error object and the
	// code would be in that, but for backward compatibility,
	// we cannot do that.
	ErrorCode string `json:"error-code,omitempty"`

	// ErrorInfo holds extra information associated with the error.
	ErrorInfo map[string]interface{} `json:"error-info,omitempty"`

	CharmURL string   `json:"charm-url,omitempty"`
	Files    []string `json:"files,omitempty"`
}

// RunParams is used to provide the parameters to the Run method.
// Commands and Timeout are expected to have values, and one or more
// values should be in the Machines, Applications, or Units slices.
type RunParams struct {
	Commands       string        `json:"commands"`
	Timeout        time.Duration `json:"timeout"`
	Machines       []string      `json:"machines,omitempty"`
	Applications   []string      `json:"applications,omitempty"`
	Units          []string      `json:"units,omitempty"`
	Parallel       *bool         `json:"parallel,omitempty"`
	ExecutionGroup *string       `json:"execution-group,omitempty"`
}

// RunResult contains the result from an individual run call on a machine.
// UnitId is populated if the command was run inside the unit context.
type RunResult struct {
	Code   int    `json:"code-id"`
	Stdout []byte `json:"stdout,omitempty"`
	Stderr []byte `json:"stderr,omitempty"`
	// FIXME: should be tags not id strings
	MachineId string `json:"machine-id"`
	UnitId    string `json:"unit-id"`
	Error     string `json:"error"`
}

// RunResults is used to return the slice of results.  API server side calls
// need to return single structure values.
type RunResults struct {
	Results []RunResult `json:"results"`
}

// AgentVersionResult is used to return the current version number of the
// agent running the API server.
type AgentVersionResult struct {
	Version semversion.Number `json:"version"`
}

// RetryProvisioningArgs holds args for retrying machine provisioning.
type RetryProvisioningArgs struct {
	Machines []string `json:"machines,omitempty"`
	All      bool     `json:"all"`
}

// ProvisioningNetworkTopology holds a network topology that is based on
// positive machine space constraints.
// This is used for creating NICs on instances where the provider is not space
// aware; I.e. not MAAS.
// We only care about positive constraints because negative constraints are
// satisfied implicitly by only creating NICs connected to subnets in inclusive
// spaces.
type ProvisioningNetworkTopology struct {
	// SubnetAZs is a map of availability zone names
	// indexed by provider subnet ID.
	SubnetAZs map[string][]string `json:"subnet-zones"`

	// SpaceSubnets is a map of subnet provider IDs from the map above
	// indexed by the space ID that the subnets reside in.
	SpaceSubnets map[string][]string `json:"space-subnets"`
}

// ProvisioningInfo holds machine provisioning info.
type ProvisioningInfo struct {
	Constraints       constraints.Value        `json:"constraints"`
	Base              Base                     `json:"base"`
	Placement         string                   `json:"placement"`
	Jobs              []model.MachineJob       `json:"jobs"`
	RootDisk          *VolumeParams            `json:"root-disk,omitempty"`
	Volumes           []VolumeParams           `json:"volumes,omitempty"`
	VolumeAttachments []VolumeAttachmentParams `json:"volume-attachments,omitempty"`
	Tags              map[string]string        `json:"tags,omitempty"`
	ImageMetadata     []CloudImageMetadata     `json:"image-metadata,omitempty"`
	EndpointBindings  map[string]string        `json:"endpoint-bindings,omitempty"`
	ControllerConfig  map[string]interface{}   `json:"controller-config,omitempty"`
	CloudInitUserData map[string]interface{}   `json:"cloudinit-userdata,omitempty"`
	CharmLXDProfiles  []string                 `json:"charm-lxd-profiles,omitempty"`

	ProvisioningNetworkTopology
}

// ProvisioningInfoResult holds machine provisioning info or an error.
type ProvisioningInfoResult struct {
	Result *ProvisioningInfo `json:"result"`
	Error  *Error            `json:"error,omitempty"`
}

// ProvisioningInfoResults holds multiple machine provisioning info results.
type ProvisioningInfoResults struct {
	Results []ProvisioningInfoResult `json:"results"`
}

// SingularClaim represents a request for exclusive administrative access
// to an entity (model or controller) on the part of the claimant.
type SingularClaim struct {
	EntityTag   string        `json:"entity-tag"`
	ClaimantTag string        `json:"claimant-tag"`
	Duration    time.Duration `json:"duration"`
}

// SingularClaims holds any number of SingularClaim~s.
type SingularClaims struct {
	Claims []SingularClaim `json:"claims"`
}

// LogMessage is a structured logging entry.
// It is used to stream log records to the log streamer client
// from the api server /logs endpoint.
// The client is used for model migration and debug-log.
type LogMessage struct {
	ModelUUID string            `json:"uuid,omitempty"`
	Entity    string            `json:"tag"`
	Timestamp time.Time         `json:"ts"`
	Severity  string            `json:"sev"`
	Module    string            `json:"mod"`
	Location  string            `json:"loc"`
	Message   string            `json:"msg"`
	Labels    map[string]string `json:"lab,omitempty"`
}

// LogMessageV1 is a structured logging entry
// for older clients expecting an array of labels.
type LogMessageV1 struct {
	Entity    string    `json:"tag"`
	Timestamp time.Time `json:"ts"`
	Severity  string    `json:"sev"`
	Module    string    `json:"mod"`
	Location  string    `json:"loc"`
	Message   string    `json:"msg"`
	Labels    []string  `json:"lab"`
}

type logMessageJSON struct {
	ModelUUID string    `json:"uuid,omitempty"`
	Entity    string    `json:"tag"`
	Timestamp time.Time `json:"ts"`
	Severity  string    `json:"sev"`
	Module    string    `json:"mod"`
	Location  string    `json:"loc"`
	Message   string    `json:"msg"`
	Labels    any       `json:"lab,omitempty"`
}

// UnmarshalJSON unmarshalls an incoming log message
// in either v1 or later format.
func (m *LogMessage) UnmarshalJSON(data []byte) error {
	var jm logMessageJSON
	if err := json.Unmarshal(data, &jm); err != nil {
		return errors.Trace(err)
	}
	m.ModelUUID = jm.ModelUUID
	m.Timestamp = jm.Timestamp
	m.Entity = jm.Entity
	m.Severity = jm.Severity
	m.Module = jm.Module
	m.Location = jm.Location
	m.Message = jm.Message
	m.Labels = unmarshallLogLabels(jm.Labels)
	return nil
}

func unmarshallLogLabels(in any) map[string]string {
	var result map[string]string
	switch lab := in.(type) {
	case []any:
		if len(lab) > 0 {
			out := make([]string, len(lab))
			for i, v := range lab {
				out[i] = fmt.Sprint(v)
			}
			result = map[string]string{
				loggo.LoggerTags: strings.Join(out, ","),
			}
		}
	case map[string]any:
		result = map[string]string{}
		for k, v := range lab {
			result[k] = fmt.Sprint(v)
		}
	default:
		// Either missing or not supported.
	}
	return result
}

// ResourceUploadResult is used to return some details about an
// uploaded resource.
type ResourceUploadResult struct {
	// Error will contain details about a failed upload attempt.
	Error *Error `json:"error,omitempty"`

	// ID uniquely identifies a resource-application pair within the model.
	ID string `json:"id"`

	// Timestamp indicates when the resource was added to the model.
	Timestamp time.Time `json:"timestamp"`
}

// UnitRefreshResult is used to return the latest values for attributes
// on a unit.
type UnitRefreshResult struct {
	Life       life.Value
	Resolved   ResolvedMode
	Error      *Error
	ProviderID string `json:"provider-id,omitempty"`
}

// UnitRefreshResults holds the results for any API call which ends
// up returning a list of UnitRefreshResult.
type UnitRefreshResults struct {
	Results []UnitRefreshResult
}

// EntityString holds an entity tag and a string value.
type EntityString struct {
	Tag   string `json:"tag"`
	Value string `json:"value"`
}

// GoalStateResults holds the results of GoalStates API call
type GoalStateResults struct {
	Results []GoalStateResult `json:"results"`
}

// GoalStateResult the result of GoalStates per entity.
type GoalStateResult struct {
	Result *GoalState `json:"result"`
	Error  *Error     `json:"error"`
}

// GoalStateStatus goal-state at unit level
type GoalStateStatus struct {
	Status string     `json:"status"`
	Since  *time.Time `json:"since"`
}

// UnitsGoalState collection of GoalStatesStatus with unit name
type UnitsGoalState map[string]GoalStateStatus

// GoalState goal-state at application level, stores Units and Units-Relations
type GoalState struct {
	Units     UnitsGoalState            `json:"units"`
	Relations map[string]UnitsGoalState `json:"relations"`
}

// ContainerTypeResult holds the result of a machine's ContainerType.
type ContainerTypeResult struct {
	Type  instance.ContainerType `json:"container-type"`
	Error *Error                 `json:"error"`
}
