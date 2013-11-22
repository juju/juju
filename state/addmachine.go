// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strconv"

	"labix.org/v2/mgo/txn"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/utils"
)

// AddMachineParams encapsulates the parameters used to create a new machine.
type AddMachineParams struct {
	// Series is the series to be associated with the new machine.
	Series string

	// Constraints are the constraints to be used when finding
	// an instance for the machine.
	Constraints constraints.Value

	// ParentId holds the machine id of the machine that
	// will contain the new machine. If this is set,
	// ContainerType must also be set.
	ParentId string

	// ContainerType holds the type of container into
	// which to deploy the new machine. This is
	// ignored if ParentId is empty.
	ContainerType instance.ContainerType

	// HardwareCharacteristics holds the h/w characteristics to
	// be associated with the machine.
	HardwareCharacteristics instance.HardwareCharacteristics

	// InstanceId holds the instance id to associate with the machine.
	// If this is empty, the provisioner will try to provision the machine.
	InstanceId instance.Id

	// Nonce holds a unique value that can be used to check
	// if a new instance was really started for this machine.
	// See Machine.SetProvisioned. This must be set if InstanceId is set.
	Nonce string

	// Jobs holds the jobs to run on the machine's instance.
	// A machine must have at least one job to do.
	Jobs []MachineJob
}

// machineTemplate holds attributes that are to be associated
// with a newly created machine.
type machineTemplate struct {
	// Series is the series to be associated with the new machine.
	Series string

	// Constraints are the constraints to be used when finding
	// an instance for the machine.
	Constraints constraints.Value

	// Jobs holds the jobs to run on the machine's instance.
	// A machine must have at least one job to do.
	Jobs []MachineJob

	// Clean signifies whether the new machine will be treated
	// as clean for unit-assignment purposes.
	Clean bool

	// Principals holds the principal units that will
	// associated with the machine.
	Principals []string

	// instanceId holds the instance id to be associated with the
	// new machine. It should only be set by InjectMachine.
	instanceId instance.Id

	// nonce holds the nonce for the new machine.
	// It should only be set by InjectMachine.
	nonce string
}

// AddMachine adds a new machine configured to run the supplied jobs on
// the supplied series. The machine's constraints will be taken from the
// environment constraints.
func (st *State) AddMachine(series string, jobs ...MachineJob) (m *Machine, err error) {
	defer utils.ErrorContextf(&err, "cannot add a new machine")
	mdoc, ops, err := st.addMachineOps(machineTemplate{
		Series: series,
		Jobs:   jobs,
		Clean:  true,
	})
	if err != nil {
		return nil, err
	}
	return st.addMachine(mdoc, ops)
}

// AddMachineWithConstraints adds a new machine configured to run the
// supplied jobs on the supplied series. The machine's constraints and
// other configuration will be taken from the supplied params struct.
func (st *State) AddMachineWithConstraints(params *AddMachineParams) (m *Machine, err error) {
	if params.InstanceId != "" {
		return nil, fmt.Errorf("cannot specify an instance id when adding a new machine")
	}
	if params.Nonce != "" {
		return nil, fmt.Errorf("cannot specify a nonce when adding a new machine")
	}

	// TODO(wallyworld) - if a container is required, and when the actual machine characteristics
	// are made available, we need to check the machine constraints to ensure the container can be
	// created on the specifed machine.
	// ie it makes no sense asking for a 16G container on a machine with 8G.

	template := machineTemplate{
		Series:      params.Series,
		Constraints: params.Constraints,
		Jobs:        params.Jobs,
		Clean:       true,
	}
	var what string
	var mdoc *machineDoc
	var ops []txn.Op
	switch {
	case params.ContainerType == "" && params.ParentId == "":
		what = "machine"
		mdoc, ops, err = st.addMachineOps(template)
	case params.ContainerType != "" && params.ParentId == "":
		what = "container"
		// We use the same template for both parent and child,
		// something that we might want to change at some point.
		mdoc, ops, err = st.addMachineInsideNewMachineOps(template, template, params.ContainerType)
	case params.ContainerType != "" && params.ParentId != "":
		what = "container"
		mdoc, ops, err = st.addMachineInsideMachineOps(template, params.ParentId, params.ContainerType)
	default:
		what = "container"
		err = fmt.Errorf("no container type specified")
	}
	if err != nil {
		return nil, fmt.Errorf("cannot add a new %s: %v", what, err)
	}
	return st.addMachine(mdoc, ops)
}

