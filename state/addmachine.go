// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strconv"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/replicaset"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
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
	// JobManageEnviron can only be part of the jobs
	// when the first (bootstrap) machine is added.
	Jobs []MachineJob

	// NoVote holds whether a machine running
	// a state server should abstain from peer voting.
	// It is ignored if Jobs does not contain JobManageEnviron.
	NoVote bool

	// Addresses holds the addresses to be associated with the
	// new machine.
	Addresses []network.Address

	// InstanceId holds the instance id to associate with the machine.
	// If this is empty, the provisioner will try to provision the machine.
	// If this is non-empty, the HardwareCharacteristics and Nonce
	// fields must be set appropriately.
	InstanceId instance.Id

	// HardwareCharacteristics holds the h/w characteristics to
	// be associated with the machine.
	HardwareCharacteristics instance.HardwareCharacteristics

	// RequestedNetworks holds a list of network names the machine
	// should be part of.
	//
	// TODO(dimitern): Drop this in favor of constraints in a follow-up.
	RequestedNetworks []string

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
	env, err := st.Environment()
	if err != nil {
		return nil, errors.Trace(err)
	} else if env.Life() != Alive {
		return nil, errors.New("environment is no longer alive")
	}
	var ops []txn.Op
	var mdocs []*machineDoc
	for _, template := range templates {
		// Adding a machine without any principals is
		// only permitted if unit placement is supported.
		if len(template.principals) == 0 && template.InstanceId == "" {
			if err := st.supportsUnitPlacement(); err != nil {
				return nil, errors.Trace(err)
			}
		}
		mdoc, addOps, err := st.addMachineOps(template)
		if err != nil {
			return nil, errors.Trace(err)
		}
		mdocs = append(mdocs, mdoc)
		ms = append(ms, newMachine(st, mdoc))
		ops = append(ops, addOps...)
	}
	ssOps, err := st.maintainStateServersOps(mdocs, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ops = append(ops, ssOps...)
	ops = append(ops, env.assertAliveOp())
	if err := st.runTransaction(ops); err != nil {
		return nil, onAbort(err, errors.New("environment is no longer alive"))
	}
	return ms, nil
}

