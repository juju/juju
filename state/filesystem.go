// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v6"
	jujutxn "github.com/juju/txn/v3"

	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/storage"
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
	mb  modelBackend
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
	// storage, if non-zero, is the tag of the storage instance
	// that the filesystem is to be assigned to.
	storage names.StorageTag

	// filesystemId, if non-empty, is the provider-allocated unique ID
	// of the filesystem. This will be unspecified for filesystems backed
	// by volumes. This is only set when creating a filesystem entity
	// for an existing, non-volume backed, filesystem.
	filesystemId string

	// volumeInfo, if non-empty, is the information for an already
	// provisioned backing volume. This is only set when creating a
	// filesystem entity for an existing volume backed filesystem.
	volumeInfo *VolumeInfo

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
	locationAutoGenerated bool
	Location              string `bson:"location"`
	ReadOnly              bool   `bson:"read-only"`
}

// validate validates the contents of the filesystem document.
func (f *filesystemDoc) validate() error {
	return nil
}

// globalKey is required to implement GlobalEntity.
func (f *filesystem) globalKey() string {
	return filesystemGlobalKey(f.doc.FilesystemId)
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
	return f.doc.Life
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
	return getStatus(f.mb.db(), filesystemGlobalKey(f.FilesystemTag().Id()), "filesystem")
}

// SetStatus is required to implement StatusSetter.
func (f *filesystem) SetStatus(fsStatus status.StatusInfo) error {
	switch fsStatus.Status {
	case status.Attaching, status.Attached, status.Detaching, status.Detached, status.Destroying:
	case status.Error:
		if fsStatus.Message == "" {
			return errors.Errorf("cannot set status %q without message", fsStatus.Status)
		}
	case status.Pending:
		// If a filesystem is not yet provisioned, we allow its status
		// to be set back to pending (when a retry is to occur).
		// First refresh.
		f, err := getFilesystemByTag(f.mb, f.FilesystemTag())
		if err != nil {
			return errors.Trace(err)
		}
		_, err = f.Info()
		if errors.Is(err, errors.NotProvisioned) {
			break
		}
		return errors.Errorf("cannot set status %q", fsStatus.Status)
	default:
		return errors.Errorf("cannot set invalid status %q", fsStatus.Status)
	}
	return setStatus(f.mb.db(), setStatusParams{
		badge:      "filesystem",
		statusKind: f.Kind(),
		statusId:   f.FilesystemTag().Id(),
		globalKey:  filesystemGlobalKey(f.FilesystemTag().Id()),
		status:     fsStatus.Status,
		message:    fsStatus.Message,
		rawData:    fsStatus.Data,
		updated:    timeOrNow(fsStatus.Since, f.mb.clock()),
	})
}

// Filesystem is required to implement FilesystemAttachment.
func (f *filesystemAttachment) Filesystem() names.FilesystemTag {
	return names.NewFilesystemTag(f.doc.Filesystem)
}

func storageAttachmentHost(id string) names.Tag {
	if names.IsValidUnit(id) {
		return names.NewUnitTag(id)
	}
	return names.NewMachineTag(id)
}

// Host is required to implement FilesystemAttachment.
func (f *filesystemAttachment) Host() names.Tag {
	return storageAttachmentHost(f.doc.Host)
}

// Life is required to implement FilesystemAttachment.
func (f *filesystemAttachment) Life() Life {
	return f.doc.Life
}

