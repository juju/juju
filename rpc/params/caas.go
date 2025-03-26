// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"github.com/juju/names/v6"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/resource"
	"github.com/juju/juju/internal/version"
)

// CAASUnitIntroductionArgs is used by sidecar units to introduce
// themselves via CAASApplication facade.
type CAASUnitIntroductionArgs struct {
	PodName string `json:"pod-name"`
	PodUUID string `json:"pod-uuid"`
}

// CAASUnitIntroduction contains the agent config for CAASApplication units.
type CAASUnitIntroduction struct {
	UnitName  string `json:"unit-name"`
	AgentConf []byte `json:"agent-conf"`
}

// CAASUnitIntroductionResult is returned from CAASApplication facade.
type CAASUnitIntroductionResult struct {
	Result *CAASUnitIntroduction `json:"result,omitempty"`
	Error  *Error                `json:"error,omitempty"`
}

// CAASApplicationProvisioningInfoResults holds OperatorProvisioningInfo results.
type CAASApplicationProvisioningInfoResults struct {
	Results []CAASApplicationProvisioningInfo `json:"results"`
}

// CAASUnitTerminationResult holds result to UnitTerminating call.
type CAASUnitTerminationResult struct {
	// WillRestart is true if the termination of the unit is temporary.
	WillRestart bool
	Error       *Error
}

// CAASApplicationProvisioningInfo holds info needed to provision a caas application.
type CAASApplicationProvisioningInfo struct {
	Version              version.Number               `json:"version"`
	APIAddresses         []string                     `json:"api-addresses"`
	CACert               string                       `json:"ca-cert"`
	Constraints          constraints.Value            `json:"constraints"`
	Tags                 map[string]string            `json:"tags,omitempty"`
	Filesystems          []KubernetesFilesystemParams `json:"filesystems,omitempty"`
	Volumes              []KubernetesVolumeParams     `json:"volumes,omitempty"`
	Devices              []KubernetesDeviceParams     `json:"devices,omitempty"`
	Base                 Base                         `json:"base,omitempty"`
	ImageRepo            DockerImageInfo              `json:"image-repo,omitempty"`
	CharmModifiedVersion int                          `json:"charm-modified-version,omitempty"`
	CharmURL             string                       `json:"charm-url,omitempty"`
	Trust                bool                         `json:"trust,omitempty"`
	Scale                int                          `json:"scale,omitempty"`
	Error                *Error                       `json:"error,omitempty"`
}

// DockerImageInfo holds the details for a Docker resource type.
type DockerImageInfo struct {
	// RegistryPath holds the path of the Docker image (including host and sha256) in a docker registry.
	RegistryPath string `json:"image-name"`

	// Username holds the username used to gain access to a non-public image.
	Username string `json:"username,omitempty"`

	// Password holds the password used to gain access to a non-public image.
	Password string `json:"password,omitempty"`

	// Auth is the base64 encoded "username:password" string.
	Auth string `json:"auth,omitempty" yaml:"auth,omitempty"`

	// IdentityToken is used to authenticate the user and get
	// an access token for the registry.
	IdentityToken string `json:"identitytoken,omitempty" yaml:"identitytoken,omitempty"`

	// RegistryToken is a bearer token to be sent to a registry
	RegistryToken string `json:"registrytoken,omitempty" yaml:"registrytoken,omitempty"`

	Email string `json:"email,omitempty" yaml:"email,omitempty"`

	// ServerAddress is the auth server address.
	ServerAddress string `json:"serveraddress,omitempty" yaml:"serveraddress,omitempty"`

	// Repository is the namespace of the image repo.
	Repository string `json:"repository,omitempty" yaml:"repository,omitempty"`
}

