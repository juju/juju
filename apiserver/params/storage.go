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

// StorageAttachmentIdsResult holds the result of an API call to retrieve the
// IDs of a unit's attached storage instances.
type StorageAttachmentIdsResult struct {
	Result StorageAttachmentIds `json:"result"`
	Error  *Error               `json:"error,omitempty"`
}

// StorageAttachmentIdsResult holds the result of an API call to retrieve the
// IDs of multiple units attached storage instances.
type StorageAttachmentIdsResults struct {
	Results []StorageAttachmentIdsResult `json:"results,omitempty"`
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

// Volume identifies and describes a storage volume in the environment.
type Volume struct {
	VolumeTag string     `json:"volumetag"`
	Info      VolumeInfo `json:"info"`
}

// Volume describes a storage volume in the environment.
type VolumeInfo struct {
	VolumeId   string `json:"volumeid"`
	HardwareId string `json:"hardwareid,omitempty"`
	// Size is the size of the volume in MiB.
	Size       uint64 `json:"size"`
	Persistent bool   `json:"persistent"`
}

// Volumes describes a set of storage volumes in the environment.
type Volumes struct {
	Volumes []Volume `json:"volumes"`
}

// VolumeAttachment identifies and describes a volume attachment.
type VolumeAttachment struct {
	VolumeTag  string               `json:"volumetag"`
	MachineTag string               `json:"machinetag"`
	Info       VolumeAttachmentInfo `json:"info"`
}

// VolumeAttachmentInfo describes a volume attachment.
type VolumeAttachmentInfo struct {
	DeviceName string `json:"devicename,omitempty"`
	DeviceLink string `json:"devicelink,omitempty"`
	BusAddress string `json:"busaddress,omitempty"`
	ReadOnly   bool   `json:"read-only,omitempty"`
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
	Tags       map[string]string       `json:"tags,omitempty"`
	Attachment *VolumeAttachmentParams `json:"attachment,omitempty"`
}

// VolumeAttachmentParams holds the parameters for creating a volume
// attachment.
type VolumeAttachmentParams struct {
	VolumeTag  string `json:"volumetag"`
	MachineTag string `json:"machinetag"`
	VolumeId   string `json:"volumeid,omitempty"`
	InstanceId string `json:"instanceid,omitempty"`
	Provider   string `json:"provider"`
	ReadOnly   bool   `json:"read-only,omitempty"`
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

// Filesystem identifies and describes a storage filesystem in the environment.
type Filesystem struct {
	FilesystemTag string         `json:"filesystemtag"`
	VolumeTag     string         `json:"volumetag,omitempty"`
	Info          FilesystemInfo `json:"info"`
}

// Filesystem describes a storage filesystem in the environment.
type FilesystemInfo struct {
	FilesystemId string `json:"filesystemid"`
	// Size is the size of the filesystem in MiB.
	Size uint64 `json:"size"`
}

// Filesystems describes a set of storage filesystems in the environment.
type Filesystems struct {
	Filesystems []Filesystem `json:"filesystems"`
}

// FilesystemAttachment identifies and describes a filesystem attachment.
type FilesystemAttachment struct {
	FilesystemTag string                   `json:"filesystemtag"`
	MachineTag    string                   `json:"machinetag"`
	Info          FilesystemAttachmentInfo `json:"info"`
}

// FilesystemAttachmentInfo describes a filesystem attachment.
type FilesystemAttachmentInfo struct {
	MountPoint string `json:"mountpoint,omitempty"`
	ReadOnly   bool   `json:"read-only,omitempty"`
}

// FilesystemAttachments describes a set of storage filesystem attachments.
type FilesystemAttachments struct {
	FilesystemAttachments []FilesystemAttachment `json:"filesystemattachments"`
}

// FilesystemParams holds the parameters for creating a storage filesystem.
type FilesystemParams struct {
	FilesystemTag string                      `json:"filesystemtag"`
	VolumeTag     string                      `json:"volumetag,omitempty"`
	Size          uint64                      `json:"size"`
	Provider      string                      `json:"provider"`
	Attributes    map[string]interface{}      `json:"attributes,omitempty"`
	Tags          map[string]string           `json:"tags,omitempty"`
	Attachment    *FilesystemAttachmentParams `json:"attachment,omitempty"`
}

// FilesystemAttachmentParams holds the parameters for creating a filesystem
// attachment.
type FilesystemAttachmentParams struct {
	FilesystemTag string `json:"filesystemtag"`
	MachineTag    string `json:"machinetag"`
	FilesystemId  string `json:"filesystemid,omitempty"`
	InstanceId    string `json:"instanceid,omitempty"`
	Provider      string `json:"provider"`
	MountPoint    string `json:"mountpoint,omitempty"`
	ReadOnly      bool   `json:"read-only,omitempty"`
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

	// Status contains the status of the storage instance.
	Status EntityStatus `json:"status"`

	// Persistent reports whether or not the underlying volume or
	// filesystem is persistent; i.e. whether or not it outlives
	// the machine that it is attached to.
	Persistent bool

	// Attachments contains a mapping from unit tag to
	// storage attachment details.
	Attachments map[string]StorageAttachmentDetails `json:"attachments,omitempty"`
}

// LegacyStorageDetails holds information about storage.
//
// NOTE(axw): this is for backwards compatibility only. This struct
// should not be changed!
type LegacyStorageDetails struct {
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
	Result *StorageDetails      `json:"details,omitempty"`
	Legacy LegacyStorageDetails `json:"result"`
	Error  *Error               `json:"error,omitempty"`
}

// StorageDetailsResults holds results for storage details or related storage error.
type StorageDetailsResults struct {
	Results []StorageDetailsResult `json:"results,omitempty"`
}

// StorageAttachmentDetails holds detailed information about a storage attachment.
type StorageAttachmentDetails struct {
	// StorageTag is the tag of the storage instance.
	StorageTag string `json:"storagetag"`

	// UnitTag is the tag of the unit attached to the storage instance.
	UnitTag string `json:"unittag"`

	// MachineTag is the tag of the machine that the attached unit is assigned to.
	MachineTag string `json:"machinetag"`

	// Location holds location (mount point/device path) of
	// the attached storage.
	Location string `json:"location,omitempty"`
}

// StoragePool holds data for a pool instance.
type StoragePool struct {

	// Name is the pool's name.
	Name string `json:"name"`

	// Provider is the type of storage provider this pool represents, eg "loop", "ebs".
	Provider string `json:"provider"`

	// Attrs are the pool's configuration attributes.
	Attrs map[string]interface{} `json:"attrs"`
}

// StoragePoolFilter holds a filter for pool API call.
type StoragePoolFilter struct {

	// Names are pool's names to filter on.
	Names []string `json:"names,omitempty"`

	// Providers are pool's storage provider types to filter on.
	Providers []string `json:"providers,omitempty"`
}

// StoragePoolsResult holds a collection of pool instances.
type StoragePoolsResult struct {
	Results []StoragePool `json:"results,omitempty"`
}

// VolumeFilter holds a filter for volume list API call.
type VolumeFilter struct {
	// Machines are machine tags to filter on.
	Machines []string `json:"machines,omitempty"`
}

// IsEmpty determines if filter is empty
func (f *VolumeFilter) IsEmpty() bool {
	return len(f.Machines) == 0
}

// FilesystemFilter holds a filter for filter list API call.
type FilesystemFilter struct {
	// Machines are machine tags to filter on.
	Machines []string `json:"machines,omitempty"`
}

// IsEmpty determines if filter is empty
func (f *FilesystemFilter) IsEmpty() bool {
	return len(f.Machines) == 0
}

// VolumeDetails describes a storage volume in the environment
// for the purpose of volume CLI commands.
//
// This is kept separate from Volume which contains only information
// specific to the volume model, whereas VolumeDetails is intended
// to contain complete information about a volume and related
// information (status, attachments, storage).
type VolumeDetails struct {

	// VolumeTag is the tag for the volume.
	VolumeTag string `json:"volumetag"`

	// Info contains information about the volume.
	Info VolumeInfo `json:"info"`

	// Status contains the status of the volume.
	Status EntityStatus `json:"status"`

	// MachineAttachments contains a mapping from
	// machine tag to volume attachment information.
	MachineAttachments map[string]VolumeAttachmentInfo `json:"machineattachments,omitempty"`

	// Storage contains details about the storage instance
	// that the volume is assigned to, if any.
	Storage *StorageDetails `json:"storage,omitempty"`
}

// LegacyVolumeDetails describes a storage volume in the environment
// for the purpose of volume CLI commands.
//
// This is kept separate from Volume which contains only information
// specific to the volume model, whereas LegacyVolumeDetails is intended
// to contain complete information about a volume.
//
// NOTE(axw): this is for backwards compatibility only. This struct
// should not be changed!
type LegacyVolumeDetails struct {

	// VolumeTag is tag for this volume instance.
	VolumeTag string `json:"volumetag"`

	// StorageInstance returns the tag of the storage instance that this
	// volume is assigned to, if any.
	StorageTag string `json:"storage,omitempty"`

	// UnitTag is the tag of the unit attached to storage instance
	// for this volume.
	UnitTag string `json:"unit,omitempty"`

	// VolumeId is a unique provider-supplied ID for the volume.
	VolumeId string `json:"volumeid,omitempty"`

	// HardwareId is the volume's hardware ID.
	HardwareId string `json:"hardwareid,omitempty"`

	// Size is the size of the volume in MiB.
	Size uint64 `json:"size,omitempty"`

	// Persistent reflects whether the volume is destroyed with the
	// machine to which it is attached.
	Persistent bool `json:"persistent"`

	// Status contains the current status of the volume.
	Status EntityStatus `json:"status"`
}

// VolumeDetailsResult contains details about a volume, its attachments or
// an error preventing retrieving those details.
type VolumeDetailsResult struct {

	// Details describes the volume in detail.
	Details *VolumeDetails `json:"details,omitempty"`

	// LegacyVolume describes the volume in detail.
	//
	// NOTE(axw): VolumeDetails contains redundant and nonsensical
	// information. Use Details if it is available, and only use
	// this for backwards-compatibility.
	LegacyVolume *LegacyVolumeDetails `json:"volume,omitempty"`

	// LegacyAttachments describes the attachments of the volume to
	// machines.
	//
	// NOTE(axw): this should have gone into VolumeDetails, but it's too
	// late for that now. We'll continue to populate it, and use it
	// if it's defined but Volume.Attachments is not. Please do not
	// copy this structure.
	LegacyAttachments []VolumeAttachment `json:"attachments,omitempty"`

	// Error contains volume retrieval error.
	Error *Error `json:"error,omitempty"`
}

// VolumeDetailsResults holds volume details.
type VolumeDetailsResults struct {
	Results []VolumeDetailsResult `json:"results,omitempty"`
}

// FilesystemDetails describes a storage filesystem in the environment
// for the purpose of filesystem CLI commands.
//
// This is kept separate from Filesystem which contains only information
// specific to the filesystem model, whereas FilesystemDetails is intended
// to contain complete information about a filesystem and related
// information (status, attachments, storage).
type FilesystemDetails struct {

	// FilesystemTag is the tag for the filesystem.
	FilesystemTag string `json:"filesystemtag"`

	// VolumeTag is the tag for the volume backing the
	// filesystem, if any.
	VolumeTag string `json:"volumetag,omitempty"`

	// Info contains information about the filesystem.
	Info FilesystemInfo `json:"info"`

	// Status contains the status of the filesystem.
	Status EntityStatus `json:"status"`

	// MachineAttachments contains a mapping from
	// machine tag to filesystem attachment information.
	MachineAttachments map[string]FilesystemAttachmentInfo `json:"machineattachments,omitempty"`

	// Storage contains details about the storage instance
	// that the volume is assigned to, if any.
	Storage *StorageDetails `json:"storage,omitempty"`
}

// FilesystemDetailsResult contains details about a filesystem, its attachments or
// an error preventing retrieving those details.
type FilesystemDetailsResult struct {
	Result *FilesystemDetails `json:"result,omitempty"`
	Error  *Error             `json:"error,omitempty"`
}

// FilesystemDetailsResults holds filesystem details.
type FilesystemDetailsResults struct {
	Results []FilesystemDetailsResult `json:"results,omitempty"`
}

// StorageConstraints contains constraints for storage instance.
type StorageConstraints struct {
	// Pool is the name of the storage pool from which to provision the
	// storage instance.
	Pool string `bson:"pool,omitempty"`

	// Size is the required size of the storage instance, in MiB.
	Size *uint64 `bson:"size,omitempty"`

	// Count is the required number of storage instances.
	Count *uint64 `bson:"count,omitempty"`
}

// StorageAddParams holds storage details to add to a unit dynamically.
type StorageAddParams struct {
	// UnitTag  is unit name.
	UnitTag string `json:"unit"`

	// StorageName is the name of the storage as specified in the charm.
	StorageName string `bson:"name"`

	// Constraints are specified storage constraints.
	Constraints StorageConstraints `json:"storage"`
}

// StoragesAddParams holds storage details to add to units dynamically.
type StoragesAddParams struct {
	Storages []StorageAddParams `json:"storages"`
}
