// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/resources"
	"github.com/juju/juju/docker"
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

// Value describes a user's requirements of the hardware on which units
// of an application will run. Constraints are used to choose an existing machine
// onto which a unit will be deployed, or to provision a new machine if no
// existing one satisfies the requirements.
type Value struct {

	// Arch, if not nil or empty, indicates that a machine must run the named
	// architecture.
	Arch *string `json:"arch,omitempty" yaml:"arch,omitempty"`

	// Container, if not nil, indicates that a machine must be the specified container type.
	Container *instance.ContainerType `json:"container,omitempty" yaml:"container,omitempty"`

	// CpuCores, if not nil, indicates that a machine must have at least that
	// number of effective cores available.
	CpuCores *uint64 `json:"cores,omitempty" yaml:"cores,omitempty"`

	// CpuPower, if not nil, indicates that a machine must have at least that
	// amount of CPU power available, where 100 CpuPower is considered to be
	// equivalent to 1 Amazon ECU (or, roughly, a single 2007-era Xeon).
	CpuPower *uint64 `json:"cpu-power,omitempty" yaml:"cpu-power,omitempty"`

	// Mem, if not nil, indicates that a machine must have at least that many
	// megabytes of RAM.
	Mem *uint64 `json:"mem,omitempty" yaml:"mem,omitempty"`

	// RootDisk, if not nil, indicates that a machine must have at least
	// that many megabytes of disk space available in the root disk. In
	// providers where the root disk is configurable at instance startup
	// time, an instance with the specified amount of disk space in the OS
	// disk might be requested.
	RootDisk *uint64 `json:"root-disk,omitempty" yaml:"root-disk,omitempty"`

	// RootDiskSource, if specified, determines what storage the root
	// disk should be allocated from. This will be provider specific -
	// in the case of vSphere it identifies the datastore the root
	// disk file should be created in.
	RootDiskSource *string `json:"root-disk-source,omitempty" yaml:"root-disk-source,omitempty"`

	// Tags, if not nil, indicates tags that the machine must have applied to it.
	// An empty list is treated the same as a nil (unspecified) list, except an
	// empty list will override any default tags, where a nil list will not.
	Tags *[]string `json:"tags,omitempty" yaml:"tags,omitempty"`

	// InstanceRole, if not nil, indicates that the specified role/profile for
	// the given cloud should be used. Only valid for clouds which support
	// instance roles. Currently only for AWS with instance-profiles
	InstanceRole *string `json:"instance-role,omitempty" yaml:"instance-role,omitempty"`

	// InstanceType, if not nil, indicates that the specified cloud instance type
	// be used. Only valid for clouds which support instance types.
	InstanceType *string `json:"instance-type,omitempty" yaml:"instance-type,omitempty"`

	// Spaces, if not nil, holds a list of juju network spaces that
	// should be available (or not) on the machine. Positive and
	// negative values are accepted, and the difference is the latter
	// have a "^" prefix to the name.
	Spaces *[]string `json:"spaces,omitempty" yaml:"spaces,omitempty"`

	// VirtType, if not nil or empty, indicates that a machine must run the named
	// virtual type. Only valid for clouds with multi-hypervisor support.
	VirtType *string `json:"virt-type,omitempty" yaml:"virt-type,omitempty"`

	// Zones, if not nil, holds a list of availability zones limiting where
	// the machine can be located.
	Zones *[]string `json:"zones,omitempty" yaml:"zones,omitempty"`

	// AllocatePublicIP, if nil or true, signals that machines should be
	// created with a public IP address instead of a cloud local one.
	// The default behaviour if the value is not specified is to allocate
	// a public IP so that public cloud behaviour works out of the box.
	AllocatePublicIP *bool `json:"allocate-public-ip,omitempty" yaml:"allocate-public-ip,omitempty"`

	// ImageID, if not nil, indicates that a machine must use the specified
	// image. This is provider specific, and for the moment is only
	// implemented on MAAS clouds.
	ImageID *string `json:"image-id,omitempty" yaml:"image-id,omitempty"`
}

// CharmValue defines the memory resource constraints for Kubernetes-based workloads.
type CharmValue struct {
	MemRequest uint64
	MemLimit   uint64
}

// CAASApplicationProvisioningInfo holds info needed to provision a caas application.

type CAASApplicationProvisioningInfo struct {
	Version              version.Number               `json:"version"`
	APIAddresses         []string                     `json:"api-addresses"`
	CACert               string                       `json:"ca-cert"`
	Constraints          constraints.Value            `json:"constraints"`
	CharmConstraints     CharmValue                   `json:"charmconstraints"`
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
func NewDockerImageInfo(info docker.ImageRepoDetails, registryPath string) DockerImageInfo {
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
func ConvertDockerImageInfo(info DockerImageInfo) resources.DockerImageDetails {
	return resources.DockerImageDetails{
		RegistryPath: info.RegistryPath,
		ImageRepoDetails: docker.ImageRepoDetails{
			Repository:    info.Repository,
			ServerAddress: info.ServerAddress,
			BasicAuthConfig: docker.BasicAuthConfig{
				Username: info.Username,
				Password: info.Password,
				Auth:     docker.NewToken(info.Auth),
			},
			TokenAuthConfig: docker.TokenAuthConfig{
				IdentityToken: docker.NewToken(info.IdentityToken),
				RegistryToken: docker.NewToken(info.RegistryToken),
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
