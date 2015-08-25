// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/storage"
)

// Volume describes a volume (disk, logical volume, etc.) in the environment.
type Volume interface {
	GlobalEntity
	LifeBinder
	StatusGetter
	StatusSetter

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
}

// VolumeAttachment describes an attachment of a volume to a machine.
type VolumeAttachment interface {
	Lifer

	// Volume returns the tag of the related Volume.
	Volume() names.VolumeTag

	// Machine returns the tag of the related Machine.
	Machine() names.MachineTag

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

type volume struct {
	st  *State
	doc volumeDoc
}

type volumeAttachment struct {
	doc volumeAttachmentDoc
}

// volumeDoc records information about a volume in the environment.
type volumeDoc struct {
	DocID           string        `bson:"_id"`
	Name            string        `bson:"name"`
	EnvUUID         string        `bson:"env-uuid"`
	Life            Life          `bson:"life"`
	StorageId       string        `bson:"storageid,omitempty"`
	AttachmentCount int           `bson:"attachmentcount"`
	Binding         string        `bson:"binding,omitempty"`
	Info            *VolumeInfo   `bson:"info,omitempty"`
	Params          *VolumeParams `bson:"params,omitempty"`
}

// volumeAttachmentDoc records information about a volume attachment.
type volumeAttachmentDoc struct {
	// DocID is the machine global key followed by the volume name.
	DocID   string                  `bson:"_id"`
	EnvUUID string                  `bson:"env-uuid"`
	Volume  string                  `bson:"volumeid"`
	Machine string                  `bson:"machineid"`
	Life    Life                    `bson:"life"`
	Info    *VolumeAttachmentInfo   `bson:"info,omitempty"`
	Params  *VolumeAttachmentParams `bson:"params,omitempty"`
}

// VolumeParams records parameters for provisioning a new volume.
type VolumeParams struct {
	// storage, if non-zero, is the tag of the storage instance
	// that the volume is to be assigned to.
	storage names.StorageTag

	// binding, if non-nil, is the tag of the entity to which
	// the volume's lifecycle will be bound.
	binding names.Tag

	Pool string `bson:"pool"`
	Size uint64 `bson:"size"`
}

// VolumeInfo describes information about a volume.
type VolumeInfo struct {
	HardwareId string `bson:"hardwareid,omitempty"`
	Size       uint64 `bson:"size"`
	Pool       string `bson:"pool"`
	VolumeId   string `bson:"volumeid"`
	Persistent bool   `bson:"persistent"`
}

// VolumeAttachmentInfo describes information about a volume attachment.
type VolumeAttachmentInfo struct {
	DeviceName string `bson:"devicename,omitempty"`
	BusAddress string `bson:"busaddress,omitempty"`
	ReadOnly   bool   `bson:"read-only"`
}

// VolumeAttachmentParams records parameters for attaching a volume to a
// machine.
type VolumeAttachmentParams struct {
	ReadOnly bool `bson:"read-only"`
}

// validate validates the contents of the volume document.
func (v *volume) validate() error {
	if v.doc.Binding != "" {
		tag, err := names.ParseTag(v.doc.Binding)
		if err != nil {
			return errors.Annotate(err, "parsing binding")
		}
		switch tag.(type) {
		case names.EnvironTag:
			// TODO(axw) support binding to environment
			return errors.NotSupportedf("binding to environment")
		case names.MachineTag:
		case names.FilesystemTag:
		case names.StorageTag:
		default:
			return errors.Errorf("invalid binding: %v", v.doc.Binding)
		}
	}
	return nil
}

// globalKey is required to implement GlobalEntity.
func (v *volume) globalKey() string {
	return volumeGlobalKey(v.doc.Name)
}

// Tag is required to implement GlobalEntity.
func (v *volume) Tag() names.Tag {
	return v.VolumeTag()
}

// VolumeTag is required to implement Volume.
func (v *volume) VolumeTag() names.VolumeTag {
	return names.NewVolumeTag(v.doc.Name)
}

// Life returns the volume's current lifecycle state.
func (v *volume) Life() Life {
	return v.doc.Life
}

// LifeBinding is required to implement LifeBinder.
//
// Below is the set of possible entity types that a volume may be bound
// to, and a description of the effects of doing so:
//
//   Machine:     If the volume is bound to a machine, then the volume
//                will be destroyed when it is detached from the
//                machine. It is not permitted for a volume to be
//                attached to multiple machines while it is bound to a
//                machine.
//   Storage:     If the volume is bound to a storage instance, then
//                the volume will be destroyed when the storage insance
//                is removed from state.
//   Filesystem:  If the volume is bound to a filesystem, i.e. the
//                volume backs that filesystem, then it will be
//                destroyed when the filesystem is removed from state.
//   Environment: If the volume is bound to the environment, then the
//                volume must be destroyed prior to the environment
//                being destroyed.
func (v *volume) LifeBinding() names.Tag {
	if v.doc.Binding == "" {
		return nil
	}
	// Tag is validated in volume.validate.
	tag, _ := names.ParseTag(v.doc.Binding)
	return tag
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

// Status is required to implement StatusGetter.
func (v *volume) Status() (StatusInfo, error) {
	return v.st.VolumeStatus(v.VolumeTag())
}

// SetStatus is required to implement StatusSetter.
func (v *volume) SetStatus(status Status, info string, data map[string]interface{}) error {
	return v.st.SetVolumeStatus(v.VolumeTag(), status, info, data)
}

// Volume is required to implement VolumeAttachment.
func (v *volumeAttachment) Volume() names.VolumeTag {
	return names.NewVolumeTag(v.doc.Volume)
}

// Machine is required to implement VolumeAttachment.
func (v *volumeAttachment) Machine() names.MachineTag {
	return names.NewMachineTag(v.doc.Machine)
}

// Life is required to implement VolumeAttachment.
func (v *volumeAttachment) Life() Life {
	return v.doc.Life
}

// Info is required to implement VolumeAttachment.
func (v *volumeAttachment) Info() (VolumeAttachmentInfo, error) {
	if v.doc.Info == nil {
		return VolumeAttachmentInfo{}, errors.NotProvisionedf("volume attachment %q on %q", v.doc.Volume, v.doc.Machine)
	}
	return *v.doc.Info, nil
}

// Params is required to implement VolumeAttachment.
func (v *volumeAttachment) Params() (VolumeAttachmentParams, bool) {
	if v.doc.Params == nil {
		return VolumeAttachmentParams{}, false
	}
	return *v.doc.Params, true
}

// Volume returns the Volume with the specified name.
func (st *State) Volume(tag names.VolumeTag) (Volume, error) {
	v, err := st.volumeByTag(tag)
	return v, err
}

func (st *State) volumes(query interface{}) ([]*volume, error) {
	coll, cleanup := st.getCollection(volumesC)
	defer cleanup()

	var docs []volumeDoc
	err := coll.Find(query).All(&docs)
	if err != nil {
		return nil, errors.Annotate(err, "querying volumes")
	}
	volumes := make([]*volume, len(docs))
	for i := range docs {
		volume := &volume{st, docs[i]}
		if err := volume.validate(); err != nil {
			return nil, errors.Annotate(err, "validating volume")
		}
		volumes[i] = volume
	}
	return volumes, nil
}

func (st *State) volume(query bson.D, description string) (*volume, error) {
	volumes, err := st.volumes(query)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(volumes) == 0 {
		return nil, errors.NotFoundf("%s", description)
	} else if len(volumes) != 1 {
		return nil, errors.Errorf("expected 1 volume, got %d", len(volumes))
	}
	return volumes[0], nil
}

func (st *State) volumeByTag(tag names.VolumeTag) (*volume, error) {
	return st.volume(bson.D{{"_id", tag.Id()}}, fmt.Sprintf("volume %q", tag.Id()))
}

func volumesToInterfaces(volumes []*volume) []Volume {
	result := make([]Volume, len(volumes))
	for i, v := range volumes {
		result[i] = v
	}
	return result
}

func (st *State) storageInstanceVolume(tag names.StorageTag) (*volume, error) {
	return st.volume(
		bson.D{{"storageid", tag.Id()}},
		fmt.Sprintf("volume for storage instance %q", tag.Id()),
	)
}

// StorageInstanceVolume returns the Volume assigned to the specified
// storage instance.
func (st *State) StorageInstanceVolume(tag names.StorageTag) (Volume, error) {
	v, err := st.storageInstanceVolume(tag)
	return v, err
}

// VolumeAttachment returns the VolumeAttachment corresponding to
// the specified volume and machine.
func (st *State) VolumeAttachment(machine names.MachineTag, volume names.VolumeTag) (VolumeAttachment, error) {
	coll, cleanup := st.getCollection(volumeAttachmentsC)
	defer cleanup()

	var att volumeAttachment
	err := coll.FindId(volumeAttachmentId(machine.Id(), volume.Id())).One(&att.doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("volume %q on machine %q", volume.Id(), machine.Id())
	} else if err != nil {
		return nil, errors.Annotatef(err, "getting volume %q on machine %q", volume.Id(), machine.Id())
	}
	return &att, nil
}

// MachineVolumeAttachments returns all of the VolumeAttachments for the
// specified machine.
func (st *State) MachineVolumeAttachments(machine names.MachineTag) ([]VolumeAttachment, error) {
	attachments, err := st.volumeAttachments(bson.D{{"machineid", machine.Id()}})
	if err != nil {
		return nil, errors.Annotatef(err, "getting volume attachments for machine %q", machine.Id())
	}
	return attachments, nil
}

// VolumeAttachments returns all of the VolumeAttachments for the specified
// volume.
func (st *State) VolumeAttachments(volume names.VolumeTag) ([]VolumeAttachment, error) {
	attachments, err := st.volumeAttachments(bson.D{{"volumeid", volume.Id()}})
	if err != nil {
		return nil, errors.Annotatef(err, "getting volume attachments for volume %q", volume.Id())
	}
	return attachments, nil
}

func (st *State) volumeAttachments(query bson.D) ([]VolumeAttachment, error) {
	coll, cleanup := st.getCollection(volumeAttachmentsC)
	defer cleanup()

	var docs []volumeAttachmentDoc
	err := coll.Find(query).All(&docs)
	if err == mgo.ErrNotFound {
		return nil, nil
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	attachments := make([]VolumeAttachment, len(docs))
	for i, doc := range docs {
		attachments[i] = &volumeAttachment{doc}
	}
	return attachments, nil
}

type errContainsFilesystem struct {
	error
}

func IsContainsFilesystem(err error) bool {
	_, ok := errors.Cause(err).(*errContainsFilesystem)
	return ok
}

// removeMachineVolumesOps returns txn.Ops to remove non-persistent volumes
// attached to the specified machine. This is used when the given machine is
// being removed from state.
func (st *State) removeMachineVolumesOps(machine names.MachineTag) ([]txn.Op, error) {
	attachments, err := st.MachineVolumeAttachments(machine)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ops := make([]txn.Op, 0, len(attachments))
	for _, a := range attachments {
		volumeTag := a.Volume()
		// When removing the machine, there should only remain
		// non-persistent storage. This will be implicitly
		// removed when the machine is removed, so we do not
		// use removeVolumeAttachmentOps or removeVolumeOps,
		// which track and update related documents.
		ops = append(ops, txn.Op{
			C:      volumeAttachmentsC,
			Id:     volumeAttachmentId(machine.Id(), volumeTag.Id()),
			Assert: txn.DocExists,
			Remove: true,
		})
		canRemove, err := isVolumeInherentlyMachineBound(st, volumeTag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if !canRemove {
			return nil, errors.Errorf("machine has non-machine bound volume %v", volumeTag.Id())
		}
		ops = append(ops, txn.Op{
			C:      volumesC,
			Id:     volumeTag.Id(),
			Assert: txn.DocExists,
			Remove: true,
		})
	}
	return ops, nil
}

// isVolumeInherentlyMachineBound reports whether or not the volume with the
// specified tag is inherently bound to the lifetime of the machine, and will
// be removed along with it, leaving no resources dangling.
func isVolumeInherentlyMachineBound(st *State, tag names.VolumeTag) (bool, error) {
	volume, err := st.Volume(tag)
	if err != nil {
		return false, errors.Trace(err)
	}
	volumeInfo, err := volume.Info()
	if errors.IsNotProvisioned(err) {
		params, _ := volume.Params()
		_, provider, err := poolStorageProvider(st, params.Pool)
		if err != nil {
			return false, errors.Trace(err)
		}
		if provider.Dynamic() {
			// Even machine-scoped storage could be provisioned
			// while the machine is Dying, and we don't know at
			// this layer whether or not it will be Persistent.
			//
			// TODO(axw) extend storage provider interface to
			// determine up-front whether or not a volume will
			// be persistent. This will have to depend on the
			// machine type, since, e.g., loop devices will
			// outlive LXC containers.
			return false, nil
		}
		// Volume is static, so even if it is provisioned, it will
		// be tied to the machine.
		return true, nil
	} else if err != nil {
		return false, errors.Trace(err)
	}
	// If volume does not outlive machine it can be removed.
	return !volumeInfo.Persistent, nil
}

// DetachVolume marks the volume attachment identified by the specified machine
// and volume tags as Dying, if it is Alive. DetachVolume will fail with a
// IsContainsFilesystem error if the volume contains an attached filesystem; the
// filesystem attachment must be removed first.
func (st *State) DetachVolume(machine names.MachineTag, volume names.VolumeTag) (err error) {
	defer errors.DeferredAnnotatef(&err, "detaching volume %s from machine %s", volume.Id(), machine.Id())
	// If the volume is backing a filesystem, the volume cannot be detached
	// until the filesystem has been detached.
	if _, err := st.volumeFilesystemAttachment(machine, volume); err == nil {
		return &errContainsFilesystem{errors.New("volume contains attached filesystem")}
	} else if !errors.IsNotFound(err) {
		return errors.Trace(err)
	}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		va, err := st.VolumeAttachment(machine, volume)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if va.Life() != Alive {
			return nil, jujutxn.ErrNoOperations
		}
		return detachVolumeOps(machine, volume), nil
	}
	return st.run(buildTxn)
}

func (st *State) volumeFilesystemAttachment(machine names.MachineTag, volume names.VolumeTag) (FilesystemAttachment, error) {
	filesystem, err := st.VolumeFilesystem(volume)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return st.FilesystemAttachment(machine, filesystem.FilesystemTag())
}

func detachVolumeOps(m names.MachineTag, v names.VolumeTag) []txn.Op {
	return []txn.Op{{
		C:      volumeAttachmentsC,
		Id:     volumeAttachmentId(m.Id(), v.Id()),
		Assert: isAliveDoc,
		Update: bson.D{{"$set", bson.D{{"life", Dying}}}},
	}}
}

// RemoveVolumeAttachment removes the volume attachment from state.
// RemoveVolumeAttachment will fail if the attachment is not Dying.
func (st *State) RemoveVolumeAttachment(machine names.MachineTag, volume names.VolumeTag) (err error) {
	defer errors.DeferredAnnotatef(&err, "removing attachment of volume %s from machine %s", volume.Id(), machine.Id())
	buildTxn := func(attempt int) ([]txn.Op, error) {
		attachment, err := st.VolumeAttachment(machine, volume)
		if errors.IsNotFound(err) && attempt > 0 {
			// We only ignore IsNotFound on attempts after the
			// first, since we expect the volume attachment to
			// be there initially.
			return nil, jujutxn.ErrNoOperations
		}
		if err != nil {
			return nil, errors.Trace(err)
		}
		if attachment.Life() != Dying {
			return nil, errors.New("volume attachment is not dying")
		}
		v, err := st.volumeByTag(volume)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return removeVolumeAttachmentOps(machine, v), nil
	}
	return st.run(buildTxn)
}

func removeVolumeAttachmentOps(m names.MachineTag, v *volume) []txn.Op {
	decrefVolumeOp := machineStorageDecrefOp(
		volumesC, v.doc.Name,
		v.doc.AttachmentCount, v.doc.Life,
		m, v.doc.Binding,
	)
	return []txn.Op{{
		C:      volumeAttachmentsC,
		Id:     volumeAttachmentId(m.Id(), v.doc.Name),
		Assert: bson.D{{"life", Dying}},
		Remove: true,
	}, decrefVolumeOp, {
		C:      machinesC,
		Id:     m.Id(),
		Assert: txn.DocExists,
		Update: bson.D{{"$pull", bson.D{{"volumes", v.doc.Name}}}},
	}}
}

// machineStorageDecrefOp returns a txn.Op that will decrement the attachment
// count for a given machine storage entity (volume or filesystem), given its
// current attachment count and lifecycle state. If the attachment count goes
// to zero, then the entity should become Dead.
func machineStorageDecrefOp(
	collection, id string,
	attachmentCount int, life Life,
	machine names.MachineTag,
	binding string,
) txn.Op {
	op := txn.Op{
		C:  collection,
		Id: id,
	}
	if life == Dying {
		if attachmentCount == 1 {
			// This is the last attachment: the volume can be
			// marked Dead. There can be no concurrent attachments
			// since it is Dying.
			op.Assert = bson.D{
				{"life", Dying},
				{"attachmentcount", 1},
			}
			op.Update = bson.D{
				{"$inc", bson.D{{"attachmentcount", -1}}},
				{"$set", bson.D{{"life", Dead}}},
			}
		} else {
			// This is not the last attachment; just decref,
			// allowing for concurrent attachment removals but
			// ensuring we don't drop to zero without marking
			// the volume Dead.
			op.Assert = bson.D{
				{"life", Dying},
				{"attachmentcount", bson.D{{"$gt", 1}}},
			}
			op.Update = bson.D{
				{"$inc", bson.D{{"attachmentcount", -1}}},
			}
		}
	} else {
		// The volume is still Alive: decref, retrying if the
		// volume is destroyed concurrently or the binding changes.
		// If the volume is bound to the machine, advance it to
		// Dead; binding storage to a machine and attaching the
		// storage to multiple machines will be mutually exclusive.
		//
		// Otherwise, when DestroyVolume is called, the volume will
		// be marked Dead if it has no attachments.
		update := bson.D{
			{"$inc", bson.D{{"attachmentcount", -1}}},
		}
		if binding == machine.String() {
			update = append(update, bson.DocElem{
				"$set", bson.D{{"life", Dead}},
			})
		}
		op.Assert = bson.D{
			{"life", Alive},
			{"binding", binding},
			{"attachmentcount", bson.D{{"$gt", 0}}},
		}
		op.Update = update
	}
	return op
}

// DestroyVolume ensures that the volume and any attachments to it will be
// destroyed and removed from state at some point in the future. DestroyVolume
// will fail with an IsContainsFilesystem error if the volume contains a
// filesystem; the filesystem must be fully removed first.
func (st *State) DestroyVolume(tag names.VolumeTag) (err error) {
	defer errors.DeferredAnnotatef(&err, "destroying volume %s", tag.Id())
	if _, err := st.VolumeFilesystem(tag); err == nil {
		return &errContainsFilesystem{errors.New("volume contains filesystem")}
	} else if !errors.IsNotFound(err) {
		return errors.Trace(err)
	}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		volume, err := st.volumeByTag(tag)
		if errors.IsNotFound(err) {
			return nil, jujutxn.ErrNoOperations
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		if volume.Life() != Alive {
			return nil, jujutxn.ErrNoOperations
		}
		return destroyVolumeOps(st, volume), nil
	}
	return st.run(buildTxn)
}

func destroyVolumeOps(st *State, v *volume) []txn.Op {
	if v.doc.AttachmentCount == 0 {
		hasNoAttachments := bson.D{{"attachmentcount", 0}}
		return []txn.Op{{
			C:      volumesC,
			Id:     v.doc.Name,
			Assert: append(hasNoAttachments, isAliveDoc...),
			Update: bson.D{{"$set", bson.D{{"life", Dead}}}},
		}}
	}
	cleanupOp := st.newCleanupOp(cleanupAttachmentsForDyingVolume, v.doc.Name)
	hasAttachments := bson.D{{"attachmentcount", bson.D{{"$gt", 0}}}}
	return []txn.Op{{
		C:      volumesC,
		Id:     v.doc.Name,
		Assert: append(hasAttachments, isAliveDoc...),
		Update: bson.D{{"$set", bson.D{{"life", Dying}}}},
	}, cleanupOp}
}

// RemoveVolume removes the volume from state. RemoveVolume will fail if
// the volume is not Dead, which implies that it still has attachments.
func (st *State) RemoveVolume(tag names.VolumeTag) (err error) {
	defer errors.DeferredAnnotatef(&err, "removing volume %s", tag.Id())
	buildTxn := func(attempt int) ([]txn.Op, error) {
		volume, err := st.Volume(tag)
		if errors.IsNotFound(err) {
			return nil, jujutxn.ErrNoOperations
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		if volume.Life() != Dead {
			return nil, errors.New("volume is not dead")
		}
		return []txn.Op{
			{
				C:      volumesC,
				Id:     tag.Id(),
				Assert: txn.DocExists,
				Remove: true,
			},
			removeStatusOp(st, volumeGlobalKey(tag.Id())),
		}, nil
	}
	return st.run(buildTxn)
}

// newVolumeName returns a unique volume name.
// If the machine ID supplied is non-empty, the
// volume ID will incorporate it as the volume's
// machine scope.
func newVolumeName(st *State, machineId string) (string, error) {
	seq, err := st.sequence("volume")
	if err != nil {
		return "", errors.Trace(err)
	}
	id := fmt.Sprint(seq)
	if machineId != "" {
		id = machineId + "/" + id
	}
	return id, nil
}

// addVolumeOps returns txn.Ops to create a new volume with the specified
// parameters. If the supplied machine ID is non-empty, and the storage
// provider is machine-scoped, then the volume will be scoped to that
// machine.
func (st *State) addVolumeOps(params VolumeParams, machineId string) ([]txn.Op, names.VolumeTag, error) {
	if params.binding == nil {
		params.binding = names.NewMachineTag(machineId)
	}
	params, err := st.volumeParamsWithDefaults(params)
	if err != nil {
		return nil, names.VolumeTag{}, errors.Trace(err)
	}
	machineId, err = st.validateVolumeParams(params, machineId)
	if err != nil {
		return nil, names.VolumeTag{}, errors.Annotate(err, "validating volume params")
	}
	name, err := newVolumeName(st, machineId)
	if err != nil {
		return nil, names.VolumeTag{}, errors.Annotate(err, "cannot generate volume name")
	}
	ops := []txn.Op{
		createStatusOp(st, volumeGlobalKey(name), statusDoc{
			Status:  StatusPending,
			Updated: time.Now().UnixNano(),
		}),
		{
			C:      volumesC,
			Id:     name,
			Assert: txn.DocMissing,
			Insert: &volumeDoc{
				Name:      name,
				StorageId: params.storage.Id(),
				Binding:   params.binding.String(),
				Params:    &params,
				// Every volume is created with one attachment.
				AttachmentCount: 1,
			},
		},
	}
	return ops, names.NewVolumeTag(name), nil
}

func (st *State) volumeParamsWithDefaults(params VolumeParams) (VolumeParams, error) {
	if params.Pool != "" {
		return params, nil
	}
	envConfig, err := st.EnvironConfig()
	if err != nil {
		return VolumeParams{}, errors.Trace(err)
	}
	cons := StorageConstraints{
		Pool:  params.Pool,
		Size:  params.Size,
		Count: 1,
	}
	poolName, err := defaultStoragePool(envConfig, storage.StorageKindBlock, cons)
	if err != nil {
		return VolumeParams{}, errors.Annotate(err, "getting default block storage pool")
	}
	params.Pool = poolName
	return params, nil
}

// validateVolumeParams validates the volume parameters, and returns the
// machine ID to use as the scope in the volume tag.
func (st *State) validateVolumeParams(params VolumeParams, machineId string) (maybeMachineId string, _ error) {
	if err := validateStoragePool(st, params.Pool, storage.StorageKindBlock, &machineId); err != nil {
		return "", err
	}
	if params.Size == 0 {
		return "", errors.New("invalid size 0")
	}
	return machineId, nil
}

// volumeAttachmentId returns a volume attachment document ID,
// given the corresponding volume name and machine ID.
func volumeAttachmentId(machineId, volumeName string) string {
	return fmt.Sprintf("%s:%s", machineId, volumeName)
}

// ParseVolumeAttachmentId parses a string as a volume attachment ID,
// returning the machine and volume components.
func ParseVolumeAttachmentId(id string) (names.MachineTag, names.VolumeTag, error) {
	fields := strings.SplitN(id, ":", 2)
	if len(fields) != 2 || !names.IsValidMachine(fields[0]) || !names.IsValidVolume(fields[1]) {
		return names.MachineTag{}, names.VolumeTag{}, errors.Errorf("invalid volume attachment ID %q", id)
	}
	machineTag := names.NewMachineTag(fields[0])
	volumeTag := names.NewVolumeTag(fields[1])
	return machineTag, volumeTag, nil
}

type volumeAttachmentTemplate struct {
	tag    names.VolumeTag
	params VolumeAttachmentParams
}

// createMachineVolumeAttachmentInfo creates volume attachments
// for the specified machine, and attachment parameters keyed
// by volume tags. The caller is responsible for incrementing
// the volume's attachmentcount field.
func createMachineVolumeAttachmentsOps(machineId string, attachments []volumeAttachmentTemplate) []txn.Op {
	ops := make([]txn.Op, len(attachments))
	for i, attachment := range attachments {
		paramsCopy := attachment.params
		ops[i] = txn.Op{
			C:      volumeAttachmentsC,
			Id:     volumeAttachmentId(machineId, attachment.tag.Id()),
			Assert: txn.DocMissing,
			Insert: &volumeAttachmentDoc{
				Volume:  attachment.tag.Id(),
				Machine: machineId,
				Params:  &paramsCopy,
			},
		}
	}
	return ops
}

// setMachineVolumeAttachmentInfo sets the volume attachment
// info for the specified machine. Each volume attachment info
// structure is keyed by the name of the volume it corresponds
// to.
func setMachineVolumeAttachmentInfo(
	st *State,
	machineId string,
	attachments map[names.VolumeTag]VolumeAttachmentInfo,
) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot set volume attachment info for machine %s", machineId)
	machineTag := names.NewMachineTag(machineId)
	for volumeTag, info := range attachments {
		if err := st.setVolumeAttachmentInfo(machineTag, volumeTag, info); err != nil {
			return errors.Annotatef(err, "setting attachment info for volume %s", volumeTag.Id())
		}
	}
	return nil
}

// SetVolumeAttachmentInfo sets the VolumeAttachmentInfo for the specified
// volume attachment.
func (st *State) SetVolumeAttachmentInfo(machineTag names.MachineTag, volumeTag names.VolumeTag, info VolumeAttachmentInfo) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot set info for volume attachment %s:%s", volumeTag.Id(), machineTag.Id())
	v, err := st.Volume(volumeTag)
	if err != nil {
		return errors.Trace(err)
	}
	// Ensure volume is provisioned before setting attachment info.
	// A volume cannot go from being provisioned to unprovisioned,
	// so there is no txn.Op for this below.
	if _, err := v.Info(); err != nil {
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
	return st.setVolumeAttachmentInfo(machineTag, volumeTag, info)
}

func (st *State) setVolumeAttachmentInfo(
	machineTag names.MachineTag,
	volumeTag names.VolumeTag,
	info VolumeAttachmentInfo,
) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		va, err := st.VolumeAttachment(machineTag, volumeTag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		// If the volume attachment has parameters, unset them
		// when we set info for the first time, ensuring that
		// params and info are mutually exclusive.
		_, unsetParams := va.Params()
		ops := setVolumeAttachmentInfoOps(
			machineTag, volumeTag, info, unsetParams,
		)
		return ops, nil
	}
	return st.run(buildTxn)
}

func setVolumeAttachmentInfoOps(machine names.MachineTag, volume names.VolumeTag, info VolumeAttachmentInfo, unsetParams bool) []txn.Op {
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
		C:      volumeAttachmentsC,
		Id:     volumeAttachmentId(machine.Id(), volume.Id()),
		Assert: asserts,
		Update: update,
	}}
}

