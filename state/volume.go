// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/status"
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

	// Host returns the tag of the related Host.
	Host() names.Tag

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

// VolumeAttachmentPlan describes the plan information for a particular volume
// Machine agents use this information to do any extra initialization that is needed
// This is separate from VolumeAttachment to allow separation of concerns between
// the controller's idea of detaching a volume and the machine agent's idea.
// This way, we can have the controller ask the environment for a volume, attach it
// to the instance, which in some cases simply means granting the instance access
// to connect to it, and then explicitly let the machine agent know that something
// has been attached to it.
type VolumeAttachmentPlan interface {
	Lifer

	// Volume returns the tag of the related Volume.
	Volume() names.VolumeTag

	// Machine returns the tag of the related Machine.
	Machine() names.MachineTag

	// PlanInfo returns the plan info for a volume
	PlanInfo() (VolumeAttachmentPlanInfo, error)

	// BlockDeviceInfo returns the block device info associated with
	// this plan, as seen by the machine agent it is plugged into
	BlockDeviceInfo() (BlockDeviceInfo, error)
}

type volume struct {
	mb  modelBackend
	doc volumeDoc
}

type volumeAttachment struct {
	doc volumeAttachmentDoc
}

type volumeAttachmentPlan struct {
	doc volumeAttachmentPlanDoc
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

	// HostId is the ID of the host that a non-detachable
	// volume is initially attached to. We use this to identify
	// the volume as being non-detachable, and to determine
	// which volumes must be removed along with said machine.
	HostId string `bson:"hostid,omitempty"`
}

// volumeAttachmentDoc records information about a volume attachment.
type volumeAttachmentDoc struct {
	// DocID is the machine global key followed by the volume name.
	DocID     string                  `bson:"_id"`
	ModelUUID string                  `bson:"model-uuid"`
	Volume    string                  `bson:"volumeid"`
	Host      string                  `bson:"hostid"`
	Life      Life                    `bson:"life"`
	Info      *VolumeAttachmentInfo   `bson:"info,omitempty"`
	Params    *VolumeAttachmentParams `bson:"params,omitempty"`
}

type volumeAttachmentPlanDoc struct {
	DocID     string                    `bson:"_id"`
	ModelUUID string                    `bson:"model-uuid"`
	Volume    string                    `bson:"volumeid"`
	Machine   string                    `bson:"machineid"`
	Life      Life                      `bson:"life"`
	PlanInfo  *VolumeAttachmentPlanInfo `bson:"plan-info,omitempty"`
	// BlockDevice represents the block device from the point
	// of view of the machine agent. Once the machine agent
	// finishes provisioning the storage attachment, it gathers
	// as much information about the new device as needed, and
	// sets it in the volume attachment plan, in state. This
	// information will later be used to match the block device
	// in state, with the block device the machine agent sees.
	BlockDevice *BlockDeviceInfo `bson:"block-device,omitempty"`
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
	// PlanInfo holds information used by the machine storage
	// provisioner to execute any needed steps in order to make
	// make sure the actual storage device becomes available.
	// For example, any storage backend that requires userspace
	// setup, like iSCSI would fall into this category.
	PlanInfo *VolumeAttachmentPlanInfo `bson:"plan-info,omitempty"`
}

type VolumeAttachmentPlanInfo struct {
	// DeviceType is the type of storage type this plan info
	// describes. For directly attached local storage, this
	// can be left to its default value, or set as storage.DeviceTypeLocal
	// This value will be used by the machine storage provisioner
	// to load the appropriate storage plan, and execute any Attach/Detach
	// operations.
	DeviceType storage.DeviceType `bson:"device-type,omitempty"`
	// DeviceAttributes holds a map of key/value pairs that may be used
	// by the storage plan backend to initialize the storage device
	// For example, if dealing with iSCSI, this can hold the IP address
	// of the remote server, the LUN, access credentials, etc.
	DeviceAttributes map[string]string `bson:"device-attributes,omitempty"`
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
	return getStatus(v.mb.db(), volumeGlobalKey(v.VolumeTag().Id()), "volume")
}

