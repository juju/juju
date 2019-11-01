// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

// MachineJob values define responsibilities that machines may be
// expected to fulfil.
type MachineJob string

const (
	JobHostUnits   MachineJob = "JobHostUnits"
	JobManageModel MachineJob = "JobManageModel"
)

// NeedsState returns true if the job requires a state connection.
func (job MachineJob) NeedsState() bool {
	return job == JobManageModel
}

// AnyJobNeedsState returns true if any of the provided jobs
// require a state connection.
func AnyJobNeedsState(jobs ...MachineJob) bool {
	for _, j := range jobs {
		if j.NeedsState() {
			return true
		}
	}
	return false
}
