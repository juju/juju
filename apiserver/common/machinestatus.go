// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
)

// MachineStatusGetter defines the machine functionality
// required to status.
type MachineStatusGetter interface {
	Status() (status.StatusInfo, error)
	Id() string
	Life() state.Life
}

// MachineStatus returns the machine agent status for a given
// machine, with special handling for agent presence.
func (c *ModelPresenceContext) MachineStatus(machine MachineStatusGetter) (status.StatusInfo, error) {
	machineStatus, err := machine.Status()
	if err != nil {
		return status.StatusInfo{}, err
	}

	if !canMachineBeDown(machineStatus) {
		// The machine still being provisioned - there's no point in
		// enquiring about the agent liveness.
		return machineStatus, nil
	}

	agentAlive, err := c.machinePresence(machine)
	if err != nil {
		// We don't want any presence errors affecting status.
		logger.Debugf("error determining presence for machine %s: %v", machine.Id(), err)
		return machineStatus, nil
	}
	if machine.Life() != state.Dead && !agentAlive {
		machineStatus.Status = status.Down
		machineStatus.Message = "agent is not communicating with the server"
	}
	return machineStatus, nil
}

func canMachineBeDown(machineStatus status.StatusInfo) bool {
	switch machineStatus.Status {
	case status.Pending, status.Stopped:
		return false
	}
	return true
}
