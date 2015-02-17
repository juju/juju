// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

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
	VolumeTag() names.DiskTag

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
	Volume() names.DiskTag

	// Machine returns the tag of the related Machine.
	Machine() names.MachineTag

	// Info returns the volume attachment's VolumeAttachmentInfo, or a
	// NotProvisioned error if the attachment has not yet been made.
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
	DocID           string        `bson:"_id"`
	Name            string        `bson:"name"`
	EnvUUID         string        `bson:"env-uuid"`
	Life            Life          `bson:"life"`
	StorageInstance string        `bson:"storageinstanceid,omitempty"`
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
	Serial   string `bson:"serial,omitempty"`
	Size     uint64 `bson:"size"`
	VolumeId string `bson:"volumeid"`
}

// VolumeAttachmentInfo describes information about a volume attachment.
type VolumeAttachmentInfo struct {
	DeviceName string `bson:"devicename,omitempty"`
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
func (v *volume) VolumeTag() names.DiskTag {
	return names.NewDiskTag(v.doc.Name)
}

// StorageInstance is required to implement Volume.
func (v *volume) StorageInstance() (names.StorageTag, error) {
	if v.doc.StorageInstance == "" {
		msg := fmt.Sprintf("volume %q is not assigned to any storage instance", v.Tag().Id())
		return names.StorageTag{}, errors.NewNotAssigned(nil, msg)
	}
	return names.NewStorageTag(v.doc.StorageInstance), nil
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
func (v *volumeAttachment) Volume() names.DiskTag {
	return names.NewDiskTag(v.doc.Volume)
}

// Machine is required to implement VolumeAttachment.
func (v *volumeAttachment) Machine() names.MachineTag {
	return names.NewMachineTag(v.doc.Machine)
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
func (st *State) Volume(tag names.DiskTag) (Volume, error) {
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

// StorageInstanceVolume returns the Volume assigned to the specified
// storage instance.
func (st *State) StorageInstanceVolume(tag names.StorageTag) (Volume, error) {
	coll, cleanup := st.getCollection(volumesC)
	defer cleanup()

	var v volume
	err := coll.Find(bson.D{{"storageinstanceid", tag.Id()}}).One(&v.doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("volume for storage instance %q", tag.Id())
	} else if err != nil {
		return nil, errors.Annotate(err, "cannot get volume")
	}
	return &v, nil
}

// VolumeAttachment returns the VolumeAttachment corresponding to
// the specified volume and machine.
func (st *State) VolumeAttachment(machine names.MachineTag, volume names.DiskTag) (VolumeAttachment, error) {
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
	coll, cleanup := st.getCollection(volumeAttachmentsC)
	defer cleanup()

	var docs []volumeAttachmentDoc
	err := coll.Find(bson.D{{"machineid", machine.Id()}}).All(&docs)
	if err == mgo.ErrNotFound {
		return nil, nil
	} else if err != nil {
		return nil, errors.Annotatef(err, "getting volume attachments for machine %q", machine.Id())
	}
	attachments := make([]VolumeAttachment, len(docs))
	for i, doc := range docs {
		attachments[i] = &volumeAttachment{doc}
	}
	return attachments, nil
}

// newVolumeName returns a unique volume name.
func newVolumeName(st *State) (string, error) {
	seq, err := st.sequence("volume")
	if err != nil {
		return "", errors.Trace(err)
	}
	return fmt.Sprint(seq), nil
}

// addVolumeOp returns a txn.Op to create a new volume with the specified
// parameters.
func (st *State) addVolumeOp(params VolumeParams) (txn.Op, names.DiskTag, error) {
	if err := st.validateVolumeParams(params); err != nil {
		return txn.Op{}, names.DiskTag{}, errors.Annotate(err, "validating volume params")
	}
	name, err := newVolumeName(st)
	if err != nil {
		return txn.Op{}, names.DiskTag{}, errors.Annotate(err, "cannot generate volume name")
	}
	op := txn.Op{
		C:      volumesC,
		Id:     name,
		Assert: txn.DocMissing,
		Insert: &volumeDoc{
			Name:            name,
			StorageInstance: params.storage.Id(),
			Params:          &params,
		},
	}
	return op, names.NewDiskTag(name), nil
}

func (st *State) validateVolumeParams(params VolumeParams) error {
	if poolName, err := validateStoragePool(st, params.Pool, storage.StorageKindBlock); err != nil {
		return err
	} else {
		params.Pool = poolName
	}
	if params.Size == 0 {
		return errors.New("invalid size 0")
	}
	return nil
}

// volumeAttachmentId returns a volume attachment document ID,
// given the corresponding volume name and machine ID.
func volumeAttachmentId(machineId, volumeName string) string {
	return fmt.Sprintf("%s#%s", machineGlobalKey(machineId), volumeName)
}

// createMachineVolumeAttachmentInfo creates volume attachment
// for the specified machine, and attachment parameters keyed
// by volume names.
func createMachineVolumeAttachmentsOps(machineId string, params map[names.DiskTag]VolumeAttachmentParams) []txn.Op {
	ops := make([]txn.Op, 0, len(params))
	for volumeTag, params := range params {
		paramsCopy := params
		ops = append(ops, txn.Op{
			C:      volumeAttachmentsC,
			Id:     volumeAttachmentId(machineId, volumeTag.Id()),
			Assert: txn.DocMissing,
			Insert: &volumeAttachmentDoc{
				Volume:  volumeTag.Id(),
				Machine: machineId,
				Params:  &paramsCopy,
			},
		})
	}
	return ops
}

// setMachineVolumeAttachmentInfo sets the volume attachment
// info for the specified machine. Each volume attachment info
// structure is keyed by the name of the volume it corresponds
// to.
func setMachineVolumeAttachmentInfo(st *State, machineId string, attachments map[names.DiskTag]VolumeAttachmentInfo) error {
	ops := make([]txn.Op, 0, len(attachments)*3)
	for volumeTag, info := range attachments {
		infoCopy := info
		ops = append(ops, txn.Op{
			C:  volumeAttachmentsC,
			Id: volumeAttachmentId(machineId, volumeTag.Id()),
			Assert: append(isAliveDoc, bson.DocElem{
				"info", bson.D{{"$exists", false}},
			}),
			Update: bson.D{
				{"$set", bson.D{{"info", &infoCopy}}},
			},
		})
	}
	if err := st.runTransaction(ops); err != nil {
		return errors.Annotate(err, "cannot set volume attachment info")
	}
	return nil
}

// setProvisionedVolumeInfo sets the initial info for newly
// provisioned volumes. If non-empty, machineId must be the
// machine ID associated with the volumes.
func setProvisionedVolumeInfo(st *State, volumes map[names.DiskTag]VolumeInfo) error {
	ops := make([]txn.Op, 0, len(volumes))
	for volumeTag, info := range volumes {
		infoCopy := info
		assert := bson.D{
			{"info", bson.D{{"$exists", false}}},
			{"params", bson.D{{"$exists", true}}},
		}
		ops = append(ops, txn.Op{
			C:      volumesC,
			Id:     volumeTag.Id(),
			Assert: assert,
			Update: bson.D{
				{"$set", bson.D{{"info", &infoCopy}}},
				{"$unset", bson.D{{"params", nil}}},
			},
		})
	}
	if err := st.runTransaction(ops); err != nil {
		return errors.Errorf("cannot set provisioned volume info: already provisioned")
	}
	return nil
}
