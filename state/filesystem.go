// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"path"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names"
	jujutxn "github.com/juju/txn"
	"gopkg.in/juju/charm.v5"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/juju/paths"
	"github.com/juju/juju/storage"
)

// ErrNoBackingVolume is returned by Filesystem.Volume() for filesystems
// without a backing volume.
var ErrNoBackingVolume = errors.New("filesystem has no backing volume")

// Filesystem describes a filesystem in the environment. Filesystems may be
// backed by a volume, and managed by Juju; otherwise they are first-class
// entities managed by a filesystem provider.
type Filesystem interface {
	Entity
	LifeBinder

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
	// imply that the filesystem is mounted; environment storage providers may
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

// filesystemDoc records information about a filesystem in the environment.
type filesystemDoc struct {
	DocID        string `bson:"_id"`
	FilesystemId string `bson:"filesystemid"`
	EnvUUID      string `bson:"env-uuid"`
	Life         Life   `bson:"life"`
	StorageId    string `bson:"storageid,omitempty"`
	VolumeId     string `bson:"volumeid,omitempty"`
	// TODO(axw) 2015-06-22 #1467379
	// upgrade step to set "attachmentcount" and "binding"
	// for 1.24 environments.
	AttachmentCount int               `bson:"attachmentcount"`
	Binding         string            `bson:"binding,omitempty"`
	Info            *FilesystemInfo   `bson:"info,omitempty"`
	Params          *FilesystemParams `bson:"params,omitempty"`
}

// filesystemAttachmentDoc records information about a filesystem attachment.
type filesystemAttachmentDoc struct {
	// DocID is the machine global key followed by the filesystem name.
	DocID      string                      `bson:"_id"`
	EnvUUID    string                      `bson:"env-uuid"`
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

	// binding, if non-nil, is the tag of the entity to which
	// the filesystem's lifecycle will be bound.
	binding names.Tag

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
	if f.doc.Binding != "" {
		tag, err := names.ParseTag(f.doc.Binding)
		if err != nil {
			return errors.Annotate(err, "parsing binding")
		}
		switch tag.(type) {
		case names.EnvironTag:
			// TODO(axw) support binding to environment
			return errors.NotSupportedf("binding to environment")
		case names.MachineTag:
		case names.StorageTag:
		default:
			return errors.Errorf("invalid binding: %v", f.doc.Binding)
		}
	}
	return nil
}

// Tag is required to implement Entity.
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

// LifeBinding is required to implement LifeBinder.
//
// Below is the set of possible entity types that a volume may be bound
// to, and a description of the effects of doing so:
//
//   Machine:     If the filesystem is bound to a machine, then the
//                filesystem will be destroyed when it is detached from
//                the machine. It is not permitted for a filesystem to
//                be attached to multiple machines while it is bound to
//                a machine.
//   Storage:     If the filesystem is bound to a storage instance,
//                then the filesystem will be destroyed when the
//                storage insance is removed from state.
//   Environment: If the filesystem is bound to the environment, then
//                the filesystem must be destroyed prior to the
//                environment being destroyed.
func (f *filesystem) LifeBinding() names.Tag {
	if f.doc.Binding == "" {
		return nil
	}
	// Tag is validated in filesystem.validate.
	tag, _ := names.ParseTag(f.doc.Binding)
	return tag
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
	coll, cleanup := st.getCollection(filesystemsC)
	defer cleanup()

	var fDocs []filesystemDoc
	err := coll.Find(query).All(&fDocs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	filesystems := make([]*filesystem, len(fDocs))
	for i, doc := range fDocs {
		f := &filesystem{doc}
		if err := f.validate(); err != nil {
			return nil, errors.Annotate(err, "filesystem validation failed")
		}
		filesystems[i] = f
	}
	return filesystems, nil
}

func (st *State) filesystem(query bson.D, description string) (*filesystem, error) {
	coll, cleanup := st.getCollection(filesystemsC)
	defer cleanup()

	var f filesystem
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
	coll, cleanup := st.getCollection(filesystemAttachmentsC)
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
	coll, cleanup := st.getCollection(filesystemAttachmentsC)
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
func (st *State) removeMachineFilesystemsOps(machine names.MachineTag) ([]txn.Op, error) {
	attachments, err := st.MachineFilesystemAttachments(machine)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ops := make([]txn.Op, 0, len(attachments))
	for _, a := range attachments {
		filesystemTag := a.Filesystem()
		// When removing the machine, there should only remain
		// non-persistent storage. This will be implicitly
		// removed when the machine is removed, so we do not
		// use removeFilesystemAttachmentOps or removeFilesystemOps,
		// which track and update related documents.
		ops = append(ops, txn.Op{
			C:      filesystemAttachmentsC,
			Id:     filesystemAttachmentId(machine.Id(), filesystemTag.Id()),
			Assert: txn.DocExists,
			Remove: true,
		})
		canRemove, err := isFilesystemInherentlyMachineBound(st, filesystemTag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if !canRemove {
			return nil, errors.Errorf("machine has non-machine bound filesystem %v", filesystemTag.Id())
		}
		ops = append(ops, txn.Op{
			C:      filesystemsC,
			Id:     filesystemTag.Id(),
			Assert: txn.DocExists,
			Remove: true,
		})
	}
	return ops, nil
}

// isFilesystemInherentlyMachineBound reports whether or not the filesystem
// with the specified tag is inherently bound to the lifetime of the machine,
// and will be removed along with it, leaving no resources dangling.
func isFilesystemInherentlyMachineBound(st *State, tag names.FilesystemTag) (bool, error) {
	// TODO(axw) when we have support for persistent filesystems,
	// e.g. NFS shares, then we need to check the filesystem info
	// to decide whether or not to remove.
	return true, nil
}

// DetachFilesystem marks the filesystem attachment identified by the specified machine
// and filesystem tags as Dying, if it is Alive.
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
		// If the filesystem is backed by a volume, the volume
		// attachment can and should be destroyed once the
		// filesystem attachment is removed.
		volumeAttachment, err := st.filesystemVolumeAttachment(machine, filesystem)
		if err != nil {
			if errors.Cause(err) != ErrNoBackingVolume && !errors.IsNotFound(err) {
				return nil, errors.Trace(err)
			}
		} else if volumeAttachment.Life() == Alive {
			ops = append(ops, detachVolumeOps(machine, volumeAttachment.Volume())...)
		}
		return ops, nil
	}
	return st.run(buildTxn)
}

func removeFilesystemAttachmentOps(m names.MachineTag, f *filesystem) []txn.Op {
	decrefFilesystemOp := machineStorageDecrefOp(
		filesystemsC, f.doc.FilesystemId,
		f.doc.AttachmentCount, f.doc.Life,
		m, f.doc.Binding,
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
	buildTxn := func(attempt int) ([]txn.Op, error) {
		filesystem, err := st.filesystemByTag(tag)
		if errors.IsNotFound(err) {
			return nil, jujutxn.ErrNoOperations
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		if filesystem.doc.Life != Alive {
			return nil, jujutxn.ErrNoOperations
		}
		return destroyFilesystemOps(st, filesystem), nil
	}
	return st.run(buildTxn)
}

func destroyFilesystemOps(st *State, f *filesystem) []txn.Op {
	if f.doc.AttachmentCount == 0 {
		hasNoAttachments := bson.D{{"attachmentcount", 0}}
		return []txn.Op{{
			C:      filesystemsC,
			Id:     f.doc.FilesystemId,
			Assert: append(hasNoAttachments, isAliveDoc...),
			Update: bson.D{{"$set", bson.D{{"life", Dead}}}},
		}}
	}
	hasAttachments := bson.D{{"attachmentcount", bson.D{{"$gt", 0}}}}
	cleanupOp := st.newCleanupOp(cleanupAttachmentsForDyingFilesystem, f.doc.FilesystemId)
	return []txn.Op{{
		C:      filesystemsC,
		Id:     f.doc.FilesystemId,
		Assert: append(hasAttachments, isAliveDoc...),
		Update: bson.D{{"$set", bson.D{{"life", Dying}}}},
	}, cleanupOp}
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
	ops := []txn.Op{{
		C:      filesystemsC,
		Id:     filesystem.Tag().Id(),
		Assert: txn.DocExists,
		Remove: true,
	}}
	// If the filesystem is backed by a volume, the volume should
	// be destroyed once the filesystem is removed if it is bound
	// to the filesystem.
	volumeTag, err := filesystem.Volume()
	if err == nil {
		volume, err := st.volumeByTag(volumeTag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if volume.LifeBinding() == filesystem.Tag() {
			ops = append(ops, destroyVolumeOps(st, volume)...)
		}
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
	if params.binding == nil {
		params.binding = names.NewMachineTag(machineId)
	}
	params, err := st.filesystemParamsWithDefaults(params)
	if err != nil {
		return nil, names.FilesystemTag{}, names.VolumeTag{}, errors.Trace(err)
	}
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
			filesystemTag, // volume is bound to filesystem
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

	filesystemOp := txn.Op{
		C:      filesystemsC,
		Id:     filesystemId,
		Assert: txn.DocMissing,
		Insert: &filesystemDoc{
			FilesystemId: filesystemId,
			VolumeId:     volumeId,
			StorageId:    params.storage.Id(),
			Binding:      params.binding.String(),
			Params:       &params,
			// Every filesystem is created with one attachment.
			AttachmentCount: 1,
		},
	}
	ops = append(ops, filesystemOp)
	return ops, filesystemTag, volumeTag, nil
}

func (st *State) filesystemParamsWithDefaults(params FilesystemParams) (FilesystemParams, error) {
	if params.Pool != "" {
		return params, nil
	}
	envConfig, err := st.EnvironConfig()
	if err != nil {
		return FilesystemParams{}, errors.Trace(err)
	}
	cons := StorageConstraints{
		Pool:  params.Pool,
		Size:  params.Size,
		Count: 1,
	}
	poolName, err := defaultStoragePool(envConfig, storage.StorageKindFilesystem, cons)
	if err != nil {
		return FilesystemParams{}, errors.Annotate(err, "getting default filesystem storage pool")
	}
	params.Pool = poolName
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
	tag     names.FilesystemTag
	storage names.StorageTag // may be zero-value
	params  FilesystemAttachmentParams
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
