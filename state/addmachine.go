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

// addMachineParams is the internal form of AddMachineParams.
// It holds parameters that are potentially valid across every
// variant of AddMachine.
type addMachineParams struct {
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
}

// AddMachine adds a new machine configured to run the supplied jobs on
// the supplied series. The machine's constraints will be taken from the
// environment constraints.
func (st *State) AddMachine(series string, jobs ...MachineJob) (m *Machine, err error) {
	defer utils.ErrorContextf(&err, "cannot add a new machine")
	mdoc, ops, err := st.addMachineOps(addMachineParams{
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

	pparams := addMachineParams{
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
		mdoc, ops, err = st.addMachineOps(pparams)
	case params.ContainerType != "" && params.ParentId == "":
		what = "container"
		mdoc, _, ops, err = st.addMachineInsideNewMachineOps(pparams, pparams, params.ContainerType)
	case params.ContainerType != "" && params.ParentId != "":
		what = "container"
		mdoc, ops, err = st.addMachineInsideMachineOps(pparams, params.ParentId, params.ContainerType)
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
		addMachineParams{
			Series:      params.Series,
			Constraints: params.Constraints,
			Jobs:        params.Jobs,
			Clean:       true,
		},
		params.InstanceId,
		params.Nonce,
		params.HardwareCharacteristics,
	)
	if err != nil {
		return nil, err
	}
	mdoc.InstanceId = params.InstanceId
	mdoc.Nonce = params.Nonce
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

func (st *State) effectiveAddMachineParams(p addMachineParams) (addMachineParams, error) {
	if p.Series == "" {
		return addMachineParams{}, fmt.Errorf("no series specified")
	}
	cons, err := st.EnvironConstraints()
	if err != nil {
		return addMachineParams{}, err
	}
	p.Constraints = p.Constraints.WithFallbacks(cons)
	// Machine constraints do not use a container constraint value.
	// Both provisioning and deployment constraints use the same
	// constraints.Value struct so here we clear the container
	// value. Provisioning ignores the container value but clearing
	// it avoids potential confusion.
	p.Constraints.Container = nil

	if len(p.Jobs) == 0 {
		return addMachineParams{}, fmt.Errorf("no jobs specified")
	}
	jset := make(map[MachineJob]bool)
	for _, j := range p.Jobs {
		if jset[j] {
			return addMachineParams{}, fmt.Errorf("duplicate job: %s", j)
		}
		jset[j] = true
	}
	return p, nil
}

func (st *State) addMachineOps(params addMachineParams) (*machineDoc, []txn.Op, error) {
	params, err := st.effectiveAddMachineParams(params)
	if err != nil {
		return nil, nil, err
	}
	seq, err := st.sequence("machine")
	if err != nil {
		return nil, nil, err
	}
	mdoc := &machineDoc{
		Id:         strconv.Itoa(seq),
		Series:     params.Series,
		Jobs:       params.Jobs,
		Clean:      params.Clean,
		Principals: params.Principals,
		Life:       Alive,
	}
	var ops []txn.Op
	ops = append(ops, st.insertNewMachineOps(mdoc, params.Constraints)...)
	ops = append(ops, st.insertNewContainerRefOp(mdoc.Id))
	return mdoc, ops, nil
}

func (st *State) addMachineWithInstanceIdOps(params addMachineParams, instId instance.Id, nonce string, hwc instance.HardwareCharacteristics) (*machineDoc, []txn.Op, error) {
	params, err := st.effectiveAddMachineParams(params)
	if err != nil {
		return nil, nil, err
	}
	mdoc, ops, err := st.addMachineOps(params)
	if err != nil {
		return nil, nil, err
	}
	ops = append(ops, txn.Op{
		C:      st.instanceData.Name,
		Id:     mdoc.Id,
		Assert: txn.DocMissing,
		Insert: &instanceData{
			Id:         mdoc.Id,
			InstanceId: instId,
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

func (st *State) addMachineInsideMachineOps(params addMachineParams, parentId string, containerType instance.ContainerType) (*machineDoc, []txn.Op, error) {
	params, err := st.effectiveAddMachineParams(params)
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
	mdoc := &machineDoc{
		Id:            newId,
		Series:        params.Series,
		Jobs:          params.Jobs,
		Clean:         params.Clean,
		ContainerType: string(containerType),
		Principals:    params.Principals,
		Life:          Alive,
	}
	var ops []txn.Op
	ops = append(ops, st.insertNewMachineOps(mdoc, params.Constraints)...)
	ops = append(ops,
		// Update containers record for host machine.
		st.addChildToContainerRefOp(parentId, mdoc.Id),
		// Create a containers reference document for the container itself.
		st.insertNewContainerRefOp(mdoc.Id),
	)
	return mdoc, ops, nil
}

func (st *State) newContainerId(parentId string, containerType instance.ContainerType) (string, error) {
	seq, err := st.sequence(fmt.Sprintf("machine%s%sContainer", parentId, containerType))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%s/%d", parentId, containerType, seq), nil
}

func (st *State) addMachineInsideNewMachineOps(params, parentParams addMachineParams, containerType instance.ContainerType) (mdoc *machineDoc, parentDoc *machineDoc, ops []txn.Op, err error) {
	seq, err := st.sequence("machine")
	if err != nil {
		return nil, nil, nil, err
	}
	parentParams, err = st.effectiveAddMachineParams(parentParams)
	if err != nil {
		return nil, nil, nil, err
	}
	parentDoc = &machineDoc{
		Id:         strconv.Itoa(seq),
		Series:     parentParams.Series,
		Jobs:       parentParams.Jobs,
		Principals: params.Principals,
		Clean:      parentParams.Clean,
		Life:       Alive,
	}
	newId, err := st.newContainerId(parentDoc.Id, containerType)
	if err != nil {
		return nil, nil, nil, err
	}
	params, err = st.effectiveAddMachineParams(params)
	if err != nil {
		return nil, nil, nil, err
	}
	mdoc = &machineDoc{
		Id:            newId,
		Series:        params.Series,
		ContainerType: string(containerType),
		Jobs:          params.Jobs,
		Clean:         params.Clean,
		Life:          Alive,
	}
	ops = append(ops, st.insertNewMachineOps(parentDoc, parentParams.Constraints)...)
	ops = append(ops, st.insertNewMachineOps(mdoc, params.Constraints)...)
	ops = append(ops,
		// The host machine doesn't exist yet, create a new containers record.
		st.insertNewContainerRefOp(mdoc.Id),
		// Create a containers reference document for the container itself.
		st.insertNewContainerRefOp(parentDoc.Id, mdoc.Id),
	)
	return mdoc, parentDoc, ops, nil
}

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
