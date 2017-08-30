// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strings"
	"time"

	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/status"
	"github.com/juju/juju/storage"
)

// Volume describes a volume (disk, logical volume, etc.) in the model.
type Volume interface {
	GlobalEntity
	Lifer
	status.StatusGetter
	status.StatusSetter

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

	// Detachable reports whether or not the volume is detachable.
	Detachable() bool

	// Releasing reports whether or not the volume is to be released
	// from the model when it is Dying/Dead.
	Releasing() bool
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
	im  *IAASModel
	doc volumeDoc
}

type volumeAttachment struct {
	doc volumeAttachmentDoc
}

// volumeDoc records information about a volume in the model.
type volumeDoc struct {
	DocID           string        `bson:"_id"`
	Name            string        `bson:"name"`
	ModelUUID       string        `bson:"model-uuid"`
	Life            Life          `bson:"life"`
	Releasing       bool          `bson:"releasing,omitempty"`
	StorageId       string        `bson:"storageid,omitempty"`
	AttachmentCount int           `bson:"attachmentcount"`
	Info            *VolumeInfo   `bson:"info,omitempty"`
	Params          *VolumeParams `bson:"params,omitempty"`

	// MachineId is the ID of the machine that a non-detachable
	// volume is initially attached to. We use this to identify
	// the volume as being non-detachable, and to determine
	// which volumes must be removed along with said machine.
	MachineId string `bson:"machineid,omitempty"`
}

// volumeAttachmentDoc records information about a volume attachment.
type volumeAttachmentDoc struct {
	// DocID is the machine global key followed by the volume name.
	DocID     string                  `bson:"_id"`
	ModelUUID string                  `bson:"model-uuid"`
	Volume    string                  `bson:"volumeid"`
	Machine   string                  `bson:"machineid"`
	Life      Life                    `bson:"life"`
	Info      *VolumeAttachmentInfo   `bson:"info,omitempty"`
	Params    *VolumeAttachmentParams `bson:"params,omitempty"`
}

// VolumeParams records parameters for provisioning a new volume.
type VolumeParams struct {
	// storage, if non-zero, is the tag of the storage instance
	// that the volume is to be assigned to.
	storage names.StorageTag

	// volumeInfo, if non-empty, is the information for an already
	// provisioned volume. This is only set when creating a volume
	// entity for an existing volume.
	volumeInfo *VolumeInfo

	Pool string `bson:"pool"`
	Size uint64 `bson:"size"`
}

// VolumeInfo describes information about a volume.
type VolumeInfo struct {
	HardwareId string `bson:"hardwareid,omitempty"`
	WWN        string `bson:"wwn,omitempty"`
	Size       uint64 `bson:"size"`
	Pool       string `bson:"pool"`
	VolumeId   string `bson:"volumeid"`
	Persistent bool   `bson:"persistent"`
}

// VolumeAttachmentInfo describes information about a volume attachment.
type VolumeAttachmentInfo struct {
	DeviceName string `bson:"devicename,omitempty"`
	DeviceLink string `bson:"devicelink,omitempty"`
	BusAddress string `bson:"busaddress,omitempty"`
	ReadOnly   bool   `bson:"read-only"`
}

// VolumeAttachmentParams records parameters for attaching a volume to a
// machine.
type VolumeAttachmentParams struct {
	ReadOnly bool `bson:"read-only"`
}

