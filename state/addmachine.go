// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strconv"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/status"
	"github.com/juju/juju/storage"
)

// MachineTemplate holds attributes that are to be associated
// with a newly created machine.
type MachineTemplate struct {
	// Series is the series to be associated with the new machine.
	Series string

	// Constraints are the constraints to be used when finding
	// an instance for the machine.
	Constraints constraints.Value

	// Jobs holds the jobs to run on the machine's instance.
	// A machine must have at least one job to do.
	// JobManageModel can only be part of the jobs
	// when the first (bootstrap) machine is added.
	Jobs []MachineJob

	// NoVote holds whether a machine running
	// a controller should abstain from peer voting.
	// It is ignored if Jobs does not contain JobManageModel.
	NoVote bool

	// Addresses holds the addresses to be associated with the
	// new machine.
	//
	// TODO(dimitern): This should be removed once all addresses
	// come from link-layer device addresses.
	Addresses []network.Address

	// InstanceId holds the instance id to associate with the machine.
	// If this is empty, the provisioner will try to provision the machine.
	// If this is non-empty, the HardwareCharacteristics and Nonce
	// fields must be set appropriately.
	InstanceId instance.Id

	// HardwareCharacteristics holds the h/w characteristics to
	// be associated with the machine.
	HardwareCharacteristics instance.HardwareCharacteristics

	// LinkLayerDevices holds a list of arguments for setting link-layer devices
	// on the machine.
	LinkLayerDevices []LinkLayerDeviceArgs

	// Volumes holds the parameters for volumes that are to be created
	// and attached to the machine.
	Volumes []MachineVolumeParams

	// VolumeAttachments holds the parameters for attaching existing
	// volumes to the machine.
	VolumeAttachments map[names.VolumeTag]VolumeAttachmentParams

	// Filesystems holds the parameters for filesystems that are to be
	// created and attached to the machine.
	Filesystems []MachineFilesystemParams

	// FilesystemAttachments holds the parameters for attaching existing
	// filesystems to the machine.
	FilesystemAttachments map[names.FilesystemTag]FilesystemAttachmentParams

	// Nonce holds a unique value that can be used to check
	// if a new instance was really started for this machine.
	// See Machine.SetProvisioned. This must be set if InstanceId is set.
	Nonce string

	// Dirty signifies whether the new machine will be treated
	// as unclean for unit-assignment purposes.
	Dirty bool

	// Placement holds the placement directive that will be associated
	// with the machine.
	Placement string

	// principals holds the principal units that will
	// associated with the machine.
	principals []string
}

// MachineVolumeParams holds the parameters for creating a volume and
// attaching it to a new machine.
type MachineVolumeParams struct {
	Volume     VolumeParams
	Attachment VolumeAttachmentParams
}

// MachineFilesystemParams holds the parameters for creating a filesystem
// and attaching it to a new machine.
type MachineFilesystemParams struct {
	Filesystem FilesystemParams
	Attachment FilesystemAttachmentParams
}

// AddMachineInsideNewMachine creates a new machine within a container
// of the given type inside another new machine. The two given templates
// specify the form of the child and parent respectively.
func (st *State) AddMachineInsideNewMachine(template, parentTemplate MachineTemplate, containerType instance.ContainerType) (*Machine, error) {
	mdoc, ops, err := st.addMachineInsideNewMachineOps(template, parentTemplate, containerType)
	if err != nil {
		return nil, errors.Annotate(err, "cannot add a new machine")
	}
	return st.addMachine(mdoc, ops)
}

// AddMachineInsideMachine adds a machine inside a container of the
// given type on the existing machine with id=parentId.
func (st *State) AddMachineInsideMachine(template MachineTemplate, parentId string, containerType instance.ContainerType) (*Machine, error) {
	mdoc, ops, err := st.addMachineInsideMachineOps(template, parentId, containerType)
	if err != nil {
		return nil, errors.Annotate(err, "cannot add a new machine")
	}
	return st.addMachine(mdoc, ops)
}

// AddMachine adds a machine with the given series and jobs.
// It is deprecated and around for testing purposes only.
func (st *State) AddMachine(series string, jobs ...MachineJob) (*Machine, error) {
	ms, err := st.AddMachines(MachineTemplate{
		Series: series,
		Jobs:   jobs,
	})
	if err != nil {
		return nil, err
	}
	return ms[0], nil
}