// setProvisionedVolumeInfo sets the initial info for newly
// provisioned volumes. If non-empty, machineId must be the
// machine ID associated with the volumes.
func setProvisionedVolumeInfo(st *State, volumes map[names.VolumeTag]VolumeInfo) error {
	for volumeTag, info := range volumes {
		if err := st.SetVolumeInfo(volumeTag, info); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// SetVolumeInfo sets the VolumeInfo for the specified volume.
func (st *State) SetVolumeInfo(tag names.VolumeTag, info VolumeInfo) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot set info for volume %q", tag.Id())
	if info.VolumeId == "" {
		return errors.New("volume ID not set")
	}
	// TODO(axw) we should reject info without VolumeId set; can't do this
	// until the providers all set it correctly.
	buildTxn := func(attempt int) ([]txn.Op, error) {
		v, err := st.Volume(tag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		// If the volume has parameters, unset them when
		// we set info for the first time, ensuring that
		// params and info are mutually exclusive.
		var unsetParams bool
		var ops []txn.Op
		if params, ok := v.Params(); ok {
			info.Pool = params.Pool
			unsetParams = true
		} else {
			// Ensure immutable properties do not change.
			oldInfo, err := v.Info()
			if err != nil {
				return nil, err
			}
			if err := validateVolumeInfoChange(info, oldInfo); err != nil {
				return nil, err
			}
		}
		ops = append(ops, setVolumeInfoOps(tag, info, unsetParams)...)
		return ops, nil
	}
	return st.run(buildTxn)
}

func validateVolumeInfoChange(newInfo, oldInfo VolumeInfo) error {
	if newInfo.Pool != oldInfo.Pool {
		return errors.Errorf(
			"cannot change pool from %q to %q",
			oldInfo.Pool, newInfo.Pool,
		)
	}
	if newInfo.VolumeId != oldInfo.VolumeId {
		return errors.Errorf(
			"cannot change volume ID from %q to %q",
			oldInfo.VolumeId, newInfo.VolumeId,
		)
	}
	return nil
}

func setVolumeInfoOps(tag names.VolumeTag, info VolumeInfo, unsetParams bool) []txn.Op {
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
		C:      volumesC,
		Id:     tag.Id(),
		Assert: asserts,
		Update: update,
	}}
}

