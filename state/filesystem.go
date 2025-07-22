// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"path"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/charm"
)

// ErrNoBackingVolume is returned by Filesystem.Volume() for filesystems
// without a backing volume.
var ErrNoBackingVolume = errors.ConstError("filesystem has no backing volume")

// Filesystem describes a filesystem in the model. Filesystems may be
// backed by a volume, and managed by Juju; otherwise they are first-class
// entities managed by a filesystem provider.
type Filesystem interface {
	GlobalEntity
	Lifer
	status.StatusGetter
	status.StatusSetter

	// FilesystemTag returns the tag for the filesystem.
	FilesystemTag() names.FilesystemTag

	// Storage returns the tag of the storage instance that this
	// filesystem is assigned to, if any. If the filesystem is not
	// assigned to a storage instance, an error satisfying
	// errors.IsNotAssigned will be returned.
	//
	// A filesystem can be assigned to at most one storage instance, and
	// a storage instance can have at most one associated filesystem.
	Storage() (names.StorageTag, error)

	// Volume returns the tag of the volume backing this filesystem,
	// or ErrNoBackingVolume if the filesystem is not backed by a volume
	// managed by Juju.
	Volume() (names.VolumeTag, error)

	// Info returns the filesystem's FilesystemInfo, or a NotProvisioned
	// error if the filesystem has not yet been provisioned.
	Info() (FilesystemInfo, error)

	// Params returns the parameters for provisioning the filesystem,
	// if it needs to be provisioned. Params returns true if the returned
	// parameters are usable for provisioning, otherwise false.
	Params() (FilesystemParams, bool)

	// Detachable reports whether or not the filesystem is detachable.
	Detachable() bool

	// Releasing reports whether or not the filesystem is to be released
	// from the model when it is Dying/Dead.
	Releasing() bool
}

// FilesystemAttachment describes an attachment of a filesystem to a machine.
type FilesystemAttachment interface {
	Lifer

	// Filesystem returns the tag of the related Filesystem.
	Filesystem() names.FilesystemTag

	// Host returns the tag of the entity to which this attachment belongs.
	Host() names.Tag

	// Info returns the filesystem attachment's FilesystemAttachmentInfo, or a
	// NotProvisioned error if the attachment has not yet been made.
	//
	// Note that the presence of FilesystemAttachmentInfo does not necessarily
	// imply that the filesystem is mounted; model storage providers may
	// need to prepare a filesystem for attachment to a machine before it can
	// be mounted.
	Info() (FilesystemAttachmentInfo, error)

	// Params returns the parameters for creating the filesystem attachment,
	// if it has not already been made. Params returns true if the returned
	// parameters are usable for creating an attachment, otherwise false.
	Params() (FilesystemAttachmentParams, bool)
}

type filesystem struct {
	doc filesystemDoc
}

type filesystemAttachment struct {
	doc filesystemAttachmentDoc
}

// filesystemDoc records information about a filesystem in the model.
type filesystemDoc struct {
	DocID           string            `bson:"_id"`
	FilesystemId    string            `bson:"filesystemid"`
	ModelUUID       string            `bson:"model-uuid"`
	Life            Life              `bson:"life"`
	Releasing       bool              `bson:"releasing,omitempty"`
	StorageId       string            `bson:"storageid,omitempty"`
	VolumeId        string            `bson:"volumeid,omitempty"`
	AttachmentCount int               `bson:"attachmentcount"`
	Info            *FilesystemInfo   `bson:"info,omitempty"`
	Params          *FilesystemParams `bson:"params,omitempty"`

	// HostId is the ID of the host that a non-detachable
	// volume is initially attached to. We use this to identify
	// the filesystem as being non-detachable, and to determine
	// which filesystems must be removed along with said machine.
	HostId string `bson:"hostid,omitempty"`
}

// filesystemAttachmentDoc records information about a filesystem attachment.
type filesystemAttachmentDoc struct {
	// DocID is the machine global key followed by the filesystem name.
	DocID      string `bson:"_id"`
	ModelUUID  string `bson:"model-uuid"`
	Filesystem string `bson:"filesystemid"`

	Host   string                      `bson:"hostid"`
	Life   Life                        `bson:"life"`
	Info   *FilesystemAttachmentInfo   `bson:"info,omitempty"`
	Params *FilesystemAttachmentParams `bson:"params,omitempty"`
}

// FilesystemParams records parameters for provisioning a new filesystem.
type FilesystemParams struct {
	Pool string `bson:"pool"`
	Size uint64 `bson:"size"`
}

// FilesystemInfo describes information about a filesystem.
type FilesystemInfo struct {
	Size uint64 `bson:"size"`
	Pool string `bson:"pool"`

	// FilesystemId is the provider-allocated unique ID of the
	// filesystem. This will be the string representation of
	// the filesystem tag for filesystems backed by volumes.
	FilesystemId string `bson:"filesystemid"`
}

