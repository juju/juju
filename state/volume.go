// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names"
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
	DocID     string        `bson:"_id"`
	Name      string        `bson:"name"`
	EnvUUID   string        `bson:"env-uuid"`
	Life      Life          `bson:"life"`
	StorageId string        `bson:"storageid,omitempty"`
	Info      *VolumeInfo   `bson:"info,omitempty"`
	Params    *VolumeParams `bson:"params,omitempty"`
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
	Serial     string `bson:"serial,omitempty"`
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
	poolName, err := defaultStoragePool(envConfig, storage.StorageKindBlock)
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
// by volume tags.
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
func setMachineVolumeAttachmentInfo(st *State, machineId string, attachments map[names.VolumeTag]VolumeAttachmentInfo) error {
	machineTag := names.NewMachineTag(machineId)
	for volumeTag, info := range attachments {
		if err := st.SetVolumeAttachmentInfo(machineTag, volumeTag, info); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// SetVolumeAttachmentInfo sets the VolumeAttachmentInfo for the specified
// volume attachment.
func (st *State) SetVolumeAttachmentInfo(machineTag names.MachineTag, volumeTag names.VolumeTag, info VolumeAttachmentInfo) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot set info for volume attachment %s:%s", volumeTag.Id(), machineTag.Id())
	buildTxn := func(attempt int) ([]txn.Op, error) {
		// TODO(axw) attempting to set volume attachment info for a
		// volume that hasn't been provisioned should fail.
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
		ops := setVolumeInfoOps(tag, info, unsetParams)

		// If there's a filesystem destined for the volume,
		// set the filesystem info.
		f, err := st.VolumeFilesystem(tag)
		if err == nil {
			filesystemInfo := FilesystemInfo{
				info.Size,
				info.Pool,
				// FilesystemId is set to "" for
				// filesystems backed by volumes.
				"",
			}
			filesystemOps := setFilesystemInfoOps(
				f.FilesystemTag(),
				filesystemInfo,
				unsetParams,
			)
			ops = append(ops, filesystemOps...)
		} else if !errors.IsNotFound(err) {
			return nil, errors.Trace(err)
		}
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
