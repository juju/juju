// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"time"

	"github.com/juju/version"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/status"
	"github.com/juju/juju/tools"
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

	// OwnerTag represents the user that will own the new model.
	// The OwnerTag must be a valid user tag.  If the user tag represents
	// a local user, that user must exist.
	OwnerTag string `json:"owner-tag"`

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
	Name     string `json:"name"`
	UUID     string `json:"uuid"`
	Type     string `json:"type"`
	OwnerTag string `json:"owner-tag"`
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

// ConfigSettings holds unit, application or cham configuration settings
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
// unit settings.
type RelationUnitSettings struct {
	Relation string   `json:"relation"`
	Unit     string   `json:"unit"`
	Settings Settings `json:"settings"`
}

// RelationUnitsSettings holds the arguments for making a EnterScope
// or WriteSettings API calls.
type RelationUnitsSettings struct {
	RelationUnits []RelationUnitSettings `json:"relation-units"`
}

// RelationResults holds the result of an API call that returns
// information about multiple relations.
type RelationResults struct {
	Results []RelationResult `json:"results"`
}

// RelationResult returns information about a single relation,
// or an error.
type RelationResult struct {
	Error            *Error                `json:"error,omitempty"`
	Life             Life                  `json:"life"`
	Suspended        bool                  `json:"bool,omitempty"`
	Id               int                   `json:"id"`
	Key              string                `json:"key"`
	Endpoint         multiwatcher.Endpoint `json:"endpoint"`
	OtherApplication string                `json:"other-application,omitempty"`
}

// RelationResultV5 returns information about a single relation,
// or an error, but doesn't include the other application name.
type RelationResultV5 struct {
	Error    *Error                `json:"error,omitempty"`
	Life     Life                  `json:"life"`
	Id       int                   `json:"id"`
	Key      string                `json:"key"`
	Endpoint multiwatcher.Endpoint `json:"endpoint"`
}

// RelationResultsV5 holds the result of an API call that returns
// information about multiple V5 relations.
type RelationResultsV5 struct {
	Results []RelationResultV5 `json:"results"`
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
	Life  Life   `json:"life"`
	Error *Error `json:"error,omitempty"`
}

// LifeResults holds the life or error status of multiple entities.
type LifeResults struct {
	Results []LifeResult `json:"results"`
}

// InstanceInfo holds a machine tag, provider-specific instance id, a nonce, and
// network config.
type InstanceInfo struct {
	Tag             string                            `json:"tag"`
	InstanceId      instance.Id                       `json:"instance-id"`
	Nonce           string                            `json:"nonce"`
	Characteristics *instance.HardwareCharacteristics `json:"characteristics"`
	Volumes         []Volume                          `json:"volumes"`
	// VolumeAttachments is a mapping from volume tag to
	// volume attachment info.
	VolumeAttachments map[string]VolumeAttachmentInfo `json:"volume-attachments"`

	NetworkConfig []NetworkConfig `json:"network-config"`
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
	Life          Life                      `json:"life"`
	Jobs          []multiwatcher.MachineJob `json:"jobs"`
	ContainerType instance.ContainerType    `json:"container-type"`
	Error         *Error                    `json:"error,omitempty"`
}

