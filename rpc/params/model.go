// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"time"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/semversion"
)

// ConfigValue encapsulates a configuration
// value and its source.
type ConfigValue struct {
	Value  interface{} `json:"value"`
	Source string      `json:"source"`
}

// ModelConfigResults contains the result of client API calls
// to get model config values.
type ModelConfigResults struct {
	Config map[string]ConfigValue `json:"config"`
}

// HostedModelConfig contains the model config and the cloud spec
// for the model, both things that a client needs to talk directly
// with the provider. This is used to take down mis-behaving models
// aggressively.
type HostedModelConfig struct {
	Name      string                 `json:"name"`
	Qualifier string                 `json:"qualifier"`
	Config    map[string]interface{} `json:"config,omitempty"`
	CloudSpec *CloudSpec             `json:"cloud-spec,omitempty"`
	Error     *Error                 `json:"error,omitempty"`
}

// HostedModelConfigsResults contains an entry for each hosted model
// in the controller.
type HostedModelConfigsResults struct {
	Models []HostedModelConfig `json:"models"`
}

// ModelDefaultsResults contains the result of client API calls to get the
// model default values.
type ModelDefaultsResults struct {
	Results []ModelDefaultsResult `json:"results"`
}

// ModelDefaultsResult contains the result of client API calls to get the
// model default values.
type ModelDefaultsResult struct {
	Config map[string]ModelDefaults `json:"config"`
	Error  *Error                   `json:"error,omitempty"`
}

// ModelSequencesResult holds the map of sequence names to next value.
type ModelSequencesResult struct {
	Sequences map[string]int `json:"sequences"`
}

// ModelDefaults holds the settings for a given ModelDefaultsResult config
// attribute.
type ModelDefaults struct {
	Default    interface{}      `json:"default,omitempty"`
	Controller interface{}      `json:"controller,omitempty"`
	Regions    []RegionDefaults `json:"regions,omitempty"`
}

// RegionDefaults contains the settings for regions in a ModelDefaults.
type RegionDefaults struct {
	RegionName string      `json:"region-name"`
	Value      interface{} `json:"value"`
}

// ModelSet contains the arguments for ModelSet client API
// call.
type ModelSet struct {
	Config map[string]interface{} `json:"config"`
}

// ModelUnset contains the arguments for ModelUnset client API
// call.
type ModelUnset struct {
	Keys []string `json:"keys"`
}

// SetModelDefaults contains the arguments for SetModelDefaults
// client API call.
type SetModelDefaults struct {
	Config []ModelDefaultValues `json:"config"`
}

// ModelDefaultValues contains the default model values for
// a cloud/region.
type ModelDefaultValues struct {
	CloudTag    string                 `json:"cloud-tag,omitempty"`
	CloudRegion string                 `json:"cloud-region,omitempty"`
	Config      map[string]interface{} `json:"config"`
}

// ModelUnsetKeys contains the config keys to unset for
// a cloud/region.
type ModelUnsetKeys struct {
	CloudTag    string   `json:"cloud-tag,omitempty"`
	CloudRegion string   `json:"cloud-region,omitempty"`
	Keys        []string `json:"keys"`
}

// UnsetModelDefaults contains the arguments for UnsetModelDefaults
// client API call.
type UnsetModelDefaults struct {
	Keys []ModelUnsetKeys `json:"keys"`
}

// SetModelAgentVersion contains the arguments for
// SetModelAgentVersion client API call.
type SetModelAgentVersion struct {
	Version             semversion.Number `json:"version"`
	AgentStream         string            `json:"agent-stream,omitempty"`
	IgnoreAgentVersions bool              `json:"force,omitempty"`
}

// ModelMigrationStatus holds information about the progress of a (possibly
// failed) migration.
type ModelMigrationStatus struct {
	Status string     `json:"status"`
	Start  *time.Time `json:"start"`
	End    *time.Time `json:"end,omitempty"`
}

