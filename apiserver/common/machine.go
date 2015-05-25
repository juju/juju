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
	case multiwatcher.JobManageEnviron:
		return state.JobManageEnviron, nil
	case multiwatcher.JobManageNetworking:
		return state.JobManageNetworking, nil
	case multiwatcher.JobManageStateDeprecated:
		// Deprecated in 1.18.
		return state.JobManageStateDeprecated, nil
	default:
		return -1, errors.Errorf("invalid machine job %q", job)
	}
}
