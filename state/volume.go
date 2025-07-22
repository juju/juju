// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/storage"
)

// Volume describes a volume (disk, logical volume, etc.) in the model.
type Volume interface {
	GlobalEntity
	Lifer
	status.StatusGetter
	status.StatusSetter

	// VolumeTag returns the tag for the volume.
	VolumeTag() names.VolumeTag

	// StorageInstance returns the tag of the storage instance that this
	// volume is assigned to, if any. If the volume is not assigned to
	// a storage instance, an error satisfying errors.IsNotAssigned will
	// be returned.
	//
	// A volume can be assigned to at most one storage instance, and a
	// storage instance can have at most one associated volume.
	StorageInstance() (names.StorageTag, error)

	// Info returns the volume's VolumeInfo, or a NotProvisioned
	// error if the volume has not yet been provisioned.
	Info() (VolumeInfo, error)

	// Params returns the parameters for provisioning the volume,
	// if it has not already been provisioned. Params returns true if the
	// returned parameters are usable for provisioning, otherwise false.
	Params() (VolumeParams, bool)

	// Detachable reports whether or not the volume is detachable.
	Detachable() bool

	// Releasing reports whether or not the volume is to be released
	// from the model when it is Dying/Dead.
	Releasing() bool
}

// VolumeAttachment describes an attachment of a volume to a machine.
type VolumeAttachment interface {
	Lifer

	// Volume returns the tag of the related Volume.
	Volume() names.VolumeTag

	// Host returns the tag of the related Host.
	Host() names.Tag

	// Info returns the volume attachment's VolumeAttachmentInfo, or a
	// NotProvisioned error if the attachment has not yet been made.
	//
	// TODO(axw) use a different error, rather than NotProvisioned
	// (say, NotAttached or NotAssociated).
	Info() (VolumeAttachmentInfo, error)

	// Params returns the parameters for creating the volume attachment,
	// if it has not already been made. Params returns true if the returned
	// parameters are usable for creating an attachment, otherwise false.
	Params() (VolumeAttachmentParams, bool)
}

// VolumeAttachmentPlan describes the plan information for a particular volume
// Machine agents use this information to do any extra initialization that is needed
// This is separate from VolumeAttachment to allow separation of concerns between
// the controller's idea of detaching a volume and the machine agent's idea.
// This way, we can have the controller ask the environment for a volume, attach it
// to the instance, which in some cases simply means granting the instance access
// to connect to it, and then explicitly let the machine agent know that something
// has been attached to it.
type VolumeAttachmentPlan interface {
	Lifer

	// Volume returns the tag of the related Volume.
	Volume() names.VolumeTag

	// Machine returns the tag of the related Machine.
	Machine() names.MachineTag

	// PlanInfo returns the plan info for a volume
	PlanInfo() (VolumeAttachmentPlanInfo, error)

	// BlockDeviceInfo returns the block device info associated with
	// this plan, as seen by the machine agent it is plugged into
	BlockDeviceInfo() (BlockDeviceInfo, error)
}

type volume struct {
	doc volumeDoc
}

type volumeAttachment struct {
	doc volumeAttachmentDoc
}

type volumeAttachmentPlan struct {
	doc volumeAttachmentPlanDoc
}

// volumeDoc records information about a volume in the model.
type volumeDoc struct {
	DocID           string        `bson:"_id"`
	Name            string        `bson:"name"`
	ModelUUID       string        `bson:"model-uuid"`
	Life            Life          `bson:"life"`
	Releasing       bool          `bson:"releasing,omitempty"`
	StorageId       string        `bson:"storageid,omitempty"`
	AttachmentCount int           `bson:"attachmentcount"`
	Info            *VolumeInfo   `bson:"info,omitempty"`
	Params          *VolumeParams `bson:"params,omitempty"`

	// HostId is the ID of the host that a non-detachable
	// volume is initially attached to. We use this to identify
	// the volume as being non-detachable, and to determine
	// which volumes must be removed along with said machine.
	HostId string `bson:"hostid,omitempty"`
}

// volumeAttachmentDoc records information about a volume attachment.
type volumeAttachmentDoc struct {
	// DocID is the machine global key followed by the volume name.
	DocID     string                  `bson:"_id"`
	ModelUUID string                  `bson:"model-uuid"`
	Volume    string                  `bson:"volumeid"`
	Host      string                  `bson:"hostid"`
	Life      Life                    `bson:"life"`
	Info      *VolumeAttachmentInfo   `bson:"info,omitempty"`
	Params    *VolumeAttachmentParams `bson:"params,omitempty"`
}

