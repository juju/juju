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

// StorageInstanceResult holds the result of an API call to retrieve details
// of a storage instance.
type StorageInstanceResult struct {
	Result storage.StorageInstance `json:"result"`
	Error  *Error                  `json:"error,omitempty"`
}

// StorageInstanceResult holds the result of an API call to retrieve details
// of multiple storage instances.
type StorageInstanceResults struct {
	Results []StorageInstanceResult `json:"results,omitempty"`
}

// Volume describes a storage volume in the environment.
type Volume struct {
	VolumeTag string `json:"volumetag"`
	VolumeId  string `json:"volumeid"`
	Serial    string `json:"serial"`
	// Size is the size of the volume in MiB.
	Size uint64 `json:"size"`
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
	VolumeId   string `json:"volumeid"`
	MachineTag string `json:"machinetag"`
	InstanceId string `json:"instanceid,omitempty"`
	DeviceName string `json:"devicename,omitempty"`
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

// VolumeFormattingInfo holds the information regarding formatting
// a storage volume.
type VolumeFormattingInfo struct {
	NeedsFormatting bool   `json:"needsformatting"`
	DevicePath      string `json:"devicepath"`
}

// VolumeFormattingInfoResult holds a singular VolumeFormattingInfo
// result, or an error.
type VolumeFormattingInfoResult struct {
	Result VolumeFormattingInfo `json:"result"`
	Error  *Error               `json:"error,omitempty"`
}

// VolumeFormattingInfoResult holds a set of VolumeFormattingInfoResults.
type VolumeFormattingInfoResults struct {
	Results []VolumeFormattingInfoResult `json:"results,omitempty"`
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
