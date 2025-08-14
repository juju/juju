// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"time"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/internal/storage"
)

// BlockDevice is a block device present on a machine.
// NB: the json labels are camel case not lower case because
// originally a mistake was made and we need to retain on the
// wire compatibility.
type BlockDevice struct {
	DeviceName     string   `json:"DeviceName"`
	DeviceLinks    []string `json:"DeviceLinks"`
	Label          string   `json:"Label"`
	UUID           string   `json:"UUID"`
	HardwareId     string   `json:"HardwareId"`
	WWN            string   `json:"WWN"`
	BusAddress     string   `json:"BusAddress"`
	Size           uint64   `json:"Size"`
	FilesystemType string   `json:"FilesystemType"`
	InUse          bool     `json:"InUse"`
	MountPoint     string   `json:"MountPoint"`
	SerialId       string   `json:"SerialId"`
}

// MachineBlockDevices holds a machine tag and the block devices present
// on that machine.
type MachineBlockDevices struct {
	Machine      string        `json:"machine"`
	BlockDevices []BlockDevice `json:"block-devices,omitempty"`
}

// SetMachineBlockDevices holds the arguments for recording the block
// devices present on a set of machines.
type SetMachineBlockDevices struct {
	MachineBlockDevices []MachineBlockDevices `json:"machine-block-devices"`
}

// BlockDeviceResult holds the result of an API call to retrieve details
// of a block device.
type BlockDeviceResult struct {
	Result BlockDevice `json:"result"`
	Error  *Error      `json:"error,omitempty"`
}

// BlockDeviceResults holds the result of an API call to retrieve details
// of multiple block devices.
type BlockDeviceResults struct {
	Results []BlockDeviceResult `json:"results,omitempty"`
}

// BlockDevicesResult holds the result of an API call to retrieve details
// of all block devices relating to some entity.
type BlockDevicesResult struct {
	Result []BlockDevice `json:"result"`
	Error  *Error        `json:"error,omitempty"`
}

// BlockDevicesResults holds the result of an API call to retrieve details
// of all block devices relating to some entities.
type BlockDevicesResults struct {
	Results []BlockDevicesResult `json:"results,omitempty"`
}

// StorageInstance describes a storage instance.
type StorageInstance struct {
	StorageTag string      `json:"storage-tag"`
	OwnerTag   string      `json:"owner-tag"`
	Kind       StorageKind `json:"kind"`
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
	StorageTag string `json:"storage-tag"`
	OwnerTag   string `json:"owner-tag"`
	UnitTag    string `json:"unit-tag"`

	Kind     StorageKind `json:"kind"`
	Location string      `json:"location"`
	Life     life.Value  `json:"life"`
}

// StorageAttachmentId identifies a storage attachment by the tags of the
// related unit and storage instance.
type StorageAttachmentId struct {
	StorageTag string `json:"storage-tag"`
	UnitTag    string `json:"unit-tag"`
}

// StorageAttachmentIds holds a set of storage attachment identifiers.
type StorageAttachmentIds struct {
	Ids []StorageAttachmentId `json:"ids"`
}

type StorageDetachmentParams struct {
	// StorageIds to detach
	StorageIds StorageAttachmentIds `json:"ids"`

	// Force specifies whether relation destruction will be forced, i.e.
	// keep going despite operational errors.
	Force *bool `json:"force,omitempty"`

	// MaxWait specifies the amount of time that each step in relation destroy process
	// will wait before forcing the next step to kick-off. This parameter
	// only makes sense in combination with 'force' set to 'true'.
	MaxWait *time.Duration `json:"max-wait,omitempty"`
}

// StorageAttachmentIdsResult holds the result of an API call to retrieve the
// IDs of a unit's attached storage instances.
type StorageAttachmentIdsResult struct {
	Result StorageAttachmentIds `json:"result"`
	Error  *Error               `json:"error,omitempty"`
}

