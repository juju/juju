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
	Series                  string
	Constraints             constraints.Value
	ParentId                string
	ContainerType           instance.ContainerType
	InstanceId              instance.Id
	HardwareCharacteristics instance.HardwareCharacteristics
	Nonce                   string
	Jobs                    []MachineJob
}

// AddMachine adds a new machine configured to run the supplied jobs on the
// supplied series. The machine's constraints will be taken from the
// environment constraints.
func (st *State) AddMachine(series string, jobs ...MachineJob) (m *Machine, err error) {
	return st.addMachine(&AddMachineParams{Series: series, Jobs: jobs})
}

// AddMachineWithConstraints adds a new machine configured to run the supplied jobs on the
// supplied series. The machine's constraints and other configuration will be taken from
// the supplied params struct.
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

	return st.addMachine(params)
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
	return st.addMachine(params)
}

// addMachine implements AddMachine and InjectMachine.
func (st *State) addMachine(params *AddMachineParams) (m *Machine, err error) {
	msg := "cannot add a new machine"
	if params.ParentId != "" || params.ContainerType != "" {
		msg = "cannot add a new container"
	}
	defer utils.ErrorContextf(&err, msg)

	cons, err := st.EnvironConstraints()
	if err != nil {
		return nil, err
	}
	cons = params.Constraints.WithFallbacks(cons)

	ops, instData, containerParams, err := st.addMachineContainerOps(params, cons)
	if err != nil {
		return nil, err
	}
	mdoc := &machineDoc{
		Series:        params.Series,
		ContainerType: string(params.ContainerType),
		Jobs:          params.Jobs,
		Clean:         true,
	}
	if mdoc.ContainerType == "" {
		mdoc.InstanceId = params.InstanceId
		mdoc.Nonce = params.Nonce
	}
	machineOps, err := st.addMachineOps(mdoc, instData, cons, containerParams)
	if err != nil {
		return nil, err
	}
	ops = append(ops, machineOps...)

	err = st.runTransaction(ops)
	if err != nil {
		return nil, err
	}
	// Refresh to pick the txn-revno.
	m = newMachine(st, mdoc)
	if err = m.Refresh(); err != nil {
		return nil, err
	}
	return m, nil
}

// addMachineOps returns the operations necessary to create the given
// machine with its associated parameters. The metadata parameter may be
// nil.
//
// It sets fields in mdoc as appropriate, including mdoc.Id and
// mdoc.Life.
func (st *State) addMachineOps(mdoc *machineDoc, metadata *instanceData, cons constraints.Value, containerParams *containerRefParams) ([]txn.Op, error) {
	if mdoc.Series == "" {
		return nil, fmt.Errorf("no series specified")
	}
	if len(mdoc.Jobs) == 0 {
		return nil, fmt.Errorf("no jobs specified")
	}
	if containerParams.hostId != "" && mdoc.ContainerType == "" {
		return nil, fmt.Errorf("no container type specified")
	}
	jset := make(map[MachineJob]bool)
	for _, j := range mdoc.Jobs {
		if jset[j] {
			return nil, fmt.Errorf("duplicate job: %s", j)
		}
		jset[j] = true
	}
	if containerParams.hostId == "" {
		// we are creating a new machine instance (not a container).
		seq, err := st.sequence("machine")
		if err != nil {
			return nil, err
		}
		mdoc.Id = strconv.Itoa(seq)
		containerParams.hostId = mdoc.Id
		containerParams.newHost = true
	}
	if mdoc.ContainerType != "" {
		// we are creating a container so set up a namespaced id.
		seq, err := st.sequence(fmt.Sprintf("machine%s%sContainer", containerParams.hostId, mdoc.ContainerType))
		if err != nil {
			return nil, err
		}
		mdoc.Id = fmt.Sprintf("%s/%s/%d", containerParams.hostId, mdoc.ContainerType, seq)
		containerParams.containerId = mdoc.Id
	}
	mdoc.Life = Alive
	sdoc := statusDoc{
		Status: params.StatusPending,
	}
	// Machine constraints do not use a container constraint value.
	// Both provisioning and deployment constraints use the same constraints.Value struct
	// so here we clear the container value. Provisioning ignores the container value but
	// clearing it avoids potential confusion.
	cons.Container = nil
	ops := []txn.Op{
		{
			C:      st.machines.Name,
			Id:     mdoc.Id,
			Assert: txn.DocMissing,
			Insert: *mdoc,
		},
		createConstraintsOp(st, machineGlobalKey(mdoc.Id), cons),
		createStatusOp(st, machineGlobalKey(mdoc.Id), sdoc),
	}
	if metadata != nil {
		ops = append(ops, txn.Op{
			C:      st.instanceData.Name,
			Id:     mdoc.Id,
			Assert: txn.DocMissing,
			Insert: *metadata,
		})
	}
	ops = append(ops, createContainerRefOp(st, containerParams)...)
	return ops, nil
}

// addMachineContainerOps returns txn operations and associated Mongo records used to create a new machine,
// accounting for the fact that a machine may require a container and may require instance data.
// This method exists to cater for:
// 1. InjectMachine, which is used to record in state an instantiated bootstrap node. When adding
// a machine to state so that it is provisioned normally, the instance id is not known at this point.
// 2. AssignToNewMachine, which is used to create a new machine on which to deploy a unit.
func (st *State) addMachineContainerOps(params *AddMachineParams, cons constraints.Value) ([]txn.Op, *instanceData, *containerRefParams, error) {
	var instData *instanceData
	if params.InstanceId != "" {
		instData = &instanceData{
			InstanceId: params.InstanceId,
			Arch:       params.HardwareCharacteristics.Arch,
			Mem:        params.HardwareCharacteristics.Mem,
			RootDisk:   params.HardwareCharacteristics.RootDisk,
			CpuCores:   params.HardwareCharacteristics.CpuCores,
			CpuPower:   params.HardwareCharacteristics.CpuPower,
			Tags:       params.HardwareCharacteristics.Tags,
		}
	}
	var ops []txn.Op
	var containerParams = &containerRefParams{hostId: params.ParentId, hostOnly: true}
	// If we are creating a container, first create the host (parent) machine if necessary.
	if params.ContainerType != "" {
		containerParams.hostOnly = false
		if params.ParentId == "" {
			// No parent machine is specified so create one.
			mdoc := &machineDoc{
				Series: params.Series,
				Jobs:   params.Jobs,
				Clean:  true,
			}
			parentOps, err := st.addMachineOps(mdoc, instData, cons, &containerRefParams{})
			if err != nil {
				return nil, nil, nil, err
			}
			ops = parentOps
			containerParams.hostId = mdoc.Id
			containerParams.newHost = true
		} else {
			// If a parent machine is specified, make sure it exists.
			host, err := st.Machine(containerParams.hostId)
			if err != nil {
				return nil, nil, nil, err
			}
			// We will try and check if the specified parent machine can run a container of the specified type.
			// If the machine's supportedContainers attribute is set, this decision can be made right here.
			// If it is not yet known what containers a machine supports, we will assume that everything will
			// be ok and later on put the container into an error state if necessary.
			if supportedContainers, ok := host.SupportedContainers(); ok {
				supported := false
				for _, containerType := range supportedContainers {
					if containerType == params.ContainerType {
						supported = true
						break
					}
				}
				if !supported {
					return nil, nil, nil, fmt.Errorf("machine %s cannot host %s containers", host, params.ContainerType)
				}
			}
		}
	}
	return ops, instData, containerParams, nil
}
