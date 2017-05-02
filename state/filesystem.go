// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"path"
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
	st  *State
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
	StorageId       string            `bson:"storageid,omitempty"`
	VolumeId        string            `bson:"volumeid,omitempty"`
	AttachmentCount int               `bson:"attachmentcount"`
	Info            *FilesystemInfo   `bson:"info,omitempty"`
	Params          *FilesystemParams `bson:"params,omitempty"`

	// MachineId is the ID of the machine that a non-detachable
	// volume is initially attached to. We use this to identify
	// the volume as being non-detachable, and to determine
	// which volumes must be removed along with said machine.
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

	Pool string `bson:"pool"`
	Size uint64 `bson:"size"`
}

// FilesystemInfo describes information about a filesystem.
type FilesystemInfo struct {
	Size uint64 `bson:"size"`
	Pool string `bson:"pool"`

	// FilesystemId is the provider-allocated unique ID of the
	// filesystem. This will be unspecified for filesystems
	// backed by volumes.
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
func (f *filesystem) validate() error {
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

// Status is required to implement StatusGetter.
func (f *filesystem) Status() (status.StatusInfo, error) {
	return f.st.FilesystemStatus(f.FilesystemTag())
}

// SetStatus is required to implement StatusSetter.
func (f *filesystem) SetStatus(fsStatus status.StatusInfo) error {
	return f.st.SetFilesystemStatus(f.FilesystemTag(), fsStatus.Status, fsStatus.Message, fsStatus.Data, fsStatus.Since)
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
func (st *State) Filesystem(tag names.FilesystemTag) (Filesystem, error) {
	f, err := st.filesystemByTag(tag)
	return f, err
}

func (st *State) filesystemByTag(tag names.FilesystemTag) (*filesystem, error) {
	query := bson.D{{"_id", tag.Id()}}
	description := fmt.Sprintf("filesystem %q", tag.Id())
	return st.filesystem(query, description)
}

func (st *State) storageInstanceFilesystem(tag names.StorageTag) (*filesystem, error) {
	query := bson.D{{"storageid", tag.Id()}}
	description := fmt.Sprintf("filesystem for storage instance %q", tag.Id())
	return st.filesystem(query, description)
}

// StorageInstanceFilesystem returns the Filesystem assigned to the specified
// storage instance.
func (st *State) StorageInstanceFilesystem(tag names.StorageTag) (Filesystem, error) {
	f, err := st.storageInstanceFilesystem(tag)
	return f, err
}

func (st *State) volumeFilesystem(tag names.VolumeTag) (*filesystem, error) {
	query := bson.D{{"volumeid", tag.Id()}}
	description := fmt.Sprintf("filesystem for volume %q", tag.Id())
	return st.filesystem(query, description)
}

// VolumeFilesystem returns the Filesystem backed by the specified volume.
func (st *State) VolumeFilesystem(tag names.VolumeTag) (Filesystem, error) {
	f, err := st.volumeFilesystem(tag)
	return f, err
}

func (st *State) filesystems(query interface{}) ([]*filesystem, error) {
	coll, cleanup := st.db().GetCollection(filesystemsC)
	defer cleanup()

	var fDocs []filesystemDoc
	err := coll.Find(query).All(&fDocs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	filesystems := make([]*filesystem, len(fDocs))
	for i, doc := range fDocs {
		f := &filesystem{st, doc}
		if err := f.validate(); err != nil {
			return nil, errors.Annotate(err, "filesystem validation failed")
		}
		filesystems[i] = f
	}
	return filesystems, nil
}

func (st *State) filesystem(query bson.D, description string) (*filesystem, error) {
	coll, cleanup := st.db().GetCollection(filesystemsC)
	defer cleanup()

	f := filesystem{st: st}
	err := coll.Find(query).One(&f.doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf(description)
	} else if err != nil {
		return nil, errors.Annotate(err, "cannot get filesystem")
	}
	if err := f.validate(); err != nil {
		return nil, errors.Annotate(err, "validating filesystem")
	}
	return &f, nil
}

// FilesystemAttachment returns the FilesystemAttachment corresponding to
// the specified filesystem and machine.
func (st *State) FilesystemAttachment(machine names.MachineTag, filesystem names.FilesystemTag) (FilesystemAttachment, error) {
	coll, cleanup := st.db().GetCollection(filesystemAttachmentsC)
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
func (st *State) FilesystemAttachments(filesystem names.FilesystemTag) ([]FilesystemAttachment, error) {
	attachments, err := st.filesystemAttachments(bson.D{{"filesystemid", filesystem.Id()}})
	if err != nil {
		return nil, errors.Annotatef(err, "getting attachments for filesystem %q", filesystem.Id())
	}
	return attachments, nil
}

// MachineFilesystemAttachments returns all of the FilesystemAttachments for the
// specified machine.
func (st *State) MachineFilesystemAttachments(machine names.MachineTag) ([]FilesystemAttachment, error) {
	attachments, err := st.filesystemAttachments(bson.D{{"machineid", machine.Id()}})
	if err != nil {
		return nil, errors.Annotatef(err, "getting filesystem attachments for machine %q", machine.Id())
	}
	return attachments, nil
}

func (st *State) filesystemAttachments(query bson.D) ([]FilesystemAttachment, error) {
	coll, cleanup := st.db().GetCollection(filesystemAttachmentsC)
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
func (st *State) removeMachineFilesystemsOps(m *Machine) ([]txn.Op, error) {
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
	machineFilesystems, err := st.filesystems(bson.D{{"machineid", m.Id()}})
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
			removeModelFilesystemRefOp(st, filesystemId),
		)
	}
	return ops, nil
}

// isDetachableFilesystemTag reports whether or not the filesystem with the
// specified tag is detachable.
func isDetachableFilesystemTag(st *State, tag names.FilesystemTag) (bool, error) {
	f, err := st.filesystemByTag(tag)
	if err != nil {
		return false, errors.Trace(err)
	}
	return f.Detachable(), nil
}

// Detachable reports whether or not the filesystem is detachable.
func (f *filesystem) Detachable() bool {
	return f.doc.MachineId == ""
}

// isDetachableFilesystemPool reports whether or not the given
// storage pool will create a filesystem that is not inherently
// bound to a machine, and therefore can be detached.
func isDetachableFilesystemPool(st *State, pool string) (bool, error) {
	_, provider, err := poolStorageProvider(st, pool)
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
	if !provider.Supports(storage.StorageKindFilesystem) {
		// TODO(axw) remove this when volume-backed filesystems
		// inherit the scope of the volume. For now, volume-backed
		// filesystems are always machine-scoped.
		return false, nil
	}
	return true, nil
}

// DetachFilesystem marks the filesystem attachment identified by the specified machine
// and filesystem tags as Dying, if it is Alive. DetachFilesystem will fail for
// inherently machine-bound filesystems.
func (st *State) DetachFilesystem(machine names.MachineTag, filesystem names.FilesystemTag) (err error) {
	defer errors.DeferredAnnotatef(&err, "detaching filesystem %s from machine %s", filesystem.Id(), machine.Id())
	buildTxn := func(attempt int) ([]txn.Op, error) {
		fsa, err := st.FilesystemAttachment(machine, filesystem)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if fsa.Life() != Alive {
			return nil, jujutxn.ErrNoOperations
		}
		detachable, err := isDetachableFilesystemTag(st, filesystem)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if !detachable {
			return nil, errors.New("filesystem is not detachable")
		}
		ops := detachFilesystemOps(machine, filesystem)
		return ops, nil
	}
	return st.run(buildTxn)
}

func (st *State) filesystemVolumeAttachment(m names.MachineTag, f names.FilesystemTag) (VolumeAttachment, error) {
	filesystem, err := st.Filesystem(f)
	if err != nil {
		return nil, errors.Trace(err)
	}
	v, err := filesystem.Volume()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return st.VolumeAttachment(m, v)
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
func (st *State) RemoveFilesystemAttachment(machine names.MachineTag, filesystem names.FilesystemTag) (err error) {
	defer errors.DeferredAnnotatef(&err, "removing attachment of filesystem %s from machine %s", filesystem.Id(), machine.Id())
	buildTxn := func(attempt int) ([]txn.Op, error) {
		attachment, err := st.FilesystemAttachment(machine, filesystem)
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
		f, err := st.filesystemByTag(filesystem)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops := removeFilesystemAttachmentOps(machine, f)
		volumeAttachment, err := st.filesystemVolumeAttachment(machine, filesystem)
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
			detachableVolume, err := isDetachableVolumeTag(st, volume)
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
	return st.run(buildTxn)
}

func removeFilesystemAttachmentOps(m names.MachineTag, f *filesystem) []txn.Op {
	decrefFilesystemOp := machineStorageDecrefOp(
		filesystemsC, f.doc.FilesystemId,
		f.doc.AttachmentCount, f.doc.Life, m,
	)
	return []txn.Op{{
		C:      filesystemAttachmentsC,
		Id:     filesystemAttachmentId(m.Id(), f.doc.FilesystemId),
		Assert: bson.D{{"life", Dying}},
		Remove: true,
	}, decrefFilesystemOp, {
		C:      machinesC,
		Id:     m.Id(),
		Assert: txn.DocExists,
		Update: bson.D{{"$pull", bson.D{{"filesystems", f.doc.FilesystemId}}}},
	}}
}

// DestroyFilesystem ensures that the filesystem and any attachments to it will
// be destroyed and removed from state at some point in the future.
func (st *State) DestroyFilesystem(tag names.FilesystemTag) (err error) {
	defer errors.DeferredAnnotatef(&err, "destroying filesystem %s", tag.Id())
	buildTxn := func(attempt int) ([]txn.Op, error) {
		filesystem, err := st.filesystemByTag(tag)
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
		return destroyFilesystemOps(st, filesystem, hasNoStorageAssignment)
	}
	return st.run(buildTxn)
}

func destroyFilesystemOps(st *State, f *filesystem, extraAssert bson.D) ([]txn.Op, error) {
	baseAssert := append(isAliveDoc, extraAssert...)
	if f.doc.AttachmentCount == 0 {
		hasNoAttachments := bson.D{{"attachmentcount", 0}}
		return []txn.Op{{
			C:      filesystemsC,
			Id:     f.doc.FilesystemId,
			Assert: append(hasNoAttachments, baseAssert...),
			Update: bson.D{{"$set", bson.D{{"life", Dead}}}},
		}}, nil
	}
	hasAttachments := bson.D{{"attachmentcount", bson.D{{"$gt", 0}}}}
	ops := []txn.Op{{
		C:      filesystemsC,
		Id:     f.doc.FilesystemId,
		Assert: append(hasAttachments, baseAssert...),
		Update: bson.D{{"$set", bson.D{{"life", Dying}}}},
	}}
	if !f.Detachable() {
		// This filesystem cannot be directly detached, so we do
		// not issue a cleanup. Since there can (should!) be only
		// one attachment for the lifetime of the filesystem, we
		// can look it up and destroy it along with the filesystem.
		attachments, err := st.FilesystemAttachments(f.FilesystemTag())
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
func (st *State) RemoveFilesystem(tag names.FilesystemTag) (err error) {
	defer errors.DeferredAnnotatef(&err, "removing filesystem %s", tag.Id())
	buildTxn := func(attempt int) ([]txn.Op, error) {
		filesystem, err := st.Filesystem(tag)
		if errors.IsNotFound(err) {
			return nil, jujutxn.ErrNoOperations
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		if filesystem.Life() != Dead {
			return nil, errors.New("filesystem is not dead")
		}
		return removeFilesystemOps(st, filesystem)
	}
	return st.run(buildTxn)
}

func removeFilesystemOps(st *State, filesystem Filesystem) ([]txn.Op, error) {
	ops := []txn.Op{
		{
			C:      filesystemsC,
			Id:     filesystem.Tag().Id(),
			Assert: txn.DocExists,
			Remove: true,
		},
		removeModelFilesystemRefOp(st, filesystem.Tag().Id()),
		removeStatusOp(st, filesystem.globalKey()),
	}
	// If the filesystem is backed by a volume, the volume should
	// be destroyed once the filesystem is removed. The volume must
	// not be destroyed before the filesystem is removed.
	volumeTag, err := filesystem.Volume()
	if err == nil {
		volume, err := st.volumeByTag(volumeTag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		volOps, err := destroyVolumeOps(st, volume, nil)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops = append(ops, volOps...)
	} else if err != ErrNoBackingVolume {
		return nil, errors.Trace(err)
	}
	return ops, nil
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
func newFilesystemId(st *State, machineId string) (string, error) {
	seq, err := st.sequence("filesystem")
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
func (st *State) addFilesystemOps(params FilesystemParams, machineId string) ([]txn.Op, names.FilesystemTag, names.VolumeTag, error) {
	// TODO(axw) the scope of a volume-backed filesystem should be the
	// same as the volume. Machine storage provisioners would be
	// responsible for managing filesystems backed by volumes attached
	// to that machine. Making this change will enable persistent
	// filesystems; until then, destroying a volume-backed filesystem
	// always destroys the volume.
	params, err := st.filesystemParamsWithDefaults(params, machineId)
	if err != nil {
		return nil, names.FilesystemTag{}, names.VolumeTag{}, errors.Trace(err)
	}
	detachable, err := isDetachableFilesystemPool(st, params.Pool)
	if err != nil {
		return nil, names.FilesystemTag{}, names.VolumeTag{}, errors.Trace(err)
	}
	origMachineId := machineId
	machineId, err = st.validateFilesystemParams(params, machineId)
	if err != nil {
		return nil, names.FilesystemTag{}, names.VolumeTag{}, errors.Annotate(err, "validating filesystem params")
	}

	filesystemId, err := newFilesystemId(st, machineId)
	if err != nil {
		return nil, names.FilesystemTag{}, names.VolumeTag{}, errors.Annotate(err, "cannot generate filesystem name")
	}
	filesystemTag := names.NewFilesystemTag(filesystemId)

	// Check if the filesystem needs a volume.
	var volumeId string
	var volumeTag names.VolumeTag
	var ops []txn.Op
	_, provider, err := poolStorageProvider(st, params.Pool)
	if err != nil {
		return nil, names.FilesystemTag{}, names.VolumeTag{}, errors.Trace(err)
	}
	if !provider.Supports(storage.StorageKindFilesystem) {
		var volumeOps []txn.Op
		volumeParams := VolumeParams{
			params.storage,
			params.Pool,
			params.Size,
		}
		volumeOps, volumeTag, err = st.addVolumeOps(volumeParams, machineId)
		if err != nil {
			return nil, names.FilesystemTag{}, names.VolumeTag{}, errors.Annotate(err, "creating backing volume")
		}
		volumeId = volumeTag.Id()
		ops = append(ops, volumeOps...)
	}

	status := statusDoc{
		Status:  status.Pending,
		Updated: st.clock.Now().UnixNano(),
	}
	doc := filesystemDoc{
		FilesystemId: filesystemId,
		VolumeId:     volumeId,
		StorageId:    params.storage.Id(),
		Params:       &params,
		// Every filesystem is created with one attachment.
		AttachmentCount: 1,
	}
	if !detachable {
		doc.MachineId = origMachineId
	}
	ops = append(ops, st.newFilesystemOps(doc, status)...)
	return ops, filesystemTag, volumeTag, nil
}

func (st *State) newFilesystemOps(doc filesystemDoc, status statusDoc) []txn.Op {
	return []txn.Op{
		createStatusOp(st, filesystemGlobalKey(doc.FilesystemId), status),
		{
			C:      filesystemsC,
			Id:     doc.FilesystemId,
			Assert: txn.DocMissing,
			Insert: &doc,
		},
	}
}

func (st *State) filesystemParamsWithDefaults(params FilesystemParams, machineId string) (FilesystemParams, error) {
	if params.Pool == "" {
		modelConfig, err := st.ModelConfig()
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
func (st *State) validateFilesystemParams(params FilesystemParams, machineId string) (maybeMachineId string, _ error) {
	err := validateStoragePool(st, params.Pool, storage.StorageKindFilesystem, &machineId)
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
func (st *State) SetFilesystemInfo(tag names.FilesystemTag, info FilesystemInfo) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot set info for filesystem %q", tag.Id())
	if info.FilesystemId == "" {
		return errors.New("filesystem ID not set")
	}
	fs, err := st.Filesystem(tag)
	if err != nil {
		return errors.Trace(err)
	}
	// If the filesystem is volume-backed, the volume must be provisioned
	// and attachment first.
	if volumeTag, err := fs.Volume(); err == nil {
		machineTag, ok := names.FilesystemMachine(tag)
		if !ok {
			return errors.Errorf("filesystem %s is not machine-scoped, but volume-backed", tag.Id())
		}
		volumeAttachment, err := st.VolumeAttachment(machineTag, volumeTag)
		if err != nil {
			return errors.Trace(err)
		}
		if _, err := volumeAttachment.Info(); err != nil {
			return errors.Trace(err)
		}
	} else if errors.Cause(err) != ErrNoBackingVolume {
		return errors.Trace(err)
	}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			fs, err = st.Filesystem(tag)
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
	return st.run(buildTxn)
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
func (st *State) SetFilesystemAttachmentInfo(
	machineTag names.MachineTag,
	filesystemTag names.FilesystemTag,
	info FilesystemAttachmentInfo,
) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot set info for filesystem attachment %s:%s", filesystemTag.Id(), machineTag.Id())
	f, err := st.Filesystem(filesystemTag)
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
	m, err := st.Machine(machineTag.Id())
	if err != nil {
		return errors.Trace(err)
	}
	if _, err := m.InstanceId(); err != nil {
		return errors.Trace(err)
	}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		fsa, err := st.FilesystemAttachment(machineTag, filesystemTag)
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
	return st.run(buildTxn)
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
	attachments, err := m.st.MachineFilesystemAttachments(m.MachineTag())
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
			oldFilesystem, err := m.st.Filesystem(oldFilesystemTag)
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
func (st *State) AllFilesystems() ([]Filesystem, error) {
	filesystems, err := st.filesystems(nil)
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
func (st *State) FilesystemStatus(tag names.FilesystemTag) (status.StatusInfo, error) {
	return getStatus(st, filesystemGlobalKey(tag.Id()), "filesystem")
}

// SetFilesystemStatus sets the status of the specified filesystem.
func (st *State) SetFilesystemStatus(tag names.FilesystemTag, fsStatus status.Status, info string, data map[string]interface{}, updated *time.Time) error {
	switch fsStatus {
	case status.Attaching, status.Attached, status.Detaching, status.Detached, status.Destroying:
	case status.Error:
		if info == "" {
			return errors.Errorf("cannot set status %q without info", fsStatus)
		}
	case status.Pending:
		// If a filesystem is not yet provisioned, we allow its status
		// to be set back to pending (when a retry is to occur).
		v, err := st.Filesystem(tag)
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
	return setStatus(st, setStatusParams{
		badge:     "filesystem",
		globalKey: filesystemGlobalKey(tag.Id()),
		status:    fsStatus,
		message:   info,
		rawData:   data,
		updated:   updated,
	})
}