// StorageAttachmentIdsResults holds the result of an API call to retrieve the
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
	MachineTag string `json:"machine-tag"`
	// AttachmentTag is the tag of the volume or filesystem whose
	// attachment to the machine is represented.
	AttachmentTag string `json:"attachment-tag"`
}

// MachineStorageIds holds a set of machine/storage-entity
// attachment identifiers.
type MachineStorageIds struct {
	Ids []MachineStorageId `json:"ids"`
}

// Volume identifies and describes a storage volume in the model.
type Volume struct {
	VolumeTag string     `json:"volume-tag"`
	Info      VolumeInfo `json:"info"`
}

// VolumeInfo describes a storage volume in the model.
type VolumeInfo struct {
	ProviderId string `json:"volume-id"`
	HardwareId string `json:"hardware-id,omitempty"`
	WWN        string `json:"wwn,omitempty"`
	// Pool is the name of the storage pool used to
	// allocate the volume. Juju controllers older
	// than 2.2 do not populate this field, so it may
	// be omitted.
	Pool string `json:"pool,omitempty"`
	// SizeMiB is the size of the volume in MiB.
	SizeMiB    uint64 `json:"size"`
	Persistent bool   `json:"persistent"`
}

// Volumes describes a set of storage volumes in the model.
type Volumes struct {
	Volumes []Volume `json:"volumes"`
}

// VolumeAttachment identifies and describes a volume attachment.
type VolumeAttachment struct {
	VolumeTag  string               `json:"volume-tag"`
	MachineTag string               `json:"machine-tag"`
	Info       VolumeAttachmentInfo `json:"info"`
}

// VolumeAttachmentPlan identifies and describes a volume attachment plan.
type VolumeAttachmentPlan struct {
	VolumeTag  string                   `json:"volume-tag"`
	MachineTag string                   `json:"machine-tag"`
	Life       life.Value               `json:"life,omitempty"`
	PlanInfo   VolumeAttachmentPlanInfo `json:"plan-info"`
	// BlockDevice should only be set by machine agents after
	// the AttachVolume() function is called. It represents the machines
	// view of the block device represented by the plan.
	BlockDevice BlockDevice `json:"block-device,omitempty"`
}

type VolumeAttachmentPlans struct {
	VolumeAttachmentPlans []VolumeAttachmentPlan `json:"volume-plans"`
}

// VolumeAttachmentPlanInfo describes info needed by machine agents
// to initialize attached volumes
type VolumeAttachmentPlanInfo struct {
	DeviceType       storage.DeviceType `json:"device-type,omitempty"`
	DeviceAttributes map[string]string  `json:"device-attributes,omitempty"`
}

// VolumeAttachmentInfo describes a volume attachment.
type VolumeAttachmentInfo struct {
	DeviceName string                    `json:"device-name,omitempty"`
	DeviceLink string                    `json:"device-link,omitempty"`
	BusAddress string                    `json:"bus-address,omitempty"`
	ReadOnly   bool                      `json:"read-only,omitempty"`
	PlanInfo   *VolumeAttachmentPlanInfo `json:"plan-info,omitempty"`
}

// VolumeAttachments describes a set of storage volume attachments.
type VolumeAttachments struct {
	VolumeAttachments []VolumeAttachment `json:"volume-attachments"`
}

// VolumeParams holds the parameters for creating a storage volume.
type VolumeParams struct {
	VolumeTag  string                  `json:"volume-tag"`
	Size       uint64                  `json:"size"`
	Provider   string                  `json:"provider"`
	Attributes map[string]interface{}  `json:"attributes,omitempty"`
	Tags       map[string]string       `json:"tags,omitempty"`
	Attachment *VolumeAttachmentParams `json:"attachment,omitempty"`
}

// RemoveVolumeParams holds the parameters for destroying or releasing a
// storage volume.
type RemoveVolumeParams struct {
	// Provider is the storage provider that manages the volume.
	Provider string `json:"provider"`

	// ProviderId is the storage provider's unique ID for the volume.
	// It is named volume-id for legacy reasons.
	ProviderId string `json:"volume-id"`

	// Destroy controls whether the volume should be completely
	// destroyed, or otherwise merely released from Juju's management.
	Destroy bool `json:"destroy,omitempty"`
}

