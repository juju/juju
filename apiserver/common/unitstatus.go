// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"

	"github.com/juju/charm/v7/hooks"

	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker/uniter/operation"
)

// StatusAndErr pairs a StatusInfo with an error associated with
// retrieving it.
type StatusAndErr struct {
	Status status.StatusInfo
	Err    error
}

// UnitStatusGetter defines the unit functionality required to
// determine unit agent and workload status.
type UnitStatusGetter interface {
	AgentStatus() (status.StatusInfo, error)
	Status() (status.StatusInfo, error)
	ShouldBeAssigned() bool
	Name() string
	Life() state.Life
}

// UnitStatus returns the unit agent and workload status for a given
// unit, with special handling for agent presence.
func (c *ModelPresenceContext) UnitStatus(unit UnitStatusGetter) (agent StatusAndErr, workload StatusAndErr) {
	agent.Status, agent.Err = unit.AgentStatus()
	workload.Status, workload.Err = unit.Status()
	if !canBeLost(agent.Status, workload.Status) {
		// The unit is allocating or installing - there's no point in
		// enquiring about the agent liveness.
		return
	}

	agentAlive, err := c.unitPresence(unit)
	if err != nil {
		return
	}
	if unit.Life() != state.Dead && !agentAlive {
		// If the unit is in error, it would be bad to throw away
		// the error information as when the agent reconnects, that
		// error information would then be lost.
		if workload.Status.Status != status.Error {
			workload.Status.Status = status.Unknown
			workload.Status.Message = fmt.Sprintf("agent lost, see 'juju show-status-log %s'", unit.Name())
		}
		agent.Status.Status = status.Lost
		agent.Status.Message = "agent is not communicating with the server"
	}
	return
}

func canBeLost(agent, workload status.StatusInfo) bool {
	switch agent.Status {
	case status.Allocating, status.Running:
		return false
	case status.Executing:
		return agent.Message != operation.RunningHookMessage(string(hooks.Install))
	}

	// TODO(fwereade/wallyworld): we should have an explicit place in the model
	// to tell us when we've hit this point, instead of piggybacking on top of
	// status and/or status history.

	return isWorkloadInstalled(workload)
}

func isWorkloadInstalled(workload status.StatusInfo) bool {
	switch workload.Status {
	case status.Maintenance:
		return workload.Message != status.MessageInstallingCharm
	case status.Waiting:
		switch workload.Message {
		case status.MessageWaitForMachine:
		case status.MessageInstallingAgent:
		case status.MessageInitializingAgent:
			return false
		}
	}
	return true
}
