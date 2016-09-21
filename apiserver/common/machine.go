// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/status"
)

// StateJobs translates a slice of multiwatcher jobs to their equivalents in state.
func StateJobs(jobs []multiwatcher.MachineJob) ([]state.MachineJob, error) {
	newJobs := make([]state.MachineJob, len(jobs))
	for i, job := range jobs {
		newJob, err := machineJobFromParams(job)
		if err != nil {
			return nil, err
		}
		newJobs[i] = newJob
	}
	return newJobs, nil
}

// machineJobFromParams returns the job corresponding to multiwatcher.MachineJob.
func machineJobFromParams(job multiwatcher.MachineJob) (state.MachineJob, error) {
	switch job {
	case multiwatcher.JobHostUnits:
		return state.JobHostUnits, nil
	case multiwatcher.JobManageModel:
		return state.JobManageModel, nil
	default:
		return -1, errors.Errorf("invalid machine job %q", job)
	}
}

type origStateInterface interface {
	Machine(string) (*state.Machine, error)
}

type stateInterface interface {
	Machine(string) (Machine, error)
}

type stateShim struct {
	origStateInterface
}

func (st *stateShim) Machine(id string) (Machine, error) {
	return st.origStateInterface.Machine(id)
}

type Machine interface {
	Id() string
	InstanceId() (instance.Id, error)
	WantsVote() bool
	HasVote() bool
	Status() (status.StatusInfo, error)
	ContainerType() instance.ContainerType
	HardwareCharacteristics() (*instance.HardwareCharacteristics, error)
	Life() state.Life
	ForceDestroy() error
	Destroy() error
	AgentPresence() (bool, error)
}

func DestroyMachines(st origStateInterface, force bool, ids ...string) error {
	return destroyMachines(&stateShim{st}, force, ids...)
}

func destroyMachines(st stateInterface, force bool, ids ...string) error {
	var errs []string
	for _, id := range ids {
		machine, err := st.Machine(id)
		switch {
		case errors.IsNotFound(err):
			err = errors.Errorf("machine %s does not exist", id)
		case err != nil:
		case force:
			err = machine.ForceDestroy()
		case machine.Life() != state.Alive:
			continue
		default:
			err = machine.Destroy()
		}
		if err != nil {
			errs = append(errs, err.Error())
		}
	}
	return DestroyErr("machines", ids, errs)
}

// ModelMachineInfo returns information about machine hardware for
// alive top level machines (not containers).
func ModelMachineInfo(st ModelManagerBackend) (machineInfo []params.ModelMachineInfo, _ error) {
	machines, err := st.AllMachines()
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, m := range machines {
		if m.Life() != state.Alive {
			continue
		}
		var status string
		statusInfo, err := MachineStatus(m)
		if err == nil {
			status = string(statusInfo.Status)
		} else {
			status = err.Error()
		}
		mInfo := params.ModelMachineInfo{
			Id:        m.Id(),
			HasVote:   m.HasVote(),
			WantsVote: m.WantsVote(),
			Status:    status,
		}
		instId, err := m.InstanceId()
		switch {
		case err == nil:
			mInfo.InstanceId = string(instId)
		case errors.IsNotProvisioned(err):
			// ok, but no instance ID to get.
		default:
			return nil, errors.Trace(err)
		}
		if m.ContainerType() != "" && m.ContainerType() != instance.NONE {
			machineInfo = append(machineInfo, mInfo)
			continue
		}
		// Only include cores for physical machines.
		hw, err := m.HardwareCharacteristics()
		if err != nil && !errors.IsNotFound(err) {
			return nil, errors.Trace(err)
		}
		if hw != nil && hw.String() != "" {
			hwParams := &params.MachineHardware{
				Cores:            hw.CpuCores,
				Arch:             hw.Arch,
				Mem:              hw.Mem,
				RootDisk:         hw.RootDisk,
				CpuPower:         hw.CpuPower,
				Tags:             hw.Tags,
				AvailabilityZone: hw.AvailabilityZone,
			}
			mInfo.Hardware = hwParams
		}
		machineInfo = append(machineInfo, mInfo)
	}
	return machineInfo, nil
}
