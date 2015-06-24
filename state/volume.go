// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strings"

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
	Entity

	// VolumeTag returns the tag for the volume.
	VolumeTag() names.VolumeTag

	// Life returns the life of the volume.
	Life() Life

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
	// Volume returns the tag of the related Volume.
	Volume() names.VolumeTag

	// Machine returns the tag of the related Machine.
	Machine() names.MachineTag

	// Life returns the life of the volume attachment.
	Life() Life

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
	doc volumeDoc
}

type volumeAttachment struct {
	doc volumeAttachmentDoc
}

// volumeDoc records information about a volume in the environment.
type volumeDoc struct {
	DocID     string `bson:"_id"`
	Name      string `bson:"name"`
	EnvUUID   string `bson:"env-uuid"`
	Life      Life   `bson:"life"`
	StorageId string `bson:"storageid,omitempty"`
	// TODO(axw) 2015-06-22 #1467379
	// upgrade step to set this for 1.24 environments
	AttachmentCount int           `bson:"attachmentcount"`
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
	ReadOnly   bool   `bson:"read-only"`
}

// VolumeAttachmentParams records parameters for attaching a volume to a
// machine.
type VolumeAttachmentParams struct {
	ReadOnly bool `bson:"read-only"`
}

// Tag is required to implement Entity.
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
	v, err := st.volume(tag)
	return v, err
}

func (st *State) volume(tag names.VolumeTag) (*volume, error) {
	coll, cleanup := st.getCollection(volumesC)
	defer cleanup()

	var v volume
	err := coll.FindId(tag.Id()).One(&v.doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("volume %q", tag.Id())
	} else if err != nil {
		return nil, errors.Annotate(err, "cannot get volume")
	}
	return &v, nil
}

// PersistentVolumes returns any alive persistent Volumes scoped to the environment or any machine.
func (st *State) PersistentVolumes() ([]Volume, error) {
	coll, cleanup := st.getCollection(volumesC)
	defer cleanup()

	var vDocs []volumeDoc
	err := coll.Find(
		bson.D{{"info.persistent", true}},
	).All(&vDocs)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get persistent volumes")
	}
	v := make([]Volume, len(vDocs))
	for i, vDoc := range vDocs {
		v[i] = &volume{vDoc}
	}
	return v, nil
}

// StorageInstanceVolume returns the Volume assigned to the specified
// storage instance.
func (st *State) StorageInstanceVolume(tag names.StorageTag) (Volume, error) {
	coll, cleanup := st.getCollection(volumesC)
	defer cleanup()

	var v volume
	err := coll.Find(bson.D{{"storageid", tag.Id()}}).One(&v.doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("volume for storage instance %q", tag.Id())
	} else if err != nil {
		return nil, errors.Annotate(err, "cannot get volume")
	}
	return &v, nil
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
		volume, err := st.volume(volumeTag)
		if err != nil {
			return nil, errors.Trace(err)
		}
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
		var remove bool
		volumeInfo, err := volume.Info()
		if errors.IsNotProvisioned(err) {
			params, _ := volume.Params()
			_, provider, err := poolStorageProvider(st, params.Pool)
			if err != nil {
				return nil, errors.Trace(err)
			}
			if provider.Dynamic() && provider.Scope() == storage.ScopeEnviron {
				// Leave cleanup to the environ storage provisioner.
				continue
			}
			// Volume will never be provisioned; remove from state.
			remove = true
		} else if err != nil {
			return nil, errors.Trace(err)
		} else {
			// If volume does not outlive machine it can be removed.
			remove = !volumeInfo.Persistent
		}
		if !remove {
			continue
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
		if errors.IsNotFound(err) {
			return nil, jujutxn.ErrNoOperations
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		if attachment.Life() != Dying {
			return nil, errors.New("volume attachment is not dying")
		}
		v, err := st.volume(volume)
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
	)
	return []txn.Op{{
		C:      volumeAttachmentsC,
		Id:     volumeAttachmentId(m.Id(), v.doc.Name),
		Assert: bson.D{{"life", Dying}},
		Remove: true,
	}, decrefVolumeOp}
}

// machineStorageDecrefOp returns a txn.Op that will decrement the attachment
// count for a given machine storage entity (volume or filesystem), given its
// current attachment count and lifecycle state. If the attachment count goes
// to zero, then the entity should become Dead.
func machineStorageDecrefOp(
	collection, id string,
	attachmentCount int, life Life,
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
		// volume is destroyed concurrently. Otherwise, when
		// DestroyVolume is called, the volume will be marked
		// Dead if it has no attachments.
		op.Assert = bson.D{
			{"life", Alive},
			{"attachmentcount", bson.D{{"$gt", 0}}},
		}
		op.Update = bson.D{
			{"$inc", bson.D{{"attachmentcount", -1}}},
		}
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
		volume, err := st.volume(tag)
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
	logger.Debugf("destroyVolumeOps(%v)", v.Tag())
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
		return []txn.Op{{
			C:      volumesC,
			Id:     tag.Id(),
			Assert: txn.DocExists,
			Remove: true,
		}}, nil
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

// addVolumeOp returns a txn.Op to create a new volume with the specified
// parameters. If the supplied machine ID is non-empty, and the storage
// provider is machine-scoped, then the volume will be scoped to that
// machine.
func (st *State) addVolumeOp(params VolumeParams, machineId string) (txn.Op, names.VolumeTag, error) {
	params, err := st.volumeParamsWithDefaults(params)
	if err != nil {
		return txn.Op{}, names.VolumeTag{}, errors.Trace(err)
	}
	machineId, err = st.validateVolumeParams(params, machineId)
	if err != nil {
		return txn.Op{}, names.VolumeTag{}, errors.Annotate(err, "validating volume params")
	}

	name, err := newVolumeName(st, machineId)
	if err != nil {
		return txn.Op{}, names.VolumeTag{}, errors.Annotate(err, "cannot generate volume name")
	}
	op := txn.Op{
		C:      volumesC,
		Id:     name,
		Assert: txn.DocMissing,
		Insert: &volumeDoc{
			Name:      name,
			StorageId: params.storage.Id(),
			Params:    &params,
			// Every volume is created with one attachment.
			AttachmentCount: 1,
		},
	}
	return op, names.NewVolumeTag(name), nil
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
		ops := setVolumeAttachmentInfoOps(machineTag, volumeTag, info, unsetParams)
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
		return setVolumeInfoOps(tag, info, unsetParams), nil
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
	coll, cleanup := st.getCollection(volumesC)
	defer cleanup()

	var vDocs []volumeDoc
	err := coll.Find(nil).All(&vDocs)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get volumes")
	}

	v := make([]Volume, len(vDocs))
	for i, vDoc := range vDocs {
		v[i] = &volume{vDoc}
	}
	return v, nil
}