// FilesystemAttachmentInfo describes information about a filesystem attachment.
type FilesystemAttachmentInfo struct {
	// MountPoint is the path at which the filesystem is mounted on the
	// machine. MountPoint may be empty, meaning that the filesystem is
	// not mounted yet.
	MountPoint string `bson:"mountpoint"`
	ReadOnly   bool   `bson:"read-only"`
}

// FilesystemAttachmentParams records parameters for attaching a filesystem to a
// machine.
type FilesystemAttachmentParams struct {
	// locationAutoGenerated records whether or not the Location
	// field's value was automatically generated, and thus known
	// to be unique. This is used to optimise away mount point
	// conflict checks.
	Location string `bson:"location"`
	ReadOnly bool   `bson:"read-only"`
}

// Tag is required to implement GlobalEntity.
func (f *filesystem) Tag() names.Tag {
	return f.FilesystemTag()
}

// Kind returns a human readable name identifying the filesystem kind.
func (f *filesystem) Kind() string {
	return f.Tag().Kind()
}

// FilesystemTag is required to implement Filesystem.
func (f *filesystem) FilesystemTag() names.FilesystemTag {
	return names.NewFilesystemTag(f.doc.FilesystemId)
}

// Life is required to implement Filesystem.
func (f *filesystem) Life() Life {
	return Dead
}

// Storage is required to implement Filesystem.
func (f *filesystem) Storage() (names.StorageTag, error) {
	if f.doc.StorageId == "" {
		msg := fmt.Sprintf("filesystem %q is not assigned to any storage instance", f.Tag().Id())
		return names.StorageTag{}, errors.NewNotAssigned(nil, msg)
	}
	return names.NewStorageTag(f.doc.StorageId), nil
}

// Volume is required to implement Filesystem.
func (f *filesystem) Volume() (names.VolumeTag, error) {
	if f.doc.VolumeId == "" {
		return names.VolumeTag{}, ErrNoBackingVolume
	}
	return names.NewVolumeTag(f.doc.VolumeId), nil
}

// Info is required to implement Filesystem.
func (f *filesystem) Info() (FilesystemInfo, error) {
	if f.doc.Info == nil {
		return FilesystemInfo{}, errors.NotProvisionedf("filesystem %q", f.doc.FilesystemId)
	}
	return *f.doc.Info, nil
}

// Params is required to implement Filesystem.
func (f *filesystem) Params() (FilesystemParams, bool) {
	if f.doc.Params == nil {
		return FilesystemParams{}, false
	}
	return *f.doc.Params, true
}

// Releasing is required to implement Filesystem.
func (f *filesystem) Releasing() bool {
	return f.doc.Releasing
}

// Status is required to implement StatusGetter.
func (f *filesystem) Status() (status.StatusInfo, error) {
	return status.StatusInfo{}, errors.New("filesystem status not implemented")
}

// SetStatus is required to implement StatusSetter.
func (f *filesystem) SetStatus(fsStatus status.StatusInfo) error {
	return errors.New("filesystem status not implemented")
}

// Filesystem is required to implement FilesystemAttachment.
func (f *filesystemAttachment) Filesystem() names.FilesystemTag {
	return names.NewFilesystemTag(f.doc.Filesystem)
}

// Host is required to implement FilesystemAttachment.
func (f *filesystemAttachment) Host() names.Tag {
	return nil
}

// Life is required to implement FilesystemAttachment.
func (f *filesystemAttachment) Life() Life {
	return Dead
}

// Info is required to implement FilesystemAttachment.
func (f *filesystemAttachment) Info() (FilesystemAttachmentInfo, error) {
	if f.doc.Info == nil {
		return FilesystemAttachmentInfo{}, nil
	}
	return *f.doc.Info, nil
}

// Params is required to implement FilesystemAttachment.
func (f *filesystemAttachment) Params() (FilesystemAttachmentParams, bool) {
	if f.doc.Params == nil {
		return FilesystemAttachmentParams{}, false
	}
	return *f.doc.Params, true
}

// Filesystem returns the Filesystem with the specified name.
func (sb *storageBackend) Filesystem(tag names.FilesystemTag) (Filesystem, error) {
	return &filesystem{}, nil
}

// StorageInstanceFilesystem returns the Filesystem assigned to the specified
// storage instance.
func (sb *storageBackend) StorageInstanceFilesystem(tag names.StorageTag) (Filesystem, error) {
	return &filesystem{}, nil
}

// VolumeFilesystem returns the Filesystem backed by the specified volume.
func (sb *storageBackend) VolumeFilesystem(tag names.VolumeTag) (Filesystem, error) {
	return &filesystem{}, nil
}

// FilesystemAttachment returns the FilesystemAttachment corresponding to
// the specified filesystem and machine.
func (sb *storageBackend) FilesystemAttachment(host names.Tag, filesystem names.FilesystemTag) (FilesystemAttachment, error) {
	var att filesystemAttachment
	return &att, nil
}

// FilesystemAttachments returns all of the FilesystemAttachments for the
// specified filesystem.
func (sb *storageBackend) FilesystemAttachments(filesystem names.FilesystemTag) ([]FilesystemAttachment, error) {
	return nil, nil
}

