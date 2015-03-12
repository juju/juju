// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import "github.com/juju/juju/storage"

// MachineBlockDevices holds a machine tag and the block devices present
// on that machine.
type MachineBlockDevices struct {
	Machine      string                `json:"machine"`
	BlockDevices []storage.BlockDevice `json:"blockdevices,omitempty"`
}

// SetMachineBlockDevices holds the arguments for recording the block
// devices present on a set of machines.
type SetMachineBlockDevices struct {
	MachineBlockDevices []MachineBlockDevices `json:"machineblockdevices"`
}

// BlockDeviceResult holds the result of an API call to retrieve details
// of a block device.
type BlockDeviceResult struct {
	Result storage.BlockDevice `json:"result"`
	Error  *Error              `json:"error,omitempty"`
}

// BlockDeviceResults holds the result of an API call to retrieve details
// of multiple block devices.
type BlockDeviceResults struct {
	Results []BlockDeviceResult `json:"results,omitempty"`
}

// BlockDevicesResult holds the result of an API call to retrieve details
// of all block devices relating to some entity.
type BlockDevicesResult struct {
	Result []storage.BlockDevice `json:"result"`
	Error  *Error                `json:"error,omitempty"`
}

// BlockDevicseResults holds the result of an API call to retrieve details
// of all block devices relating to some entities.
type BlockDevicesResults struct {
	Results []BlockDevicesResult `json:"results,omitempty"`
}

// StorageInstance describes a storage instance.
type StorageInstance struct {
	StorageTag string
	OwnerTag   string
	Kind       StorageKind
}

// StorageKind is the kind of a storage instance.
type StorageKind int

const (
	StorageKindUnknown StorageKind = iota
	StorageKindBlock
	StorageKindFilesystem
)

// StorageInstanceResult holds the result of an API call to retrieve details
// of a storage instance.
type StorageInstanceResult struct {
	Result StorageInstance `json:"result"`
	Error  *Error          `json:"error,omitempty"`
}

// StorageInstanceResults holds the result of an API call to retrieve details
// of multiple storage instances.
type StorageInstanceResults struct {
	Results []StorageInstanceResult `json:"results,omitempty"`
}

// StorageAttachment describes a unit's attached storage instance.
type StorageAttachment struct {
	StorageTag string
	OwnerTag   string
	UnitTag    string

	Kind     StorageKind
	Location string
	Life     Life
}

// StorageAttachmentId identifies a storage attachment by the tags of the
// related unit and storage instance.
type StorageAttachmentId struct {
	StorageTag string `json:"storagetag"`
	UnitTag    string `json:"unittag"`
}

// StorageAttachmentIds holds a set of storage attachment identifiers.
type StorageAttachmentIds struct {
	Ids []StorageAttachmentId `json:"ids"`
}

// StorageAttachmentsResult holds the result of an API call to retrieve details
// of a unit's attached storage instances.
type StorageAttachmentsResult struct {
	Result []StorageAttachment `json:"result"`
	Error  *Error              `json:"error,omitempty"`
}

// StorageAttachmentsResults holds the result of an API call to retrieve details
// of multiple units' attached storage instances.
type StorageAttachmentsResults struct {
	Results []StorageAttachmentsResult `json:"results,omitempty"`
}

// StorageAttachmentResult holds the result of an API call to retrieve details
// of a storage attachment.
type StorageAttachmentResult struct {
	Result StorageAttachment `json:"result"`
	Error  *Error            `json:"error,omitempty"`
}

// StorageAttachmentResults holds the result of an API call to retrieve details
// of multiple storage attachments.
type StorageAttachmentResults struct {
	Results []StorageAttachmentResult `json:"results,omitempty"`
}

// Volume describes a storage volume in the environment.
type Volume struct {
	VolumeTag string `json:"volumetag"`
	VolumeId  string `json:"volumeid"`
	Serial    string `json:"serial"`
	// Size is the size of the volume in MiB.
	Size uint64 `json:"size"`
}

// Volumes describes a set of storage volumes in the environment.
type Volumes struct {
	Volumes []Volume `json:"volumes"`
}

// VolumeAttachmentId identifies a volume attachment by the tags of the
// related machine and volume.
type VolumeAttachmentId struct {
	VolumeTag  string `json:"volumetag"`
	MachineTag string `json:"machinetag"`
}

// VolumeAttachmentIds holds a set of volume attachment identifiers.
type VolumeAttachmentIds struct {
	Ids []VolumeAttachmentId `json:"ids"`
}

// VolumeAttachment describes a volume attachment.
type VolumeAttachment struct {
	VolumeTag  string `json:"volumetag"`
	MachineTag string `json:"machinetag"`
	DeviceName string `json:"devicename,omitempty"`
	ReadOnly   bool   `json:"readonly"`
}

// VolumeParams holds the parameters for creating a storage volume.
type VolumeParams struct {
	VolumeTag  string                 `json:"volumetag"`
	Size       uint64                 `json:"size"`
	Provider   string                 `json:"provider"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`

	// Machine is the tag of the machine that the volume should
	// be initially attached to, if any.
	MachineTag string `json:"machinetag,omitempty"`
}

// VolumePreparationInfo holds the information regarding preparing
// a storage volume for use.
type VolumePreparationInfo struct {
	NeedsFilesystem bool   `json:"needsfilesystem"`
	DevicePath      string `json:"devicepath"`
}

// VolumePreparationInfoResult holds a singular VolumePreparationInfo
// result, or an error.
type VolumePreparationInfoResult struct {
	Result VolumePreparationInfo `json:"result"`
	Error  *Error                `json:"error,omitempty"`
}

// VolumePreparationInfoResult holds a set of VolumePreparationInfoResults.
type VolumePreparationInfoResults struct {
	Results []VolumePreparationInfoResult `json:"results,omitempty"`
}

// VolumeAttachmentsResult holds the volume attachments for a single
// machine, or an error.
type VolumeAttachmentsResult struct {
	Attachments []VolumeAttachment `json:"attachments,omitempty"`
	Error       *Error             `json:"error,omitempty"`
}

// VolumeAttachmensResult holds a set of VolumeAttachmentsResults for
// a set of machines.
type VolumeAttachmentsResults struct {
	Results []VolumeAttachmentsResult `json:"results,omitempty"`
}

// VolumeResult holds information about a volume.
type VolumeResult struct {
	Result Volume `json:"result"`
	Error  *Error `json:"error,omitempty"`
}

// VolumeResults holds information about multiple volumes.
type VolumeResults struct {
	Results []VolumeResult `json:"results,omitempty"`
}

// VolumeParamsResults holds provisioning parameters for a volume.
type VolumeParamsResult struct {
	Result VolumeParams `json:"result"`
	Error  *Error       `json:"error,omitempty"`
}

// VolumeParamsResults holds provisioning parameters for multiple volumes.
type VolumeParamsResults struct {
	Results []VolumeParamsResult `json:"results,omitempty"`
}

// StorageShowResult holds information about a storage instance
// or error related to its retrieval.
type StorageShowResult struct {
	Result StorageInstance `json:"result"`
	Error  *Error          `json:"error,omitempty"`
}

// StorageShowResults holds a collection of storage instances.
type StorageShowResults struct {
	Results []StorageShowResult `json:"results,omitempty"`
}
