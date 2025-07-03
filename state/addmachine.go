// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"
	"strconv"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v6"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
)

// MachineTemplate holds attributes that are to be associated
// with a newly created machine.
type MachineTemplate struct {
	// Base is the base to be associated with the new machine.
	Base Base

	// Constraints are the constraints to be used when finding
	// an instance for the machine.
	Constraints constraints.Value

	// Addresses holds the addresses to be associated with the
	// new machine.
	//
	// TODO(dimitern): This should be removed once all addresses
	// come from link-layer device addresses.
	Addresses network.SpaceAddresses

	// InstanceId holds the instance id to associate with the machine.
	// If this is empty, the provisioner will try to provision the machine.
	// If this is non-empty, the HardwareCharacteristics and Nonce
	// fields must be set appropriately.
	InstanceId instance.Id

	// DisplayName holds the human readable name for the instance
	// associated with the machine.
	DisplayName string

	// HardwareCharacteristics holds the h/w characteristics to
	// be associated with the machine.
	HardwareCharacteristics instance.HardwareCharacteristics

	// LinkLayerDevices holds a list of arguments for setting link-layer devices
	// on the machine.
	LinkLayerDevices []LinkLayerDeviceArgs

	// Volumes holds the parameters for volumes that are to be created
	// and attached to the machine.
	Volumes []HostVolumeParams

	// VolumeAttachments holds the parameters for attaching existing
	// volumes to the machine.
	VolumeAttachments map[names.VolumeTag]VolumeAttachmentParams

	// Filesystems holds the parameters for filesystems that are to be
	// created and attached to the machine.
	Filesystems []HostFilesystemParams

	// FilesystemAttachments holds the parameters for attaching existing
	// filesystems to the machine.
	FilesystemAttachments map[names.FilesystemTag]FilesystemAttachmentParams

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

// HostVolumeParams holds the parameters for creating a volume and
// attaching it to a new host.
type HostVolumeParams struct {
	Volume     VolumeParams
	Attachment VolumeAttachmentParams
}

// HostFilesystemParams holds the parameters for creating a filesystem
// and attaching it to a new host.
type HostFilesystemParams struct {
	Filesystem FilesystemParams
	Attachment FilesystemAttachmentParams
}

// AddMachineInsideNewMachine creates a new machine within a container
// of the given type inside another new machine. The two given templates
// specify the form of the child and parent respectively.
func (st *State) AddMachineInsideNewMachine(
	template, parentTemplate MachineTemplate,
	containerType instance.ContainerType,
) (*Machine, error) {
	mdoc, ops, err := st.addMachineInsideNewMachineOps(template, parentTemplate, containerType)
	if err != nil {
		return nil, errors.Annotate(err, "cannot add a new machine")
	}
	return st.addMachine(mdoc, ops)
}

// AddMachineInsideMachine adds a machine inside a container of the
// given type on the existing machine with id=parentId.
func (st *State) AddMachineInsideMachine(
	template MachineTemplate,
	parentId string,
	containerType instance.ContainerType,
) (*Machine, error) {
	mdoc, ops, err := st.addMachineInsideMachineOps(template, parentId, containerType)
	if err != nil {
		return nil, errors.Annotate(err, "cannot add a new machine")
	}
	return st.addMachine(mdoc, ops)
}

// AddMachine adds a machine with the given series and jobs.
// It is deprecated and around for testing purposes only.
func (st *State) AddMachine(
	base Base,
) (*Machine, error) {
	ms, err := st.AddMachines(MachineTemplate{
		Base: base,
	})
	if err != nil {
		return nil, err
	}
	return ms[0], nil
}

// AddOneMachine machine adds a new machine configured according to the
// given template.
func (st *State) AddOneMachine(
	template MachineTemplate,
) (*Machine, error) {
	ms, err := st.AddMachines(template)
	if err != nil {
		return nil, err
	}
	return ms[0], nil
}

// AddMachines adds new machines configured according to the
// given templates.
func (st *State) AddMachines(
	templates ...MachineTemplate,
) (_ []*Machine, err error) {
	defer errors.DeferredAnnotatef(&err, "cannot add a new machine")
	var ms []*Machine
	var ops []txn.Op
	for _, template := range templates {
		mdoc, addOps, err := st.addMachineOps(template)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ms = append(ms, newMachine(st, mdoc))
		ops = append(ops, addOps...)
	}
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
	return cons, nil
}

// effectiveMachineTemplate verifies that the given template is
// valid and combines it with values from the state
// to produce a resulting template that more accurately
// represents the data that will be inserted into the state.
func (st *State) effectiveMachineTemplate(p MachineTemplate, allowController bool) (tmpl MachineTemplate, err error) {
	// First check for obvious errors.
	if p.Base.String() == "" {
		return tmpl, errors.New("no base specified")
	}

	// We ignore all constraints if there's a placement directive.
	if p.Placement == "" {
		p.Constraints, err = st.resolveMachineConstraints(p.Constraints)
		if err != nil {
			return tmpl, err
		}
	}

	return p, nil
}

// addMachineOps returns operations to add a new top level machine
// based on the given template. It also returns the machine document
// that will be inserted.
func (st *State) addMachineOps(
	template MachineTemplate,
) (*machineDoc, []txn.Op, error) {
	template, err := st.effectiveMachineTemplate(template, st.IsController())
	if err != nil {
		return nil, nil, err
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

	return mdoc, append(prereqOps, machineOp), nil
}

// addMachineInsideMachineOps returns operations to add a machine inside
// a container of the given type on an existing machine.
func (st *State) addMachineInsideMachineOps(
	template MachineTemplate,
	parentId string,
	containerType instance.ContainerType,
) (*machineDoc, []txn.Op, error) {
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
	name := fmt.Sprintf("machine%s%sContainer", parentId, containerType)
	seq, err := sequence(st, name)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%s/%d", parentId, containerType, seq), nil
}

// addMachineInsideNewMachineOps returns operations to create a new
// machine within a container of the given type inside another
// new machine. The two given templates specify the form
// of the child and parent respectively.
func (st *State) addMachineInsideNewMachineOps(
	template, parentTemplate MachineTemplate,
	containerType instance.ContainerType,
) (*machineDoc, []txn.Op, error) {
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

func (st *State) machineDocForTemplate(template MachineTemplate, id string) *machineDoc {
	// We ignore the error from Select*Address as an error indicates
	// no address is available, in which case the empty address is returned
	// and setting the preferred address to an empty one is the correct
	// thing to do when none is available.
	privateAddr, _ := template.Addresses.OneMatchingScope(network.ScopeMatchCloudLocal)
	publicAddr, _ := template.Addresses.OneMatchingScope(network.ScopeMatchPublic)
	logger.Infof(context.TODO(),
		"new machine %q has preferred addresses: private %q, public %q",
		id, privateAddr, publicAddr,
	)
	base := template.Base.Normalise()
	return &machineDoc{
		DocID:                   st.docID(id),
		Id:                      id,
		ModelUUID:               st.ModelUUID(),
		Base:                    base,
		Clean:                   !template.Dirty,
		Principals:              template.principals,
		Life:                    Alive,
		Addresses:               fromNetworkAddresses(template.Addresses, network.OriginMachine),
		PreferredPrivateAddress: fromNetworkAddress(privateAddr, network.OriginMachine),
		PreferredPublicAddress:  fromNetworkAddress(publicAddr, network.OriginMachine),
		Placement:               template.Placement,
	}
}

// insertNewMachineOps returns operations to insert the given machine document
// into the database, based on the given template. Only the constraints are
// taken from the template.
func (st *State) insertNewMachineOps(
	mdoc *machineDoc,
	template MachineTemplate,
) (prereqOps []txn.Op, machineOp txn.Op, err error) {
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
	modificationStatusDoc := statusDoc{
		Status:    status.Idle,
		ModelUUID: st.ModelUUID(),
		Updated:   now.UnixNano(),
	}

	prereqOps, machineOp = st.baseNewMachineOps(
		mdoc,
		machineStatusDoc,
		instanceStatusDoc,
		modificationStatusDoc,
		template.Constraints,
	)

	sb, err := NewStorageConfigBackend(st)
	if err != nil {
		return nil, txn.Op{}, errors.Trace(err)
	}
	storageOps, volumeAttachments, filesystemAttachments, err := sb.hostStorageOps(
		mdoc.Id, &storageParams{
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

	return prereqOps, machineOp, nil
}

func (st *State) baseNewMachineOps(mdoc *machineDoc,
	machineStatusDoc, instanceStatusDoc, modificationStatusDoc statusDoc,
	cons constraints.Value,
) (prereqOps []txn.Op, machineOp txn.Op) {
	machineOp = txn.Op{
		C:      machinesC,
		Id:     mdoc.DocID,
		Assert: txn.DocMissing,
		Insert: mdoc,
	}

	globalKey := machineGlobalKey(mdoc.Id)
	globalInstanceKey := machineGlobalInstanceKey(mdoc.Id)
	globalModificationKey := machineGlobalModificationKey(mdoc.Id)

	prereqOps = []txn.Op{
		createConstraintsOp(globalKey, cons),
		createStatusOp(st, globalKey, machineStatusDoc),
		createStatusOp(st, globalInstanceKey, instanceStatusDoc),
		createStatusOp(st, globalModificationKey, modificationStatusDoc),
		addModelMachineRefOp(st, mdoc.Id),
	}
	return prereqOps, machineOp
}