// VersionResult holds the version and possibly error for a given
// DesiredVersion() API call.
type VersionResult struct {
	Version *version.Number `json:"version,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// VersionResults is a list of versions for the requested entities.
type VersionResults struct {
	Results []VersionResult `json:"results"`
}

// SetModelEnvironVersions holds the tags and associated environ versions
// of a collection of models.
type SetModelEnvironVersions struct {
	Models []SetModelEnvironVersion `json:"models,omitempty"`
}

// SetModelEnvironVersions holds the tag and associated environ version
// of a model.
type SetModelEnvironVersion struct {
	// ModelTag is the string representation of a model tag, which
	// should be parseable using names.ParseModelTag.
	ModelTag string `json:"model-tag"`

	// Version is the environ version to set for the model.
	Version int `json:"version"`
}

// ToolsResult holds the tools and possibly error for a given
// Tools() API call.
type ToolsResult struct {
	ToolsList                      tools.List `json:"tools"`
	DisableSSLHostnameVerification bool       `json:"disable-ssl-hostname-verification"`
	Error                          *Error     `json:"error,omitempty"`
}

// ToolsResults is a list of tools for various requested agents.
type ToolsResults struct {
	Results []ToolsResult `json:"results"`
}

// Version holds a specific binary version.
type Version struct {
	Version version.Binary `json:"version"`
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

// RelationUnitStatusResult holds details about scope
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
	// Like ErrorCode, this should really be in an Error object.
	ErrorInfo *ErrorInfo `json:"error-info,omitempty"`

	CharmURL string   `json:"charm-url,omitempty"`
	Files    []string `json:"files,omitempty"`
}

// RunParams is used to provide the parameters to the Run method.
// Commands and Timeout are expected to have values, and one or more
// values should be in the Machines, Applications, or Units slices.
type RunParams struct {
	Commands     string        `json:"commands"`
	Timeout      time.Duration `json:"timeout"`
	Machines     []string      `json:"machines,omitempty"`
	Applications []string      `json:"applications,omitempty"`
	Units        []string      `json:"units,omitempty"`
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
	Version version.Number `json:"version"`
}

// ProvisioningInfo holds machine provisioning info.
type ProvisioningInfo struct {
	Constraints       constraints.Value         `json:"constraints"`
	Series            string                    `json:"series"`
	Placement         string                    `json:"placement"`
	Jobs              []multiwatcher.MachineJob `json:"jobs"`
	Volumes           []VolumeParams            `json:"volumes,omitempty"`
	VolumeAttachments []VolumeAttachmentParams  `json:"volume-attachments,omitempty"`
	Tags              map[string]string         `json:"tags,omitempty"`
	SubnetsToZones    map[string][]string       `json:"subnets-to-zones,omitempty"`
	ImageMetadata     []CloudImageMetadata      `json:"image-metadata,omitempty"`
	EndpointBindings  map[string]string         `json:"endpoint-bindings,omitempty"`
	ControllerConfig  map[string]interface{}    `json:"controller-config,omitempty"`
	CloudInitUserData map[string]interface{}    `json:"cloudinit-userdata,omitempty"`
}

// ProvisioningInfoResult holds machine provisioning info or an error.
type ProvisioningInfoResult struct {
	Error  *Error            `json:"error,omitempty"`
	Result *ProvisioningInfo `json:"result"`
}

// ProvisioningInfoResults holds multiple machine provisioning info results.
type ProvisioningInfoResults struct {
	Results []ProvisioningInfoResult `json:"results"`
}

// Metric holds a single metric.
type Metric struct {
	Key   string    `json:"key"`
	Value string    `json:"value"`
	Time  time.Time `json:"time"`
}

// MetricsParam contains the metrics for a single unit.
type MetricsParam struct {
	Tag     string   `json:"tag"`
	Metrics []Metric `json:"metrics"`
}

// MetricsParams contains the metrics for multiple units.
type MetricsParams struct {
	Metrics []MetricsParam `json:"metrics"`
}

// MetricBatch is a list of metrics with metadata.
type MetricBatch struct {
	UUID     string    `json:"uuid"`
	CharmURL string    `json:"charm-url"`
	Created  time.Time `json:"created"`
	Metrics  []Metric  `json:"metrics"`
}

// MetricBatchParam contains a single metric batch.
type MetricBatchParam struct {
	Tag   string      `json:"tag"`
	Batch MetricBatch `json:"batch"`
}

// MetricBatchParams contains multiple metric batches.
type MetricBatchParams struct {
	Batches []MetricBatchParam `json:"batches"`
}

// MeterStatusResult holds unit meter status or error.
type MeterStatusResult struct {
	Code  string `json:"code"`
	Info  string `json:"info"`
	Error *Error `json:"error,omitempty"`
}

// MeterStatusResults holds meter status results for multiple units.
type MeterStatusResults struct {
	Results []MeterStatusResult `json:"results"`
}

// SingularClaim represents a request for exclusive administrative access
// to an entity (model or controller) on the part of the claimaint.
type SingularClaim struct {
	EntityTag   string        `json:"entity-tag"`
	ClaimantTag string        `json:"claimant-tag"`
	Duration    time.Duration `json:"duration"`
}

// SingularClaims holds any number of SingularClaim~s.
type SingularClaims struct {
	Claims []SingularClaim `json:"claims"`
}

// GUIArchiveVersion holds information on a specific GUI archive version.
type GUIArchiveVersion struct {
	// Version holds the Juju GUI version number.
	Version version.Number `json:"version"`
	// SHA256 holds the SHA256 hash of the GUI tar.bz2 archive.
	SHA256 string `json:"sha256"`
	// Current holds whether this specific version is the current one served
	// by the controller.
	Current bool `json:"current"`
}

// GUIArchiveResponse holds the response to /gui-archive GET requests.
type GUIArchiveResponse struct {
	Versions []GUIArchiveVersion `json:"versions"`
}

// GUIVersionRequest holds the body for /gui-version PUT requests.
type GUIVersionRequest struct {
	// Version holds the Juju GUI version number.
	Version version.Number `json:"version"`
}

// LogMessage is a structured logging entry.
type LogMessage struct {
	Entity    string    `json:"tag"`
	Timestamp time.Time `json:"ts"`
	Severity  string    `json:"sev"`
	Module    string    `json:"mod"`
	Location  string    `json:"loc"`
	Message   string    `json:"msg"`
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
	Life     Life
	Resolved ResolvedMode
	Series   string
	Error    *Error
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

// SetPodSpecParams holds the arguments for setting the pod
// spec for a set of applications.
type SetPodSpecParams struct {
	Specs []EntityString `json:"specs"`
}