// MachineFilesystemAttachments returns all of the FilesystemAttachments for the
// specified machine.
func (sb *storageBackend) MachineFilesystemAttachments(machine names.MachineTag) ([]FilesystemAttachment, error) {
	return nil, nil
}

// UnitFilesystemAttachments returns all of the FilesystemAttachments for the
// specified unit.
func (sb *storageBackend) UnitFilesystemAttachments(unit names.UnitTag) ([]FilesystemAttachment, error) {
	return nil, nil
}

// Detachable reports whether or not the filesystem is detachable.
func (f *filesystem) Detachable() bool {
	return f.doc.HostId == ""
}

// DetachFilesystem marks the filesystem attachment identified by the specified machine
// and filesystem tags as Dying, if it is Alive. DetachFilesystem will fail for
// inherently machine-bound filesystems.
func (sb *storageBackend) DetachFilesystem(host names.Tag, filesystem names.FilesystemTag) (err error) {
	return nil
}

// RemoveFilesystemAttachment removes the filesystem attachment from state.
// Removing a volume-backed filesystem attachment will cause the volume to
// be detached.
func (sb *storageBackend) RemoveFilesystemAttachment(host names.Tag, filesystem names.FilesystemTag, force bool) (err error) {
	return
}

// DestroyFilesystem ensures that the filesystem and any attachments to it will
// be destroyed and removed from state at some point in the future.
func (sb *storageBackend) DestroyFilesystem(tag names.FilesystemTag, force bool) (err error) {
	return
}

// RemoveFilesystem removes the filesystem from state. RemoveFilesystem will
// fail if there are any attachments remaining, or if the filesystem is not
// Dying. Removing a volume-backed filesystem will cause the volume to be
// destroyed.
func (sb *storageBackend) RemoveFilesystem(tag names.FilesystemTag) (err error) {
	return
}

// AddExistingFilesystem imports an existing, already-provisioned
// filesystem into the model. The model will start out with
// the status "detached". The filesystem and associated backing
// volume (if any) will be associated with the given storage
// name, with the allocated storage tag being returned.
func (sb *storageConfigBackend) AddExistingFilesystem(
	info FilesystemInfo,
	backingVolume *VolumeInfo,
	storageName string,
) (_ names.StorageTag, err error) {
	return names.NewStorageTag("1"), nil
}

// ParseFilesystemAttachmentId parses a string as a filesystem attachment ID,
// returning the host and filesystem components.
func ParseFilesystemAttachmentId(id string) (names.Tag, names.FilesystemTag, error) {
	fields := strings.SplitN(id, ":", 2)
	isValidHost := names.IsValidMachine(fields[0]) || names.IsValidUnit(fields[0])
	if len(fields) != 2 || !isValidHost || !names.IsValidFilesystem(fields[1]) {
		return names.MachineTag{}, names.FilesystemTag{}, errors.Errorf("invalid filesystem attachment ID %q", id)
	}
	var hostTag names.Tag
	if names.IsValidMachine(fields[0]) {
		hostTag = names.NewMachineTag(fields[0])
	} else {
		hostTag = names.NewUnitTag(fields[0])
	}
	filesystemTag := names.NewFilesystemTag(fields[1])
	return hostTag, filesystemTag, nil
}

// SetFilesystemInfo sets the FilesystemInfo for the specified filesystem.
func (sb *storageBackend) SetFilesystemInfo(tag names.FilesystemTag, info FilesystemInfo) (err error) {
	return nil
}

// SetFilesystemAttachmentInfo sets the FilesystemAttachmentInfo for the
// specified filesystem attachment.
func (sb *storageBackend) SetFilesystemAttachmentInfo(
	hostTag names.Tag,
	filesystemTag names.FilesystemTag,
	info FilesystemAttachmentInfo,
) (err error) {
	return nil
}

// FilesystemMountPoint returns a mount point to use for the given charm
// storage. For stores with potentially multiple instances, the instance
// name is appended to the location.
func FilesystemMountPoint(
	meta charm.Storage,
	tag names.StorageTag,
	osname string,
) (string, error) {
	storageDir := paths.StorageDir(paths.OSType(osname))
	if strings.HasPrefix(meta.Location, storageDir) {
		return "", errors.Errorf(
			"invalid location %q: must not fall within %q",
			meta.Location, storageDir,
		)
	}
	if meta.Location != "" && meta.CountMax == 1 {
		// The location is specified and it's a singleton
		// store, so just use the location as-is.
		return meta.Location, nil
	}
	// If the location is unspecified then we use
	// <storage-dir>/<storage-id> as the location.
	// Otherwise, we use <location>/<storage-id>.
	if meta.Location != "" {
		storageDir = meta.Location
	}
	return path.Join(storageDir, tag.Id()), nil
}

// AllFilesystems returns all Filesystems for this state.
func (sb *storageBackend) AllFilesystems() ([]Filesystem, error) {
	return nil, nil
}
