// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"github.com/juju/version/v2"

	"github.com/juju/juju/core/constraints"
)

// KubernetesDeploymentInfo holds deployment info from charm metadata.
type KubernetesDeploymentInfo struct {
	DeploymentType string `json:"deployment-type"`
	ServiceType    string `json:"service-type"`
}

// KubernetesProvisioningInfo holds unit provisioning info.
type KubernetesProvisioningInfo struct {
	DeploymentInfo       *KubernetesDeploymentInfo    `json:"deployment-info,omitempty"`
	PodSpec              string                       `json:"pod-spec"`
	RawK8sSpec           string                       `json:"raw-k8s-spec,omitempty"`
	Constraints          constraints.Value            `json:"constraints"`
	Tags                 map[string]string            `json:"tags,omitempty"`
	Filesystems          []KubernetesFilesystemParams `json:"filesystems,omitempty"`
	Volumes              []KubernetesVolumeParams     `json:"volumes,omitempty"`
	Devices              []KubernetesDeviceParams     `json:"devices,omitempty"`
	CharmModifiedVersion int                          `json:"charm-modified-version,omitempty"`
	ImageRepo            DockerImageInfo              `json:"image-repo,omitempty"`
}

// KubernetesProvisioningInfoResult holds unit provisioning info or an error.
type KubernetesProvisioningInfoResult struct {
	Error  *Error                      `json:"error,omitempty"`
	Result *KubernetesProvisioningInfo `json:"result"`
}

// KubernetesProvisioningInfoResults holds multiple provisioning info results.
type KubernetesProvisioningInfoResults struct {
	Results []KubernetesProvisioningInfoResult `json:"results"`
}

// KubernetesFilesystemParams holds the parameters for creating a storage filesystem.
type KubernetesFilesystemParams struct {
	StorageName string                                `json:"storagename"`
	Size        uint64                                `json:"size"`
	Provider    string                                `json:"provider"`
	Attributes  map[string]interface{}                `json:"attributes,omitempty"`
	Tags        map[string]string                     `json:"tags,omitempty"`
	Attachment  *KubernetesFilesystemAttachmentParams `json:"attachment,omitempty"`
}

// KubernetesFilesystemAttachmentParams holds the parameters for
// creating a filesystem attachment.
type KubernetesFilesystemAttachmentParams struct {
	Provider   string `json:"provider"`
	MountPoint string `json:"mount-point,omitempty"`
	ReadOnly   bool   `json:"read-only,omitempty"`
}

// KubernetesFilesystemUnitAttachmentParams holds the parameters for
// creating a filesystem attachment for the unit.
type KubernetesFilesystemUnitAttachmentParams struct {
	UnitTag  string `json:"unit-tag"`
	VolumeId string `json:"volume-id"`
}

// KubernetesVolumeParams holds the parameters for creating a storage volume.
type KubernetesVolumeParams struct {
	StorageName string                            `json:"storagename"`
	Size        uint64                            `json:"size"`
	Provider    string                            `json:"provider"`
	Attributes  map[string]interface{}            `json:"attributes,omitempty"`
	Tags        map[string]string                 `json:"tags,omitempty"`
	Attachment  *KubernetesVolumeAttachmentParams `json:"attachment,omitempty"`
}

// KubernetesVolumeAttachmentParams holds the parameters for
// creating a volume attachment.
type KubernetesVolumeAttachmentParams struct {
	Provider string `json:"provider"`
	ReadOnly bool   `json:"read-only,omitempty"`
}

// KubernetesFilesystemInfo describes a storage filesystem in the cloud
// as reported to the model.
type KubernetesFilesystemInfo struct {
	StorageName  string                 `json:"storagename"`
	Pool         string                 `json:"pool"`
	Size         uint64                 `json:"size"`
	MountPoint   string                 `json:"mount-point,omitempty"`
	ReadOnly     bool                   `json:"read-only,omitempty"`
	FilesystemId string                 `json:"filesystem-id"`
	Status       string                 `json:"status"`
	Info         string                 `json:"info"`
	Data         map[string]interface{} `json:"data,omitempty"`
	Volume       KubernetesVolumeInfo   `json:"volume"`
}

// Volume describes a storage volume in the cloud
// as reported to the model.
type KubernetesVolumeInfo struct {
	VolumeId   string                 `json:"volume-id"`
	Pool       string                 `json:"pool,omitempty"`
	Size       uint64                 `json:"size"`
	Persistent bool                   `json:"persistent"`
	Status     string                 `json:"status"`
	Info       string                 `json:"info"`
	Data       map[string]interface{} `json:"data,omitempty"`
}

// DeviceType defines a device type.
type DeviceType string

// KubernetesDeviceParams holds a set of device constraints.
type KubernetesDeviceParams struct {
	Type       DeviceType        `bson:"type"`
	Count      int64             `bson:"count"`
	Attributes map[string]string `bson:"attributes,omitempty"`
}

// KubernetesUpgradeArg holds args used to upgrade an operator.
type KubernetesUpgradeArg struct {
	AgentTag string         `json:"agent-tag"`
	Version  version.Number `json:"version"`
}
