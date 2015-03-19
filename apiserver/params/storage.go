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

// String returns representation of StorageKind for readability.
func (k *StorageKind) String() string {
	switch *k {
	case StorageKindBlock:
		return "block"
	case StorageKindFilesystem:
		return "filesystem"
	default:
		return "unknown"
	}
}

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

// MachineStorageId identifies the attachment of a storage entity
// to a machine, by their tags.
type MachineStorageId struct {
	MachineTag string `json:"machinetag"`
	// AttachmentTag is the tag of the volume or filesystem whose
	// attachment to the machine is represented.
	AttachmentTag string `json:"attachmenttag"`
}

// MachineStorageIds holds a set of machine/storage-entity
// attachment identifiers.
type MachineStorageIds struct {
	Ids []MachineStorageId `json:"ids"`
}

// Volume describes a storage volume in the environment.
type Volume struct {
	VolumeTag string `json:"volumetag"`
	VolumeId  string `json:"volumeid"`
	Serial    string `json:"serial"`
	// Size is the size of the volume in MiB.
	Size       uint64 `json:"size"`
	Persistent bool   `json:"persistent"`
}

// Volumes describes a set of storage volumes in the environment.
type Volumes struct {
	Volumes []Volume `json:"volumes"`
}

// VolumeAttachment describes a volume attachment.
type VolumeAttachment struct {
	VolumeTag  string `json:"volumetag"`
	MachineTag string `json:"machinetag"`
	DeviceName string `json:"devicename,omitempty"`
	ReadOnly   bool   `json:"readonly"`
}

// VolumeAttachments describes a set of storage volume attachments.
type VolumeAttachments struct {
	VolumeAttachments []VolumeAttachment `json:"volumeattachments"`
}

// VolumeParams holds the parameters for creating a storage volume.
type VolumeParams struct {
	VolumeTag  string                  `json:"volumetag"`
	Size       uint64                  `json:"size"`
	Provider   string                  `json:"provider"`
	Attributes map[string]interface{}  `json:"attributes,omitempty"`
	Attachment *VolumeAttachmentParams `json:"attachment,omitempty"`
}