// ModelInfo holds information about the Juju model.
type ModelInfo struct {
	Name               string `json:"name"`
	Type               string `json:"type"`
	UUID               string `json:"uuid"`
	ControllerUUID     string `json:"controller-uuid"`
	IsController       bool   `json:"is-controller"`
	ProviderType       string `json:"provider-type,omitempty"`
	CloudTag           string `json:"cloud-tag"`
	CloudRegion        string `json:"cloud-region,omitempty"`
	CloudCredentialTag string `json:"cloud-credential-tag,omitempty"`

	// CloudCredentialValidity contains if model credential is valid, if known.
	CloudCredentialValidity *bool `json:"cloud-credential-validity,omitempty"`

	// Qualifier disambiguates the model name.
	Qualifier string `json:"qualifier"`

	// Life is the current lifecycle state of the model.
	Life life.Value `json:"life"`

	// Status is the current status of the model.
	Status EntityStatus `json:"status,omitempty"`

	// Users contains information about the users that have access
	// to the model. Owners and administrators can see all users
	// that have access; other users can only see their own details.
	Users []ModelUserInfo `json:"users"`

	// Machines contains information about the machines in the model.
	// This information is available to owners and users with write
	// access or greater.
	Machines []ModelMachineInfo `json:"machines"`

	// SecretBackends contains information about the secret backends
	// currently in use by the model.
	SecretBackends []SecretBackendResult `json:"secret-backends"`

	// Migration contains information about the latest failed or
	// currently-running migration. It'll be nil if there isn't one.
	Migration *ModelMigrationStatus `json:"migration,omitempty"`

	// AgentVersion is the agent version for this model.
	AgentVersion *semversion.Number `json:"agent-version"`

	// SupportedFeatures provides information about the set of features
	// supported by this model. The feature set contains both Juju-specific
	// entries (e.g. juju version) and other features that depend on the
	// substrate the model is deployed to.
	SupportedFeatures []SupportedFeature `json:"supported-features,omitempty"`
}

// SupportedFeature describes a feature that is supported by a particular model.
type SupportedFeature struct {
	Name        string `json:"name"`
	Description string `json:"description"`

	// Version is optional; some features might simply be booleans with
	// no particular version attached.
	Version string `json:"version,omitempty"`
}

// ModelSummary holds summary about a Juju model.
type ModelSummary struct {
	Name               string `json:"name"`
	Qualifier          string `json:"qualifier"`
	UUID               string `json:"uuid"`
	Type               string `json:"type"`
	ControllerUUID     string `json:"controller-uuid"`
	IsController       bool   `json:"is-controller"`
	ProviderType       string `json:"provider-type,omitempty"`
	CloudTag           string `json:"cloud-tag"`
	CloudRegion        string `json:"cloud-region,omitempty"`
	CloudCredentialTag string `json:"cloud-credential-tag,omitempty"`

	// Life is the current lifecycle state of the model.
	Life life.Value `json:"life"`

	// Status is the current status of the model.
	Status EntityStatus `json:"status,omitempty"`

	// UserAccess is model access level for the  current user.
	UserAccess UserAccessPermission `json:"user-access"`

	// UserLastConnection contains the time when current user logged in
	// into the model last.
	UserLastConnection *time.Time `json:"last-connection"`

	// Counts contains counts of interesting entities
	// in the model, for example machines, cores, containers, units, etc.
	Counts []ModelEntityCount `json:"counts"`

	// Migration contains information about the latest failed or
	// currently-running migration. It'll be nil if there isn't one.
	Migration *ModelMigrationStatus `json:"migration,omitempty"`

	// AgentVersion is the agent version for this model.
	AgentVersion *semversion.Number `json:"agent-version"`
}

// ModelEntityCount represent a count for a model entity where entities could be
// machines, units, etc...
type ModelEntityCount struct {
	Entity CountedEntity `json:"entity"`
	Count  int64         `json:"count"`
}

// CountedEntity identifies an entity that has a count.
type CountedEntity string

const (
	Machines CountedEntity = "machines"
	Cores    CountedEntity = "cores"
	Units    CountedEntity = "units"
)

// ModelSummaryResult holds the result of a ListModelsWithInfo call.
type ModelSummaryResult struct {
	Result *ModelSummary `json:"result,omitempty"`
	Error  *Error        `json:"error,omitempty"`
}

// ModelSummaryResults holds the result of a bulk ListModelsWithInfo call.
type ModelSummaryResults struct {
	Results []ModelSummaryResult `json:"results"`
}