// NewDockerImageInfo converts docker.ImageRepoDetails to DockerImageInfo.
func NewDockerImageInfo(info resource.ImageRepoDetails, registryPath string) DockerImageInfo {
	return DockerImageInfo{
		Username:      info.Username,
		Password:      info.Password,
		Email:         info.Email,
		Repository:    info.Repository,
		Auth:          info.Auth.Content(),
		IdentityToken: info.IdentityToken.Content(),
		RegistryToken: info.RegistryToken.Content(),
		RegistryPath:  registryPath,
	}
}

// ConvertDockerImageInfo converts DockerImageInfo to resources.ImageRepoDetails.
func ConvertDockerImageInfo(info DockerImageInfo) resource.DockerImageDetails {
	return resource.DockerImageDetails{
		RegistryPath: info.RegistryPath,
		ImageRepoDetails: resource.ImageRepoDetails{
			Repository:    info.Repository,
			ServerAddress: info.ServerAddress,
			BasicAuthConfig: resource.BasicAuthConfig{
				Username: info.Username,
				Password: info.Password,
				Auth:     resource.NewToken(info.Auth),
			},
			TokenAuthConfig: resource.TokenAuthConfig{
				IdentityToken: resource.NewToken(info.IdentityToken),
				RegistryToken: resource.NewToken(info.RegistryToken),
				Email:         info.Email,
			},
		},
	}
}

// CAASApplicationOCIResourceResults holds all the image results for queried applications.
type CAASApplicationOCIResourceResults struct {
	Results []CAASApplicationOCIResourceResult `json:"results"`
}

// CAASApplicationOCIResourceResult holds the image result or error for the queried application.
type CAASApplicationOCIResourceResult struct {
	Result *CAASApplicationOCIResources `json:"result,omitempty"`
	Error  *Error                       `json:"error,omitempty"`
}

// CAASApplicationOCIResources holds a list of image OCI resources.
type CAASApplicationOCIResources struct {
	Images map[string]DockerImageInfo `json:"images"`
}

// CAASUnitInfo holds CAAS unit information.
type CAASUnitInfo struct {
	Tag        string      `json:"tag"`
	UnitStatus *UnitStatus `json:"unit-status,omitempty"`
}

// CAASUnit holds CAAS unit information.
type CAASUnit struct {
	Tag        names.Tag
	UnitStatus *UnitStatus
}

// CAASUnitsResult holds a slice of CAAS unit information or an error.
type CAASUnitsResult struct {
	Units []CAASUnitInfo `json:"units,omitempty"`
	Error *Error         `json:"error,omitempty"`
}

// CAASUnitsResults contains multiple CAAS units result.
type CAASUnitsResults struct {
	Results []CAASUnitsResult `json:"results"`
}

// CAASApplicationProvisioningState represents the provisioning state for a CAAS application.
type CAASApplicationProvisioningState struct {
	Scaling     bool `json:"scaling"`
	ScaleTarget int  `json:"scale-target"`
}

// CAASApplicationProvisioningStateResult represents the result of getting the
// provisioning state for a CAAS application.
type CAASApplicationProvisioningStateResult struct {
	ProvisioningState *CAASApplicationProvisioningState `json:"provisioning-state,omitempty"`
	Error             *Error                            `json:"error,omitempty"`
}

// CAASApplicationProvisioningStateArg holds the arguments for setting a CAAS application's
// provisioning state.
type CAASApplicationProvisioningStateArg struct {
	Application       Entity                           `json:"application"`
	ProvisioningState CAASApplicationProvisioningState `json:"provisioning-state"`
}

// CAASApplicationProvisionerConfig holds the configuration for the caasapplicationprovisioner worker.
type CAASApplicationProvisionerConfig struct {
	UnmanagedApplications Entities `json:"unmanaged-applications,omitempty"`
}

// CAASApplicationProvisionerConfigResult is the result of getting the caasapplicationprovisioner worker's
// configuration for the current model.
type CAASApplicationProvisionerConfigResult struct {
	ProvisionerConfig *CAASApplicationProvisionerConfig `json:"provisioner-config,omitempty"`
	Error             *Error                            `json:"error,omitempty"`
}
