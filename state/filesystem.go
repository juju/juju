// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/juju/paths"
	"github.com/juju/juju/status"
	"github.com/juju/juju/storage"
)

// ErrNoBackingVolume is returned by Filesystem.Volume() for filesystems
// without a backing volume.
var ErrNoBackingVolume = errors.New("filesystem has no backing volume")

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

	// Machine returns the tag of the related Machine.
	Machine() names.MachineTag

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
	im  *IAASModel
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

	// MachineId is the ID of the machine that a non-detachable
	// volume is initially attached to. We use this to identify
	// the filesystem as being non-detachable, and to determine
	// which filesystems must be removed along with said machine.
	MachineId string `bson:"machineid,omitempty"`
}

// filesystemAttachmentDoc records information about a filesystem attachment.
type filesystemAttachmentDoc struct {
	// DocID is the machine global key followed by the filesystem name.
	DocID      string                      `bson:"_id"`
	ModelUUID  string                      `bson:"model-uuid"`
	Filesystem string                      `bson:"filesystemid"`
	Machine    string                      `bson:"machineid"`
	Life       Life                        `bson:"life"`
	Info       *FilesystemAttachmentInfo   `bson:"info,omitempty"`
	Params     *FilesystemAttachmentParams `bson:"params,omitempty"`
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
	return f.im.FilesystemStatus(f.FilesystemTag())
}

// SetStatus is required to implement StatusSetter.
func (f *filesystem) SetStatus(fsStatus status.StatusInfo) error {
	return f.im.SetFilesystemStatus(f.FilesystemTag(), fsStatus.Status, fsStatus.Message, fsStatus.Data, fsStatus.Since)
}

// Filesystem is required to implement FilesystemAttachment.
func (f *filesystemAttachment) Filesystem() names.FilesystemTag {
	return names.NewFilesystemTag(f.doc.Filesystem)
}

// Machine is required to implement FilesystemAttachment.
func (f *filesystemAttachment) Machine() names.MachineTag {
	return names.NewMachineTag(f.doc.Machine)
}

// Life is required to implement FilesystemAttachment.
func (f *filesystemAttachment) Life() Life {
	return f.doc.Life
}

