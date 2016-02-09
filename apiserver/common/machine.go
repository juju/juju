// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
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
	case multiwatcher.JobManageNetworking:
		return state.JobManageNetworking, nil
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
	Life() state.Life
	ForceDestroy() error
	Destroy() error
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
