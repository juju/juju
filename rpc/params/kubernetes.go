// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"github.com/juju/juju/core/semversion"
)

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

// KubernetesVolumeInfo describes a storage volume in the cloud
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
	AgentTag string            `json:"agent-tag"`
	Version  semversion.Number `json:"version"`
}
