// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"
	"fmt"

	"github.com/juju/names/v6"

	"github.com/juju/juju/core/presence"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/charm/hooks"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/state"
)

// ModelPresence represents the API server connections for a model.
type ModelPresence interface {
	// For a given non controller agent, return the Status for that agent.
	AgentStatus(agent string) (presence.Status, error)
}

// ModelPresenceContext represents the known agent presence state for the
// entire model.
type ModelPresenceContext struct {
	// Presence represents the API server connections for a model.
	Presence ModelPresence
}

func (c *ModelPresenceContext) unitPresence(unit UnitStatusGetter) (bool, error) {
	agent := names.NewUnitTag(unit.Name()).String()
	status, err := c.Presence.AgentStatus(agent)
	return status == presence.Alive, err
}

// MachineStatusGetter defines the machine functionality
// required to status.
type MachineStatusGetter interface {
	Status() (status.StatusInfo, error)
	Id() string
	Life() state.Life
}

// MachineStatus returns the machine agent status for a given
// machine, with special handling for agent presence.
func (c *ModelPresenceContext) MachineStatus(ctx context.Context, machine MachineStatusGetter) (status.StatusInfo, error) {
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
		logger.Debugf(context.TODO(), "error determining presence for machine %s: %v", machine.Id(), err)
		return machineStatus, nil
	}
	if machine.Life() != state.Dead && !agentAlive {
		machineStatus.Status = status.Down
		machineStatus.Message = "agent is not communicating with the server"
	}
	return machineStatus, nil
}

func (c *ModelPresenceContext) machinePresence(machine MachineStatusGetter) (bool, error) {
	agent := names.NewMachineTag(machine.Id())
	status, err := c.Presence.AgentStatus(agent.String())
	return status == presence.Alive, err
}

func canMachineBeDown(machineStatus status.StatusInfo) bool {
	switch machineStatus.Status {
	case status.Pending, status.Stopped:
		return false
	}
	return true
}

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
func (c *ModelPresenceContext) UnitStatus(ctx context.Context, unit UnitStatusGetter) (agent StatusAndErr, workload StatusAndErr) {
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
		// NOTE(nvinuesa): we must also keep the same workload status
		// and *not* add the "agent lost" message when the workload is
		// terminated. This happens on k8s sometimes when we remove an
		// application but the pod is not removed immediately. See:
		// https://bugs.launchpad.net/juju/+bug/1979292
		if workload.Status.Status != status.Error &&
			workload.Status.Status != status.Terminated {

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
		installMsg := operation.RunningHookMessage(
			string(hooks.Install),
			hook.Info{Kind: hooks.Install},
		)
		return agent.Message != installMsg
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
