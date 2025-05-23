// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"time"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/semversion"
)

// ModelBlockInfoLegacy holds information about a model and its
// current blocks.
type ModelBlockInfoLegacy struct {
	Name     string   `json:"name"`
	UUID     string   `json:"model-uuid"`
	OwnerTag string   `json:"owner-tag"`
	Blocks   []string `json:"blocks"`
}

// ModelBlockInfoListLegacy holds information about the blocked models
// for a controller.
type ModelBlockInfoListLegacy struct {
	Models []ModelBlockInfoLegacy `json:"models,omitempty"`
}

// ModelStatusLegacy holds information about the status of a juju model.
type ModelStatusLegacy struct {
	ModelTag           string                 `json:"model-tag"`
	Life               life.Value             `json:"life"`
	Type               string                 `json:"type"`
	HostedMachineCount int                    `json:"hosted-machine-count"`
	ApplicationCount   int                    `json:"application-count"`
	UnitCount          int                    `json:"unit-count"`
	OwnerTag           string                 `json:"owner-tag"`
	Applications       []ModelApplicationInfo `json:"applications,omitempty"`
	Machines           []ModelMachineInfo     `json:"machines,omitempty"`
	Volumes            []ModelVolumeInfo      `json:"volumes,omitempty"`
	Filesystems        []ModelFilesystemInfo  `json:"filesystems,omitempty"`
	Error              *Error                 `json:"error,omitempty"`
}

// ModelStatusResultsLegacy holds status information about a group of models.
type ModelStatusResultsLegacy struct {
	Results []ModelStatusLegacy `json:"models"`
}

// ModelLegacy holds the result of an API call returning a name and UUID
// for a model and the tag of the server in which it is running.
type ModelLegacy struct {
	Name     string `json:"name"`
	UUID     string `json:"uuid"`
	Type     string `json:"type"`
	OwnerTag string `json:"owner-tag"`
}

// UserModelLegacy holds information about a model and the last
// time the model was accessed for a particular user.
type UserModelLegacy struct {
	ModelLegacy    `json:"model"`
	LastConnection *time.Time `json:"last-connection"`
}

// UserModelListLegacy holds information about a list of models
// for a particular user.
type UserModelListLegacy struct {
	UserModels []UserModelLegacy `json:"user-models"`
}