// ModelSummariesRequest encapsulates how we request a list of model summaries.
type ModelSummariesRequest struct {
	UserTag string `json:"user-tag"`
	All     bool   `json:"all,omitempty"`
}

// ModelInfoResult holds the result of a ModelInfo call.
type ModelInfoResult struct {
	Result *ModelInfo `json:"result,omitempty"`
	Error  *Error     `json:"error,omitempty"`
}

// ModelInfoResults holds the result of a bulk ModelInfo call.
type ModelInfoResults struct {
	Results []ModelInfoResult `json:"results"`
}

// ModelInfoList holds a list of ModelInfo structures.
type ModelInfoList struct {
	Models []ModelInfo `json:"models,omitempty"`
}

// ModelInfoListResult holds the result of a call that returns a list
// of ModelInfo structures.
type ModelInfoListResult struct {
	Result *ModelInfoList `json:"result,omitempty"`
	Error  *Error         `json:"error,omitempty"`
}

// ModelInfoListResults holds the result of a bulk call that returns
// multiple lists of ModelInfo structures.
type ModelInfoListResults struct {
	Results []ModelInfoListResult `json:"results"`
}

// ModelMachineInfo holds information about a machine in a model.
type ModelMachineInfo struct {
	Id          string           `json:"id"`
	Hardware    *MachineHardware `json:"hardware,omitempty"`
	InstanceId  string           `json:"instance-id,omitempty"`
	DisplayName string           `json:"display-name,omitempty"`
	Status      string           `json:"status,omitempty"`
	Message     string           `json:"message,omitempty"`
}

// ModelApplicationInfo holds information about an application in a model.
type ModelApplicationInfo struct {
	Name string `json:"name"`
}

// MachineHardware holds information about a machine's hardware characteristics.
type MachineHardware struct {
	Arch             *string   `json:"arch,omitempty"`
	Mem              *uint64   `json:"mem,omitempty"`
	RootDisk         *uint64   `json:"root-disk,omitempty"`
	Cores            *uint64   `json:"cores,omitempty"`
	CpuPower         *uint64   `json:"cpu-power,omitempty"`
	Tags             *[]string `json:"tags,omitempty"`
	AvailabilityZone *string   `json:"availability-zone,omitempty"`
	VirtType         *string   `json:"virt-type,omitempty"`
}

// ModelVolumeInfo holds information about a volume in a model.
type ModelVolumeInfo struct {
	Id         string `json:"id"`
	ProviderId string `json:"provider-id,omitempty"`
	Status     string `json:"status,omitempty"`
	Message    string `json:"message,omitempty"`
	Detachable bool   `json:"detachable,omitempty"`
}

// ModelFilesystemInfo holds information about a filesystem in a model.
type ModelFilesystemInfo struct {
	Id         string `json:"id"`
	ProviderId string `json:"provider-id,omitempty"`
	Status     string `json:"status,omitempty"`
	Message    string `json:"message,omitempty"`
	Detachable bool   `json:"detachable,omitempty"`
}

// ModelUserInfo holds information on a user who has access to a
// model. Owners of a model can see this information for all users
// who have access, so it should not include sensitive information.
type ModelUserInfo struct {
	ModelTag       string               `json:"model-tag"`
	UserName       string               `json:"user"`
	DisplayName    string               `json:"display-name"`
	LastConnection *time.Time           `json:"last-connection"`
	Access         UserAccessPermission `json:"access"`
}

// ModelUserInfoResult holds the result of an ModelUserInfo call.
type ModelUserInfoResult struct {
	Result *ModelUserInfo `json:"result,omitempty"`
	Error  *Error         `json:"error,omitempty"`
}

// ModelUserInfoResults holds the result of a bulk ModelUserInfo API call.
type ModelUserInfoResults struct {
	Results []ModelUserInfoResult `json:"results"`
}

// ModifyModelAccessRequest holds the parameters for making grant and revoke model calls.
type ModifyModelAccessRequest struct {
	Changes []ModifyModelAccess `json:"changes"`
}