// VolumeAttachmentParams holds the parameters for creating a volume
// attachment.
type VolumeAttachmentParams struct {
	VolumeTag  string `json:"volumetag"`
	MachineTag string `json:"machinetag"`
	InstanceId string `json:"instanceid,omitempty"`
	VolumeId   string `json:"volumeid,omitempty"`
	Provider   string `json:"provider"`
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

// VolumeAttachmentsResults holds a set of VolumeAttachmentsResults for
// a set of machines.
type VolumeAttachmentsResults struct {
	Results []VolumeAttachmentsResult `json:"results,omitempty"`
}

// VolumeAttachmentResult holds the details of a single volume attachment,
// or an error.
type VolumeAttachmentResult struct {
	Result VolumeAttachment `json:"result"`
	Error  *Error           `json:"error,omitempty"`
}

// VolumeAttachmentResults holds a set of VolumeAttachmentResults.
type VolumeAttachmentResults struct {
	Results []VolumeAttachmentResult `json:"results,omitempty"`
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

// VolumeAttachmentParamsResults holds provisioning parameters for a volume
// attachment.
type VolumeAttachmentParamsResult struct {
	Result VolumeAttachmentParams `json:"result"`
	Error  *Error                 `json:"error,omitempty"`
}

// VolumeAttachmentParamsResults holds provisioning parameters for multiple
// volume attachments.
type VolumeAttachmentParamsResults struct {
	Results []VolumeAttachmentParamsResult `json:"results,omitempty"`
}

// Filesystem describes a storage filesystem in the environment.
type Filesystem struct {
	FilesystemTag string `json:"filesystemtag"`
	FilesystemId  string `json:"filesystemid"`
	// Size is the size of the filesystem in MiB.
	Size uint64 `json:"size"`
}

// Filesystems describes a set of storage filesystems in the environment.
type Filesystems struct {
	Filesystems []Filesystem `json:"filesystems"`
}

// FilesystemAttachment describes a filesystem attachment.
type FilesystemAttachment struct {
	FilesystemTag string `json:"filesystemtag"`
	MachineTag    string `json:"machinetag"`
	MountPoint    string `json:"mountpoint,omitempty"`
}

// FilesystemAttachments describes a set of storage filesystem attachments.
type FilesystemAttachments struct {
	FilesystemAttachments []FilesystemAttachment `json:"filesystemattachments"`
}

// FilesystemParams holds the parameters for creating a storage filesystem.
type FilesystemParams struct {
	FilesystemTag string                      `json:"filesystemtag"`
	Size          uint64                      `json:"size"`
	Provider      string                      `json:"provider"`
	Attributes    map[string]interface{}      `json:"attributes,omitempty"`
	Attachment    *FilesystemAttachmentParams `json:"attachment,omitempty"`
}

// FilesystemAttachmentParams holds the parameters for creating a filesystem
// attachment.
type FilesystemAttachmentParams struct {
	FilesystemTag string `json:"filesystemtag"`
	MachineTag    string `json:"machinetag"`
	InstanceId    string `json:"instanceid,omitempty"`
	FilesystemId  string `json:"filesystemid,omitempty"`
	Provider      string `json:"provider"`
	MountPoint    string `json:"mountpoint,omitempty"`
}

// FilesystemAttachmentResult holds the details of a single filesystem attachment,
// or an error.
type FilesystemAttachmentResult struct {
	Result FilesystemAttachment `json:"result"`
	Error  *Error               `json:"error,omitempty"`
}

// FilesystemAttachmentResults holds a set of FilesystemAttachmentResults.
type FilesystemAttachmentResults struct {
	Results []FilesystemAttachmentResult `json:"results,omitempty"`
}

// FilesystemResult holds information about a filesystem.
type FilesystemResult struct {
	Result Filesystem `json:"result"`
	Error  *Error     `json:"error,omitempty"`
}

// FilesystemResults holds information about multiple filesystems.
type FilesystemResults struct {
	Results []FilesystemResult `json:"results,omitempty"`
}

// FilesystemParamsResults holds provisioning parameters for a filesystem.
type FilesystemParamsResult struct {
	Result FilesystemParams `json:"result"`
	Error  *Error           `json:"error,omitempty"`
}

// FilesystemParamsResults holds provisioning parameters for multiple filesystems.
type FilesystemParamsResults struct {
	Results []FilesystemParamsResult `json:"results,omitempty"`
}

// FilesystemAttachmentParamsResults holds provisioning parameters for a filesystem
// attachment.
type FilesystemAttachmentParamsResult struct {
	Result FilesystemAttachmentParams `json:"result"`
	Error  *Error                     `json:"error,omitempty"`
}

// FilesystemAttachmentParamsResults holds provisioning parameters for multiple
// filesystem attachments.
type FilesystemAttachmentParamsResults struct {
	Results []FilesystemAttachmentParamsResult `json:"results,omitempty"`
}

// StorageDetails holds information about storage.
type StorageDetails struct {

	// StorageTag holds tag for this storage.
	StorageTag string `json:"storagetag"`

	// OwnerTag holds tag for the owner of this storage, unit or service.
	OwnerTag string `json:"ownertag"`

	// Kind holds what kind of storage this instance is.
	Kind StorageKind `json:"kind"`

	// Status indicates storage status, e.g. pending, provisioned, attached.
	Status string `json:"status,omitempty"`

	// UnitTag holds tag for unit for attached instances.
	UnitTag string `json:"unittag,omitempty"`

	// Location holds location for provisioned attached instances.
	Location string `json:"location,omitempty"`

	// Persistent indicates whether the storage is persistent or not.
	Persistent bool `json:"persistent"`
}

// StorageDetailsResult holds information about a storage instance
// or error related to its retrieval.
type StorageDetailsResult struct {
	Result StorageDetails `json:"result"`
	Error  *Error         `json:"error,omitempty"`
}

// StorageDetailsResults holds results for storage details or related storage error.
type StorageDetailsResults struct {
	Results []StorageDetailsResult `json:"results,omitempty"`
}

// StorageInfo contains information about a storage as well as
// potentially an error related to information retrieval.
type StorageInfo struct {
	StorageDetails `json:"result"`
	Error          *Error `json:"error,omitempty"`
}

// StorageInfosResult holds storage details.
type StorageInfosResult struct {
	Results []StorageInfo `json:"results,omitempty"`
}