// BlockDeviceInfo describes information about a block device.
type BlockDeviceInfo struct {
	DeviceName     string   `bson:"devicename"`
	DeviceLinks    []string `bson:"devicelinks,omitempty"`
	Label          string   `bson:"label,omitempty"`
	UUID           string   `bson:"uuid,omitempty"`
	HardwareId     string   `bson:"hardwareid,omitempty"`
	WWN            string   `bson:"wwn,omitempty"`
	BusAddress     string   `bson:"busaddress,omitempty"`
	Size           uint64   `bson:"size"`
	FilesystemType string   `bson:"fstype,omitempty"`
	InUse          bool     `bson:"inuse"`
	MountPoint     string   `bson:"mountpoint,omitempty"`
	SerialId       string   `bson:"serialid,omitempty"`
}

type volumeAttachmentPlanDoc struct {
	DocID     string                    `bson:"_id"`
	ModelUUID string                    `bson:"model-uuid"`
	Volume    string                    `bson:"volumeid"`
	Machine   string                    `bson:"machineid"`
	Life      Life                      `bson:"life"`
	PlanInfo  *VolumeAttachmentPlanInfo `bson:"plan-info,omitempty"`
	// BlockDevice represents the block device from the point
	// of view of the machine agent. Once the machine agent
	// finishes provisioning the storage attachment, it gathers
	// as much information about the new device as needed, and
	// sets it in the volume attachment plan, in state. This
	// information will later be used to match the block device
	// in state, with the block device the machine agent sees.
	BlockDevice *BlockDeviceInfo `bson:"block-device,omitempty"`
}

// VolumeParams records parameters for provisioning a new volume.
type VolumeParams struct {
	Pool string `bson:"pool"`
	Size uint64 `bson:"size"`
}

// VolumeInfo describes information about a volume.
type VolumeInfo struct {
	HardwareId string `bson:"hardwareid,omitempty"`
	WWN        string `bson:"wwn,omitempty"`
	Size       uint64 `bson:"size"`
	Pool       string `bson:"pool"`
	VolumeId   string `bson:"volumeid"`
	Persistent bool   `bson:"persistent"`
}

// VolumeAttachmentInfo describes information about a volume attachment.
type VolumeAttachmentInfo struct {
	DeviceName string `bson:"devicename,omitempty"`
	DeviceLink string `bson:"devicelink,omitempty"`
	BusAddress string `bson:"busaddress,omitempty"`
	ReadOnly   bool   `bson:"read-only"`
	// PlanInfo holds information used by the machine storage
	// provisioner to execute any needed steps in order to make
	// make sure the actual storage device becomes available.
	// For example, any storage backend that requires userspace
	// setup, like iSCSI would fall into this category.
	PlanInfo *VolumeAttachmentPlanInfo `bson:"plan-info,omitempty"`
}

type VolumeAttachmentPlanInfo struct {
	// DeviceType is the type of storage type this plan info
	// describes. For directly attached local storage, this
	// can be left to its default value, or set as storage.DeviceTypeLocal
	// This value will be used by the machine storage provisioner
	// to load the appropriate storage plan, and execute any Attach/Detach
	// operations.
	DeviceType storage.DeviceType `bson:"device-type,omitempty"`
	// DeviceAttributes holds a map of key/value pairs that may be used
	// by the storage plan backend to initialize the storage device
	// For example, if dealing with iSCSI, this can hold the IP address
	// of the remote server, the LUN, access credentials, etc.
	DeviceAttributes map[string]string `bson:"device-attributes,omitempty"`
}

// VolumeAttachmentParams records parameters for attaching a volume to a
// machine.
type VolumeAttachmentParams struct {
	ReadOnly bool `bson:"read-only"`
}

// Tag is required to implement GlobalEntity.
func (v *volume) Tag() names.Tag {
	return v.VolumeTag()
}

// Kind returns a human readable name identifying the volume kind.
func (v *volume) Kind() string {
	return v.Tag().Kind()
}

// VolumeTag is required to implement Volume.
func (v *volume) VolumeTag() names.VolumeTag {
	return names.NewVolumeTag(v.doc.Name)
}

// Life returns the volume's current lifecycle state.
func (v *volume) Life() Life {
	return Dead
}