// Info is required to implement FilesystemAttachment.
func (f *filesystemAttachment) Info() (FilesystemAttachmentInfo, error) {
	if f.doc.Info == nil {
		hostTag := storageAttachmentHost(f.doc.Host)
		return FilesystemAttachmentInfo{}, errors.NotProvisionedf(
			"filesystem attachment %q on %q", f.doc.Filesystem, names.ReadableString(hostTag))
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
	f, err := getFilesystemByTag(sb.mb, tag)
	return f, err
}

func getFilesystemByTag(mb modelBackend, tag names.FilesystemTag) (*filesystem, error) {
	doc, err := getFilesystemDocByTag(mb.db(), tag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &filesystem{mb, doc}, nil
}

func (sb *storageBackend) storageInstanceFilesystem(tag names.StorageTag) (*filesystem, error) {
	query := bson.D{{"storageid", tag.Id()}}
	description := fmt.Sprintf("filesystem for storage instance %q", tag.Id())
	return sb.filesystem(query, description)
}

// StorageInstanceFilesystem returns the Filesystem assigned to the specified
// storage instance.
func (sb *storageBackend) StorageInstanceFilesystem(tag names.StorageTag) (Filesystem, error) {
	f, err := sb.storageInstanceFilesystem(tag)
	return f, err
}

func (sb *storageBackend) volumeFilesystem(tag names.VolumeTag) (*filesystem, error) {
	query := bson.D{{"volumeid", tag.Id()}}
	description := fmt.Sprintf("filesystem for volume %q", tag.Id())
	return sb.filesystem(query, description)
}

// VolumeFilesystem returns the Filesystem backed by the specified volume.
func (sb *storageBackend) VolumeFilesystem(tag names.VolumeTag) (Filesystem, error) {
	f, err := sb.volumeFilesystem(tag)
	return f, err
}

func (sb *storageBackend) filesystems(query interface{}) ([]*filesystem, error) {
	fDocs, err := getFilesystemDocs(sb.mb.db(), query)
	if err != nil {
		return nil, errors.Trace(err)
	}
	filesystems := make([]*filesystem, len(fDocs))
	for i, doc := range fDocs {
		filesystems[i] = &filesystem{sb.mb, doc}
	}
	return filesystems, nil
}

func (sb *storageBackend) filesystem(query bson.D, description string) (*filesystem, error) {
	doc, err := getFilesystemDoc(sb.mb.db(), query, description)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &filesystem{sb.mb, doc}, nil
}

func getFilesystemDocByTag(db Database, tag names.FilesystemTag) (filesystemDoc, error) {
	query := bson.D{{"_id", tag.Id()}}
	description := fmt.Sprintf("filesystem %q", tag.Id())
	return getFilesystemDoc(db, query, description)
}

func getFilesystemDoc(db Database, query bson.D, description string) (filesystemDoc, error) {
	coll, cleanup := db.GetCollection(filesystemsC)
	defer cleanup()

	var doc filesystemDoc
	err := coll.Find(query).One(&doc)
	if err == mgo.ErrNotFound {
		return doc, errors.NotFoundf(description)
	} else if err != nil {
		return doc, errors.Annotate(err, "cannot get filesystem")
	}
	if err := doc.validate(); err != nil {
		return doc, errors.Annotate(err, "validating filesystem")
	}
	return doc, nil
}

func getFilesystemDocs(db Database, query interface{}) ([]filesystemDoc, error) {
	coll, cleanup := db.GetCollection(filesystemsC)
	defer cleanup()

	var docs []filesystemDoc
	err := coll.Find(query).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, doc := range docs {
		if err := doc.validate(); err != nil {
			return nil, errors.Annotate(err, "filesystem validation failed")
		}
	}
	return docs, nil
}

// FilesystemAttachment returns the FilesystemAttachment corresponding to
// the specified filesystem and machine.
func (sb *storageBackend) FilesystemAttachment(host names.Tag, filesystem names.FilesystemTag) (FilesystemAttachment, error) {
	coll, cleanup := sb.mb.db().GetCollection(filesystemAttachmentsC)
	defer cleanup()

	var att filesystemAttachment
	err := coll.FindId(filesystemAttachmentId(host.Id(), filesystem.Id())).One(&att.doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("filesystem %q on %q", filesystem.Id(), names.ReadableString(host))
	} else if err != nil {
		return nil, errors.Annotatef(err, "getting filesystem %q on %q", filesystem.Id(), names.ReadableString(host))
	}
	return &att, nil
}

// FilesystemAttachments returns all of the FilesystemAttachments for the
// specified filesystem.
func (sb *storageBackend) FilesystemAttachments(filesystem names.FilesystemTag) ([]FilesystemAttachment, error) {
	attachments, err := sb.filesystemAttachments(bson.D{{"filesystemid", filesystem.Id()}})
	if err != nil {
		return nil, errors.Annotatef(err, "getting attachments for filesystem %q", filesystem.Id())
	}
	return attachments, nil
}

// MachineFilesystemAttachments returns all of the FilesystemAttachments for the
// specified machine.
func (sb *storageBackend) MachineFilesystemAttachments(machine names.MachineTag) ([]FilesystemAttachment, error) {
	attachments, err := sb.filesystemAttachments(bson.D{{"hostid", machine.Id()}})
	if err != nil {
		return nil, errors.Annotatef(err, "getting filesystem attachments for %q", names.ReadableString(machine))
	}
	return attachments, nil
}

// UnitFilesystemAttachments returns all of the FilesystemAttachments for the
// specified unit.
func (sb *storageBackend) UnitFilesystemAttachments(unit names.UnitTag) ([]FilesystemAttachment, error) {
	attachments, err := sb.filesystemAttachments(bson.D{{"hostid", unit.Id()}})
	if err != nil {
		return nil, errors.Annotatef(err, "getting filesystem attachments for %q", names.ReadableString(unit))
	}
	return attachments, nil
}

func (sb *storageBackend) filesystemAttachments(query bson.D) ([]FilesystemAttachment, error) {
	coll, cleanup := sb.mb.db().GetCollection(filesystemAttachmentsC)
	defer cleanup()

	var docs []filesystemAttachmentDoc
	err := coll.Find(query).All(&docs)
	if err == mgo.ErrNotFound {
		return nil, nil
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	attachments := make([]FilesystemAttachment, len(docs))
	for i, doc := range docs {
		attachments[i] = &filesystemAttachment{doc}
	}
	return attachments, nil
}

// isDetachableFilesystemTag reports whether or not the filesystem with the
// specified tag is detachable.
func isDetachableFilesystemTag(db Database, tag names.FilesystemTag) (bool, error) {
	doc, err := getFilesystemDocByTag(db, tag)
	if err != nil {
		return false, errors.Trace(err)
	}
	return doc.HostId == "", nil
}

// Detachable reports whether or not the filesystem is detachable.
func (f *filesystem) Detachable() bool {
	return f.doc.HostId == ""
}

func (f *filesystem) pool() string {
	if f.doc.Info != nil {
		return f.doc.Info.Pool
	}
	return f.doc.Params.Pool
}

// isDetachableFilesystemPool reports whether or not the given
// storage pool will create a filesystem that is not inherently
// bound to a machine, and therefore can be detached.
func isDetachableFilesystemPool(sb *storageBackend, pool string) (bool, error) {
	_, provider, _, err := poolStorageProvider(sb, pool)
	if err != nil {
		return false, errors.Trace(err)
	}
	if provider.Scope() == storage.ScopeMachine {
		// Any storage created by a machine cannot be detached from
		// the machine, and must be destroyed along with it.
		return false, nil
	}
	if !provider.Dynamic() {
		// The storage provider only accommodates provisioning storage
		// statically along with the machine. Such storage is bound
		// to the machine.
		return false, nil
	}
	return true, nil
}

// DetachFilesystem marks the filesystem attachment identified by the specified machine
// and filesystem tags as Dying, if it is Alive. DetachFilesystem will fail for
// inherently machine-bound filesystems.
func (sb *storageBackend) DetachFilesystem(host names.Tag, filesystem names.FilesystemTag) (err error) {
	defer errors.DeferredAnnotatef(&err, "detaching filesystem %s from %s", filesystem.Id(), names.ReadableString(host))
	buildTxn := func(attempt int) ([]txn.Op, error) {
		fsa, err := sb.FilesystemAttachment(host, filesystem)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if fsa.Life() != Alive {
			return nil, jujutxn.ErrNoOperations
		}
		detachable, err := isDetachableFilesystemTag(sb.mb.db(), filesystem)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if !detachable {
			return nil, errors.New("filesystem is not detachable")
		}
		ops := detachFilesystemOps(host, filesystem)
		return ops, nil
	}
	return sb.mb.db().Run(buildTxn)
}

func (sb *storageBackend) filesystemVolumeAttachment(host names.Tag, f names.FilesystemTag) (VolumeAttachment, error) {
	filesystem, err := getFilesystemByTag(sb.mb, f)
	if err != nil {
		return nil, errors.Trace(err)
	}
	v, err := filesystem.Volume()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return sb.VolumeAttachment(host, v)
}

func detachFilesystemOps(host names.Tag, f names.FilesystemTag) []txn.Op {
	return []txn.Op{{
		C:      filesystemAttachmentsC,
		Id:     filesystemAttachmentId(host.Id(), f.Id()),
		Assert: isAliveDoc,
		Update: bson.D{{"$set", bson.D{{"life", Dying}}}},
	}}
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
	defer errors.DeferredAnnotatef(&err, "cannot add existing filesystem")
	if err := validateAddExistingFilesystem(sb.storageBackend, info, backingVolume, storageName); err != nil {
		return names.StorageTag{}, errors.Trace(err)
	}
	storageId, err := newStorageInstanceId(sb.mb, storageName)
	if err != nil {
		return names.StorageTag{}, errors.Trace(err)
	}
	storageTag := names.NewStorageTag(storageId)
	fsOps, _, volumeTag, err := sb.addFilesystemOps(
		FilesystemParams{
			Pool:         info.Pool,
			Size:         info.Size,
			filesystemId: info.FilesystemId,
			volumeInfo:   backingVolume,
			storage:      storageTag,
		},
		"", // no machine ID
	)
	if err != nil {
		return names.StorageTag{}, errors.Trace(err)
	}
	if volumeTag != (names.VolumeTag{}) && backingVolume == nil {
		return names.StorageTag{}, errors.Errorf("backing volume info missing")
	}
	ops := []txn.Op{{
		C:      storageInstancesC,
		Id:     storageId,
		Assert: txn.DocMissing,
		Insert: &storageInstanceDoc{
			Id:          storageId,
			Kind:        StorageKindFilesystem,
			StorageName: storageName,
			Constraints: storageInstanceConstraints{
				Pool: info.Pool,
				Size: info.Size,
			},
		},
	}}
	ops = append(ops, fsOps...)
	if err := sb.mb.db().RunTransaction(ops); err != nil {
		return names.StorageTag{}, errors.Trace(err)
	}
	return storageTag, nil
}

var storageNameRE = regexp.MustCompile(names.StorageNameSnippet)

func validateAddExistingFilesystem(
	sb *storageBackend,
	info FilesystemInfo,
	backingVolume *VolumeInfo,
	storageName string,
) error {
	if !storage.IsValidPoolName(info.Pool) {
		return errors.NotValidf("pool name %q", info.Pool)
	}
	if !storageNameRE.MatchString(storageName) {
		return errors.NotValidf("storage name %q", storageName)
	}
	if backingVolume == nil {
		if info.FilesystemId == "" {
			return errors.NotValidf("empty filesystem ID")
		}
	} else {
		if info.FilesystemId != "" {
			return errors.NotValidf("non-empty filesystem ID with backing volume")
		}
		if backingVolume.VolumeId == "" {
			return errors.NotValidf("empty backing volume ID")
		}
		if backingVolume.Pool != info.Pool {
			return errors.Errorf(
				"volume pool %q does not match filesystem pool %q",
				backingVolume.Pool, info.Pool,
			)
		}
		if backingVolume.Size != info.Size {
			return errors.Errorf(
				"volume size %d does not match filesystem size %d",
				backingVolume.Size, info.Size,
			)
		}
	}
	_, provider, _, err := poolStorageProvider(sb, info.Pool)
	if err != nil {
		return errors.Trace(err)
	}
	if !provider.Supports(storage.StorageKindFilesystem) {
		if backingVolume == nil {
			return errors.New("backing volume info missing")
		}
	} else {
		if backingVolume != nil {
			return errors.New("unexpected volume info")
		}
	}
	return nil
}

// filesystemAttachmentId returns a filesystem attachment document ID,
// given the corresponding filesystem name and machine ID.
func filesystemAttachmentId(hostId, filesystemId string) string {
	return fmt.Sprintf("%s:%s", hostId, filesystemId)
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

// newFilesystemId returns a unique filesystem ID.
// If the host ID supplied is non-empty, the
// filesystem ID will incorporate it as the
// filesystem's machine scope.
func newFilesystemId(mb modelBackend, hostId string) (string, error) {
	seq, err := sequence(mb, "filesystem")
	if err != nil {
		return "", errors.Trace(err)
	}
	id := fmt.Sprint(seq)
	if hostId != "" {
		id = hostId + "/" + id
	}
	return id, nil
}

// addFilesystemOps returns txn.Ops to create a new filesystem with the
// specified parameters. If the storage source cannot create filesystems
// directly, a volume will be created and Juju will manage a filesystem
// on it.
func (sb *storageConfigBackend) addFilesystemOps(params FilesystemParams, hostId string) ([]txn.Op, names.FilesystemTag, names.VolumeTag, error) {
	var err error
	params, err = sb.filesystemParamsWithDefaults(params)
	if err != nil {
		return nil, names.FilesystemTag{}, names.VolumeTag{}, errors.Trace(err)
	}
	detachable, err := isDetachableFilesystemPool(sb.storageBackend, params.Pool)
	if err != nil {
		return nil, names.FilesystemTag{}, names.VolumeTag{}, errors.Trace(err)
	}
	origHostId := hostId
	hostId, err = sb.validateFilesystemParams(params, hostId)
	if err != nil {
		return nil, names.FilesystemTag{}, names.VolumeTag{}, errors.Annotate(err, "validating filesystem params")
	}

	filesystemId, err := newFilesystemId(sb.mb, hostId)
	if err != nil {
		return nil, names.FilesystemTag{}, names.VolumeTag{}, errors.Annotate(err, "cannot generate filesystem name")
	}
	filesystemTag := names.NewFilesystemTag(filesystemId)

	// Check if the filesystem needs a volume.
	var volumeId string
	var volumeTag names.VolumeTag
	var ops []txn.Op
	_, provider, _, err := poolStorageProvider(sb.storageBackend, params.Pool)
	if err != nil {
		return nil, names.FilesystemTag{}, names.VolumeTag{}, errors.Trace(err)
	}
	if !provider.Supports(storage.StorageKindFilesystem) {
		var volumeOps []txn.Op
		if params.volumeInfo != nil {
			// The filesystem ID for volume-backed filesystems
			// is the string representation of the filesystem tag.
			params.filesystemId = filesystemTag.String()
		}
		volumeParams := VolumeParams{
			params.storage,
			params.volumeInfo,
			params.Pool,
			params.Size,
		}
		volumeOps, volumeTag, err = sb.addVolumeOps(volumeParams, hostId)
		if err != nil {
			return nil, names.FilesystemTag{}, names.VolumeTag{}, errors.Annotate(err, "creating backing volume")
		}
		volumeId = volumeTag.Id()
		ops = append(ops, volumeOps...)
	}

	statusDoc := statusDoc{
		Status:  status.Pending,
		Updated: sb.mb.clock().Now().UnixNano(),
	}
	doc := filesystemDoc{
		FilesystemId: filesystemId,
		VolumeId:     volumeId,
		StorageId:    params.storage.Id(),
	}
	if params.filesystemId != "" {
		// We're importing an already provisioned filesystem into the
		// model. Set provisioned info rather than params, and set the
		// status to "detached".
		statusDoc.Status = status.Detached
		doc.Info = &FilesystemInfo{
			Size:         params.Size,
			Pool:         params.Pool,
			FilesystemId: params.filesystemId,
		}
	} else {
		// Every new filesystem is created with one attachment.
		doc.Params = &params
		doc.AttachmentCount = 1
	}
	if !detachable {
		doc.HostId = origHostId
	}
	ops = append(ops, sb.newFilesystemOps(doc, statusDoc)...)
	return ops, filesystemTag, volumeTag, nil
}

func (sb *storageBackend) newFilesystemOps(doc filesystemDoc, status statusDoc) []txn.Op {
	return []txn.Op{
		createStatusOp(sb.mb, filesystemGlobalKey(doc.FilesystemId), status),
		{
			C:      filesystemsC,
			Id:     doc.FilesystemId,
			Assert: txn.DocMissing,
			Insert: &doc,
		},
		addModelFilesystemRefOp(sb.mb, doc.FilesystemId),
	}
}

func (sb *storageConfigBackend) filesystemParamsWithDefaults(params FilesystemParams) (FilesystemParams, error) {
	if params.Pool == "" {
		cons := StorageConstraints{
			Pool:  params.Pool,
			Size:  params.Size,
			Count: 1,
		}
		poolName, err := defaultStoragePool(sb.modelType, storage.StorageKindFilesystem, cons)
		if err != nil {
			return FilesystemParams{}, errors.Annotate(err, "getting default filesystem storage pool")
		}
		params.Pool = poolName
	}
	return params, nil
}

// validateFilesystemParams validates the filesystem parameters, and returns the
// machine ID to use as the scope in the filesystem tag.
func (sb *storageBackend) validateFilesystemParams(params FilesystemParams, machineId string) (maybeMachineId string, _ error) {
	err := validateStoragePool(sb, params.Pool, storage.StorageKindFilesystem, &machineId)
	if err != nil {
		return "", errors.Trace(err)
	}
	if params.Size == 0 {
		return "", errors.New("invalid size 0")
	}
	return machineId, nil
}

type filesystemAttachmentTemplate struct {
	tag      names.FilesystemTag
	storage  names.StorageTag // may be zero-value
	params   FilesystemAttachmentParams
	existing bool
}

// createMachineFilesystemAttachmentInfo creates filesystem
// attachments for the specified host, and attachment
// parameters keyed by filesystem tags.
func createMachineFilesystemAttachmentsOps(hostId string, attachments []filesystemAttachmentTemplate) []txn.Op {
	ops := make([]txn.Op, len(attachments))
	for i, attachment := range attachments {
		paramsCopy := attachment.params
		ops[i] = txn.Op{
			C:      filesystemAttachmentsC,
			Id:     filesystemAttachmentId(hostId, attachment.tag.Id()),
			Assert: txn.DocMissing,
			Insert: &filesystemAttachmentDoc{
				Filesystem: attachment.tag.Id(),
				Host:       hostId,
				Params:     &paramsCopy,
			},
		}
		if attachment.existing {
			ops = append(ops, txn.Op{
				C:      filesystemsC,
				Id:     attachment.tag.Id(),
				Assert: txn.DocExists,
				Update: bson.D{{"$inc", bson.D{{"attachmentcount", 1}}}},
			})
		}
	}
	return ops
}

// SetFilesystemInfo sets the FilesystemInfo for the specified filesystem.
func (sb *storageBackend) SetFilesystemInfo(tag names.FilesystemTag, info FilesystemInfo) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot set info for filesystem %q", tag.Id())

	if info.FilesystemId == "" {
		return errors.New("filesystem ID not set")
	}
	fs, err := sb.Filesystem(tag)
	if err != nil {
		return errors.Trace(err)
	}
	// If the filesystem is volume-backed, the volume must be provisioned
	// and attached first.
	if volumeTag, err := fs.Volume(); err == nil {
		volumeAttachments, err := sb.VolumeAttachments(volumeTag)
		if err != nil {
			return errors.Trace(err)
		}
		var anyAttached bool
		for _, a := range volumeAttachments {
			if _, err := a.Info(); err == nil {
				anyAttached = true
			} else if !errors.Is(err, errors.NotProvisioned) {
				return err
			}
		}
		if !anyAttached {
			return errors.Errorf(
				"backing volume %q is not attached",
				volumeTag.Id(),
			)
		}
	} else if errors.Cause(err) != ErrNoBackingVolume {
		return errors.Trace(err)
	}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			fs, err = sb.Filesystem(tag)
			if err != nil {
				return nil, errors.Trace(err)
			}
		}
		// If the filesystem has parameters, unset them
		// when we set info for the first time, ensuring
		// that params and info are mutually exclusive.
		var unsetParams bool
		if params, ok := fs.Params(); ok {
			info.Pool = params.Pool
			unsetParams = true
		} else {
			// Ensure immutable properties do not change.
			oldInfo, err := fs.Info()
			if err != nil {
				return nil, err
			}
			if err := validateFilesystemInfoChange(info, oldInfo); err != nil {
				return nil, err
			}
		}
		ops := setFilesystemInfoOps(tag, info, unsetParams)
		return ops, nil
	}
	return sb.mb.db().Run(buildTxn)
}

func validateFilesystemInfoChange(newInfo, oldInfo FilesystemInfo) error {
	if newInfo.Pool != oldInfo.Pool {
		return errors.Errorf(
			"cannot change pool from %q to %q",
			oldInfo.Pool, newInfo.Pool,
		)
	}
	if newInfo.FilesystemId != oldInfo.FilesystemId {
		return errors.Errorf(
			"cannot change filesystem ID from %q to %q",
			oldInfo.FilesystemId, newInfo.FilesystemId,
		)
	}
	return nil
}

func setFilesystemInfoOps(tag names.FilesystemTag, info FilesystemInfo, unsetParams bool) []txn.Op {
	asserts := isAliveDoc
	update := bson.D{
		{"$set", bson.D{{"info", &info}}},
	}
	if unsetParams {
		asserts = append(asserts, bson.DocElem{"info", bson.D{{"$exists", false}}})
		asserts = append(asserts, bson.DocElem{"params", bson.D{{"$exists", true}}})
		update = append(update, bson.DocElem{"$unset", bson.D{{"params", nil}}})
	}
	return []txn.Op{{
		C:      filesystemsC,
		Id:     tag.Id(),
		Assert: asserts,
		Update: update,
	}}
}

// SetFilesystemAttachmentInfo sets the FilesystemAttachmentInfo for the
// specified filesystem attachment.
func (sb *storageBackend) SetFilesystemAttachmentInfo(
	hostTag names.Tag,
	filesystemTag names.FilesystemTag,
	info FilesystemAttachmentInfo,
) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot set info for filesystem attachment %s:%s", filesystemTag.Id(), hostTag.Id())
	f, err := sb.Filesystem(filesystemTag)
	if err != nil {
		return errors.Trace(err)
	}
	// Ensure filesystem is provisioned before setting attachment info.
	// A filesystem cannot go from being provisioned to unprovisioned,
	// so there is no txn.Op for this below.
	if _, err := f.Info(); err != nil {
		return errors.Trace(err)
	}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		fsa, err := sb.FilesystemAttachment(hostTag, filesystemTag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		// If the filesystem attachment has parameters, unset them
		// when we set info for the first time, ensuring that params
		// and info are mutually exclusive.
		_, unsetParams := fsa.Params()
		ops := setFilesystemAttachmentInfoOps(hostTag, filesystemTag, info, unsetParams)
		return ops, nil
	}
	return sb.mb.db().Run(buildTxn)
}