func (st *State) addMachine(mdoc *machineDoc, ops []txn.Op) (*Machine, error) {
	env, err := st.Environment()
	if err != nil {
		return nil, err
	} else if env.Life() != Alive {
		return nil, errors.New("environment is no longer alive")
	}
	ops = append([]txn.Op{env.assertAliveOp()}, ops...)
	if err := st.runTransaction(ops); err != nil {
		enverr := env.Refresh()
		if (enverr == nil && env.Life() != Alive) || errors.IsNotFound(enverr) {
			return nil, errors.New("environment is no longer alive")
		} else if enverr != nil {
			err = enverr
		}
		return nil, err
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
func (st *State) effectiveMachineTemplate(p MachineTemplate, allowStateServer bool) (tmpl MachineTemplate, err error) {
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

	p.Constraints, err = st.resolveMachineConstraints(p.Constraints)
	if err != nil {
		return tmpl, err
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
	if jset[JobManageEnviron] {
		if !allowStateServer {
			return tmpl, errStateServerNotAllowed
		}
	}
	return p, nil
}

// addMachineOps returns operations to add a new top level machine
// based on the given template. It also returns the machine document
// that will be inserted.
func (st *State) addMachineOps(template MachineTemplate) (*machineDoc, []txn.Op, error) {
	template, err := st.effectiveMachineTemplate(template, st.IsStateServer())
	if err != nil {
		return nil, nil, err
	}
	if template.InstanceId == "" {
		if err := st.precheckInstance(template.Series, template.Constraints, template.Placement); err != nil {
			return nil, nil, err
		}
	}
	seq, err := st.sequence("machine")
	if err != nil {
		return nil, nil, err
	}
	mdoc := st.machineDocForTemplate(template, strconv.Itoa(seq))
	prereqOps, machineOp, err := st.insertNewMachineOps(mdoc, template)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	prereqOps = append(prereqOps, assertEnvAliveOp(st.EnvironUUID()))
	prereqOps = append(prereqOps, st.insertNewContainerRefOp(mdoc.Id))
	if template.InstanceId != "" {
		prereqOps = append(prereqOps, txn.Op{
			C:      instanceDataC,
			Id:     mdoc.DocID,
			Assert: txn.DocMissing,
			Insert: &instanceData{
				DocID:      mdoc.DocID,
				MachineId:  mdoc.Id,
				InstanceId: template.InstanceId,
				EnvUUID:    mdoc.EnvUUID,
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
	// Adding a machine within a machine implies add-machine or placement.
	if err := st.supportsUnitPlacement(); err != nil {
		return nil, nil, err
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
		st.addChildToContainerRefOp(parentId, mdoc.Id),
		// Create a containers reference document for the container itself.
		st.insertNewContainerRefOp(mdoc.Id),
	)
	return mdoc, append(prereqOps, machineOp), nil
}

// newContainerId returns a new id for a machine within the machine
// with id parentId and the given container type.
func (st *State) newContainerId(parentId string, containerType instance.ContainerType) (string, error) {
	seq, err := st.sequence(fmt.Sprintf("machine%s%sContainer", parentId, containerType))
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
	seq, err := st.sequence("machine")
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
		// Adding a machine within a machine implies add-machine or placement.
		if err := st.supportsUnitPlacement(); err != nil {
			return nil, nil, err
		}
		if err := st.precheckInstance(parentTemplate.Series, parentTemplate.Constraints, parentTemplate.Placement); err != nil {
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
		st.insertNewContainerRefOp(mdoc.Id),
		// Create a containers reference document for the container itself.
		st.insertNewContainerRefOp(parentDoc.Id, mdoc.Id),
	)
	return mdoc, append(prereqOps, parentOp, machineOp), nil
}

func (st *State) machineDocForTemplate(template MachineTemplate, id string) *machineDoc {
	// We ignore the error from Select*Address as an error indicates
	// no address is available, in which case the empty address is returned
	// and setting the preferred address to an empty one is the correct
	// thing to do when none is available.
	privateAddr, _ := network.SelectInternalAddress(template.Addresses, false)
	publicAddr, _ := network.SelectPublicAddress(template.Addresses)
	return &machineDoc{
		DocID:                   st.docID(id),
		Id:                      id,
		EnvUUID:                 st.EnvironUUID(),
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

// insertNewMachineOps returns operations to insert the given machine
// document into the database, based on the given template. Only the
// constraints and networks are used from the template.
func (st *State) insertNewMachineOps(mdoc *machineDoc, template MachineTemplate) (prereqOps []txn.Op, machineOp txn.Op, err error) {
	machineOp = txn.Op{
		C:      machinesC,
		Id:     mdoc.DocID,
		Assert: txn.DocMissing,
		Insert: mdoc,
	}

	statusDoc := statusDoc{
		Status:  StatusPending,
		EnvUUID: st.EnvironUUID(),
		Updated: time.Now().UnixNano(),
	}
	globalKey := machineGlobalKey(mdoc.Id)
	prereqOps = []txn.Op{
		createConstraintsOp(st, globalKey, template.Constraints),
		createStatusOp(st, globalKey, statusDoc),
		// TODO(dimitern): Drop requested networks across the board in a
		// follow-up.
		createRequestedNetworksOp(st, globalKey, template.RequestedNetworks),
		createMachineBlockDevicesOp(mdoc.Id),
	}

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
	probablyUpdateStatusHistory(st, globalKey, statusDoc)
	return prereqOps, machineOp, nil
}

type machineStorageParams struct {
	volumes               []MachineVolumeParams
	volumeAttachments     map[names.VolumeTag]VolumeAttachmentParams
	filesystems           []MachineFilesystemParams
	filesystemAttachments map[names.FilesystemTag]FilesystemAttachmentParams
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

	// Create filesystems and filesystem attachments.
	for _, f := range args.filesystems {
		ops, filesystemTag, volumeTag, err := st.addFilesystemOps(f.Filesystem, mdoc.Id)
		if err != nil {
			return nil, nil, nil, errors.Trace(err)
		}
		filesystemOps = append(filesystemOps, ops...)
		fsAttachments = append(fsAttachments, filesystemAttachmentTemplate{
			filesystemTag, f.Filesystem.storage, f.Attachment,
		})
		if volumeTag != (names.VolumeTag{}) {
			// The filesystem requires a volume, so create a volume attachment too.
			volumeAttachments = append(volumeAttachments, volumeAttachmentTemplate{
				volumeTag, VolumeAttachmentParams{},
			})
		}
	}

	// Create volumes and volume attachments.
	for _, v := range args.volumes {
		ops, tag, err := st.addVolumeOps(v.Volume, mdoc.Id)
		if err != nil {
			return nil, nil, nil, errors.Trace(err)
		}
		volumeOps = append(volumeOps, ops...)
		volumeAttachments = append(volumeAttachments, volumeAttachmentTemplate{
			tag, v.Attachment,
		})
	}

	// TODO(axw) handle args.filesystemAttachments, args.volumeAttachments
	// when we handle attaching to existing (e.g. shared) storage.

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
	var updates bson.D
	assert := isAliveDoc
	if len(volumes) > 0 {
		volumeIds := make([]string, len(volumes))
		for i, v := range volumes {
			volumeIds[i] = v.tag.Id()
		}
		updates = append(updates, bson.DocElem{"$addToSet", bson.D{{
			"volumes", bson.D{{"$each", volumeIds}}}},
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
		updates = append(updates, bson.DocElem{"$addToSet", bson.D{{
			"filesystems", bson.D{{"$each", filesystemIds}}}},
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
	return []txn.Op{{
		C:      machinesC,
		Id:     machine.doc.Id,
		Assert: assert,
		Update: updates,
	}}, nil
}

func hasJob(jobs []MachineJob, job MachineJob) bool {
	for _, j := range jobs {
		if j == job {
			return true
		}
	}
	return false
}

var errStateServerNotAllowed = errors.New("state server jobs specified but not allowed")

// maintainStateServersOps returns a set of operations that will maintain
// the state server information when the given machine documents
// are added to the machines collection. If currentInfo is nil,
// there can be only one machine document and it must have
// id 0 (this is a special case to allow adding the bootstrap machine)
func (st *State) maintainStateServersOps(mdocs []*machineDoc, currentInfo *StateServerInfo) ([]txn.Op, error) {
	var newIds, newVotingIds []string
	for _, doc := range mdocs {
		if !hasJob(doc.Jobs, JobManageEnviron) {
			continue
		}
		newIds = append(newIds, doc.Id)
		if !doc.NoVote {
			newVotingIds = append(newVotingIds, doc.Id)
		}
	}
	if len(newIds) == 0 {
		return nil, nil
	}
	if currentInfo == nil {
		// Allow bootstrap machine only.
		if len(mdocs) != 1 || mdocs[0].Id != "0" {
			return nil, errStateServerNotAllowed
		}
		var err error
		currentInfo, err = st.StateServerInfo()
		if err != nil {
			return nil, errors.Annotate(err, "cannot get state server info")
		}
		if len(currentInfo.MachineIds) > 0 || len(currentInfo.VotingMachineIds) > 0 {
			return nil, errors.New("state servers already exist")
		}
	}
	ops := []txn.Op{{
		C:  stateServersC,
		Id: environGlobalKey,
		Assert: bson.D{{
			"$and", []bson.D{
				{{"machineids", bson.D{{"$size", len(currentInfo.MachineIds)}}}},
				{{"votingmachineids", bson.D{{"$size", len(currentInfo.VotingMachineIds)}}}},
			},
		}},
		Update: bson.D{
			{"$addToSet", bson.D{{"machineids", bson.D{{"$each", newIds}}}}},
			{"$addToSet", bson.D{{"votingmachineids", bson.D{{"$each", newVotingIds}}}}},
		},
	}}
	return ops, nil
}

// EnsureAvailability adds state server machines as necessary to make
// the number of live state servers equal to numStateServers. The given
// constraints and series will be attached to any new machines.
// If placement is not empty, any new machines which may be required are started
// according to the specified placement directives until the placement list is
// exhausted; thereafter any new machines are started according to the constraints and series.
func (st *State) EnsureAvailability(
	numStateServers int, cons constraints.Value, series string, placement []string,
) (StateServersChanges, error) {

	if numStateServers < 0 || (numStateServers != 0 && numStateServers%2 != 1) {
		return StateServersChanges{}, errors.New("number of state servers must be odd and non-negative")
	}
	if numStateServers > replicaset.MaxPeers {
		return StateServersChanges{}, errors.Errorf("state server count is too large (allowed %d)", replicaset.MaxPeers)
	}
	var change StateServersChanges
	buildTxn := func(attempt int) ([]txn.Op, error) {
		currentInfo, err := st.StateServerInfo()
		if err != nil {
			return nil, err
		}
		desiredStateServerCount := numStateServers
		if desiredStateServerCount == 0 {
			desiredStateServerCount = len(currentInfo.VotingMachineIds)
			if desiredStateServerCount <= 1 {
				desiredStateServerCount = 3
			}
		}
		if len(currentInfo.VotingMachineIds) > desiredStateServerCount {
			return nil, errors.New("cannot reduce state server count")
		}

		intent, err := st.ensureAvailabilityIntentions(currentInfo, placement)
		if err != nil {
			return nil, err
		}
		voteCount := 0
		for _, m := range intent.maintain {
			if m.WantsVote() {
				voteCount++
			}
		}
		if voteCount == desiredStateServerCount && len(intent.remove) == 0 {
			return nil, jujutxn.ErrNoOperations
		}
		// Promote as many machines as we can to fulfil the shortfall.
		if n := desiredStateServerCount - voteCount; n < len(intent.promote) {
			intent.promote = intent.promote[:n]
		}
		voteCount += len(intent.promote)

		if n := desiredStateServerCount - voteCount; n < len(intent.convert) {
			intent.convert = intent.convert[:n]
		}
		voteCount += len(intent.convert)

		intent.newCount = desiredStateServerCount - voteCount

		logger.Infof("%d new machines; promoting %v; converting %v", intent.newCount, intent.promote, intent.convert)

		var ops []txn.Op
		ops, change, err = st.ensureAvailabilityIntentionOps(intent, currentInfo, cons, series)
		return ops, err
	}
	if err := st.run(buildTxn); err != nil {
		err = errors.Annotate(err, "failed to create new state server machines")
		return StateServersChanges{}, err
	}
	return change, nil
}

// Change in state servers after the ensure availability txn has committed.
type StateServersChanges struct {
	Added      []string
	Removed    []string
	Maintained []string
	Promoted   []string
	Demoted    []string
	Converted  []string
}

// ensureAvailabilityIntentionOps returns operations to fulfil the desired intent.
func (st *State) ensureAvailabilityIntentionOps(
	intent *ensureAvailabilityIntent,
	currentInfo *StateServerInfo,
	cons constraints.Value,
	series string,
) ([]txn.Op, StateServersChanges, error) {
	var ops []txn.Op
	var change StateServersChanges
	for _, m := range intent.promote {
		ops = append(ops, promoteStateServerOps(m)...)
		change.Promoted = append(change.Promoted, m.doc.Id)
	}
	for _, m := range intent.demote {
		ops = append(ops, demoteStateServerOps(m)...)
		change.Demoted = append(change.Demoted, m.doc.Id)
	}
	for _, m := range intent.convert {
		ops = append(ops, convertStateServerOps(m)...)
		change.Converted = append(change.Converted, m.doc.Id)
	}
	// Use any placement directives that have been provided
	// when adding new machines, until the directives have
	// been all used up. Set up a helper function to do the
	// work required.
	placementCount := 0
	getPlacement := func() string {
		if placementCount >= len(intent.placement) {
			return ""
		}
		result := intent.placement[placementCount]
		placementCount++
		return result
	}
	mdocs := make([]*machineDoc, intent.newCount)
	for i := range mdocs {
		template := MachineTemplate{
			Series: series,
			Jobs: []MachineJob{
				JobHostUnits,
				JobManageEnviron,
			},
			Constraints: cons,
			Placement:   getPlacement(),
		}
		mdoc, addOps, err := st.addMachineOps(template)
		if err != nil {
			return nil, StateServersChanges{}, err
		}
		mdocs[i] = mdoc
		ops = append(ops, addOps...)
		change.Added = append(change.Added, mdoc.Id)

	}
	for _, m := range intent.remove {
		ops = append(ops, removeStateServerOps(m)...)
		change.Removed = append(change.Removed, m.doc.Id)

	}

	for _, m := range intent.maintain {
		tag, err := names.ParseTag(m.Tag().String())
		if err != nil {
			return nil, StateServersChanges{}, errors.Annotate(err, "could not parse machine tag")
		}
		if tag.Kind() != names.MachineTagKind {
			return nil, StateServersChanges{}, errors.Errorf("expected machine tag kind, got %s", tag.Kind())
		}
		change.Maintained = append(change.Maintained, tag.Id())
	}
	ssOps, err := st.maintainStateServersOps(mdocs, currentInfo)
	if err != nil {
		return nil, StateServersChanges{}, errors.Annotate(err, "cannot prepare machine add operations")
	}
	ops = append(ops, ssOps...)
	return ops, change, nil
}

// stateServerAvailable returns true if the specified state server machine is
// available.
var stateServerAvailable = func(m *Machine) (bool, error) {
	// TODO(axw) #1271504 2014-01-22
	// Check the state server's associated mongo health;
	// requires coordination with worker/peergrouper.
	return m.AgentPresence()
}

type ensureAvailabilityIntent struct {
	newCount  int
	placement []string

	promote, maintain, demote, remove, convert []*Machine
}

// ensureAvailabilityIntentions returns what we would like
// to do to maintain the availability of the existing servers
// mentioned in the given info, including:
//   demoting unavailable, voting machines;
//   removing unavailable, non-voting, non-vote-holding machines;
//   gathering available, non-voting machines that may be promoted;
func (st *State) ensureAvailabilityIntentions(info *StateServerInfo, placement []string) (*ensureAvailabilityIntent, error) {
	var intent ensureAvailabilityIntent
	for _, s := range placement {
		// TODO(natefinch): unscoped placements shouldn't ever get here (though
		// they do currently).  We should fix up the CLI to always add a scope
		// to placements and then we can remove the need to deal with unscoped
		// placements.
		p, err := instance.ParsePlacement(s)
		if err == instance.ErrPlacementScopeMissing {
			intent.placement = append(intent.placement, s)
			continue
		}
		if err == nil && p.Scope == instance.MachineScope {
			// TODO(natefinch) add env provider policy to check if conversion is
			// possible (e.g. cannot be supported by Azure in HA mode).

			if names.IsContainerMachine(p.Directive) {
				return nil, errors.New("container placement directives not supported")
			}

			m, err := st.Machine(p.Directive)
			if err != nil {
				return nil, errors.Annotatef(err, "can't find machine for placement directive %q", s)
			}
			if m.IsManager() {
				return nil, errors.Errorf("machine for placement directive %q is already a state server", s)
			}
			intent.convert = append(intent.convert, m)
			intent.placement = append(intent.placement, s)
			continue
		}
		return nil, errors.Errorf("unsupported placement directive %q", s)
	}

	for _, mid := range info.MachineIds {
		m, err := st.Machine(mid)
		if err != nil {
			return nil, err
		}
		available, err := stateServerAvailable(m)
		if err != nil {
			return nil, err
		}
		logger.Infof("machine %q, available %v, wants vote %v, has vote %v", m, available, m.WantsVote(), m.HasVote())
		if available {
			if m.WantsVote() {
				intent.maintain = append(intent.maintain, m)
			} else {
				intent.promote = append(intent.promote, m)
			}
			continue
		}
		if m.WantsVote() {
			// The machine wants to vote, so we simply set novote and allow it
			// to run its course to have its vote removed by the worker that
			// maintains the replicaset. We will replace it with an existing
			// non-voting state server if there is one, starting a new one if
			// not.
			intent.demote = append(intent.demote, m)
		} else if m.HasVote() {
			// The machine still has a vote, so keep it around for now.
			intent.maintain = append(intent.maintain, m)
		} else {
			// The machine neither wants to nor has a vote, so remove its
			// JobManageEnviron job immediately.
			intent.remove = append(intent.remove, m)
		}
	}
	logger.Infof("initial intentions: promote %v; maintain %v; demote %v; remove %v; convert: %v",
		intent.promote, intent.maintain, intent.demote, intent.remove, intent.convert)
	return &intent, nil
}

func convertStateServerOps(m *Machine) []txn.Op {
	return []txn.Op{{
		C:  machinesC,
		Id: m.doc.DocID,
		Update: bson.D{
			{"$addToSet", bson.D{{"jobs", JobManageEnviron}}},
			{"$set", bson.D{{"novote", false}}},
		},
		Assert: bson.D{{"jobs", bson.D{{"$nin", []MachineJob{JobManageEnviron}}}}},
	}, {
		C:  stateServersC,
		Id: environGlobalKey,
		Update: bson.D{
			{"$addToSet", bson.D{{"votingmachineids", m.doc.Id}}},
			{"$addToSet", bson.D{{"machineids", m.doc.Id}}},
		},
	}}
}

func promoteStateServerOps(m *Machine) []txn.Op {
	return []txn.Op{{
		C:      machinesC,
		Id:     m.doc.DocID,
		Assert: bson.D{{"novote", true}},
		Update: bson.D{{"$set", bson.D{{"novote", false}}}},
	}, {
		C:      stateServersC,
		Id:     environGlobalKey,
		Update: bson.D{{"$addToSet", bson.D{{"votingmachineids", m.doc.Id}}}},
	}}
}

func demoteStateServerOps(m *Machine) []txn.Op {
	return []txn.Op{{
		C:      machinesC,
		Id:     m.doc.DocID,
		Assert: bson.D{{"novote", false}},
		Update: bson.D{{"$set", bson.D{{"novote", true}}}},
	}, {
		C:      stateServersC,
		Id:     environGlobalKey,
		Update: bson.D{{"$pull", bson.D{{"votingmachineids", m.doc.Id}}}},
	}}
}

func removeStateServerOps(m *Machine) []txn.Op {
	return []txn.Op{{
		C:      machinesC,
		Id:     m.doc.DocID,
		Assert: bson.D{{"novote", true}, {"hasvote", false}},
		Update: bson.D{
			{"$pull", bson.D{{"jobs", JobManageEnviron}}},
			{"$set", bson.D{{"novote", false}}},
		},
	}, {
		C:      stateServersC,
		Id:     environGlobalKey,
		Update: bson.D{{"$pull", bson.D{{"machineids", m.doc.Id}}}},
	}}
}