// InjectMachine adds a new machine, corresponding to an existing provider
// instance, configured according to the supplied params struct.
func (st *State) InjectMachine(params *AddMachineParams) (m *Machine, err error) {
	if params.InstanceId == "" {
		return nil, fmt.Errorf("cannot inject a machine without an instance id")
	}
	if params.Nonce == "" {
		return nil, fmt.Errorf("cannot inject a machine without a nonce")
	}
	defer utils.ErrorContextf(&err, "cannot add a new machine")
	mdoc, ops, err := st.addMachineWithInstanceIdOps(
		machineTemplate{
			Series:      params.Series,
			Constraints: params.Constraints,
			Jobs:        params.Jobs,
			Clean:       true,
			instanceId:  params.InstanceId,
			nonce:       params.Nonce,
		},
		params.HardwareCharacteristics,
	)
	if err != nil {
		return nil, err
	}
	return st.addMachine(mdoc, ops)
}

func (st *State) addMachine(mdoc *machineDoc, ops []txn.Op) (*Machine, error) {
	if err := st.runTransaction(ops); err != nil {
		return nil, err
	}
	m := newMachine(st, mdoc)
	// Refresh to pick the txn-revno.
	if err := m.Refresh(); err != nil {
		return nil, err
	}
	return m, nil
}

// effectiveMachineTemplate verifies that the given template is
// valid and combines it with values from the state
// to produce a resulting template that more accurately
// represents the data that will be inserted into the state.
func (st *State) effectiveMachineTemplate(p machineTemplate) (machineTemplate, error) {
	if p.Series == "" {
		return machineTemplate{}, fmt.Errorf("no series specified")
	}
	cons, err := st.EnvironConstraints()
	if err != nil {
		return machineTemplate{}, err
	}
	p.Constraints = p.Constraints.WithFallbacks(cons)
	// Machine constraints do not use a container constraint value.
	// Both provisioning and deployment constraints use the same
	// constraints.Value struct so here we clear the container
	// value. Provisioning ignores the container value but clearing
	// it avoids potential confusion.
	p.Constraints.Container = nil

	if len(p.Jobs) == 0 {
		return machineTemplate{}, fmt.Errorf("no jobs specified")
	}
	jset := make(map[MachineJob]bool)
	for _, j := range p.Jobs {
		if jset[j] {
			return machineTemplate{}, fmt.Errorf("duplicate job: %s", j)
		}
		jset[j] = true
	}
	return p, nil
}

// addMachineOps returns operations to add a new top level machine
// based on the given template. It also returns the machine document
// that will be inserted.
func (st *State) addMachineOps(template machineTemplate) (*machineDoc, []txn.Op, error) {
	template, err := st.effectiveMachineTemplate(template)
	if err != nil {
		return nil, nil, err
	}
	seq, err := st.sequence("machine")
	if err != nil {
		return nil, nil, err
	}
	mdoc := machineDocForTemplate(template, strconv.Itoa(seq))
	var ops []txn.Op
	ops = append(ops, st.insertNewMachineOps(mdoc, template.Constraints)...)
	ops = append(ops, st.insertNewContainerRefOp(mdoc.Id))
	return mdoc, ops, nil
}