func setFilesystemAttachmentInfoOps(
	host names.Tag,
	filesystem names.FilesystemTag,
	info FilesystemAttachmentInfo,
	unsetParams bool,
) []txn.Op {
	asserts := isAliveDoc
	update := bson.D{
		{"$set", bson.D{{"info", &info}}},
	}
	if unsetParams {
		asserts = append(asserts, bson.DocElem{"info", bson.D{{"$exists", false}}})
		asserts = append(asserts, bson.DocElem{"params", bson.D{{"$exists", true}}})
		update = append(update, bson.DocElem{"$unset", bson.D{{"params", nil}}})
	}
	return []txn.Op{{
		C:      filesystemAttachmentsC,
		Id:     filesystemAttachmentId(host.Id(), filesystem.Id()),
		Assert: asserts,
		Update: update,
	}}
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

// validateFilesystemMountPoints validates the mount points of filesystems
// being attached to the specified machine. If there are any mount point
// path conflicts, an error will be returned.
func validateFilesystemMountPoints(st *State, m MachineRef, newFilesystems []filesystemAttachmentTemplate) error {
	sb, err := NewStorageBackend(st)
	if err != nil {
		return errors.Trace(err)
	}

	attachments, err := sb.MachineFilesystemAttachments(m.MachineTag())
	if err != nil {
		return errors.Trace(err)
	}
	existing := make(map[names.FilesystemTag]string)
	for _, a := range attachments {
		params, ok := a.Params()
		if ok {
			existing[a.Filesystem()] = params.Location
			continue
		}
		info, err := a.Info()
		if err != nil {
			return errors.Trace(err)
		}
		existing[a.Filesystem()] = info.MountPoint
	}

	storageName := func(
		filesystemTag names.FilesystemTag,
		storageTag names.StorageTag,
	) string {
		if storageTag == (names.StorageTag{}) {
			return names.ReadableString(filesystemTag)
		}
		// We know the tag is valid, so ignore the error.
		storageName, _ := names.StorageName(storageTag.Id())
		return fmt.Sprintf("%q storage", storageName)
	}

	containsPath := func(a, b string) bool {
		a = path.Clean(a) + "/"
		b = path.Clean(b) + "/"
		return strings.HasPrefix(b, a)
	}

	// These sets are expected to be small, so sorting and comparing
	// adjacent values is not worth the cost of creating a reverse
	// lookup from location to filesystem.
	for _, template := range newFilesystems {
		newMountPoint := template.params.Location
		for oldFilesystemTag, oldMountPoint := range existing {
			var conflicted, swapOrder bool
			if containsPath(oldMountPoint, newMountPoint) {
				conflicted = true
			} else if containsPath(newMountPoint, oldMountPoint) {
				conflicted = true
				swapOrder = true
			}
			if !conflicted {
				continue
			}

			// Get a helpful identifier for the new filesystem. If it
			// is being created for a storage instance, then use
			// the storage name; otherwise use the filesystem name.
			newStorageName := storageName(template.tag, template.storage)

			// Likewise for the old filesystem, but this time we'll
			// need to consult state.
			oldFilesystem, err := sb.Filesystem(oldFilesystemTag)
			if err != nil {
				return errors.Trace(err)
			}
			storageTag, err := oldFilesystem.Storage()
			if errors.Is(err, errors.NotAssigned) {
				storageTag = names.StorageTag{}
			} else if err != nil {
				return errors.Trace(err)
			}
			oldStorageName := storageName(oldFilesystemTag, storageTag)

			lhs := fmt.Sprintf("mount point %q for %s", oldMountPoint, oldStorageName)
			rhs := fmt.Sprintf("mount point %q for %s", newMountPoint, newStorageName)
			if swapOrder {
				lhs, rhs = rhs, lhs
			}
			return errors.Errorf("%s contains %s", lhs, rhs)
		}
	}
	return nil
}

// AllFilesystems returns all Filesystems for this state.
func (sb *storageBackend) AllFilesystems() ([]Filesystem, error) {
	filesystems, err := sb.filesystems(nil)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get filesystems")
	}
	return filesystemsToInterfaces(filesystems), nil
}

func filesystemsToInterfaces(sb []*filesystem) []Filesystem {
	result := make([]Filesystem, len(sb))
	for i, f := range sb {
		result[i] = f
	}
	return result
}

// filesystemGlobalKeyPrefix is the kind string we use to denote filesystem
// kind.
const filesystemGlobalKeyPrefix = "f#"

// filesystemGlobalKey returns the global database key for the filesystem.
func filesystemGlobalKey(name string) string {
	return filesystemGlobalKeyPrefix + name
}