// Info is required to implement FilesystemAttachment.
func (f *filesystemAttachment) Info() (FilesystemAttachmentInfo, error) {
	if f.doc.Info == nil {
		return FilesystemAttachmentInfo{}, errors.NotProvisionedf("filesystem attachment %q on %q", f.doc.Filesystem, f.doc.Machine)
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
func (im *IAASModel) Filesystem(tag names.FilesystemTag) (Filesystem, error) {
	f, err := im.filesystemByTag(tag)
	return f, err
}

func (im *IAASModel) filesystemByTag(tag names.FilesystemTag) (*filesystem, error) {
	doc, err := getFilesystemDocByTag(im.mb.db(), tag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &filesystem{im, doc}, nil
}

func (im *IAASModel) storageInstanceFilesystem(tag names.StorageTag) (*filesystem, error) {
	query := bson.D{{"storageid", tag.Id()}}
	description := fmt.Sprintf("filesystem for storage instance %q", tag.Id())
	return im.filesystem(query, description)
}

// StorageInstanceFilesystem returns the Filesystem assigned to the specified
// storage instance.
func (im *IAASModel) StorageInstanceFilesystem(tag names.StorageTag) (Filesystem, error) {
	f, err := im.storageInstanceFilesystem(tag)
	return f, err
}

func (im *IAASModel) volumeFilesystem(tag names.VolumeTag) (*filesystem, error) {
	query := bson.D{{"volumeid", tag.Id()}}
	description := fmt.Sprintf("filesystem for volume %q", tag.Id())
	return im.filesystem(query, description)
}

// VolumeFilesystem returns the Filesystem backed by the specified volume.
func (im *IAASModel) VolumeFilesystem(tag names.VolumeTag) (Filesystem, error) {
	f, err := im.volumeFilesystem(tag)
	return f, err
}

func (im *IAASModel) filesystems(query interface{}) ([]*filesystem, error) {
	fDocs, err := getFilesystemDocs(im.mb.db(), query)
	if err != nil {
		return nil, errors.Trace(err)
	}
	filesystems := make([]*filesystem, len(fDocs))
	for i, doc := range fDocs {
		filesystems[i] = &filesystem{im, doc}
	}
	return filesystems, nil
}

func (im *IAASModel) filesystem(query bson.D, description string) (*filesystem, error) {
	doc, err := getFilesystemDoc(im.mb.db(), query, description)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &filesystem{im, doc}, nil
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
func (im *IAASModel) FilesystemAttachment(machine names.MachineTag, filesystem names.FilesystemTag) (FilesystemAttachment, error) {
	coll, cleanup := im.mb.db().GetCollection(filesystemAttachmentsC)
	defer cleanup()

	var att filesystemAttachment
	err := coll.FindId(filesystemAttachmentId(machine.Id(), filesystem.Id())).One(&att.doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("filesystem %q on machine %q", filesystem.Id(), machine.Id())
	} else if err != nil {
		return nil, errors.Annotatef(err, "getting filesystem %q on machine %q", filesystem.Id(), machine.Id())
	}
	return &att, nil
}

// FilesystemAttachments returns all of the FilesystemAttachments for the
// specified filesystem.
func (im *IAASModel) FilesystemAttachments(filesystem names.FilesystemTag) ([]FilesystemAttachment, error) {
	attachments, err := im.filesystemAttachments(bson.D{{"filesystemid", filesystem.Id()}})
	if err != nil {
		return nil, errors.Annotatef(err, "getting attachments for filesystem %q", filesystem.Id())
	}
	return attachments, nil
}

// MachineFilesystemAttachments returns all of the FilesystemAttachments for the
// specified machine.
func (im *IAASModel) MachineFilesystemAttachments(machine names.MachineTag) ([]FilesystemAttachment, error) {
	attachments, err := im.filesystemAttachments(bson.D{{"machineid", machine.Id()}})
	if err != nil {
		return nil, errors.Annotatef(err, "getting filesystem attachments for machine %q", machine.Id())
	}
	return attachments, nil
}

func (im *IAASModel) filesystemAttachments(query bson.D) ([]FilesystemAttachment, error) {
	coll, cleanup := im.mb.db().GetCollection(filesystemAttachmentsC)
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

// removeMachineFilesystemsOps returns txn.Ops to remove non-persistent filesystems
// attached to the specified machine. This is used when the given machine is
// being removed from state.
func (im *IAASModel) removeMachineFilesystemsOps(m *Machine) ([]txn.Op, error) {
	// A machine cannot transition to Dead if it has any detachable storage
	// attached, so any attachments are for machine-bound storage.
	//
	// Even if a filesystem is "non-detachable", there still exist filesystem
	// attachments, and they may be removed independently of the filesystem.
	// For example, the user may request that the filesystem be destroyed.
	// This will cause the filesystem to become Dying, and the attachment
	// to be Dying, then Dead, and finally removed. Only once the attachment
	// is removed will the filesystem transition to Dead and then be removed.
	// Therefore, there may be filesystems that are bound, but not attached,
	// to the machine.
	machineFilesystems, err := im.filesystems(bson.D{{"machineid", m.Id()}})
	if err != nil {
		return nil, errors.Trace(err)
	}
	ops := make([]txn.Op, 0, 2*len(machineFilesystems)+len(m.doc.Filesystems))
	for _, filesystemId := range m.doc.Filesystems {
		ops = append(ops, txn.Op{
			C:      filesystemAttachmentsC,
			Id:     filesystemAttachmentId(m.Id(), filesystemId),
			Assert: txn.DocExists,
			Remove: true,
		})
	}
	for _, f := range machineFilesystems {
		filesystemId := f.Tag().Id()
		if f.doc.StorageId != "" {
			// The volume is assigned to a storage instance;
			// make sure we also remove the storage instance.
			// There should be no storage attachments remaining,
			// as the units must have been removed before the
			// machine can be; and the storage attachments must
			// have been removed before the unit can be.
			ops = append(ops,
				txn.Op{
					C:      storageInstancesC,
					Id:     f.doc.StorageId,
					Assert: txn.DocExists,
					Remove: true,
				},
			)
		}
		ops = append(ops,
			txn.Op{
				C:      filesystemsC,
				Id:     filesystemId,
				Assert: txn.DocExists,
				Remove: true,
			},
			removeModelFilesystemRefOp(im.mb, filesystemId),
		)
	}
	return ops, nil
}

// isDetachableFilesystemTag reports whether or not the filesystem with the
// specified tag is detachable.
func isDetachableFilesystemTag(db Database, tag names.FilesystemTag) (bool, error) {
	doc, err := getFilesystemDocByTag(db, tag)
	if err != nil {
		return false, errors.Trace(err)
	}
	return doc.MachineId == "", nil
}

// Detachable reports whether or not the filesystem is detachable.
func (f *filesystem) Detachable() bool {
	return f.doc.MachineId == ""
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
func isDetachableFilesystemPool(im *IAASModel, pool string) (bool, error) {
	_, provider, err := poolStorageProvider(im, pool)
	if err != nil {
		return false, errors.Trace(err)
	}
	if provider.Scope() == storage.ScopeMachine {
		// Any storage created by a machine cannot be detached from
		// the machine, and must be destroyed along with it.
		return false, nil
	}
	if !provider.Dynamic() {
		// The storage provider only accomodates provisioning storage
		// statically along with the machine. Such storage is bound
		// to the machine.
		return false, nil
	}
	return true, nil
}

// DetachFilesystem marks the filesystem attachment identified by the specified machine
// and filesystem tags as Dying, if it is Alive. DetachFilesystem will fail for
// inherently machine-bound filesystems.
func (im *IAASModel) DetachFilesystem(machine names.MachineTag, filesystem names.FilesystemTag) (err error) {
	defer errors.DeferredAnnotatef(&err, "detaching filesystem %s from machine %s", filesystem.Id(), machine.Id())
	buildTxn := func(attempt int) ([]txn.Op, error) {
		fsa, err := im.FilesystemAttachment(machine, filesystem)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if fsa.Life() != Alive {
			return nil, jujutxn.ErrNoOperations
		}
		detachable, err := isDetachableFilesystemTag(im.mb.db(), filesystem)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if !detachable {
			return nil, errors.New("filesystem is not detachable")
		}
		ops := detachFilesystemOps(machine, filesystem)
		return ops, nil
	}
	return im.mb.db().Run(buildTxn)
}

func (im *IAASModel) filesystemVolumeAttachment(m names.MachineTag, f names.FilesystemTag) (VolumeAttachment, error) {
	filesystem, err := im.Filesystem(f)
	if err != nil {
		return nil, errors.Trace(err)
	}
	v, err := filesystem.Volume()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return im.VolumeAttachment(m, v)
}

func detachFilesystemOps(m names.MachineTag, f names.FilesystemTag) []txn.Op {
	return []txn.Op{{
		C:      filesystemAttachmentsC,
		Id:     filesystemAttachmentId(m.Id(), f.Id()),
		Assert: isAliveDoc,
		Update: bson.D{{"$set", bson.D{{"life", Dying}}}},
	}}
}

// RemoveFilesystemAttachment removes the filesystem attachment from state.
// Removing a volume-backed filesystem attachment will cause the volume to
// be detached.
func (im *IAASModel) RemoveFilesystemAttachment(machine names.MachineTag, filesystem names.FilesystemTag) (err error) {
	defer errors.DeferredAnnotatef(&err, "removing attachment of filesystem %s from machine %s", filesystem.Id(), machine.Id())
	buildTxn := func(attempt int) ([]txn.Op, error) {
		attachment, err := im.FilesystemAttachment(machine, filesystem)
		if errors.IsNotFound(err) && attempt > 0 {
			// We only ignore IsNotFound on attempts after the
			// first, since we expect the filesystem attachment to
			// be there initially.
			return nil, jujutxn.ErrNoOperations
		}
		if err != nil {
			return nil, errors.Trace(err)
		}
		if attachment.Life() != Dying {
			return nil, errors.New("filesystem attachment is not dying")
		}
		f, err := im.filesystemByTag(filesystem)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops, err := removeFilesystemAttachmentOps(im, machine, f)
		if err != nil {
			return nil, errors.Trace(err)
		}
		volumeAttachment, err := im.filesystemVolumeAttachment(machine, filesystem)
		if err != nil {
			if errors.Cause(err) != ErrNoBackingVolume && !errors.IsNotFound(err) {
				return nil, errors.Trace(err)
			}
		} else {
			// The filesystem is backed by a volume. Since the
			// filesystem has been detached, we should now
			// detach the volume as well if it is detachable.
			// If the volume is not detachable, we'll just
			// destroy it along with the filesystem.
			volume := volumeAttachment.Volume()
			detachableVolume, err := isDetachableVolumeTag(im.mb.db(), volume)
			if err != nil {
				return nil, errors.Trace(err)
			}
			if detachableVolume {
				volOps := detachVolumeOps(machine, volume)
				ops = append(ops, volOps...)
			}
		}
		return ops, nil
	}
	return im.mb.db().Run(buildTxn)
}

func removeFilesystemAttachmentOps(im *IAASModel, m names.MachineTag, f *filesystem) ([]txn.Op, error) {
	var ops []txn.Op
	if f.doc.VolumeId != "" && f.doc.Life == Dying && f.doc.AttachmentCount == 1 {
		// Volume-backed filesystems are removed immediately, instead
		// of transitioning to Dead.
		assert := bson.D{
			{"life", Dying},
			{"attachmentcount", 1},
		}
		removeFilesystemOps, err := removeFilesystemOps(im, f, f.doc.Releasing, assert)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops = removeFilesystemOps
	} else {
		decrefFilesystemOp := machineStorageDecrefOp(
			filesystemsC, f.doc.FilesystemId,
			f.doc.AttachmentCount, f.doc.Life, m,
		)
		ops = []txn.Op{decrefFilesystemOp}
	}
	return append(ops, txn.Op{
		C:      filesystemAttachmentsC,
		Id:     filesystemAttachmentId(m.Id(), f.doc.FilesystemId),
		Assert: bson.D{{"life", Dying}},
		Remove: true,
	}, txn.Op{
		C:      machinesC,
		Id:     m.Id(),
		Assert: txn.DocExists,
		Update: bson.D{{"$pull", bson.D{{"filesystems", f.doc.FilesystemId}}}},
	}), nil
}

// DestroyFilesystem ensures that the filesystem and any attachments to it will
// be destroyed and removed from state at some point in the future.
func (im *IAASModel) DestroyFilesystem(tag names.FilesystemTag) (err error) {
	defer errors.DeferredAnnotatef(&err, "destroying filesystem %s", tag.Id())
	buildTxn := func(attempt int) ([]txn.Op, error) {
		filesystem, err := im.filesystemByTag(tag)
		if errors.IsNotFound(err) && attempt > 0 {
			// On the first attempt, we expect it to exist.
			return nil, jujutxn.ErrNoOperations
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		if filesystem.doc.Life != Alive {
			return nil, jujutxn.ErrNoOperations
		}
		if filesystem.doc.StorageId != "" {
			return nil, errors.Errorf(
				"filesystem is assigned to %s",
				names.ReadableString(names.NewStorageTag(filesystem.doc.StorageId)),
			)
		}
		hasNoStorageAssignment := bson.D{{"$or", []bson.D{
			{{"storageid", ""}},
			{{"storageid", bson.D{{"$exists", false}}}},
		}}}
		return destroyFilesystemOps(im, filesystem, false, hasNoStorageAssignment)
	}
	return im.mb.db().Run(buildTxn)
}

func destroyFilesystemOps(im *IAASModel, f *filesystem, release bool, extraAssert bson.D) ([]txn.Op, error) {
	baseAssert := append(isAliveDoc, extraAssert...)
	setFields := bson.D{}
	if release {
		setFields = append(setFields, bson.DocElem{"releasing", true})
	}
	if f.doc.AttachmentCount == 0 {
		hasNoAttachments := bson.D{{"attachmentcount", 0}}
		assert := append(hasNoAttachments, baseAssert...)
		if f.doc.VolumeId != "" {
			// Filesystem is volume-backed, and since it has no
			// attachments, it has no provisioner responsible
			// for it. Removing the filesystem will destroy the
			// backing volume, which effectively destroys the
			// filesystem contents anyway.
			return removeFilesystemOps(im, f, release, assert)
		}
		// The filesystem is not volume-backed, so leave it to the
		// storage provisioner to destroy it.
		setFields = append(setFields, bson.DocElem{"life", Dead})
		return []txn.Op{{
			C:      filesystemsC,
			Id:     f.doc.FilesystemId,
			Assert: assert,
			Update: bson.D{{"$set", setFields}},
		}}, nil
	}
	hasAttachments := bson.D{{"attachmentcount", bson.D{{"$gt", 0}}}}
	setFields = append(setFields, bson.DocElem{"life", Dying})
	ops := []txn.Op{{
		C:      filesystemsC,
		Id:     f.doc.FilesystemId,
		Assert: append(hasAttachments, baseAssert...),
		Update: bson.D{{"$set", setFields}},
	}}
	if !f.Detachable() {
		// This filesystem cannot be directly detached, so we do
		// not issue a cleanup. Since there can (should!) be only
		// one attachment for the lifetime of the filesystem, we
		// can look it up and destroy it along with the filesystem.
		attachments, err := im.FilesystemAttachments(f.FilesystemTag())
		if err != nil {
			return nil, errors.Trace(err)
		}
		if len(attachments) != 1 {
			return nil, errors.Errorf(
				"expected 1 attachment, found %d",
				len(attachments),
			)
		}
		detachOps := detachFilesystemOps(
			attachments[0].Machine(),
			f.FilesystemTag(),
		)
		ops = append(ops, detachOps...)
	} else {
		ops = append(ops, newCleanupOp(
			cleanupAttachmentsForDyingFilesystem,
			f.doc.FilesystemId,
		))
	}
	return ops, nil
}

// RemoveFilesystem removes the filesystem from state. RemoveFilesystem will
// fail if there are any attachments remaining, or if the filesystem is not
// Dying. Removing a volume-backed filesystem will cause the volume to be
// destroyed.
func (im *IAASModel) RemoveFilesystem(tag names.FilesystemTag) (err error) {
	defer errors.DeferredAnnotatef(&err, "removing filesystem %s", tag.Id())
	buildTxn := func(attempt int) ([]txn.Op, error) {
		filesystem, err := im.Filesystem(tag)
		if errors.IsNotFound(err) {
			return nil, jujutxn.ErrNoOperations
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		if filesystem.Life() != Dead {
			return nil, errors.New("filesystem is not dead")
		}
		return removeFilesystemOps(im, filesystem, false, isDeadDoc)
	}
	return im.mb.db().Run(buildTxn)
}

func removeFilesystemOps(im *IAASModel, filesystem Filesystem, release bool, assert interface{}) ([]txn.Op, error) {
	ops := []txn.Op{
		{
			C:      filesystemsC,
			Id:     filesystem.Tag().Id(),
			Assert: assert,
			Remove: true,
		},
		removeModelFilesystemRefOp(im.mb, filesystem.Tag().Id()),
		removeStatusOp(im.mb, filesystem.globalKey()),
	}
	// If the filesystem is backed by a volume, the volume should
	// be destroyed once the filesystem is removed. The volume must
	// not be destroyed before the filesystem is removed.
	volumeTag, err := filesystem.Volume()
	if err == nil {
		volume, err := im.volumeByTag(volumeTag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		volOps, err := destroyVolumeOps(im, volume, release, nil)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops = append(ops, volOps...)
	} else if err != ErrNoBackingVolume {
		return nil, errors.Trace(err)
	}
	return ops, nil
}

// AddExistingFilesystem imports an existing, already-provisioned
// filesystem into the model. The model will start out with
// the status "detached". The filesystem and associated backing
// volume (if any) will be associated with the given storage
// name, with the allocated storage tag being returned.
func (im *IAASModel) AddExistingFilesystem(
	info FilesystemInfo,
	backingVolume *VolumeInfo,
	storageName string,
) (_ names.StorageTag, err error) {
	defer errors.DeferredAnnotatef(&err, "cannot add existing filesystem")
	if err := validateAddExistingFilesystem(im, info, backingVolume, storageName); err != nil {
		return names.StorageTag{}, errors.Trace(err)
	}
	storageId, err := newStorageInstanceId(im.mb, storageName)
	if err != nil {
		return names.StorageTag{}, errors.Trace(err)
	}
	storageTag := names.NewStorageTag(storageId)
	fsOps, _, volumeTag, err := im.addFilesystemOps(
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
	if err := im.mb.db().RunTransaction(ops); err != nil {
		return names.StorageTag{}, errors.Trace(err)
	}
	return storageTag, nil
}

var storageNameRE = regexp.MustCompile(names.StorageNameSnippet)

func validateAddExistingFilesystem(
	im *IAASModel,
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
	_, provider, err := poolStorageProvider(im, info.Pool)
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
func filesystemAttachmentId(machineId, filesystemId string) string {
	return fmt.Sprintf("%s:%s", machineId, filesystemId)
}

// ParseFilesystemAttachmentId parses a string as a filesystem attachment ID,
// returning the machine and filesystem components.
func ParseFilesystemAttachmentId(id string) (names.MachineTag, names.FilesystemTag, error) {
	fields := strings.SplitN(id, ":", 2)
	if len(fields) != 2 || !names.IsValidMachine(fields[0]) || !names.IsValidFilesystem(fields[1]) {
		return names.MachineTag{}, names.FilesystemTag{}, errors.Errorf("invalid filesystem attachment ID %q", id)
	}
	machineTag := names.NewMachineTag(fields[0])
	filesystemTag := names.NewFilesystemTag(fields[1])
	return machineTag, filesystemTag, nil
}

// newFilesystemId returns a unique filesystem ID.
// If the machine ID supplied is non-empty, the
// filesystem ID will incorporate it as the
// filesystem's machine scope.
func newFilesystemId(mb modelBackend, machineId string) (string, error) {
	seq, err := sequence(mb, "filesystem")
	if err != nil {
		return "", errors.Trace(err)
	}
	id := fmt.Sprint(seq)
	if machineId != "" {
		id = machineId + "/" + id
	}
	return id, nil
}

// addFilesystemOps returns txn.Ops to create a new filesystem with the
// specified parameters. If the storage source cannot create filesystems
// directly, a volume will be created and Juju will manage a filesystem
// on it.
func (im *IAASModel) addFilesystemOps(params FilesystemParams, machineId string) ([]txn.Op, names.FilesystemTag, names.VolumeTag, error) {
	var err error
	params, err = im.filesystemParamsWithDefaults(params, machineId)
	if err != nil {
		return nil, names.FilesystemTag{}, names.VolumeTag{}, errors.Trace(err)
	}
	detachable, err := isDetachableFilesystemPool(im, params.Pool)
	if err != nil {
		return nil, names.FilesystemTag{}, names.VolumeTag{}, errors.Trace(err)
	}
	origMachineId := machineId
	machineId, err = im.validateFilesystemParams(params, machineId)
	if err != nil {
		return nil, names.FilesystemTag{}, names.VolumeTag{}, errors.Annotate(err, "validating filesystem params")
	}

	filesystemId, err := newFilesystemId(im.mb, machineId)
	if err != nil {
		return nil, names.FilesystemTag{}, names.VolumeTag{}, errors.Annotate(err, "cannot generate filesystem name")
	}
	filesystemTag := names.NewFilesystemTag(filesystemId)

	// Check if the filesystem needs a volume.
	var volumeId string
	var volumeTag names.VolumeTag
	var ops []txn.Op
	_, provider, err := poolStorageProvider(im, params.Pool)
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
		volumeOps, volumeTag, err = im.addVolumeOps(volumeParams, machineId)
		if err != nil {
			return nil, names.FilesystemTag{}, names.VolumeTag{}, errors.Annotate(err, "creating backing volume")
		}
		volumeId = volumeTag.Id()
		ops = append(ops, volumeOps...)
	}

	statusDoc := statusDoc{
		Status:  status.Pending,
		Updated: im.mb.clock().Now().UnixNano(),
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
		doc.MachineId = origMachineId
	}
	ops = append(ops, im.newFilesystemOps(doc, statusDoc)...)
	return ops, filesystemTag, volumeTag, nil
}

func (im *IAASModel) newFilesystemOps(doc filesystemDoc, status statusDoc) []txn.Op {
	return []txn.Op{
		createStatusOp(im.mb, filesystemGlobalKey(doc.FilesystemId), status),
		{
			C:      filesystemsC,
			Id:     doc.FilesystemId,
			Assert: txn.DocMissing,
			Insert: &doc,
		},
		addModelFilesystemRefOp(im.mb, doc.FilesystemId),
	}
}

func (im *IAASModel) filesystemParamsWithDefaults(params FilesystemParams, machineId string) (FilesystemParams, error) {
	if params.Pool == "" {
		modelConfig, err := im.st.ModelConfig()
		if err != nil {
			return FilesystemParams{}, errors.Trace(err)
		}
		cons := StorageConstraints{
			Pool:  params.Pool,
			Size:  params.Size,
			Count: 1,
		}
		poolName, err := defaultStoragePool(modelConfig, storage.StorageKindFilesystem, cons)
		if err != nil {
			return FilesystemParams{}, errors.Annotate(err, "getting default filesystem storage pool")
		}
		params.Pool = poolName
	}
	return params, nil
}

// validateFilesystemParams validates the filesystem parameters, and returns the
// machine ID to use as the scope in the filesystem tag.
func (im *IAASModel) validateFilesystemParams(params FilesystemParams, machineId string) (maybeMachineId string, _ error) {
	err := validateStoragePool(im, params.Pool, storage.StorageKindFilesystem, &machineId)
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
// attachments for the specified machine, and attachment
// parameters keyed by filesystem tags.
func createMachineFilesystemAttachmentsOps(machineId string, attachments []filesystemAttachmentTemplate) []txn.Op {
	ops := make([]txn.Op, len(attachments))
	for i, attachment := range attachments {
		paramsCopy := attachment.params
		ops[i] = txn.Op{
			C:      filesystemAttachmentsC,
			Id:     filesystemAttachmentId(machineId, attachment.tag.Id()),
			Assert: txn.DocMissing,
			Insert: &filesystemAttachmentDoc{
				Filesystem: attachment.tag.Id(),
				Machine:    machineId,
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
func (im *IAASModel) SetFilesystemInfo(tag names.FilesystemTag, info FilesystemInfo) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot set info for filesystem %q", tag.Id())

	if info.FilesystemId == "" {
		return errors.New("filesystem ID not set")
	}
	fs, err := im.Filesystem(tag)
	if err != nil {
		return errors.Trace(err)
	}
	// If the filesystem is volume-backed, the volume must be provisioned
	// and attached first.
	if volumeTag, err := fs.Volume(); err == nil {
		volumeAttachments, err := im.VolumeAttachments(volumeTag)
		if err != nil {
			return errors.Trace(err)
		}
		var anyAttached bool
		for _, a := range volumeAttachments {
			if _, err := a.Info(); err == nil {
				anyAttached = true
			} else if !errors.IsNotProvisioned(err) {
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
			fs, err = im.Filesystem(tag)
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
	return im.mb.db().Run(buildTxn)
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
func (im *IAASModel) SetFilesystemAttachmentInfo(
	machineTag names.MachineTag,
	filesystemTag names.FilesystemTag,
	info FilesystemAttachmentInfo,
) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot set info for filesystem attachment %s:%s", filesystemTag.Id(), machineTag.Id())
	f, err := im.Filesystem(filesystemTag)
	if err != nil {
		return errors.Trace(err)
	}
	// Ensure filesystem is provisioned before setting attachment info.
	// A filesystem cannot go from being provisioned to unprovisioned,
	// so there is no txn.Op for this below.
	if _, err := f.Info(); err != nil {
		return errors.Trace(err)
	}
	// Also ensure the machine is provisioned.
	m, err := im.st.Machine(machineTag.Id())
	if err != nil {
		return errors.Trace(err)
	}
	if _, err := m.InstanceId(); err != nil {
		return errors.Trace(err)
	}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		fsa, err := im.FilesystemAttachment(machineTag, filesystemTag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		// If the filesystem attachment has parameters, unset them
		// when we set info for the first time, ensuring that params
		// and info are mutually exclusive.
		_, unsetParams := fsa.Params()
		ops := setFilesystemAttachmentInfoOps(machineTag, filesystemTag, info, unsetParams)
		return ops, nil
	}
	return im.mb.db().Run(buildTxn)
}

func setFilesystemAttachmentInfoOps(
	machine names.MachineTag,
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
		Id:     filesystemAttachmentId(machine.Id(), filesystem.Id()),
		Assert: asserts,
		Update: update,
	}}
}

// filesystemMountPoint returns a mount point to use for the given charm
// storage. For stores with potentially multiple instances, the instance
// name is appended to the location.
func filesystemMountPoint(
	meta charm.Storage,
	tag names.StorageTag,
	series string,
) (string, error) {
	storageDir, err := paths.StorageDir(series)
	if err != nil {
		return "", errors.Trace(err)
	}
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
func validateFilesystemMountPoints(m *Machine, newFilesystems []filesystemAttachmentTemplate) error {
	im, err := m.st.IAASModel()
	if err != nil {
		return errors.Trace(err)
	}

	attachments, err := im.MachineFilesystemAttachments(m.MachineTag())
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
			oldFilesystem, err := im.Filesystem(oldFilesystemTag)
			if err != nil {
				return errors.Trace(err)
			}
			storageTag, err := oldFilesystem.Storage()
			if errors.IsNotAssigned(err) {
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
func (im *IAASModel) AllFilesystems() ([]Filesystem, error) {
	filesystems, err := im.filesystems(nil)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get filesystems")
	}
	return filesystemsToInterfaces(filesystems), nil
}

func filesystemsToInterfaces(fs []*filesystem) []Filesystem {
	result := make([]Filesystem, len(fs))
	for i, f := range fs {
		result[i] = f
	}
	return result
}

func filesystemGlobalKey(name string) string {
	return "f#" + name
}

// FilesystemStatus returns the status of the specified filesystem.
func (im *IAASModel) FilesystemStatus(tag names.FilesystemTag) (status.StatusInfo, error) {
	return getStatus(im.mb.db(), filesystemGlobalKey(tag.Id()), "filesystem")
}

// SetFilesystemStatus sets the status of the specified filesystem.
func (im *IAASModel) SetFilesystemStatus(tag names.FilesystemTag, fsStatus status.Status, info string, data map[string]interface{}, updated *time.Time) error {
	switch fsStatus {
	case status.Attaching, status.Attached, status.Detaching, status.Detached, status.Destroying:
	case status.Error:
		if info == "" {
			return errors.Errorf("cannot set status %q without info", fsStatus)
		}
	case status.Pending:
		// If a filesystem is not yet provisioned, we allow its status
		// to be set back to pending (when a retry is to occur).
		v, err := im.Filesystem(tag)
		if err != nil {
			return errors.Trace(err)
		}
		_, err = v.Info()
		if errors.IsNotProvisioned(err) {
			break
		}
		return errors.Errorf("cannot set status %q", fsStatus)
	default:
		return errors.Errorf("cannot set invalid status %q", fsStatus)
	}
	return setStatus(im.mb.db(), setStatusParams{
		badge:     "filesystem",
		globalKey: filesystemGlobalKey(tag.Id()),
		status:    fsStatus,
		message:   info,
		rawData:   data,
		updated:   timeOrNow(updated, im.mb.clock()),
	})
}