// SetStatus is required to implement StatusSetter.
func (v *volume) SetStatus(volumeStatus status.StatusInfo) error {
	switch volumeStatus.Status {
	case status.Attaching, status.Attached, status.Detaching, status.Detached, status.Destroying:
	case status.Error:
		if volumeStatus.Message == "" {
			return errors.Errorf("cannot set status %q without info", volumeStatus.Status)
		}
	case status.Pending:
		// If a volume is not yet provisioned, we allow its status
		// to be set back to pending (when a retry is to occur).
		// First refresh.
		v, err := getVolumeByTag(v.mb, v.VolumeTag())
		if err != nil {
			return errors.Trace(err)
		}
		_, err = v.Info()
		if errors.IsNotProvisioned(err) {
			break
		}
		return errors.Errorf("cannot set status %q", volumeStatus.Status)
	default:
		return errors.Errorf("cannot set invalid status %q", volumeStatus.Status)
	}
	return setStatus(v.mb.db(), setStatusParams{
		badge:     "volume",
		globalKey: volumeGlobalKey(v.VolumeTag().Id()),
		status:    volumeStatus.Status,
		message:   volumeStatus.Message,
		rawData:   volumeStatus.Data,
		updated:   timeOrNow(volumeStatus.Since, v.mb.clock()),
	})
}

func (v *volumeAttachmentPlan) Volume() names.VolumeTag {
	return names.NewVolumeTag(v.doc.Volume)
}

// Machine is required to implement VolumeAttachmentPlan.
func (v *volumeAttachmentPlan) Machine() names.MachineTag {
	return names.NewMachineTag(v.doc.Machine)
}

// Life is required to implement VolumeAttachmentPlan.
func (v *volumeAttachmentPlan) Life() Life {
	return v.doc.Life
}

// PlanInfo is required to implement VolumeAttachment.
func (v *volumeAttachmentPlan) PlanInfo() (VolumeAttachmentPlanInfo, error) {
	if v.doc.PlanInfo == nil {
		return VolumeAttachmentPlanInfo{}, errors.NotProvisionedf("volume attachment plan %q on %q", v.doc.Volume, v.doc.Machine)
	}
	return *v.doc.PlanInfo, nil
}

func (v *volumeAttachmentPlan) BlockDeviceInfo() (BlockDeviceInfo, error) {
	if v.doc.BlockDevice == nil {
		return BlockDeviceInfo{}, errors.NotFoundf("volume attachment plan block device %q on %q", v.doc.Volume, v.doc.Machine)
	}
	return *v.doc.BlockDevice, nil
}

// Volume is required to implement VolumeAttachment.
func (v *volumeAttachment) Volume() names.VolumeTag {
	return names.NewVolumeTag(v.doc.Volume)
}

// Host is required to implement VolumeAttachment.
func (v *volumeAttachment) Host() names.Tag {
	return storageAttachmentHost(v.doc.Host)
}

// Life is required to implement VolumeAttachment.
func (v *volumeAttachment) Life() Life {
	return v.doc.Life
}