// AllVolumes returns all Volumes scoped to the environment.
func (st *State) AllVolumes() ([]Volume, error) {
	volumes, err := st.volumes(nil)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get volumes")
	}
	return volumesToInterfaces(volumes), nil
}

func volumeGlobalKey(name string) string {
	return "v#" + name
}

// VolumeStatus returns the status of the specified volume.
func (st *State) VolumeStatus(tag names.VolumeTag) (StatusInfo, error) {
	return getStatus(st, volumeGlobalKey(tag.Id()), "volume")
}

// SetVolumeStatus sets the status of the specified volume.
func (st *State) SetVolumeStatus(tag names.VolumeTag, status Status, info string, data map[string]interface{}) error {
	switch status {
	case StatusAttaching, StatusAttached, StatusDetaching, StatusDestroying:
	case StatusError:
		if info == "" {
			return errors.Errorf("cannot set status %q without info", status)
		}
	case StatusPending:
		// If a volume is not yet provisioned, we allow its status
		// to be set back to pending (when a retry is to occur).
		v, err := st.Volume(tag)
		if err != nil {
			return errors.Trace(err)
		}
		_, err = v.Info()
		if errors.IsNotProvisioned(err) {
			break
		}
		return errors.Errorf("cannot set status %q", status)
	default:
		return errors.Errorf("cannot set invalid status %q", status)
	}
	return setStatus(st, setStatusParams{
		badge:     "volume",
		globalKey: volumeGlobalKey(tag.Id()),
		status:    status,
		message:   info,
		rawData:   data,
	})
}