// AddOneMachine machine adds a new machine configured according to the
// given template.
func (st *State) AddOneMachine(template MachineTemplate) (*Machine, error) {
	ms, err := st.AddMachines(template)
	if err != nil {
		return nil, err
	}
	return ms[0], nil
}

// AddMachines adds new machines configured according to the
// given templates.
func (st *State) AddMachines(templates ...MachineTemplate) (_ []*Machine, err error) {
	defer errors.DeferredAnnotatef(&err, "cannot add a new machine")
	var ms []*Machine
	var ops []txn.Op
	var mdocs []*machineDoc
	for _, template := range templates {
		mdoc, addOps, err := st.addMachineOps(template)
		if err != nil {
			return nil, errors.Trace(err)
		}
		mdocs = append(mdocs, mdoc)
		ms = append(ms, newMachine(st, mdoc))
		ops = append(ops, addOps...)
	}
	ssOps, err := st.maintainControllersOps(mdocs, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ops = append(ops, ssOps...)
	ops = append(ops, assertModelActiveOp(st.ModelUUID()))
	if err := st.db().RunTransaction(ops); err != nil {
		if errors.Cause(err) == txn.ErrAborted {
			if err := checkModelActive(st); err != nil {
				return nil, errors.Trace(err)
			}
		}
		return nil, errors.Trace(err)
	}
	return ms, nil
}

func (st *State) addMachine(mdoc *machineDoc, ops []txn.Op) (*Machine, error) {
	ops = append([]txn.Op{assertModelActiveOp(st.ModelUUID())}, ops...)
	if err := st.db().RunTransaction(ops); err != nil {
		if errors.Cause(err) == txn.ErrAborted {
			if err := checkModelActive(st); err != nil {
				return nil, errors.Trace(err)
			}
		}
		return nil, errors.Trace(err)
	}
	return newMachine(st, mdoc), nil
}

func (st *State) resolveMachineConstraints(cons constraints.Value) (constraints.Value, error) {
	mcons, err := st.resolveConstraints(cons)
	if err != nil {
		return constraints.Value{}, err
	}
	// Machine constraints do not use a container constraint value.
	// Both provisioning and deployment constraints use the same
	// constraints.Value struct so here we clear the container
	// value. Provisioning ignores the container value but clearing
	// it avoids potential confusion.
	mcons.Container = nil
	return mcons, nil
}

// effectiveMachineTemplate verifies that the given template is
// valid and combines it with values from the state
// to produce a resulting template that more accurately
// represents the data that will be inserted into the state.
func (st *State) effectiveMachineTemplate(p MachineTemplate, allowController bool) (tmpl MachineTemplate, err error) {
	// First check for obvious errors.
	if p.Series == "" {
		return tmpl, errors.New("no series specified")
	}
	if p.InstanceId != "" {
		if p.Nonce == "" {
			return tmpl, errors.New("cannot add a machine with an instance id and no nonce")
		}
	} else if p.Nonce != "" {
		return tmpl, errors.New("cannot specify a nonce without an instance id")
	}

	// We ignore all constraints if there's a placement directive.
	if p.Placement == "" {
		p.Constraints, err = st.resolveMachineConstraints(p.Constraints)
		if err != nil {
			return tmpl, err
		}
	}

	if len(p.Jobs) == 0 {
		return tmpl, errors.New("no jobs specified")
	}
	jset := make(map[MachineJob]bool)
	for _, j := range p.Jobs {
		if jset[j] {
			return MachineTemplate{}, errors.Errorf("duplicate job: %s", j)
		}
		jset[j] = true
	}
	if jset[JobManageModel] {
		if !allowController {
			return tmpl, errControllerNotAllowed
		}
	}
	return p, nil
}

// addMachineOps returns operations to add a new top level machine
// based on the given template. It also returns the machine document
// that will be inserted.
func (st *State) addMachineOps(template MachineTemplate) (*machineDoc, []txn.Op, error) {
	template, err := st.effectiveMachineTemplate(template, st.IsController())
	if err != nil {
		return nil, nil, err
	}
	if template.InstanceId == "" {
		volumeAttachments, err := st.machineTemplateVolumeAttachmentParams(template)
		if err != nil {
			return nil, nil, err
		}
		if err := st.precheckInstance(
			template.Series,
			template.Constraints,
			template.Placement,
			volumeAttachments,
		); err != nil {
			return nil, nil, err
		}
	}
	seq, err := sequence(st, "machine")
	if err != nil {
		return nil, nil, err
	}
	mdoc := st.machineDocForTemplate(template, strconv.Itoa(seq))
	prereqOps, machineOp, err := st.insertNewMachineOps(mdoc, template)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	prereqOps = append(prereqOps, assertModelActiveOp(st.ModelUUID()))
	prereqOps = append(prereqOps, insertNewContainerRefOp(st, mdoc.Id))
	if template.InstanceId != "" {
		prereqOps = append(prereqOps, txn.Op{
			C:      instanceDataC,
			Id:     mdoc.DocID,
			Assert: txn.DocMissing,
			Insert: &instanceData{
				DocID:      mdoc.DocID,
				MachineId:  mdoc.Id,
				InstanceId: template.InstanceId,
				ModelUUID:  mdoc.ModelUUID,
				Arch:       template.HardwareCharacteristics.Arch,
				Mem:        template.HardwareCharacteristics.Mem,
				RootDisk:   template.HardwareCharacteristics.RootDisk,
				CpuCores:   template.HardwareCharacteristics.CpuCores,
				CpuPower:   template.HardwareCharacteristics.CpuPower,
				Tags:       template.HardwareCharacteristics.Tags,
				AvailZone:  template.HardwareCharacteristics.AvailabilityZone,
			},
		})
	}

	return mdoc, append(prereqOps, machineOp), nil
}

// supportsContainerType reports whether the machine supports the given
// container type. If the machine's supportedContainers attribute is
// set, this decision can be made right here, otherwise we assume that
// everything will be ok and later on put the container into an error
// state if necessary.
func (m *Machine) supportsContainerType(ctype instance.ContainerType) bool {
	supportedContainers, ok := m.SupportedContainers()
	if !ok {
		// We don't know yet, so we report that we support the container.
		return true
	}
	for _, ct := range supportedContainers {
		if ct == ctype {
			return true
		}
	}
	return false
}

// addMachineInsideMachineOps returns operations to add a machine inside
// a container of the given type on an existing machine.
func (st *State) addMachineInsideMachineOps(template MachineTemplate, parentId string, containerType instance.ContainerType) (*machineDoc, []txn.Op, error) {
	if template.InstanceId != "" {
		return nil, nil, errors.New("cannot specify instance id for a new container")
	}
	template, err := st.effectiveMachineTemplate(template, false)
	if err != nil {
		return nil, nil, err
	}
	if containerType == "" {
		return nil, nil, errors.New("no container type specified")
	}

	// If a parent machine is specified, make sure it exists
	// and can support the requested container type.
	parent, err := st.Machine(parentId)
	if err != nil {
		return nil, nil, err
	}
	if !parent.supportsContainerType(containerType) {
		return nil, nil, errors.Errorf("machine %s cannot host %s containers", parentId, containerType)
	}

	newId, err := st.newContainerId(parentId, containerType)
	if err != nil {
		return nil, nil, err
	}
	mdoc := st.machineDocForTemplate(template, newId)
	mdoc.ContainerType = string(containerType)
	prereqOps, machineOp, err := st.insertNewMachineOps(mdoc, template)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	prereqOps = append(prereqOps,
		// Update containers record for host machine.
		addChildToContainerRefOp(st, parentId, mdoc.Id),
		// Create a containers reference document for the container itself.
		insertNewContainerRefOp(st, mdoc.Id),
	)
	return mdoc, append(prereqOps, machineOp), nil
}

// newContainerId returns a new id for a machine within the machine
// with id parentId and the given container type.
func (st *State) newContainerId(parentId string, containerType instance.ContainerType) (string, error) {
	seq, err := sequence(st, fmt.Sprintf("machine%s%sContainer", parentId, containerType))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%s/%d", parentId, containerType, seq), nil
}

// addMachineInsideNewMachineOps returns operations to create a new
// machine within a container of the given type inside another
// new machine. The two given templates specify the form
// of the child and parent respectively.
func (st *State) addMachineInsideNewMachineOps(template, parentTemplate MachineTemplate, containerType instance.ContainerType) (*machineDoc, []txn.Op, error) {
	if template.InstanceId != "" || parentTemplate.InstanceId != "" {
		return nil, nil, errors.New("cannot specify instance id for a new container")
	}
	seq, err := sequence(st, "machine")
	if err != nil {
		return nil, nil, err
	}
	parentTemplate, err = st.effectiveMachineTemplate(parentTemplate, false)
	if err != nil {
		return nil, nil, err
	}
	if containerType == "" {
		return nil, nil, errors.New("no container type specified")
	}
	if parentTemplate.InstanceId == "" {
		volumeAttachments, err := st.machineTemplateVolumeAttachmentParams(parentTemplate)
		if err != nil {
			return nil, nil, err
		}
		if err := st.precheckInstance(
			parentTemplate.Series,
			parentTemplate.Constraints,
			parentTemplate.Placement,
			volumeAttachments,
		); err != nil {
			return nil, nil, err
		}
	}

	parentDoc := st.machineDocForTemplate(parentTemplate, strconv.Itoa(seq))
	newId, err := st.newContainerId(parentDoc.Id, containerType)
	if err != nil {
		return nil, nil, err
	}
	template, err = st.effectiveMachineTemplate(template, false)
	if err != nil {
		return nil, nil, err
	}
	mdoc := st.machineDocForTemplate(template, newId)
	mdoc.ContainerType = string(containerType)
	parentPrereqOps, parentOp, err := st.insertNewMachineOps(parentDoc, parentTemplate)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	prereqOps, machineOp, err := st.insertNewMachineOps(mdoc, template)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	prereqOps = append(prereqOps, parentPrereqOps...)
	prereqOps = append(prereqOps,
		// The host machine doesn't exist yet, create a new containers record.
		insertNewContainerRefOp(st, mdoc.Id),
		// Create a containers reference document for the container itself.
		insertNewContainerRefOp(st, parentDoc.Id, mdoc.Id),
	)
	return mdoc, append(prereqOps, parentOp, machineOp), nil
}

func (st *State) machineTemplateVolumeAttachmentParams(t MachineTemplate) ([]storage.VolumeAttachmentParams, error) {
	im, err := st.IAASModel()
	if err != nil {
		return nil, errors.Trace(err)
	}
	out := make([]storage.VolumeAttachmentParams, 0, len(t.VolumeAttachments))
	for volumeTag, a := range t.VolumeAttachments {
		v, err := im.Volume(volumeTag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		volumeInfo, err := v.Info()
		if err != nil {
			return nil, errors.Trace(err)
		}
		providerType, _, err := poolStorageProvider(im, volumeInfo.Pool)
		if err != nil {
			return nil, errors.Trace(err)
		}
		out = append(out, storage.VolumeAttachmentParams{
			AttachmentParams: storage.AttachmentParams{
				Provider: providerType,
				ReadOnly: a.ReadOnly,
			},
			Volume:   volumeTag,
			VolumeId: volumeInfo.VolumeId,
		})
	}
	return out, nil
}

func (st *State) machineDocForTemplate(template MachineTemplate, id string) *machineDoc {
	// We ignore the error from Select*Address as an error indicates
	// no address is available, in which case the empty address is returned
	// and setting the preferred address to an empty one is the correct
	// thing to do when none is available.
	privateAddr, _ := network.SelectInternalAddress(template.Addresses, false)
	publicAddr, _ := network.SelectPublicAddress(template.Addresses)
	logger.Infof(
		"new machine %q has preferred addresses: private %q, public %q",
		id, privateAddr, publicAddr,
	)
	return &machineDoc{
		DocID:                   st.docID(id),
		Id:                      id,
		ModelUUID:               st.ModelUUID(),
		Series:                  template.Series,
		Jobs:                    template.Jobs,
		Clean:                   !template.Dirty,
		Principals:              template.principals,
		Life:                    Alive,
		Nonce:                   template.Nonce,
		Addresses:               fromNetworkAddresses(template.Addresses, OriginMachine),
		PreferredPrivateAddress: fromNetworkAddress(privateAddr, OriginMachine),
		PreferredPublicAddress:  fromNetworkAddress(publicAddr, OriginMachine),
		NoVote:                  template.NoVote,
		Placement:               template.Placement,
	}
}

// insertNewMachineOps returns operations to insert the given machine document
// into the database, based on the given template. Only the constraints are
// taken from the template.
func (st *State) insertNewMachineOps(mdoc *machineDoc, template MachineTemplate) (prereqOps []txn.Op, machineOp txn.Op, err error) {
	now := st.clock().Now()
	machineStatusDoc := statusDoc{
		Status:    status.Pending,
		ModelUUID: st.ModelUUID(),
		Updated:   now.UnixNano(),
	}
	instanceStatusDoc := statusDoc{
		Status:    status.Pending,
		ModelUUID: st.ModelUUID(),
		Updated:   now.UnixNano(),
	}

	prereqOps, machineOp = st.baseNewMachineOps(
		mdoc,
		machineStatusDoc,
		instanceStatusDoc,
		template.Constraints,
	)

	storageOps, volumeAttachments, filesystemAttachments, err := st.machineStorageOps(
		mdoc, &machineStorageParams{
			filesystems:           template.Filesystems,
			filesystemAttachments: template.FilesystemAttachments,
			volumes:               template.Volumes,
			volumeAttachments:     template.VolumeAttachments,
		},
	)
	if err != nil {
		return nil, txn.Op{}, errors.Trace(err)
	}
	for _, a := range volumeAttachments {
		mdoc.Volumes = append(mdoc.Volumes, a.tag.Id())
	}
	for _, a := range filesystemAttachments {
		mdoc.Filesystems = append(mdoc.Filesystems, a.tag.Id())
	}
	prereqOps = append(prereqOps, storageOps...)

	// At the last moment we still have statusDoc in scope, set the initial
	// history entry. This is risky, and may lead to extra entries, but that's
	// an intrinsic problem with mixing txn and non-txn ops -- we can't sync
	// them cleanly.
	probablyUpdateStatusHistory(st.db(), machineGlobalKey(mdoc.Id), machineStatusDoc)
	probablyUpdateStatusHistory(st.db(), machineGlobalInstanceKey(mdoc.Id), instanceStatusDoc)
	return prereqOps, machineOp, nil
}

func (st *State) baseNewMachineOps(mdoc *machineDoc, machineStatusDoc, instanceStatusDoc statusDoc, cons constraints.Value) (prereqOps []txn.Op, machineOp txn.Op) {
	machineOp = txn.Op{
		C:      machinesC,
		Id:     mdoc.DocID,
		Assert: txn.DocMissing,
		Insert: mdoc,
	}

	globalKey := machineGlobalKey(mdoc.Id)
	globalInstanceKey := machineGlobalInstanceKey(mdoc.Id)

	prereqOps = []txn.Op{
		createConstraintsOp(globalKey, cons),
		createStatusOp(st, globalKey, machineStatusDoc),
		createStatusOp(st, globalInstanceKey, instanceStatusDoc),
		createMachineBlockDevicesOp(mdoc.Id),
		addModelMachineRefOp(st, mdoc.Id),
	}
	return prereqOps, machineOp
}

type machineStorageParams struct {
	volumes               []MachineVolumeParams
	volumeAttachments     map[names.VolumeTag]VolumeAttachmentParams
	filesystems           []MachineFilesystemParams
	filesystemAttachments map[names.FilesystemTag]FilesystemAttachmentParams
}

func combineMachineStorageParams(lhs, rhs *machineStorageParams) *machineStorageParams {
	out := &machineStorageParams{}
	out.volumes = append(lhs.volumes[:], rhs.volumes...)
	out.filesystems = append(lhs.filesystems[:], rhs.filesystems...)
	if lhs.volumeAttachments != nil || rhs.volumeAttachments != nil {
		out.volumeAttachments = make(map[names.VolumeTag]VolumeAttachmentParams)
		for k, v := range lhs.volumeAttachments {
			out.volumeAttachments[k] = v
		}
		for k, v := range rhs.volumeAttachments {
			out.volumeAttachments[k] = v
		}
	}
	if lhs.filesystemAttachments != nil || rhs.filesystemAttachments != nil {
		out.filesystemAttachments = make(map[names.FilesystemTag]FilesystemAttachmentParams)
		for k, v := range lhs.filesystemAttachments {
			out.filesystemAttachments[k] = v
		}
		for k, v := range rhs.filesystemAttachments {
			out.filesystemAttachments[k] = v
		}
	}
	return out
}

// machineStorageOps creates txn.Ops for creating volumes, filesystems,
// and attachments to the specified machine. The results are the txn.Ops,
// and the tags of volumes and filesystems newly attached to the machine.
func (st *State) machineStorageOps(
	mdoc *machineDoc, args *machineStorageParams,
) ([]txn.Op, []volumeAttachmentTemplate, []filesystemAttachmentTemplate, error) {
	var filesystemOps, volumeOps []txn.Op
	var fsAttachments []filesystemAttachmentTemplate
	var volumeAttachments []volumeAttachmentTemplate

	const (
		createAndAttach = false
		attachOnly      = true
	)

	im, err := st.IAASModel()
	if err != nil {
		return nil, nil, nil, errors.Trace(err)
	}

	// Create filesystems and filesystem attachments.
	for _, f := range args.filesystems {
		ops, filesystemTag, volumeTag, err := im.addFilesystemOps(f.Filesystem, mdoc.Id)
		if err != nil {
			return nil, nil, nil, errors.Trace(err)
		}
		filesystemOps = append(filesystemOps, ops...)
		fsAttachments = append(fsAttachments, filesystemAttachmentTemplate{
			filesystemTag, f.Filesystem.storage, f.Attachment, createAndAttach,
		})
		if volumeTag != (names.VolumeTag{}) {
			// The filesystem requires a volume, so create a volume attachment too.
			volumeAttachments = append(volumeAttachments, volumeAttachmentTemplate{
				volumeTag, VolumeAttachmentParams{}, createAndAttach,
			})
		}
	}
	for tag, filesystemAttachment := range args.filesystemAttachments {
		fsAttachments = append(fsAttachments, filesystemAttachmentTemplate{
			tag, names.StorageTag{}, filesystemAttachment, attachOnly,
		})
	}

	// Create volumes and volume attachments.
	for _, v := range args.volumes {
		ops, tag, err := im.addVolumeOps(v.Volume, mdoc.Id)
		if err != nil {
			return nil, nil, nil, errors.Trace(err)
		}
		volumeOps = append(volumeOps, ops...)
		volumeAttachments = append(volumeAttachments, volumeAttachmentTemplate{
			tag, v.Attachment, createAndAttach,
		})
	}
	for tag, volumeAttachment := range args.volumeAttachments {
		volumeAttachments = append(volumeAttachments, volumeAttachmentTemplate{
			tag, volumeAttachment, attachOnly,
		})
	}

	ops := make([]txn.Op, 0, len(filesystemOps)+len(volumeOps)+len(fsAttachments)+len(volumeAttachments))
	if len(fsAttachments) > 0 {
		attachmentOps := createMachineFilesystemAttachmentsOps(mdoc.Id, fsAttachments)
		ops = append(ops, filesystemOps...)
		ops = append(ops, attachmentOps...)
	}
	if len(volumeAttachments) > 0 {
		attachmentOps := createMachineVolumeAttachmentsOps(mdoc.Id, volumeAttachments)
		ops = append(ops, volumeOps...)
		ops = append(ops, attachmentOps...)
	}
	return ops, volumeAttachments, fsAttachments, nil
}

// addMachineStorageAttachmentsOps returns txn.Ops for adding the IDs of
// attached volumes and filesystems to an existing machine. Filesystem
// mount points are checked against existing filesystem attachments for
// conflicts, with a txn.Op added to prevent concurrent additions as
// necessary.
func addMachineStorageAttachmentsOps(
	machine *Machine,
	volumes []volumeAttachmentTemplate,
	filesystems []filesystemAttachmentTemplate,
) ([]txn.Op, error) {
	var addToSet bson.D
	assert := isAliveDoc
	if len(volumes) > 0 {
		volumeIds := make([]string, len(volumes))
		for i, v := range volumes {
			volumeIds[i] = v.tag.Id()
		}
		addToSet = append(addToSet, bson.DocElem{
			"volumes", bson.D{{"$each", volumeIds}},
		})
	}
	if len(filesystems) > 0 {
		filesystemIds := make([]string, len(filesystems))
		var withLocation []filesystemAttachmentTemplate
		for i, f := range filesystems {
			filesystemIds[i] = f.tag.Id()
			if !f.params.locationAutoGenerated {
				// If the location was not automatically
				// generated, we must ensure it does not
				// conflict with any existing storage.
				// Generated paths are guaranteed to be
				// unique.
				withLocation = append(withLocation, f)
			}
		}
		addToSet = append(addToSet, bson.DocElem{
			"filesystems", bson.D{{"$each", filesystemIds}},
		})
		if len(withLocation) > 0 {
			if err := validateFilesystemMountPoints(machine, withLocation); err != nil {
				return nil, errors.Annotate(err, "validating filesystem mount points")
			}
			// Make sure no filesystems are added concurrently.
			assert = append(assert, bson.DocElem{
				"filesystems", bson.D{{"$not", bson.D{{
					"$elemMatch", bson.D{{
						"$nin", machine.doc.Filesystems,
					}},
				}}}},
			})
		}
	}
	var update interface{}
	if len(addToSet) > 0 {
		update = bson.D{{"$addToSet", addToSet}}
	}
	return []txn.Op{{
		C:      machinesC,
		Id:     machine.doc.Id,
		Assert: assert,
		Update: update,
	}}, nil
}