// validate validates the contents of the volume document.
func (v *volumeDoc) validate() error {
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

// Releasing is required to imeplement Volume.
func (v *volume) Releasing() bool {
	return v.doc.Releasing
}

// Status is required to implement StatusGetter.
func (v *volume) Status() (status.StatusInfo, error) {
	return v.im.VolumeStatus(v.VolumeTag())
}

// SetStatus is required to implement StatusSetter.
func (v *volume) SetStatus(volumeStatus status.StatusInfo) error {
	return v.im.SetVolumeStatus(v.VolumeTag(), volumeStatus.Status, volumeStatus.Message, volumeStatus.Data, volumeStatus.Since)
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
func (im *IAASModel) Volume(tag names.VolumeTag) (Volume, error) {
	v, err := im.volumeByTag(tag)
	return v, err
}

func (im *IAASModel) volumes(query interface{}) ([]*volume, error) {
	docs, err := getVolumeDocs(im.mb.db(), query)
	if err != nil {
		return nil, errors.Trace(err)
	}
	volumes := make([]*volume, len(docs))
	for i := range docs {
		volumes[i] = &volume{im, docs[i]}
	}
	return volumes, nil
}

func (im *IAASModel) volumeByTag(tag names.VolumeTag) (*volume, error) {
	doc, err := getVolumeDocByTag(im.mb.db(), tag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &volume{im, doc}, nil
}

func (im *IAASModel) volume(query bson.D, description string) (*volume, error) {
	doc, err := getVolumeDoc(im.mb.db(), query, description)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &volume{im, doc}, nil
}

func getVolumeDocByTag(db Database, tag names.VolumeTag) (volumeDoc, error) {
	return getVolumeDoc(db, bson.D{{"_id", tag.Id()}}, fmt.Sprintf("volume %q", tag.Id()))
}

func getVolumeDoc(db Database, query bson.D, description string) (volumeDoc, error) {
	docs, err := getVolumeDocs(db, query)
	if err != nil {
		return volumeDoc{}, errors.Trace(err)
	}
	if len(docs) == 0 {
		return volumeDoc{}, errors.NotFoundf("%s", description)
	} else if len(docs) != 1 {
		return volumeDoc{}, errors.Errorf("expected 1 volume, got %d", len(docs))
	}
	return docs[0], nil
}

func getVolumeDocs(db Database, query interface{}) ([]volumeDoc, error) {
	coll, cleanup := db.GetCollection(volumesC)
	defer cleanup()

	var docs []volumeDoc
	err := coll.Find(query).All(&docs)
	if err != nil {
		return nil, errors.Annotate(err, "querying volumes")
	}
	for _, doc := range docs {
		if err := doc.validate(); err != nil {
			return nil, errors.Annotate(err, "validating volume")
		}
	}
	return docs, nil
}

func volumesToInterfaces(volumes []*volume) []Volume {
	result := make([]Volume, len(volumes))
	for i, v := range volumes {
		result[i] = v
	}
	return result
}

func (im *IAASModel) storageInstanceVolume(tag names.StorageTag) (*volume, error) {
	return im.volume(
		bson.D{{"storageid", tag.Id()}},
		fmt.Sprintf("volume for storage instance %q", tag.Id()),
	)
}

// StorageInstanceVolume returns the Volume assigned to the specified
// storage instance.
func (im *IAASModel) StorageInstanceVolume(tag names.StorageTag) (Volume, error) {
	v, err := im.storageInstanceVolume(tag)
	return v, err
}

// VolumeAttachment returns the VolumeAttachment corresponding to
// the specified volume and machine.
func (im *IAASModel) VolumeAttachment(machine names.MachineTag, volume names.VolumeTag) (VolumeAttachment, error) {
	coll, cleanup := im.mb.db().GetCollection(volumeAttachmentsC)
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
func (im *IAASModel) MachineVolumeAttachments(machine names.MachineTag) ([]VolumeAttachment, error) {
	attachments, err := im.volumeAttachments(bson.D{{"machineid", machine.Id()}})
	if err != nil {
		return nil, errors.Annotatef(err, "getting volume attachments for machine %q", machine.Id())
	}
	return attachments, nil
}

// VolumeAttachments returns all of the VolumeAttachments for the specified
// volume.
func (im *IAASModel) VolumeAttachments(volume names.VolumeTag) ([]VolumeAttachment, error) {
	attachments, err := im.volumeAttachments(bson.D{{"volumeid", volume.Id()}})
	if err != nil {
		return nil, errors.Annotatef(err, "getting volume attachments for volume %q", volume.Id())
	}
	return attachments, nil
}

func (im *IAASModel) volumeAttachments(query bson.D) ([]VolumeAttachment, error) {
	coll, cleanup := im.mb.db().GetCollection(volumeAttachmentsC)
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
// bound or attached to the specified machine. This is used when the given
// machine is being removed from state.
func (im *IAASModel) removeMachineVolumesOps(m *Machine) ([]txn.Op, error) {
	// A machine cannot transition to Dead if it has any detachable storage
	// attached, so any attachments are for machine-bound storage.
	//
	// Even if a volume is "non-detachable", there still exist volume
	// attachments, and they may be removed independently of the volume.
	// For example, the user may request that the volume be destroyed.
	// This will cause the volume to become Dying, and the attachment to
	// be Dying, then Dead, and finally removed. Only once the attachment
	// is removed will the volume transition to Dead and then be removed.
	// Therefore, there may be volumes that are bound, but not attached,
	// to the machine.

	machineVolumes, err := im.volumes(bson.D{{"machineid", m.Id()}})
	if err != nil {
		return nil, errors.Trace(err)
	}
	ops := make([]txn.Op, 0, 2*len(machineVolumes)+len(m.doc.Volumes))
	for _, volumeId := range m.doc.Volumes {
		ops = append(ops, txn.Op{
			C:      volumeAttachmentsC,
			Id:     volumeAttachmentId(m.Id(), volumeId),
			Assert: txn.DocExists,
			Remove: true,
		})
	}
	for _, v := range machineVolumes {
		volumeId := v.Tag().Id()
		if v.doc.StorageId != "" {
			// The volume is assigned to a storage instance;
			// make sure we also remove the storage instance.
			// There should be no storage attachments remaining,
			// as the units must have been removed before the
			// machine can be; and the storage attachments must
			// have been removed before the unit can be.
			ops = append(ops,
				txn.Op{
					C:      storageInstancesC,
					Id:     v.doc.StorageId,
					Assert: txn.DocExists,
					Remove: true,
				},
			)
		}
		ops = append(ops,
			txn.Op{
				C:      volumesC,
				Id:     volumeId,
				Assert: txn.DocExists,
				Remove: true,
			},
			removeModelVolumeRefOp(im.mb, volumeId),
		)
	}
	return ops, nil
}

// isDetachableVolumeTag reports whether or not the volume with the specified
// tag is detachable.
func isDetachableVolumeTag(db Database, tag names.VolumeTag) (bool, error) {
	doc, err := getVolumeDocByTag(db, tag)
	if err != nil {
		return false, errors.Trace(err)
	}
	return detachableVolumeDoc(&doc), nil
}

// Detachable reports whether or not the volume is detachable.
func (v *volume) Detachable() bool {
	return detachableVolumeDoc(&v.doc)
}

func (v *volume) pool() string {
	if v.doc.Info != nil {
		return v.doc.Info.Pool
	}
	return v.doc.Params.Pool
}

func detachableVolumeDoc(doc *volumeDoc) bool {
	return doc.MachineId == ""
}

// isDetachableVolumePool reports whether or not the given storage
// pool will create a volume that is not inherently bound to a machine,
// and therefore can be detached.
func isDetachableVolumePool(im *IAASModel, pool string) (bool, error) {
	_, provider, err := poolStorageProvider(im, pool)
	if err != nil {
		return false, errors.Trace(err)
	}
	if provider.Scope() == storage.ScopeMachine {
		// Any storage created by a machine cannot be detached from
		// the machine, and must be destroyed along with it.
		return false, nil
	}
	if provider.Dynamic() {
		// NOTE(axw) In theory, we don't know ahead of time
		// whether the storage will be Persistent, as the model
		// allows for a dynamic storage provider to create non-
		// persistent storage. None of the storage providers do
		// this, so we assume it will be persistent for now.
		//
		// TODO(axw) get rid of the Persistent field from Volume
		// and Filesystem. We only need to care whether the
		// storage is dynamic and model-scoped.
		return true, nil
	}
	// Volume is static, so it will be tied to the machine.
	return false, nil
}

// DetachVolume marks the volume attachment identified by the specified machine
// and volume tags as Dying, if it is Alive. DetachVolume will fail with a
// IsContainsFilesystem error if the volume contains an attached filesystem; the
// filesystem attachment must be removed first. DetachVolume will fail for
// inherently machine-bound volumes.
func (im *IAASModel) DetachVolume(machine names.MachineTag, volume names.VolumeTag) (err error) {
	defer errors.DeferredAnnotatef(&err, "detaching volume %s from machine %s", volume.Id(), machine.Id())
	// If the volume is backing a filesystem, the volume cannot be detached
	// until the filesystem has been detached.
	if _, err := im.volumeFilesystemAttachment(machine, volume); err == nil {
		return &errContainsFilesystem{errors.New("volume contains attached filesystem")}
	} else if !errors.IsNotFound(err) {
		return errors.Trace(err)
	}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		va, err := im.VolumeAttachment(machine, volume)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if va.Life() != Alive {
			return nil, jujutxn.ErrNoOperations
		}
		detachable, err := isDetachableVolumeTag(im.mb.db(), volume)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if !detachable {
			return nil, errors.New("volume is not detachable")
		}
		return detachVolumeOps(machine, volume), nil
	}
	return im.mb.db().Run(buildTxn)
}

func (im *IAASModel) volumeFilesystemAttachment(machine names.MachineTag, volume names.VolumeTag) (FilesystemAttachment, error) {
	filesystem, err := im.VolumeFilesystem(volume)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return im.FilesystemAttachment(machine, filesystem.FilesystemTag())
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
func (im *IAASModel) RemoveVolumeAttachment(machine names.MachineTag, volume names.VolumeTag) (err error) {
	defer errors.DeferredAnnotatef(&err, "removing attachment of volume %s from machine %s", volume.Id(), machine.Id())
	buildTxn := func(attempt int) ([]txn.Op, error) {
		attachment, err := im.VolumeAttachment(machine, volume)
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
		v, err := im.volumeByTag(volume)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return removeVolumeAttachmentOps(machine, v), nil
	}
	return im.mb.db().Run(buildTxn)
}

func removeVolumeAttachmentOps(m names.MachineTag, v *volume) []txn.Op {
	decrefVolumeOp := machineStorageDecrefOp(
		volumesC, v.doc.Name,
		v.doc.AttachmentCount, v.doc.Life, m,
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
func machineStorageDecrefOp(collection, id string, attachmentCount int, life Life, machine names.MachineTag) txn.Op {
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
		// volume is destroyed concurrently.
		//
		// Otherwise, when DestroyVolume is called, the volume
		// will be marked Dead if it has no attachments.
		update := bson.D{
			{"$inc", bson.D{{"attachmentcount", -1}}},
		}
		op.Assert = bson.D{
			{"life", Alive},
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
func (im *IAASModel) DestroyVolume(tag names.VolumeTag) (err error) {
	defer errors.DeferredAnnotatef(&err, "destroying volume %s", tag.Id())
	if _, err := im.VolumeFilesystem(tag); err == nil {
		return &errContainsFilesystem{errors.New("volume contains filesystem")}
	} else if !errors.IsNotFound(err) {
		return errors.Trace(err)
	}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		volume, err := im.volumeByTag(tag)
		if errors.IsNotFound(err) && attempt > 0 {
			// On the first attempt, we expect it to exist.
			return nil, jujutxn.ErrNoOperations
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		if volume.doc.StorageId != "" {
			return nil, errors.Errorf(
				"volume is assigned to %s",
				names.ReadableString(names.NewStorageTag(volume.doc.StorageId)),
			)
		}
		if volume.Life() != Alive {
			return nil, jujutxn.ErrNoOperations
		}
		hasNoStorageAssignment := bson.D{{"$or", []bson.D{
			{{"storageid", ""}},
			{{"storageid", bson.D{{"$exists", false}}}},
		}}}
		return destroyVolumeOps(im, volume, false, hasNoStorageAssignment)
	}
	return im.mb.db().Run(buildTxn)
}

func destroyVolumeOps(im *IAASModel, v *volume, release bool, extraAssert bson.D) ([]txn.Op, error) {
	baseAssert := append(isAliveDoc, extraAssert...)
	setFields := bson.D{}
	if release {
		setFields = append(setFields, bson.DocElem{"releasing", true})
	}
	if v.doc.AttachmentCount == 0 {
		hasNoAttachments := bson.D{{"attachmentcount", 0}}
		setFields = append(setFields, bson.DocElem{"life", Dead})
		return []txn.Op{{
			C:      volumesC,
			Id:     v.doc.Name,
			Assert: append(hasNoAttachments, baseAssert...),
			Update: bson.D{{"$set", setFields}},
		}}, nil
	}
	hasAttachments := bson.D{{"attachmentcount", bson.D{{"$gt", 0}}}}
	setFields = append(setFields, bson.DocElem{"life", Dying})
	ops := []txn.Op{{
		C:      volumesC,
		Id:     v.doc.Name,
		Assert: append(hasAttachments, baseAssert...),
		Update: bson.D{{"$set", setFields}},
	}}
	if !v.Detachable() {
		// This volume cannot be directly detached, so we do not
		// issue a cleanup. Since there can (should!) be only one
		// attachment for the lifetime of the filesystem, we can
		// look it up and destroy it along with the filesystem.
		attachments, err := im.VolumeAttachments(v.VolumeTag())
		if err != nil {
			return nil, errors.Trace(err)
		}
		if len(attachments) != 1 {
			return nil, errors.Errorf(
				"expected 1 attachment, found %d",
				len(attachments),
			)
		}
		detachOps := detachVolumeOps(
			attachments[0].Machine(),
			v.VolumeTag(),
		)
		ops = append(ops, detachOps...)
	} else {
		ops = append(ops, newCleanupOp(
			cleanupAttachmentsForDyingVolume,
			v.doc.Name,
		))
	}
	return ops, nil
}

// RemoveVolume removes the volume from state. RemoveVolume will fail if
// the volume is not Dead, which implies that it still has attachments.
func (im *IAASModel) RemoveVolume(tag names.VolumeTag) (err error) {
	defer errors.DeferredAnnotatef(&err, "removing volume %s", tag.Id())
	buildTxn := func(attempt int) ([]txn.Op, error) {
		volume, err := im.Volume(tag)
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
			removeModelVolumeRefOp(im.mb, tag.Id()),
			removeStatusOp(im.mb, volumeGlobalKey(tag.Id())),
		}, nil
	}
	return im.mb.db().Run(buildTxn)
}

// newVolumeName returns a unique volume name.
// If the machine ID supplied is non-empty, the
// volume ID will incorporate it as the volume's
// machine scope.
func newVolumeName(mb modelBackend, machineId string) (string, error) {
	seq, err := sequence(mb, "volume")
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
func (im *IAASModel) addVolumeOps(params VolumeParams, machineId string) ([]txn.Op, names.VolumeTag, error) {
	params, err := im.volumeParamsWithDefaults(params, machineId)
	if err != nil {
		return nil, names.VolumeTag{}, errors.Trace(err)
	}
	detachable, err := isDetachableVolumePool(im, params.Pool)
	if err != nil {
		return nil, names.VolumeTag{}, errors.Trace(err)
	}
	origMachineId := machineId
	machineId, err = im.validateVolumeParams(params, machineId)
	if err != nil {
		return nil, names.VolumeTag{}, errors.Annotate(err, "validating volume params")
	}
	name, err := newVolumeName(im.mb, machineId)
	if err != nil {
		return nil, names.VolumeTag{}, errors.Annotate(err, "cannot generate volume name")
	}
	statusDoc := statusDoc{
		Status:  status.Pending,
		Updated: im.mb.clock().Now().UnixNano(),
	}
	doc := volumeDoc{
		Name:      name,
		StorageId: params.storage.Id(),
	}
	if params.volumeInfo != nil {
		// We're importing an already provisioned volume into the
		// model. Set provisioned info rather than params, and set
		// the status to "detached".
		statusDoc.Status = status.Detached
		doc.Info = params.volumeInfo
	} else {
		// Every new volume is created with one attachment.
		doc.Params = &params
		doc.AttachmentCount = 1
	}
	if !detachable {
		doc.MachineId = origMachineId
	}
	return im.newVolumeOps(doc, statusDoc), names.NewVolumeTag(name), nil
}

func (im *IAASModel) newVolumeOps(doc volumeDoc, status statusDoc) []txn.Op {
	return []txn.Op{
		createStatusOp(im.mb, volumeGlobalKey(doc.Name), status),
		{
			C:      volumesC,
			Id:     doc.Name,
			Assert: txn.DocMissing,
			Insert: &doc,
		},
		addModelVolumeRefOp(im.mb, doc.Name),
	}
}

func (im *IAASModel) volumeParamsWithDefaults(params VolumeParams, machineId string) (VolumeParams, error) {
	if params.Pool == "" {
		modelConfig, err := im.st.ModelConfig()
		if err != nil {
			return VolumeParams{}, errors.Trace(err)
		}
		cons := StorageConstraints{
			Pool:  params.Pool,
			Size:  params.Size,
			Count: 1,
		}
		poolName, err := defaultStoragePool(modelConfig, storage.StorageKindBlock, cons)
		if err != nil {
			return VolumeParams{}, errors.Annotate(err, "getting default block storage pool")
		}
		params.Pool = poolName
	}
	return params, nil
}

// validateVolumeParams validates the volume parameters, and returns the
// machine ID to use as the scope in the volume tag.
func (im *IAASModel) validateVolumeParams(params VolumeParams, machineId string) (maybeMachineId string, _ error) {
	if err := validateStoragePool(im, params.Pool, storage.StorageKindBlock, &machineId); err != nil {
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
	tag      names.VolumeTag
	params   VolumeAttachmentParams
	existing bool
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
		if attachment.existing {
			ops = append(ops, txn.Op{
				C:      volumesC,
				Id:     attachment.tag.Id(),
				Assert: txn.DocExists,
				Update: bson.D{{"$inc", bson.D{{"attachmentcount", 1}}}},
			})
		}
	}
	return ops
}

// setMachineVolumeAttachmentInfo sets the volume attachment
// info for the specified machine. Each volume attachment info
// structure is keyed by the name of the volume it corresponds
// to.
func setMachineVolumeAttachmentInfo(im *IAASModel, machineId string, attachments map[names.VolumeTag]VolumeAttachmentInfo) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot set volume attachment info for machine %s", machineId)
	machineTag := names.NewMachineTag(machineId)
	for volumeTag, info := range attachments {
		if err := im.setVolumeAttachmentInfo(machineTag, volumeTag, info); err != nil {
			return errors.Annotatef(err, "setting attachment info for volume %s", volumeTag.Id())
		}
	}
	return nil
}

// SetVolumeAttachmentInfo sets the VolumeAttachmentInfo for the specified
// volume attachment.
func (im *IAASModel) SetVolumeAttachmentInfo(machineTag names.MachineTag, volumeTag names.VolumeTag, info VolumeAttachmentInfo) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot set info for volume attachment %s:%s", volumeTag.Id(), machineTag.Id())
	v, err := im.Volume(volumeTag)
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
	m, err := im.st.Machine(machineTag.Id())
	if err != nil {
		return errors.Trace(err)
	}
	if _, err := m.InstanceId(); err != nil {
		return errors.Trace(err)
	}
	return im.setVolumeAttachmentInfo(machineTag, volumeTag, info)
}

func (im *IAASModel) setVolumeAttachmentInfo(machineTag names.MachineTag, volumeTag names.VolumeTag, info VolumeAttachmentInfo) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		va, err := im.VolumeAttachment(machineTag, volumeTag)
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
	return im.mb.db().Run(buildTxn)
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
func setProvisionedVolumeInfo(im *IAASModel, volumes map[names.VolumeTag]VolumeInfo) error {
	for volumeTag, info := range volumes {
		if err := im.SetVolumeInfo(volumeTag, info); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// SetVolumeInfo sets the VolumeInfo for the specified volume.
func (im *IAASModel) SetVolumeInfo(tag names.VolumeTag, info VolumeInfo) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot set info for volume %q", tag.Id())
	if info.VolumeId == "" {
		return errors.New("volume ID not set")
	}
	// TODO(axw) we should reject info without VolumeId set; can't do this
	// until the providers all set it correctly.
	buildTxn := func(attempt int) ([]txn.Op, error) {
		v, err := im.Volume(tag)
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
	return im.mb.db().Run(buildTxn)
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

// AllVolumes returns all Volumes scoped to the model.
func (im *IAASModel) AllVolumes() ([]Volume, error) {
	volumes, err := im.volumes(nil)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get volumes")
	}
	return volumesToInterfaces(volumes), nil
}

func volumeGlobalKey(name string) string {
	return "v#" + name
}

// VolumeStatus returns the status of the specified volume.
func (im *IAASModel) VolumeStatus(tag names.VolumeTag) (status.StatusInfo, error) {
	return getStatus(im.mb.db(), volumeGlobalKey(tag.Id()), "volume")
}

// SetVolumeStatus sets the status of the specified volume.
func (im *IAASModel) SetVolumeStatus(tag names.VolumeTag, volumeStatus status.Status, info string, data map[string]interface{}, updated *time.Time) error {
	switch volumeStatus {
	case status.Attaching, status.Attached, status.Detaching, status.Detached, status.Destroying:
	case status.Error:
		if info == "" {
			return errors.Errorf("cannot set status %q without info", volumeStatus)
		}
	case status.Pending:
		// If a volume is not yet provisioned, we allow its status
		// to be set back to pending (when a retry is to occur).
		v, err := im.Volume(tag)
		if err != nil {
			return errors.Trace(err)
		}
		_, err = v.Info()
		if errors.IsNotProvisioned(err) {
			break
		}
		return errors.Errorf("cannot set status %q", volumeStatus)
	default:
		return errors.Errorf("cannot set invalid status %q", volumeStatus)
	}
	return setStatus(im.mb.db(), setStatusParams{
		badge:     "volume",
		globalKey: volumeGlobalKey(tag.Id()),
		status:    volumeStatus,
		message:   info,
		rawData:   data,
		updated:   timeOrNow(updated, im.mb.clock()),
	})
}