// Info is required to implement VolumeAttachment.
func (v *volumeAttachment) Info() (VolumeAttachmentInfo, error) {
	if v.doc.Info == nil {
		host := storageAttachmentHost(v.doc.Host)
		return VolumeAttachmentInfo{}, errors.NotProvisionedf("volume attachment %q on %q", v.doc.Volume, names.ReadableString(host))
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
func (sb *storageBackend) Volume(tag names.VolumeTag) (Volume, error) {
	v, err := getVolumeByTag(sb.mb, tag)
	return v, err
}

func (sb *storageBackend) volumes(query interface{}) ([]*volume, error) {
	docs, err := getVolumeDocs(sb.mb.db(), query)
	if err != nil {
		return nil, errors.Trace(err)
	}
	volumes := make([]*volume, len(docs))
	for i := range docs {
		volumes[i] = &volume{sb.mb, docs[i]}
	}
	return volumes, nil
}

func getVolumeByTag(mb modelBackend, tag names.VolumeTag) (*volume, error) {
	doc, err := getVolumeDocByTag(mb.db(), tag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &volume{mb, doc}, nil
}

func (sb *storageBackend) volume(query bson.D, description string) (*volume, error) {
	doc, err := getVolumeDoc(sb.mb.db(), query, description)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &volume{sb.mb, doc}, nil
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

func (sb *storageBackend) storageInstanceVolume(tag names.StorageTag) (*volume, error) {
	return sb.volume(
		bson.D{{"storageid", tag.Id()}},
		fmt.Sprintf("volume for storage instance %q", tag.Id()),
	)
}

// StorageInstanceVolume returns the Volume assigned to the specified
// storage instance.
func (sb *storageBackend) StorageInstanceVolume(tag names.StorageTag) (Volume, error) {
	v, err := sb.storageInstanceVolume(tag)
	return v, err
}

// VolumeAttachment returns the VolumeAttachment corresponding to
// the specified volume and machine.
func (sb *storageBackend) VolumeAttachment(host names.Tag, volume names.VolumeTag) (VolumeAttachment, error) {
	coll, cleanup := sb.mb.db().GetCollection(volumeAttachmentsC)
	defer cleanup()

	var att volumeAttachment
	err := coll.FindId(volumeAttachmentId(host.Id(), volume.Id())).One(&att.doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("volume %q on %q", volume.Id(), names.ReadableString(host))
	} else if err != nil {
		return nil, errors.Annotatef(err, "getting volume %q on %q", volume.Id(), names.ReadableString(host))
	}
	return &att, nil
}

func (sb *storageBackend) VolumeAttachmentPlan(host names.Tag, volume names.VolumeTag) (VolumeAttachmentPlan, error) {
	coll, cleanup := sb.mb.db().GetCollection(volumeAttachmentPlanC)
	defer cleanup()

	var att volumeAttachmentPlan
	err := coll.FindId(volumeAttachmentId(host.Id(), volume.Id())).One(&att.doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("volume attachment plan %q on host %q", volume.Id(), host.Id())
	} else if err != nil {
		return nil, errors.Annotatef(err, "getting volume attachment plan %q on host %q", volume.Id(), host.Id())
	}
	return &att, nil
}

// MachineVolumeAttachments returns all of the VolumeAttachments for the
// specified machine.
func (sb *storageBackend) MachineVolumeAttachments(machine names.MachineTag) ([]VolumeAttachment, error) {
	attachments, err := sb.volumeAttachments(bson.D{{"hostid", machine.Id()}})
	if err != nil {
		return nil, errors.Annotatef(err, "getting volume attachments for machine %q", machine.Id())
	}
	return attachments, nil
}

// UnitVolumeAttachments returns all of the VolumeAttachments for the
// specified unit.
func (sb *storageBackend) UnitVolumeAttachments(unit names.UnitTag) ([]VolumeAttachment, error) {
	attachments, err := sb.volumeAttachments(bson.D{{"hostid", unit.Id()}})
	if err != nil {
		return nil, errors.Annotatef(err, "getting volume attachments for unit %q", unit.Id())
	}
	return attachments, nil
}

// VolumeAttachments returns all of the VolumeAttachments for the specified
// volume.
func (sb *storageBackend) VolumeAttachments(volume names.VolumeTag) ([]VolumeAttachment, error) {
	attachments, err := sb.volumeAttachments(bson.D{{"volumeid", volume.Id()}})
	if err != nil {
		return nil, errors.Annotatef(err, "getting volume attachments for volume %q", volume.Id())
	}
	return attachments, nil
}

func (sb *storageBackend) volumeAttachments(query bson.D) ([]VolumeAttachment, error) {
	coll, cleanup := sb.mb.db().GetCollection(volumeAttachmentsC)
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

func (sb *storageBackend) machineVolumeAttachmentPlans(host names.Tag, v names.VolumeTag) ([]VolumeAttachmentPlan, error) {
	id := volumeAttachmentId(host.Id(), v.Id())
	attachmentPlans, err := sb.volumeAttachmentPlans(bson.D{{"_id", id}})
	if err != nil {
		return nil, errors.Annotatef(err, "getting volume attachment plans for volume %q attached to machine %q", v.Id(), host.Id())
	}
	return attachmentPlans, nil
}

// VolumeAttachmentPlans returns all of the VolumeAttachmentPlans for the specified
// volume.
func (sb *storageBackend) VolumeAttachmentPlans(volume names.VolumeTag) ([]VolumeAttachmentPlan, error) {
	attachmentPlans, err := sb.volumeAttachmentPlans(bson.D{{"volumeid", volume.Id()}})
	if err != nil {
		return nil, errors.Annotatef(err, "getting volume attachment plans for volume %q", volume.Id())
	}
	return attachmentPlans, nil
}

func (sb *storageBackend) volumeAttachmentPlans(query bson.D) ([]VolumeAttachmentPlan, error) {
	coll, cleanup := sb.mb.db().GetCollection(volumeAttachmentPlanC)
	defer cleanup()

	var docs []volumeAttachmentPlanDoc
	err := coll.Find(query).All(&docs)
	if err == mgo.ErrNotFound {
		return []VolumeAttachmentPlan{}, nil
	} else if err != nil {
		return []VolumeAttachmentPlan{}, errors.Trace(err)
	}
	attachmentPlans := make([]VolumeAttachmentPlan, len(docs))
	for i, doc := range docs {
		attachmentPlans[i] = &volumeAttachmentPlan{doc}
	}
	return attachmentPlans, nil
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
func (sb *storageBackend) removeMachineVolumesOps(m *Machine) ([]txn.Op, error) {
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

	machineVolumes, err := sb.volumes(bson.D{{"hostid", m.Id()}})
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
		ops = append(ops, sb.removeVolumeOps(v.VolumeTag())...)
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
	return doc.HostId == ""
}

// isDetachableVolumePool reports whether or not the given storage
// pool will create a volume that is not inherently bound to a machine,
// and therefore can be detached.
func isDetachableVolumePool(im *storageBackend, pool string) (bool, error) {
	_, provider, _, err := poolStorageProvider(im, pool)
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
func (sb *storageBackend) DetachVolume(host names.Tag, volume names.VolumeTag) (err error) {
	defer errors.DeferredAnnotatef(&err, "detaching volume %s from %s", volume.Id(), names.ReadableString(host))
	// If the volume is backing a filesystem, the volume cannot be detached
	// until the filesystem has been detached.
	if _, err := sb.volumeFilesystemAttachment(host, volume); err == nil {
		return &errContainsFilesystem{errors.New("volume contains attached filesystem")}
	} else if !errors.IsNotFound(err) {
		return errors.Trace(err)
	}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		va, err := sb.VolumeAttachment(host, volume)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if va.Life() != Alive {
			return nil, jujutxn.ErrNoOperations
		}
		detachable, err := isDetachableVolumeTag(sb.mb.db(), volume)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if !detachable {
			return nil, errors.New("volume is not detachable")
		}
		if plans, err := sb.machineVolumeAttachmentPlans(host, volume); err != nil {
			return nil, errors.Trace(err)
		} else {
			if len(plans) > 0 {
				return detachStorageAttachmentOps(host, volume), nil
			}
		}
		return detachVolumeOps(host, volume), nil
	}
	return sb.mb.db().Run(buildTxn)
}

func (sb *storageBackend) volumeFilesystemAttachment(host names.Tag, volume names.VolumeTag) (FilesystemAttachment, error) {
	filesystem, err := sb.VolumeFilesystem(volume)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return sb.FilesystemAttachment(host, filesystem.FilesystemTag())
}

func detachStorageAttachmentOps(host names.Tag, v names.VolumeTag) []txn.Op {
	id := volumeAttachmentId(host.Id(), v.Id())
	return []txn.Op{{
		C:      volumeAttachmentPlanC,
		Id:     id,
		Assert: isAliveDoc,
		Update: bson.D{{"$set", bson.D{{"life", Dying}}}},
	}}
}

func detachVolumeOps(host names.Tag, v names.VolumeTag) []txn.Op {
	return []txn.Op{{
		C:      volumeAttachmentsC,
		Id:     volumeAttachmentId(host.Id(), v.Id()),
		Assert: isAliveDoc,
		Update: bson.D{{"$set", bson.D{{"life", Dying}}}},
	}}
}

// RemoveVolumeAttachment removes the volume attachment from state.
// RemoveVolumeAttachment will fail if the attachment is not Dying.
func (sb *storageBackend) RemoveVolumeAttachment(host names.Tag, volume names.VolumeTag) (err error) {
	defer errors.DeferredAnnotatef(&err, "removing attachment of volume %s from %s", volume.Id(), names.ReadableString(host))
	buildTxn := func(attempt int) ([]txn.Op, error) {
		attachment, err := sb.VolumeAttachment(host, volume)
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
		v, err := getVolumeByTag(sb.mb, volume)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return removeVolumeAttachmentOps(host, v), nil
	}
	return sb.mb.db().Run(buildTxn)
}

func removeVolumeAttachmentOps(host names.Tag, v *volume) []txn.Op {
	decrefVolumeOp := machineStorageDecrefOp(
		volumesC, v.doc.Name,
		v.doc.AttachmentCount, v.doc.Life,
	)
	ops := []txn.Op{{
		C:      volumeAttachmentsC,
		Id:     volumeAttachmentId(host.Id(), v.doc.Name),
		Assert: bson.D{{"life", Dying}},
		Remove: true,
	}, decrefVolumeOp}
	if host.Kind() == names.MachineTagKind {
		ops = append(ops, txn.Op{
			C:      machinesC,
			Id:     host.Id(),
			Assert: txn.DocExists,
			Update: bson.D{{"$pull", bson.D{{"volumes", v.doc.Name}}}},
		})
	}
	return ops
}

// machineStorageDecrefOp returns a txn.Op that will decrement the attachment
// count for a given machine storage entity (volume or filesystem), given its
// current attachment count and lifecycle state. If the attachment count goes
// to zero, then the entity should become Dead.
func machineStorageDecrefOp(collection, id string, attachmentCount int, life Life) txn.Op {
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
func (sb *storageBackend) DestroyVolume(tag names.VolumeTag) (err error) {
	defer errors.DeferredAnnotatef(&err, "destroying volume %s", tag.Id())
	if _, err := sb.VolumeFilesystem(tag); err == nil {
		return &errContainsFilesystem{errors.New("volume contains filesystem")}
	} else if !errors.IsNotFound(err) {
		return errors.Trace(err)
	}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		volume, err := getVolumeByTag(sb.mb, tag)
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
		return destroyVolumeOps(sb, volume, false, hasNoStorageAssignment)
	}
	return sb.mb.db().Run(buildTxn)
}

func destroyVolumeOps(im *storageBackend, v *volume, release bool, extraAssert bson.D) ([]txn.Op, error) {
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
			attachments[0].Host(),
			v.VolumeTag(),
		)
		ops = append(ops, detachOps...)
	} else {
		// TODO(gsamfira): add cleanup for volume plans
		ops = append(ops, newCleanupOp(
			cleanupAttachmentsForDyingVolume,
			v.doc.Name,
		))
	}
	return ops, nil
}

// RemoveVolume removes the volume from state. RemoveVolume will fail if
// the volume is not Dead, which implies that it still has attachments.
func (sb *storageBackend) RemoveVolume(tag names.VolumeTag) (err error) {
	defer errors.DeferredAnnotatef(&err, "removing volume %s", tag.Id())
	buildTxn := func(attempt int) ([]txn.Op, error) {
		volume, err := sb.Volume(tag)
		if errors.IsNotFound(err) {
			return nil, jujutxn.ErrNoOperations
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		if volume.Life() != Dead {
			return nil, errors.New("volume is not dead")
		}
		return sb.removeVolumeOps(tag), nil
	}
	return sb.mb.db().Run(buildTxn)
}

func (sb *storageBackend) removeVolumeOps(tag names.VolumeTag) []txn.Op {
	return []txn.Op{
		{
			C:      volumesC,
			Id:     tag.Id(),
			Assert: txn.DocExists,
			Remove: true,
		},
		removeModelVolumeRefOp(sb.mb, tag.Id()),
		removeStatusOp(sb.mb, volumeGlobalKey(tag.Id())),
	}
}

// newVolumeName returns a unique volume name.
// If the host ID supplied is non-empty, the
// volume ID will incorporate it as the volume's
// machine scope.
func newVolumeName(mb modelBackend, hostId string) (string, error) {
	seq, err := sequence(mb, "volume")
	if err != nil {
		return "", errors.Trace(err)
	}
	id := fmt.Sprint(seq)
	if hostId != "" {
		id = hostId + "/" + id
	}
	return id, nil
}

// addVolumeOps returns txn.Ops to create a new volume with the specified
// parameters. If the supplied host ID is non-empty, and the storage
// provider is machine-scoped, then the volume will be scoped to that
// machine.
func (sb *storageBackend) addVolumeOps(params VolumeParams, hostId string) ([]txn.Op, names.VolumeTag, error) {
	params, err := sb.volumeParamsWithDefaults(params)
	if err != nil {
		return nil, names.VolumeTag{}, errors.Trace(err)
	}
	detachable, err := isDetachableVolumePool(sb, params.Pool)
	if err != nil {
		return nil, names.VolumeTag{}, errors.Trace(err)
	}
	origHostId := hostId
	hostId, err = sb.validateVolumeParams(params, hostId)
	if err != nil {
		return nil, names.VolumeTag{}, errors.Annotate(err, "validating volume params")
	}
	name, err := newVolumeName(sb.mb, hostId)
	if err != nil {
		return nil, names.VolumeTag{}, errors.Annotate(err, "cannot generate volume name")
	}
	statusDoc := statusDoc{
		Status:  status.Pending,
		Updated: sb.mb.clock().Now().UnixNano(),
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
		doc.HostId = origHostId
	}
	return sb.newVolumeOps(doc, statusDoc), names.NewVolumeTag(name), nil
}

func (sb *storageBackend) newVolumeOps(doc volumeDoc, status statusDoc) []txn.Op {
	return []txn.Op{
		createStatusOp(sb.mb, volumeGlobalKey(doc.Name), status),
		{
			C:      volumesC,
			Id:     doc.Name,
			Assert: txn.DocMissing,
			Insert: &doc,
		},
		addModelVolumeRefOp(sb.mb, doc.Name),
	}
}

func (sb *storageBackend) volumeParamsWithDefaults(params VolumeParams) (VolumeParams, error) {
	if params.Pool == "" {
		modelConfig, err := sb.config()
		if err != nil {
			return VolumeParams{}, errors.Trace(err)
		}
		cons := StorageConstraints{
			Pool:  params.Pool,
			Size:  params.Size,
			Count: 1,
		}
		poolName, err := defaultStoragePool(sb.modelType, modelConfig, storage.StorageKindBlock, cons)
		if err != nil {
			return VolumeParams{}, errors.Annotate(err, "getting default block storage pool")
		}
		params.Pool = poolName
	}
	return params, nil
}

// validateVolumeParams validates the volume parameters, and returns the
// machine ID to use as the scope in the volume tag.
func (sb *storageBackend) validateVolumeParams(params VolumeParams, machineId string) (maybeMachineId string, _ error) {
	if err := validateStoragePool(sb, params.Pool, storage.StorageKindBlock, &machineId); err != nil {
		return "", err
	}
	if params.Size == 0 {
		return "", errors.New("invalid size 0")
	}
	return machineId, nil
}

// volumeAttachmentId returns a volume attachment document ID,
// given the corresponding volume name and host ID.
func volumeAttachmentId(hostId, volumeName string) string {
	return fmt.Sprintf("%s:%s", hostId, volumeName)
}

// ParseVolumeAttachmentId parses a string as a volume attachment ID,
// returning the host and volume components.
func ParseVolumeAttachmentId(id string) (names.Tag, names.VolumeTag, error) {
	fields := strings.SplitN(id, ":", 2)
	isValidHost := names.IsValidMachine(fields[0]) || names.IsValidUnit(fields[0])
	if len(fields) != 2 || !isValidHost || !names.IsValidVolume(fields[1]) {
		return names.MachineTag{}, names.VolumeTag{}, errors.Errorf("invalid volume attachment ID %q", id)
	}
	var hostTag names.Tag
	if names.IsValidMachine(fields[0]) {
		hostTag = names.NewMachineTag(fields[0])
	} else {
		hostTag = names.NewUnitTag(fields[0])
	}
	volumeTag := names.NewVolumeTag(fields[1])
	return hostTag, volumeTag, nil
}

type volumeAttachmentTemplate struct {
	tag      names.VolumeTag
	params   VolumeAttachmentParams
	existing bool
}

// createMachineVolumeAttachmentInfo creates volume attachments
// for the specified host, and attachment parameters keyed
// by volume tags. The caller is responsible for incrementing
// the volume's attachmentcount field.
func createMachineVolumeAttachmentsOps(hostId string, attachments []volumeAttachmentTemplate) []txn.Op {
	ops := make([]txn.Op, len(attachments))
	for i, attachment := range attachments {
		paramsCopy := attachment.params
		ops[i] = txn.Op{
			C:      volumeAttachmentsC,
			Id:     volumeAttachmentId(hostId, attachment.tag.Id()),
			Assert: txn.DocMissing,
			Insert: &volumeAttachmentDoc{
				Volume: attachment.tag.Id(),
				Host:   hostId,
				Params: &paramsCopy,
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
func setMachineVolumeAttachmentInfo(sb *storageBackend, machineId string, attachments map[names.VolumeTag]VolumeAttachmentInfo) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot set volume attachment info for machine %s", machineId)
	machineTag := names.NewMachineTag(machineId)
	for volumeTag, info := range attachments {
		if err := sb.setVolumeAttachmentInfo(machineTag, volumeTag, info); err != nil {
			return errors.Annotatef(err, "setting attachment info for volume %s", volumeTag.Id())
		}
	}
	return nil
}

// SetVolumeAttachmentInfo sets the VolumeAttachmentInfo for the specified
// volume attachment.
func (sb *storageBackend) SetVolumeAttachmentInfo(hostTag names.Tag, volumeTag names.VolumeTag, info VolumeAttachmentInfo) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot set info for volume attachment %s:%s", volumeTag.Id(), hostTag.Id())
	v, err := sb.Volume(volumeTag)
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
	if _, ok := hostTag.(names.MachineTag); ok {
		m, err := sb.machine(hostTag.Id())
		if err != nil {
			return errors.Trace(err)
		}
		if _, err := m.InstanceId(); err != nil {
			return errors.Trace(err)
		}
	}
	return sb.setVolumeAttachmentInfo(hostTag, volumeTag, info)
}

func (sb *storageBackend) SetVolumeAttachmentPlanBlockInfo(hostTag names.Tag, volumeTag names.VolumeTag, info BlockDeviceInfo) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot set block device plan info for volume attachment %s:%s", volumeTag.Id(), hostTag.Id())
	v, err := sb.Volume(volumeTag)
	if err != nil {
		return errors.Trace(err)
	}
	if _, err := v.Info(); err != nil {
		return errors.Trace(err)
	}
	// Also ensure the machine is provisioned.
	m, err := sb.machine(hostTag.Id())
	if err != nil {
		return errors.Trace(err)
	}
	if _, err := m.InstanceId(); err != nil {
		return errors.Trace(err)
	}
	return sb.setVolumePlanBlockInfo(hostTag, volumeTag, &info)
}

func (sb *storageBackend) setVolumePlanBlockInfo(hostTag names.Tag, volumeTag names.VolumeTag, info *BlockDeviceInfo) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		va, err := sb.VolumeAttachment(hostTag, volumeTag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if va.Life() != Alive {
			return nil, jujutxn.ErrNoOperations
		}
		volumePlan, err := sb.machineVolumeAttachmentPlans(hostTag, volumeTag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if volumePlan == nil || len(volumePlan) == 0 {
			return nil, jujutxn.ErrNoOperations
		}
		ops := setVolumePlanBlockInfoOps(
			hostTag, volumeTag, info,
		)
		return ops, nil
	}
	return sb.mb.db().Run(buildTxn)
}

func setVolumePlanBlockInfoOps(hostTag names.Tag, volumeTag names.VolumeTag, info *BlockDeviceInfo) []txn.Op {
	asserts := isAliveDoc
	update := bson.D{
		{"$set", bson.D{{"block-device", info}}},
	}
	return []txn.Op{{
		C:      volumeAttachmentPlanC,
		Id:     volumeAttachmentId(hostTag.Id(), volumeTag.Id()),
		Assert: asserts,
		Update: update,
	}}
}

func (sb *storageBackend) CreateVolumeAttachmentPlan(hostTag names.Tag, volumeTag names.VolumeTag, info VolumeAttachmentPlanInfo) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot set plan info for volume attachment %s:%s", volumeTag.Id(), hostTag.Id())
	v, err := sb.Volume(volumeTag)
	if err != nil {
		return errors.Trace(err)
	}
	if _, err := v.Info(); err != nil {
		return errors.Trace(err)
	}
	// Also ensure the machine is provisioned.
	m, err := sb.machine(hostTag.Id())
	if err != nil {
		return errors.Trace(err)
	}
	if _, err := m.InstanceId(); err != nil {
		return errors.Trace(err)
	}
	return sb.createVolumePlan(hostTag, volumeTag, &info)
}

func (sb *storageBackend) createVolumePlan(hostTag names.Tag, volumeTag names.VolumeTag, info *VolumeAttachmentPlanInfo) error {
	if info != nil && info.DeviceType == "" {
		info.DeviceType = storage.DeviceTypeLocal
	}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		va, err := sb.VolumeAttachment(hostTag, volumeTag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if va.Life() != Alive {
			return nil, jujutxn.ErrNoOperations
		}
		volumePlan, err := sb.machineVolumeAttachmentPlans(hostTag, volumeTag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if volumePlan != nil && len(volumePlan) > 0 {
			return nil, jujutxn.ErrNoOperations
		}
		ops := createVolumeAttachmentPlanOps(
			hostTag, volumeTag, info,
		)
		return ops, nil
	}
	return sb.mb.db().Run(buildTxn)
}

func createVolumeAttachmentPlanOps(hostTag names.Tag, volume names.VolumeTag, info *VolumeAttachmentPlanInfo) []txn.Op {
	return []txn.Op{
		{
			C:      volumeAttachmentPlanC,
			Id:     volumeAttachmentId(hostTag.Id(), volume.Id()),
			Assert: txn.DocMissing,
			Insert: &volumeAttachmentPlanDoc{
				Volume:   volume.Id(),
				Machine:  hostTag.Id(),
				Life:     Alive,
				PlanInfo: info,
			},
		},
	}
}

func (sb *storageBackend) setVolumeAttachmentInfo(hostTag names.Tag, volumeTag names.VolumeTag, info VolumeAttachmentInfo) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		va, err := sb.VolumeAttachment(hostTag, volumeTag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		// If the volume attachment has parameters, unset them
		// when we set info for the first time, ensuring that
		// params and info are mutually exclusive.
		_, unsetParams := va.Params()
		ops := setVolumeAttachmentInfoOps(
			hostTag, volumeTag, info, unsetParams,
		)
		return ops, nil
	}
	return sb.mb.db().Run(buildTxn)
}

func setVolumeAttachmentInfoOps(host names.Tag, volume names.VolumeTag, info VolumeAttachmentInfo, unsetParams bool) []txn.Op {
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
		Id:     volumeAttachmentId(host.Id(), volume.Id()),
		Assert: asserts,
		Update: update,
	}}
}

// RemoveVolumeAttachmentPlan removes the volume attachment plan from state.
func (sb *storageBackend) RemoveVolumeAttachmentPlan(hostTag names.Tag, volume names.VolumeTag) (err error) {
	defer errors.DeferredAnnotatef(&err, "removing attachment plan of volume %s from machine %s", volume.Id(), hostTag.Id())
	buildTxn := func(attempt int) ([]txn.Op, error) {
		plans, err := sb.machineVolumeAttachmentPlans(hostTag, volume)
		if errors.IsNotFound(err) {
			return nil, jujutxn.ErrNoOperations
		}
		if err != nil {
			return nil, errors.Trace(err)
		}
		// We should only have one plan for a volume
		if plans != nil && len(plans) > 0 {
			if plans[0].Life() != Dying {
				return nil, jujutxn.ErrNoOperations
			}
		} else {
			return nil, jujutxn.ErrNoOperations
		}
		return removeVolumeAttachmentPlanOps(hostTag, volume), nil
	}
	return sb.mb.db().Run(buildTxn)
}

// removeVolumeAttachmentPlanOps removes the plan from state and sets the volume attachment to Dying.
// this will trigger the storageprovisioner on the controller to eventually detach the volume from
// the machine.
func removeVolumeAttachmentPlanOps(hostTag names.Tag, volume names.VolumeTag) []txn.Op {
	detachOps := detachVolumeOps(hostTag, volume)
	removeOps := []txn.Op{{
		C:      volumeAttachmentPlanC,
		Id:     volumeAttachmentId(hostTag.Id(), volume.Id()),
		Assert: bson.D{{"life", Dying}},
		Remove: true,
	}}
	removeOps = append(removeOps, detachOps...)
	return removeOps
}

// setProvisionedVolumeInfo sets the initial info for newly
// provisioned volumes. If non-empty, machineId must be the
// machine ID associated with the volumes.
func setProvisionedVolumeInfo(sb *storageBackend, volumes map[names.VolumeTag]VolumeInfo) error {
	for volumeTag, info := range volumes {
		if err := sb.SetVolumeInfo(volumeTag, info); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// SetVolumeInfo sets the VolumeInfo for the specified volume.
func (sb *storageBackend) SetVolumeInfo(tag names.VolumeTag, info VolumeInfo) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot set info for volume %q", tag.Id())
	if info.VolumeId == "" {
		return errors.New("volume ID not set")
	}
	// TODO(axw) we should reject info without VolumeId set; can't do this
	// until the providers all set it correctly.
	buildTxn := func(attempt int) ([]txn.Op, error) {
		v, err := sb.Volume(tag)
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
	return sb.mb.db().Run(buildTxn)
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
func (sb *storageBackend) AllVolumes() ([]Volume, error) {
	volumes, err := sb.volumes(nil)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get volumes")
	}
	return volumesToInterfaces(volumes), nil
}

func volumeGlobalKey(name string) string {
	return "v#" + name
}