// ModelCreateArgsLegacy holds the arguments that are necessary to create
// a model.
type ModelCreateArgsLegacy struct {
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

// MigrationModelInfoLegacy is used to report basic model information to the
// migrationmaster worker.
type MigrationModelInfoLegacy struct {
	UUID                   string            `json:"uuid"`
	Name                   string            `json:"name"`
	OwnerTag               string            `json:"owner-tag"`
	AgentVersion           semversion.Number `json:"agent-version"`
	ControllerAgentVersion semversion.Number `json:"controller-agent-version"`
	FacadeVersions         map[string][]int  `json:"facade-versions,omitempty"`
	ModelDescription       []byte            `json:"model-description,omitempty"`
}

// HostedModelConfigLegacy contains the model config and the cloud spec
// for the model, both things that a client needs to talk directly
// with the provider. This is used to take down mis-behaving models
// aggressively.
type HostedModelConfigLegacy struct {
	Name      string                 `json:"name"`
	OwnerTag  string                 `json:"owner"`
	Config    map[string]interface{} `json:"config,omitempty"`
	CloudSpec *CloudSpec             `json:"cloud-spec,omitempty"`
	Error     *Error                 `json:"error,omitempty"`
}

// HostedModelConfigsResultsLegacy contains an entry for each hosted model
// in the controller.
type HostedModelConfigsResultsLegacy struct {
	Models []HostedModelConfigLegacy `json:"models"`
}

// ModelInfoLegacy holds information about the Juju model.
type ModelInfoLegacy struct {
	Name                    string                `json:"name"`
	Type                    string                `json:"type"`
	UUID                    string                `json:"uuid"`
	ControllerUUID          string                `json:"controller-uuid"`
	IsController            bool                  `json:"is-controller"`
	ProviderType            string                `json:"provider-type,omitempty"`
	CloudTag                string                `json:"cloud-tag"`
	CloudRegion             string                `json:"cloud-region,omitempty"`
	CloudCredentialTag      string                `json:"cloud-credential-tag,omitempty"`
	CloudCredentialValidity *bool                 `json:"cloud-credential-validity,omitempty"`
	OwnerTag                string                `json:"owner-tag"`
	Life                    life.Value            `json:"life"`
	Status                  EntityStatus          `json:"status,omitempty"`
	Users                   []ModelUserInfo       `json:"users"`
	Machines                []ModelMachineInfo    `json:"machines"`
	SecretBackends          []SecretBackendResult `json:"secret-backends"`
	Migration               *ModelMigrationStatus `json:"migration,omitempty"`
	AgentVersion            *semversion.Number    `json:"agent-version"`
	SupportedFeatures       []SupportedFeature    `json:"supported-features,omitempty"`
}

// ModelSummaryLegacy holds summary about a Juju model.
type ModelSummaryLegacy struct {
	Name               string                `json:"name"`
	UUID               string                `json:"uuid"`
	Type               string                `json:"type"`
	ControllerUUID     string                `json:"controller-uuid"`
	IsController       bool                  `json:"is-controller"`
	ProviderType       string                `json:"provider-type,omitempty"`
	CloudTag           string                `json:"cloud-tag"`
	CloudRegion        string                `json:"cloud-region,omitempty"`
	CloudCredentialTag string                `json:"cloud-credential-tag,omitempty"`
	OwnerTag           string                `json:"owner-tag"`
	Life               life.Value            `json:"life"`
	Status             EntityStatus          `json:"status,omitempty"`
	UserAccess         UserAccessPermission  `json:"user-access"`
	UserLastConnection *time.Time            `json:"last-connection"`
	Counts             []ModelEntityCount    `json:"counts"`
	Migration          *ModelMigrationStatus `json:"migration,omitempty"`
	AgentVersion       *semversion.Number    `json:"agent-version"`
}

// ModelSummaryResultLegacy holds the result of a ListModelsWithInfo call.
type ModelSummaryResultLegacy struct {
	Result *ModelSummaryLegacy `json:"result,omitempty"`
	Error  *Error              `json:"error,omitempty"`
}

// ModelSummaryResultsLegacy holds the result of a bulk ListModelsWithInfo call.
type ModelSummaryResultsLegacy struct {
	Results []ModelSummaryResultLegacy `json:"results"`
}

// ModelInfoResultLegacy holds the result of a ModelInfo call.
type ModelInfoResultLegacy struct {
	Result *ModelInfoLegacy `json:"result,omitempty"`
	Error  *Error           `json:"error,omitempty"`
}

// ModelInfoResultsLegacy holds the result of a bulk ModelInfo call.
type ModelInfoResultsLegacy struct {
	Results []ModelInfoResultLegacy `json:"results"`
}

// OfferFiltersLegacy is used to query offers.
// Offers matching any of the filters are returned.
type OfferFiltersLegacy struct {
	Filters []OfferFilterLegacy
}

// OfferFilterLegacy is used to query offers.
type OfferFilterLegacy struct {
	// OwnerName is the owner of the model hosting the offer.
	OwnerName string `json:"owner-name"`

	// ModelName is the name of the model hosting the offer.
	ModelName string `json:"model-name"`

	OfferName              string                     `json:"offer-name"`
	ApplicationName        string                     `json:"application-name"`
	ApplicationDescription string                     `json:"application-description"`
	ApplicationUser        string                     `json:"application-user"`
	Endpoints              []EndpointFilterAttributes `json:"endpoints"`
	ConnectedUserTags      []string                   `json:"connected-users"`
	AllowedConsumerTags    []string                   `json:"allowed-users"`
}