// StorageInstance is required to implement Volume.
func (v *volume) StorageInstance() (names.StorageTag, error) {
	if v.doc.StorageId == "" {
		msg := fmt.Sprintf("volume %q is not assigned to any storage instance", v.Tag().Id())
		return names.StorageTag{}, errors.NewNotAssigned(nil, msg)
	}
	return names.NewStorageTag(v.doc.StorageId), nil
}

// Info is required to implement Volume.
func (v *volume) Info() (VolumeInfo, error) {
	if v.doc.Info == nil {
		return VolumeInfo{}, errors.NotProvisionedf("volume %q", v.doc.Name)
	}
	return *v.doc.Info, nil
}

// Params is required to implement Volume.
func (v *volume) Params() (VolumeParams, bool) {
	if v.doc.Params == nil {
		return VolumeParams{}, false
	}
	return *v.doc.Params, true
}

// Releasing is required to imeplement Volume.
func (v *volume) Releasing() bool {
	return v.doc.Releasing
}

// Status is required to implement StatusGetter.
func (v *volume) Status() (status.StatusInfo, error) {
	return status.StatusInfo{}, errors.New("volume status not implemented")
}

// SetStatus is required to implement StatusSetter.
func (v *volume) SetStatus(volumeStatus status.StatusInfo) error {
	return errors.New("volume status not implemented")
}

func (v *volumeAttachmentPlan) Volume() names.VolumeTag {
	return names.NewVolumeTag(v.doc.Volume)
}

// Machine is required to implement VolumeAttachmentPlan.
func (v *volumeAttachmentPlan) Machine() names.MachineTag {
	return names.NewMachineTag(v.doc.Machine)
}

// Life is required to implement VolumeAttachmentPlan.
func (v *volumeAttachmentPlan) Life() Life {
	return Dead
}

// PlanInfo is required to implement VolumeAttachment.
func (v *volumeAttachmentPlan) PlanInfo() (VolumeAttachmentPlanInfo, error) {
	if v.doc.PlanInfo == nil {
		return VolumeAttachmentPlanInfo{}, errors.NotProvisionedf("volume attachment plan %q on %q", v.doc.Volume, v.doc.Machine)
	}
	return *v.doc.PlanInfo, nil
}

func (v *volumeAttachmentPlan) BlockDeviceInfo() (BlockDeviceInfo, error) {
	if v.doc.BlockDevice == nil {
		return BlockDeviceInfo{}, errors.NotFoundf("volume attachment plan block device %q on %q", v.doc.Volume, v.doc.Machine)
	}
	return *v.doc.BlockDevice, nil
}

// Volume is required to implement VolumeAttachment.
func (v *volumeAttachment) Volume() names.VolumeTag {
	return names.NewVolumeTag(v.doc.Volume)
}

// Host is required to implement VolumeAttachment.
func (v *volumeAttachment) Host() names.Tag {
	return names.NewMachineTag("12")
}

// Life is required to implement VolumeAttachment.
func (v *volumeAttachment) Life() Life {
	return Dead
}

// Info is required to implement VolumeAttachment.
func (v *volumeAttachment) Info() (VolumeAttachmentInfo, error) {
	return VolumeAttachmentInfo{}, nil
}

// Params is required to implement VolumeAttachment.
func (v *volumeAttachment) Params() (VolumeAttachmentParams, bool) {
	return VolumeAttachmentParams{}, false
}

// Volume returns the Volume with the specified name.
func (sb *storageBackend) Volume(tag names.VolumeTag) (Volume, error) {
	return &volume{}, nil
}

// StorageInstanceVolume returns the Volume assigned to the specified
// storage instance.
func (sb *storageBackend) StorageInstanceVolume(tag names.StorageTag) (Volume, error) {
	return &volume{}, nil
}

// VolumeAttachment returns the VolumeAttachment corresponding to
// the specified volume and machine.
func (sb *storageBackend) VolumeAttachment(host names.Tag, volume names.VolumeTag) (VolumeAttachment, error) {
	var att volumeAttachment
	return &att, nil
}

func (sb *storageBackend) VolumeAttachmentPlan(host names.Tag, volume names.VolumeTag) (VolumeAttachmentPlan, error) {
	var att volumeAttachmentPlan
	return &att, nil
}

// MachineVolumeAttachments returns all of the VolumeAttachments for the
// specified machine.
func (sb *storageBackend) MachineVolumeAttachments(machine names.MachineTag) ([]VolumeAttachment, error) {
	return nil, nil
}