// addMachineWithInstanceIdOps returns operations to add a new
// top level machine based on the given template and with the
// given hardward characteristics.
// The template must contain a valid instance id and nonce.
func (st *State) addMachineWithInstanceIdOps(template machineTemplate, hwc instance.HardwareCharacteristics) (*machineDoc, []txn.Op, error) {
	if template.instanceId == "" || template.nonce == "" {
		return nil, nil, fmt.Errorf("instance id and nonce not set up correctly in addMachineWithInstanceIdOps")
	}
	template, err := st.effectiveMachineTemplate(template)
	if err != nil {
		return nil, nil, err
	}
	mdoc, ops, err := st.addMachineOps(template)
	if err != nil {
		return nil, nil, err
	}
	ops = append(ops, txn.Op{
		C:      st.instanceData.Name,
		Id:     mdoc.Id,
		Assert: txn.DocMissing,
		Insert: &instanceData{
			Id:         mdoc.Id,
			InstanceId: template.instanceId,
			Arch:       hwc.Arch,
			Mem:        hwc.Mem,
			RootDisk:   hwc.RootDisk,
			CpuCores:   hwc.CpuCores,
			CpuPower:   hwc.CpuPower,
			Tags:       hwc.Tags,
		},
	})
	return mdoc, ops, nil
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
func (st *State) addMachineInsideMachineOps(template machineTemplate, parentId string, containerType instance.ContainerType) (*machineDoc, []txn.Op, error) {
	template, err := st.effectiveMachineTemplate(template)
	if err != nil {
		return nil, nil, err
	}

	// If a parent machine is specified, make sure it exists
	// and can support the requested container type.
	parent, err := st.Machine(parentId)
	if err != nil {
		return nil, nil, err
	}
	if !parent.supportsContainerType(containerType) {
		return nil, nil, fmt.Errorf("machine %s cannot host %s containers", parentId, containerType)
	}
	newId, err := st.newContainerId(parentId, containerType)
	if err != nil {
		return nil, nil, err
	}
	mdoc := machineDocForTemplate(template, newId)
	mdoc.ContainerType = string(containerType)
	var ops []txn.Op
	ops = append(ops, st.insertNewMachineOps(mdoc, template.Constraints)...)
	ops = append(ops,
		// Update containers record for host machine.
		st.addChildToContainerRefOp(parentId, mdoc.Id),
		// Create a containers reference document for the container itself.
		st.insertNewContainerRefOp(mdoc.Id),
	)
	return mdoc, ops, nil
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
func (st *State) addMachineInsideNewMachineOps(template, parentTemplate machineTemplate, containerType instance.ContainerType) (*machineDoc, []txn.Op, error) {
	seq, err := st.sequence("machine")
	if err != nil {
		return nil, nil, err
	}
	parentTemplate, err = st.effectiveMachineTemplate(parentTemplate)
	if err != nil {
		return nil, nil, err
	}
	parentDoc := machineDocForTemplate(parentTemplate, strconv.Itoa(seq))
	newId, err := st.newContainerId(parentDoc.Id, containerType)
	if err != nil {
		return nil, nil, err
	}
	template, err = st.effectiveMachineTemplate(template)
	if err != nil {
		return nil, nil, err
	}
	mdoc := machineDocForTemplate(template, newId)
	mdoc.ContainerType = string(containerType)
	var ops []txn.Op
	ops = append(ops, st.insertNewMachineOps(parentDoc, parentTemplate.Constraints)...)
	ops = append(ops, st.insertNewMachineOps(mdoc, template.Constraints)...)
	ops = append(ops,
		// The host machine doesn't exist yet, create a new containers record.
		st.insertNewContainerRefOp(mdoc.Id),
		// Create a containers reference document for the container itself.
		st.insertNewContainerRefOp(parentDoc.Id, mdoc.Id),
	)
	return mdoc, ops, nil
}

func machineDocForTemplate(template machineTemplate, id string) *machineDoc {
	return &machineDoc{
		Id:         id,
		Series:     template.Series,
		Jobs:       template.Jobs,
		Clean:      template.Clean,
		Principals: template.Principals,
		Life:       Alive,
		InstanceId: template.instanceId,
		Nonce:      template.nonce,
	}
}

// insertNewMachineOps returns operations to insert the given machine
// document and its associated constraints into the database.
func (st *State) insertNewMachineOps(mdoc *machineDoc, cons constraints.Value) []txn.Op {
	return []txn.Op{
		{
			C:      st.machines.Name,
			Id:     mdoc.Id,
			Assert: txn.DocMissing,
			Insert: mdoc,
		},
		createConstraintsOp(st, machineGlobalKey(mdoc.Id), cons),
		createStatusOp(st, machineGlobalKey(mdoc.Id), statusDoc{
			Status: params.StatusPending,
		}),
	}
}
