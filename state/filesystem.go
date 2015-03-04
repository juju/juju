// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/juju/storage"
	"github.com/juju/names"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

// ErrNoBackingVolume is returned by Filesystem.Volume() for filesystems
// without a backing volume.
var ErrNoBackingVolume = errors.New("filesystem has no backing volume")

// Filesystem describes a filesystem in the environment. Filesystems may be
// backed by a volume, and managed by Juju; otherwise they are first-class
// entities managed by a filesystem provider.
type Filesystem interface {
	Entity

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
	// Filesystem returns the tag of the related Filesystem.
	Filesystem() names.FilesystemTag

	// Machine returns the tag of the related Machine.
	Machine() names.MachineTag

	// Info returns the filesystem attachment's FilesystemAttachmentInfo, or a
	// NotProvisioned error if the attachment has not yet been made.
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
	DocID        string            `bson:"_id"`
	FilesystemId string            `bson:"filesystemid"`
	EnvUUID      string            `bson:"env-uuid"`
	Life         Life              `bson:"life"`
	StorageId    string            `bson:"storageid,omitempty"`
	VolumeId     string            `bson:"volumeid,omitempty"`
	Info         *FilesystemInfo   `bson:"info,omitempty"`
	Params       *FilesystemParams `bson:"params,omitempty"`
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

	Pool string `bson:"pool"`
	Size uint64 `bson:"size"`
}

// FilesystemInfo describes information about a filesystem.
type FilesystemInfo struct {
	Size uint64 `bson:"size"`

	// FilesystemId is the provider-allocated unique ID of the
	// filesystem. This will be unspecified for filesystems
	// backed by volumes.
	FilesystemId string `bson:"filesystemid"`
}

// FilesystemAttachmentInfo describes information about a filesystem attachment.
type FilesystemAttachmentInfo struct {
	MountPoint string `bson:"mountpoint"`
}

// FilesystemAttachmentParams records parameters for attaching a filesystem to a
// machine.
type FilesystemAttachmentParams struct {
	Location string `bson:"location"`
	ReadOnly bool   `bson:"read-only"`
}

// Tag is required to implement Entity.
func (f *filesystem) Tag() names.Tag {
	return f.FilesystemTag()
}

// FilesystemTag is required to implement Filesystem.
func (f *filesystem) FilesystemTag() names.FilesystemTag {
	return names.NewFilesystemTag(f.doc.FilesystemId)
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
	coll, cleanup := st.getCollection(filesystemsC)
	defer cleanup()

	var fs filesystem
	err := coll.FindId(tag.Id()).One(&fs.doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("filesystem %q", tag.Id())
	} else if err != nil {
		return nil, errors.Annotate(err, "cannot get filesystem")
	}
	return &fs, nil
}