// VolumeAttachmentParams holds the parameters for creating a volume
// attachment.
type VolumeAttachmentParams struct {
	VolumeTag  string `json:"volume-tag"`
	MachineTag string `json:"machine-tag"`
	// ProviderId is the storage provider's unique ID for the volume.
	// It is named volume-id for legacy reasons.
	ProviderId string `json:"volume-id,omitempty"`
	InstanceId string `json:"instance-id,omitempty"`
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

// VolumeAttachmentPlanResult holds the details of a single volume attachment plan,
// or an error.
type VolumeAttachmentPlanResult struct {
	Result VolumeAttachmentPlan `json:"result"`
	Error  *Error               `json:"error,omitempty"`
}

// VolumeAttachmentPlanResults holds a set of VolumeAttachmentPlanResult.
type VolumeAttachmentPlanResults struct {
	Results []VolumeAttachmentPlanResult `json:"results,omitempty"`
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

// VolumeParamsResult holds provisioning parameters for a volume.
type VolumeParamsResult struct {
	Result VolumeParams `json:"result"`
	Error  *Error       `json:"error,omitempty"`
}

// VolumeParamsResults holds provisioning parameters for multiple volumes.
type VolumeParamsResults struct {
	Results []VolumeParamsResult `json:"results,omitempty"`
}

// RemoveVolumeParamsResult holds parameters for destroying a volume.
type RemoveVolumeParamsResult struct {
	Result RemoveVolumeParams `json:"result"`
	Error  *Error             `json:"error,omitempty"`
}

// RemoveVolumeParamsResults holds parameters for destroying multiple volumes.
type RemoveVolumeParamsResults struct {
	Results []RemoveVolumeParamsResult `json:"results,omitempty"`
}

// VolumeAttachmentParamsResult holds provisioning parameters for a volume
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

// Filesystem identifies and describes a storage filesystem in the model.
type Filesystem struct {
	FilesystemTag string         `json:"filesystem-tag"`
	VolumeTag     string         `json:"volume-tag,omitempty"`
	Info          FilesystemInfo `json:"info"`
}

// FilesystemInfo describes a storage filesystem in the model.
type FilesystemInfo struct {
	// ProviderId is called the filesystem-id over the wire.
	ProviderId string `json:"filesystem-id"`
	// Pool is the name of the storage pool used to
	// allocate the filesystem. Juju controllers older
	// than 2.2 do not populate this field, so it may
	// be omitted.
	Pool string `json:"pool"`
	// Size is the size of the filesystem in MiB.
	Size uint64 `json:"size"`
}

// Filesystems describes a set of storage filesystems in the model.
type Filesystems struct {
	Filesystems []Filesystem `json:"filesystems"`
}

// FilesystemAttachment identifies and describes a filesystem attachment.
type FilesystemAttachment struct {
	FilesystemTag string                   `json:"filesystem-tag"`
	MachineTag    string                   `json:"machine-tag"`
	Info          FilesystemAttachmentInfo `json:"info"`
}

// FilesystemAttachmentInfo describes a filesystem attachment.
type FilesystemAttachmentInfo struct {
	MountPoint string `json:"mount-point,omitempty"`
	ReadOnly   bool   `json:"read-only,omitempty"`
}

// FilesystemAttachments describes a set of storage filesystem attachments.
type FilesystemAttachments struct {
	FilesystemAttachments []FilesystemAttachment `json:"filesystem-attachments"`
}

// FilesystemParams holds the parameters for creating a storage filesystem.
type FilesystemParams struct {
	FilesystemTag string                      `json:"filesystem-tag"`
	VolumeTag     string                      `json:"volume-tag,omitempty"`
	Size          uint64                      `json:"size"`
	Provider      string                      `json:"provider"`
	Attributes    map[string]interface{}      `json:"attributes,omitempty"`
	Tags          map[string]string           `json:"tags,omitempty"`
	Attachment    *FilesystemAttachmentParams `json:"attachment,omitempty"`
}

// RemoveFilesystemParams holds the parameters for destroying or releasing
// a filesystem.
type RemoveFilesystemParams struct {
	// Provider is the storage provider that manages the filesystem.
	Provider string `json:"provider"`

	// ProviderId is the storage provider's unique ID for the filesystem.
	// It is named filesystem-id for legacy reasons.
	ProviderId string `json:"filesystem-id"`

	// Destroy controls whether the filesystem should be completely
	// destroyed, or otherwise merely released from Juju's management.
	Destroy bool `json:"destroy,omitempty"`
}

// FilesystemAttachmentParams holds the parameters for creating a filesystem
// attachment.
type FilesystemAttachmentParams struct {
	FilesystemTag string `json:"filesystem-tag"`
	MachineTag    string `json:"machine-tag"`
	// ProviderId is the storage provider's unique ID for the filesystem.
	// It is named filesystem-id for legacy reasons.
	ProviderId string `json:"filesystem-id,omitempty"`
	InstanceId string `json:"instance-id,omitempty"`
	Provider   string `json:"provider"`
	MountPoint string `json:"mount-point,omitempty"`
	ReadOnly   bool   `json:"read-only,omitempty"`
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

// FilesystemParamsResult holds provisioning parameters for a filesystem.
type FilesystemParamsResult struct {
	Result FilesystemParams `json:"result"`
	Error  *Error           `json:"error,omitempty"`
}

// FilesystemParamsResults holds provisioning parameters for multiple filesystems.
type FilesystemParamsResults struct {
	Results []FilesystemParamsResult `json:"results,omitempty"`
}

// RemoveFilesystemParamsResult holds parameters for destroying or releasing
// a filesystem.
type RemoveFilesystemParamsResult struct {
	Result RemoveFilesystemParams `json:"result"`
	Error  *Error                 `json:"error,omitempty"`
}

// RemoveFilesystemParamsResults holds parameters for destroying or releasing
// multiple filesystems.
type RemoveFilesystemParamsResults struct {
	Results []RemoveFilesystemParamsResult `json:"results,omitempty"`
}

// FilesystemAttachmentParamsResult holds provisioning parameters for a filesystem
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
	StorageTag string `json:"storage-tag"`

	// OwnerTag holds tag for the owner of this storage, unit or application.
	OwnerTag string `json:"owner-tag"`

	// Kind holds what kind of storage this instance is.
	Kind StorageKind `json:"kind"`

	// Status contains the status of the storage instance.
	Status EntityStatus `json:"status"`

	// Life contains the lifecycle state of the storage.
	// Juju controllers older than 2.2 do not populate this
	// field, so it may be omitted.
	Life life.Value `json:"life,omitempty"`

	// Persistent reports whether or not the underlying volume or
	// filesystem is persistent; i.e. whether or not it outlives
	// the machine that it is attached to.
	Persistent bool `json:"persistent"`

	// Attachments contains a mapping from unit tag to
	// storage attachment details.
	Attachments map[string]StorageAttachmentDetails `json:"attachments,omitempty"`
}

// StorageFilter holds filter terms for listing storage details.
type StorageFilter struct {
	// We don't currently implement any filters. This exists to get the
	// API structure right, and so we can add filters later as necessary.
}

// StorageFilters holds a set of storage filters.
type StorageFilters struct {
	Filters []StorageFilter `json:"filters,omitempty"`
}

// StorageDetailsResult holds information about a storage instance
// or error related to its retrieval.
type StorageDetailsResult struct {
	Result *StorageDetails `json:"result,omitempty"`
	Error  *Error          `json:"error,omitempty"`
}

// StorageDetailsResults holds results for storage details or related storage error.
type StorageDetailsResults struct {
	Results []StorageDetailsResult `json:"results,omitempty"`
}

// StorageDetailsListResult holds a collection of storage details.
type StorageDetailsListResult struct {
	Result []StorageDetails `json:"result,omitempty"`
	Error  *Error           `json:"error,omitempty"`
}

// StorageDetailsListResults holds a collection of collections of storage details.
type StorageDetailsListResults struct {
	Results []StorageDetailsListResult `json:"results,omitempty"`
}

// StorageAttachmentDetails holds detailed information about a storage attachment.
type StorageAttachmentDetails struct {
	// StorageTag is the tag of the storage instance.
	StorageTag string `json:"storage-tag"`

	// UnitTag is the tag of the unit attached to the storage instance.
	UnitTag string `json:"unit-tag"`

	// MachineTag is the tag of the machine that the attached unit is assigned to.
	MachineTag string `json:"machine-tag"`

	// Location holds location (mount point/device path) of
	// the attached storage.
	Location string `json:"location,omitempty"`

	// Life contains the lifecycle state of the storage attachment.
	// Juju controllers older than 2.2 do not populate this
	// field, so it may be omitted.
	Life life.Value `json:"life,omitempty"`
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

// StoragePoolArgs contains a set of StoragePool.
type StoragePoolArgs struct {
	Pools []StoragePool `json:"pools"`
}

// StoragePoolDeleteArg holds data for a pool instance to be deleted.
type StoragePoolDeleteArg struct {
	Name string `json:"name"`
}

// StoragePoolDeleteArgs contains a set of StorageDeleteArg.
type StoragePoolDeleteArgs struct {
	Pools []StoragePoolDeleteArg `json:"pools"`
}

// StoragePoolFilter holds a filter for matching storage pools.
type StoragePoolFilter struct {
	// Names are pool's names to filter on.
	Names []string `json:"names,omitempty"`

	// Providers are pool's storage provider types to filter on.
	Providers []string `json:"providers,omitempty"`
}

// StoragePoolFilters holds a collection of storage pool filters.
type StoragePoolFilters struct {
	Filters []StoragePoolFilter `json:"filters,omitempty"`
}

// StoragePoolsResult holds a collection of storage pools.
type StoragePoolsResult struct {
	Result []StoragePool `json:"storage-pools,omitempty"`
	Error  *Error        `json:"error,omitempty"`
}

// StoragePoolsResults holds a collection of storage pools results.
type StoragePoolsResults struct {
	Results []StoragePoolsResult `json:"results,omitempty"`
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

// VolumeFilters holds a collection of volume filters.
type VolumeFilters struct {
	Filters []VolumeFilter `json:"filters,omitempty"`
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

// FilesystemFilters holds a collection of filesystem filters.
type FilesystemFilters struct {
	Filters []FilesystemFilter `json:"filters,omitempty"`
}

// VolumeDetails describes a storage volume in the model
// for the purpose of volume CLI commands.
//
// This is kept separate from Volume which contains only information
// specific to the volume model, whereas VolumeDetails is intended
// to contain complete information about a volume and related
// information (status, attachments, storage).
type VolumeDetails struct {
	// VolumeTag is the tag for the volume.
	VolumeTag string `json:"volume-tag"`

	// Info contains information about the volume.
	Info VolumeInfo `json:"info"`

	// Life contains the lifecycle state of the volume.
	// Juju controllers older than 2.2 do not populate this
	// field, so it may be omitted.
	Life life.Value `json:"life,omitempty"`

	// Status contains the status of the volume.
	Status EntityStatus `json:"status"`

	// MachineAttachments contains a mapping from
	// machine tag to volume attachment information.
	MachineAttachments map[string]VolumeAttachmentDetails `json:"machine-attachments,omitempty"`

	// UnitAttachments contains a mapping from
	// unit tag to volume attachment information (CAAS models).
	UnitAttachments map[string]VolumeAttachmentDetails `json:"unit-attachments,omitempty"`

	// Storage contains details about the storage instance
	// that the volume is assigned to, if any.
	Storage *StorageDetails `json:"storage,omitempty"`
}

// VolumeAttachmentDetails describes a volume attachment.
type VolumeAttachmentDetails struct {
	// NOTE(axw) for backwards-compatibility, this must not be given a
	// json tag. This ensures that we collapse VolumeAttachmentInfo.
	//
	// TODO(axw) when we can break backwards-compatibility (Juju 3.0),
	// give this a field name of "info", like we have in VolumeDetails
	// above.
	VolumeAttachmentInfo

	// Life contains the lifecycle state of the volume attachment.
	// Juju controllers older than 2.2 do not populate this
	// field, so it may be omitted.
	Life life.Value `json:"life,omitempty"`
}

// VolumeDetailsResult contains details about a volume, its attachments or
// an error preventing retrieving those details.
type VolumeDetailsResult struct {
	// Result describes the volume in detail.
	Result *VolumeDetails `json:"details,omitempty"`

	// Error contains volume retrieval error.
	Error *Error `json:"error,omitempty"`
}

// VolumeDetailsResults holds volume details.
type VolumeDetailsResults struct {
	Results []VolumeDetailsResult `json:"results,omitempty"`
}

// VolumeDetailsListResult holds a collection of volume details.
type VolumeDetailsListResult struct {
	Result []VolumeDetails `json:"result,omitempty"`
	Error  *Error          `json:"error,omitempty"`
}

// VolumeDetailsListResults holds a collection of collections of volume details.
type VolumeDetailsListResults struct {
	Results []VolumeDetailsListResult `json:"results,omitempty"`
}

// FilesystemDetails describes a storage filesystem in the model
// for the purpose of filesystem CLI commands.
//
// This is kept separate from Filesystem which contains only information
// specific to the filesystem model, whereas FilesystemDetails is intended
// to contain complete information about a filesystem and related
// information (status, attachments, storage).
type FilesystemDetails struct {
	// FilesystemTag is the tag for the filesystem.
	FilesystemTag string `json:"filesystem-tag"`

	// VolumeTag is the tag for the volume backing the
	// filesystem, if any.
	VolumeTag string `json:"volume-tag,omitempty"`

	// Info contains information about the filesystem.
	Info FilesystemInfo `json:"info"`

	// Life contains the lifecycle state of the filesystem.
	// Juju controllers older than 2.2 do not populate this
	// field, so it may be omitted.
	Life life.Value `json:"life,omitempty"`

	// Status contains the status of the filesystem.
	Status EntityStatus `json:"status"`

	// MachineAttachments contains a mapping from
	// machine tag to filesystem attachment information (IAAS models).
	MachineAttachments map[string]FilesystemAttachmentDetails `json:"machine-attachments,omitempty"`

	// UnitAttachments contains a mapping from
	// unit tag to filesystem attachment information.
	UnitAttachments map[string]FilesystemAttachmentDetails `json:"unit-attachments,omitempty"`

	// Storage contains details about the storage instance
	// that the volume is assigned to, if any.
	Storage *StorageDetails `json:"storage,omitempty"`
}

// FilesystemAttachmentDetails describes a filesystem attachment.
type FilesystemAttachmentDetails struct {
	// NOTE(axw) for backwards-compatibility, this must not be given a
	// json tag. This ensures that we collapse FilesystemAttachmentInfo.
	//
	// TODO(axw) when we can break backwards-compatibility (Juju 3.0),
	// give this a field name of "info", like we have in FilesystemDetails
	// above.
	FilesystemAttachmentInfo

	// Life contains the lifecycle state of the filesystem attachment.
	// Juju controllers older than 2.2 do not populate this
	// field, so it may be omitted.
	Life life.Value `json:"life,omitempty"`
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

// FilesystemDetailsListResult holds a collection of filesystem details.
type FilesystemDetailsListResult struct {
	Result []FilesystemDetails `json:"result,omitempty"`
	Error  *Error              `json:"error,omitempty"`
}

// FilesystemDetailsListResults holds a collection of collections of
// filesystem details.
type FilesystemDetailsListResults struct {
	Results []FilesystemDetailsListResult `json:"results,omitempty"`
}

// StorageDirectives contains directives for storage instance.
type StorageDirectives struct {
	// Pool is the name of the storage pool from which to provision the
	// storage instance.
	Pool string `json:"pool,omitempty"`

	// Size is the required size of the storage instance, in MiB.
	Size *uint64 `json:"size,omitempty"`

	// Count is the required number of storage instances.
	Count *uint64 `json:"count,omitempty"`
}

// StorageAddParams holds storage details to add to a unit dynamically.
type StorageAddParams struct {
	// UnitTag  is unit name.
	UnitTag string `json:"unit"`

	// StorageName is the name of the storage as specified in the charm.
	StorageName string `json:"name"`

	// Directives are specified storage directives.
	Directives StorageDirectives `json:"storage"`
}

// StoragesAddParams holds storage details to add to units dynamically.
type StoragesAddParams struct {
	Storages []StorageAddParams `json:"storages"`
}

// RemoveStorage holds the parameters for removing storage from the model.
type RemoveStorage struct {
	Storage []RemoveStorageInstance `json:"storage"`
}

// RemoveStorageInstance holds the parameters for removing a storage instance.
type RemoveStorageInstance struct {
	// Tag is the tag of the storage instance to be destroyed.
	Tag string `json:"tag"`

	// DestroyAttachments controls whether or not the storage attachments
	// will be destroyed automatically. If DestroyAttachments is false,
	// then the storage must already be detached.
	DestroyAttachments bool `json:"destroy-attachments,omitempty"`

	// DestroyStorage controls whether or not the associated cloud storage
	// is destroyed. If DestroyStorage is true, the cloud storage will be
	// destroyed; otherwise it will only be released from Juju's control.
	DestroyStorage bool `json:"destroy-storage,omitempty"`

	// Force specifies whether relation destruction will be forced, i.e.
	// keep going despite operational errors.
	Force *bool `json:"force,omitempty"`

	// MaxWait specifies the amount of time that each step in relation destroy process
	// will wait before forcing the next step to kick-off. This parameter
	// only makes sense in combination with 'force' set to 'true'.
	MaxWait *time.Duration `json:"max-wait,omitempty"`
}

// BulkImportStorageParams contains the parameters for importing a collection
// of storage entities.
type BulkImportStorageParams struct {
	Storage []ImportStorageParams `json:"storage"`
}

// ImportStorageParams contains the parameters for importing a storage entity.
type ImportStorageParams struct {
	// Kind is the kind of the storage entity to import.
	Kind StorageKind `json:"kind"`

	// Pool is the name of the storage pool into which the storage is to
	// be imported.
	Pool string `json:"pool"`

	// ProviderId is the storage provider's unique ID for the storage,
	// e.g. the EBS volume ID.
	ProviderId string `json:"provider-id"`

	// StorageName is the name of the storage to assign to the entity.
	StorageName string `json:"storage-name"`
}

// ImportStorageResults contains the results of importing a collection of
// storage entities.
type ImportStorageResults struct {
	Results []ImportStorageResult `json:"results"`
}

// ImportStorageResult contains the result of importing a storage entity.
type ImportStorageResult struct {
	Result *ImportStorageDetails `json:"result,omitempty"`
	Error  *Error                `json:"error,omitempty"`
}

// ImportStorageDetails contains the details of an imported storage entity.
type ImportStorageDetails struct {
	// StorageTag contains the string representation of the storage tag
	// assigned to the imported storage entity.
	StorageTag string `json:"storage-tag"`
}

// AddStorageResults contains the results of adding storage to units.
type AddStorageResults struct {
	Results []AddStorageResult `json:"results"`
}

// AddStorageResult contains the result of adding storage to a unit.
type AddStorageResult struct {
	Result *AddStorageDetails `json:"result,omitempty"`
	Error  *Error             `json:"error,omitempty"`
}

// AddStorageDetails contains the details of added storage.
type AddStorageDetails struct {
	// StorageTags contains the string representation of the storage tags
	// of the added storage instances.
	StorageTags []string `json:"storage-tags"`
}
