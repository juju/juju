// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"github.com/juju/juju/core/constraints"
	"github.com/juju/version/v2"
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
	ImagePath            string                       `json:"image-path"`
	Version              version.Number               `json:"version"`
	APIAddresses         []string                     `json:"api-addresses"`
	CACert               string                       `json:"ca-cert"`
	Constraints          constraints.Value            `json:"constraints"`
	Tags                 map[string]string            `json:"tags,omitempty"`
	Filesystems          []KubernetesFilesystemParams `json:"filesystems,omitempty"`
	Volumes              []KubernetesVolumeParams     `json:"volumes,omitempty"`
	Devices              []KubernetesDeviceParams     `json:"devices,omitempty"`
	Series               string                       `json:"series,omitempty"`
	ImageRepo            string                       `json:"image-repo,omitempty"`
	CharmModifiedVersion int                          `json:"charm-modified-version,omitempty"`
	CharmURL             string                       `json:"charm-url,omitempty"`
	Error                *Error                       `json:"error,omitempty"`
}

// CAASApplicationGarbageCollectArg holds info needed to cleanup units that have
// gone away permanently.
type CAASApplicationGarbageCollectArg struct {
	Application     Entity   `json:"application"`
	ObservedUnits   Entities `json:"observed-units"`
	DesiredReplicas int      `json:"desired-replicas"`
	ActivePodNames  []string `json:"active-pod-names"`
	Force           bool     `json:"force"`
}

// CAASApplicationGarbageCollectArgs holds info needed to cleanup units that have
// gone away permanently.
type CAASApplicationGarbageCollectArgs struct {
	Args []CAASApplicationGarbageCollectArg `json:"args"`
}

// DockerImageInfo holds the details for a Docker resource type.
type DockerImageInfo struct {
	// RegistryPath holds the path of the Docker image (including host and sha256) in a docker registry.
	RegistryPath string `json:"image-name"`

	// Username holds the username used to gain access to a non-public image.
	Username string `json:"username,omitempty"`

	// Password holds the password used to gain access to a non-public image.
	Password string `json:"password,omitempty"`
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