// StorageInstanceFilesystem returns the Filesystem assigned to the specified
// storage instance.
func (st *State) StorageInstanceFilesystem(tag names.StorageTag) (Filesystem, error) {
	coll, cleanup := st.getCollection(filesystemsC)
	defer cleanup()

	var f filesystem
	err := coll.Find(bson.D{{"storageid", tag.Id()}}).One(&f.doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("filesystem for storage instance %q", tag.Id())
	} else if err != nil {
		return nil, errors.Annotate(err, "cannot get filesystem")
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

// MachineFilesystemAttachments returns all of the FilesystemAttachments for the
// specified machine.
func (st *State) MachineFilesystemAttachments(machine names.MachineTag) ([]FilesystemAttachment, error) {
	coll, cleanup := st.getCollection(filesystemAttachmentsC)
	defer cleanup()

	var docs []filesystemAttachmentDoc
	err := coll.Find(bson.D{{"machineid", machine.Id()}}).All(&docs)
	if err == mgo.ErrNotFound {
		return nil, nil
	} else if err != nil {
		return nil, errors.Annotatef(err, "getting filesystem attachments for machine %q", machine.Id())
	}
	attachments := make([]FilesystemAttachment, len(docs))
	for i, doc := range docs {
		attachments[i] = &filesystemAttachment{doc}
	}
	return attachments, nil
}

// filesystemAttachmentId returns a filesystem attachment document ID,
// given the corresponding filesystem name and machine ID.
func filesystemAttachmentId(machineId, filesystemId string) string {
	return fmt.Sprintf("%s#%s", machineGlobalKey(machineId), filesystemId)
}

// newFilesystemId returns a unique filesystem ID.
func newFilesystemId(st *State) (string, error) {
	seq, err := st.sequence("filesystem")
	if err != nil {
		return "", errors.Trace(err)
	}
	return fmt.Sprint(seq), nil
}

// addFilesystemOps returns txn.Ops to create a new filesystem with the
// specified parameters. If the storage source cannot create filesystems
// directly, a volume will be created and Juju will manage a filesystem
// on it.
func (st *State) addFilesystemOps(params FilesystemParams) ([]txn.Op, names.FilesystemTag, names.VolumeTag, error) {
	params, err := st.filesystemParamsWithDefaults(params)
	if err != nil {
		return nil, names.FilesystemTag{}, names.VolumeTag{}, errors.Trace(err)
	}
	if err := st.validateFilesystemParams(&params); err != nil {
		return nil, names.FilesystemTag{}, names.VolumeTag{}, errors.Annotate(err, "validating filesystem params")
	}

	// Check if the filesystem needs a volume.
	var volumeId string
	var volumeTag names.VolumeTag
	var ops []txn.Op
	_, provider, err := poolStorageProvider(st, params.Pool)
	if err != nil {
		return nil, names.FilesystemTag{}, names.VolumeTag{}, errors.Trace(err)
	}
	if !provider.Supports(storage.StorageKindFilesystem) {
		var volumeOp txn.Op
		volumeParams := VolumeParams{
			params.storage,
			params.Pool,
			params.Size,
		}
		volumeOp, volumeTag, err = st.addVolumeOp(volumeParams)
		if err != nil {
			return nil, names.FilesystemTag{}, names.VolumeTag{}, errors.Annotate(err, "creating backing volume")
		}
		volumeId = volumeTag.Id()
		ops = append(ops, volumeOp)
	}

	id, err := newFilesystemId(st)
	if err != nil {
		return nil, names.FilesystemTag{}, names.VolumeTag{}, errors.Annotate(err, "cannot generate filesystem name")
	}
	filesystemOp := txn.Op{
		C:      filesystemsC,
		Id:     id,
		Assert: txn.DocMissing,
		Insert: &filesystemDoc{
			FilesystemId: id,
			VolumeId:     volumeId,
			StorageId:    params.storage.Id(),
			Params:       &params,
		},
	}
	ops = append(ops, filesystemOp)
	return ops, names.NewFilesystemTag(id), volumeTag, nil
}

func (st *State) filesystemParamsWithDefaults(params FilesystemParams) (FilesystemParams, error) {
	if params.Pool != "" {
		return params, nil
	}
	envConfig, err := st.EnvironConfig()
	if err != nil {
		return FilesystemParams{}, errors.Trace(err)
	}
	poolName, err := defaultStoragePool(envConfig, storage.StorageKindFilesystem)
	if err != nil {
		return FilesystemParams{}, errors.Annotate(err, "getting default filesystem storage pool")
	}
	params.Pool = poolName
	return params, nil
}

func (st *State) validateFilesystemParams(params *FilesystemParams) error {
	err := validateStoragePool(st, params.Pool, storage.StorageKindFilesystem)
	if err != nil {
		return errors.Trace(err)
	}
	if params.Size == 0 {
		return errors.New("invalid size 0")
	}
	return nil
}

// createMachineFilesystemAttachmentInfo creates filesystem
// attachments for the specified machine, and attachment
// parameters keyed by filesystem tags.
func createMachineFilesystemAttachmentsOps(machineId string, params map[names.FilesystemTag]FilesystemAttachmentParams) []txn.Op {
	ops := make([]txn.Op, 0, len(params))
	for fsTag, params := range params {
		paramsCopy := params
		ops = append(ops, txn.Op{
			C:      filesystemAttachmentsC,
			Id:     filesystemAttachmentId(machineId, fsTag.Id()),
			Assert: txn.DocMissing,
			Insert: &filesystemAttachmentDoc{
				Filesystem: fsTag.Id(),
				Machine:    machineId,
				Params:     &paramsCopy,
			},
		})
	}
	return ops
}

// SetFilesystemInfo sets the FilesystemInfo for the specified filesystem.
func (st *State) SetFilesystemInfo(tag names.FilesystemTag, info FilesystemInfo) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot set info for filesystem %q", tag.Id())
	// TODO(axw) we should reject info without FilesystemId set; can't do this
	// until the providers all set it correctly.
	buildTxn := func(attempt int) ([]txn.Op, error) {
		fs, err := st.Filesystem(tag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		var unsetParams bool
		if _, ok := fs.Params(); ok {
			// filesystem has parameters, unset them when
			// we set info for the first time.
			unsetParams = true
		}
		ops := setFilesystemInfoOps(tag, info, unsetParams)
		return ops, nil
	}
	return st.run(buildTxn)
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
	buildTxn := func(attempt int) ([]txn.Op, error) {
		fsa, err := st.FilesystemAttachment(machineTag, filesystemTag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		var unsetParams bool
		if _, ok := fsa.Params(); ok {
			// filesystem attachment has parameters, unset them
			// when we set info for the first time.
			unsetParams = true
		}
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