// UnitVolumeAttachments returns all of the VolumeAttachments for the
// specified unit.
func (sb *storageBackend) UnitVolumeAttachments(unit names.UnitTag) ([]VolumeAttachment, error) {
	return nil, nil
}

// VolumeAttachments returns all of the VolumeAttachments for the specified
// volume.
func (sb *storageBackend) VolumeAttachments(volume names.VolumeTag) ([]VolumeAttachment, error) {
	return nil, nil
}

// VolumeAttachmentPlans returns all of the VolumeAttachmentPlans for the specified
// volume.
func (sb *storageBackend) VolumeAttachmentPlans(volume names.VolumeTag) ([]VolumeAttachmentPlan, error) {
	return nil, nil
}

func IsContainsFilesystem(err error) bool {
	return false
}

// Detachable reports whether or not the volume is detachable.
func (v *volume) Detachable() bool {
	return false
}

// DetachVolume marks the volume attachment identified by the specified machine
// and volume tags as Dying, if it is Alive. DetachVolume will fail with a
// IsContainsFilesystem error if the volume contains an attached filesystem; the
// filesystem attachment must be removed first. DetachVolume will fail for
// inherently machine-bound volumes.
func (sb *storageBackend) DetachVolume(host names.Tag, volume names.VolumeTag, force bool) (err error) {
	return nil
}

// RemoveVolumeAttachment removes the volume attachment from state.
// RemoveVolumeAttachment will fail if the attachment is not Dying.
func (sb *storageBackend) RemoveVolumeAttachment(host names.Tag, volume names.VolumeTag, force bool) (err error) {
	return nil
}

// DestroyVolume ensures that the volume and any attachments to it will be
// destroyed and removed from state at some point in the future. DestroyVolume
// will fail with an IsContainsFilesystem error if the volume contains a
// filesystem; the filesystem must be fully removed first.
func (sb *storageBackend) DestroyVolume(tag names.VolumeTag, force bool) (err error) {
	return
}

// RemoveVolume removes the volume from state. RemoveVolume will fail if
// the volume is not Dead, which implies that it still has attachments.
func (sb *storageBackend) RemoveVolume(tag names.VolumeTag) (err error) {
	return
}

// ParseVolumeAttachmentId parses a string as a volume attachment ID,
// returning the host and volume components.
func ParseVolumeAttachmentId(id string) (names.Tag, names.VolumeTag, error) {
	fields := strings.SplitN(id, ":", 2)
	isValidHost := names.IsValidMachine(fields[0]) || names.IsValidUnit(fields[0])
	if len(fields) != 2 || !isValidHost || !names.IsValidVolume(fields[1]) {
		return names.MachineTag{}, names.VolumeTag{}, errors.Errorf("invalid volume attachment ID %q", id)
	}
	var hostTag names.Tag
	if names.IsValidMachine(fields[0]) {
		hostTag = names.NewMachineTag(fields[0])
	} else {
		hostTag = names.NewUnitTag(fields[0])
	}
	volumeTag := names.NewVolumeTag(fields[1])
	return hostTag, volumeTag, nil
}

// SetVolumeAttachmentInfo sets the VolumeAttachmentInfo for the specified
// volume attachment.
func (sb *storageBackend) SetVolumeAttachmentInfo(hostTag names.Tag, volumeTag names.VolumeTag, info VolumeAttachmentInfo) (err error) {
	return nil
}

func (sb *storageBackend) SetVolumeAttachmentPlanBlockInfo(hostTag names.Tag, volumeTag names.VolumeTag, info BlockDeviceInfo) (err error) {
	return nil
}

func (sb *storageBackend) CreateVolumeAttachmentPlan(hostTag names.Tag, volumeTag names.VolumeTag, info VolumeAttachmentPlanInfo) (err error) {
	return nil
}

// RemoveVolumeAttachmentPlan removes the volume attachment plan from state.
func (sb *storageBackend) RemoveVolumeAttachmentPlan(hostTag names.Tag, volume names.VolumeTag, force bool) (err error) {
	return nil
}

// SetVolumeInfo sets the VolumeInfo for the specified volume.
func (sb *storageBackend) SetVolumeInfo(tag names.VolumeTag, info VolumeInfo) (err error) {
	return nil
}

// AllVolumes returns all Volumes scoped to the model.
func (sb *storageBackend) AllVolumes() ([]Volume, error) {
	return nil, nil
}