type ModifyModelAccess struct {
	UserTag  string               `json:"user-tag"`
	Action   ModelAction          `json:"action"`
	Access   UserAccessPermission `json:"access"`
	ModelTag string               `json:"model-tag"`
}

// ModelAction is an action that can be performed on a model.
type ModelAction string

// Actions that can be preformed on a model.
const (
	GrantModelAccess  ModelAction = "grant"
	RevokeModelAccess ModelAction = "revoke"
)

// UserAccessPermission is the type of permission that a user has to access a
// model.
type UserAccessPermission string

// Model access permissions that may be set on a user.
const (
	ModelAdminAccess UserAccessPermission = "admin"
	ModelReadAccess  UserAccessPermission = "read"
	ModelWriteAccess UserAccessPermission = "write"
)

// DestroyModelsParams holds the arguments for destroying models.
type DestroyModelsParams struct {
	Models []DestroyModelParams `json:"models"`
}

// DestroyModelParams holds the arguments for destroying a model.
type DestroyModelParams struct {
	// ModelTag is the tag of the model to destroy.
	ModelTag string `json:"model-tag"`

	// DestroyStorage controls whether or not storage in the model.
	//
	// This is ternary: nil, false, or true. If nil and there is persistent
	// storage in the model, an error with the code
	// params.CodeHasPersistentStorage will be returned.
	DestroyStorage *bool `json:"destroy-storage,omitempty"`

	// Force specifies whether model destruction will be forced, i.e.
	// keep going despite operational errors.
	Force *bool `json:"force,omitempty"`

	// MaxWait specifies the amount of time that each step in model destroy process
	// will wait before forcing the next step to kick-off. This parameter
	// only makes sense in combination with 'force' set to 'true'.
	MaxWait *time.Duration `json:"max-wait,omitempty"`

	// Timeout specifies how long to wait for the entire destroy process before
	// timing out.
	Timeout *time.Duration `json:"timeout,omitempty"`
}

// ModelCredential stores information about cloud credential that a model uses:
// what credential is being used, is it valid for this model, etc.
type ModelCredential struct {
	// Model is a tag for the model.
	Model string `json:"model-tag"`

	// Exists indicates whether credential was set on the model.
	// It is valid for model not to have a credential if it is on the
	// cloud that does not require auth.
	Exists bool `json:"exists,omitempty"`

	// CloudCredential is the tag for the cloud credential that the model uses.
	CloudCredential string `json:"credential-tag"`

	// Valid stores whether this credential is valid, for example, not expired,
	// and whether this credential works for this model, i.e. all model
	// machines can be accessed with this credential.
	Valid bool `json:"valid,omitempty"`
}

// ChangeModelCredentialParams holds the argument to replace cloud credential
// used by a model.
type ChangeModelCredentialParams struct {
	// ModelTag is a tag for the model where cloud credential change takes place.
	ModelTag string `json:"model-tag"`

	// CloudCredentialTag is the tag for the new cloud credential.
	CloudCredentialTag string `json:"credential-tag"`
}

// ChangeModelCredentialsParams holds the arguments for changing
// cloud credentials on models.
type ChangeModelCredentialsParams struct {
	Models []ChangeModelCredentialParams `json:"model-credentials"`
}

// ValidateModelUpgradeParams is used to ensure that a model can be upgraded.
type ValidateModelUpgradeParams struct {
	Models []ModelParam `json:"model"`
	Force  bool         `json:"force"`
}

// ModelParam is used to identify a model.
type ModelParam struct {
	// ModelTag is a tag for the model.
	ModelTag string `json:"model-tag"`
}

// UpgradeModel contains the arguments for UpgradeModel API call.
type UpgradeModelParams struct {
	ModelTag            string            `json:"model-tag"`
	TargetVersion       semversion.Number `json:"target-version"`
	AgentStream         string            `json:"agent-stream,omitempty"`
	IgnoreAgentVersions bool              `json:"ignore-agent-versions,omitempty"`
	DryRun              bool              `json:"dry-run,omitempty"`
}

// UpgradeModelResult holds the result of a UpgradeModel API call.
type UpgradeModelResult struct {
	ChosenVersion semversion.Number `json:"chosen-version"`
	Error         *Error            `json:"error,omitempty"`
}
