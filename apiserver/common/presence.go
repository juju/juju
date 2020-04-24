// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/core/presence"
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

func (c *ModelPresenceContext) machinePresence(machine MachineStatusGetter) (bool, error) {
	agent := names.NewMachineTag(machine.Id())
	status, err := c.Presence.AgentStatus(agent.String())
	return status == presence.Alive, err
}

func (c *ModelPresenceContext) unitPresence(unit UnitStatusGetter) (bool, error) {
	agent := names.NewUnitTag(unit.Name()).String()
	if !unit.ShouldBeAssigned() {
		// Units in CAAS models rely on the operator pings.
		// These are for the application itself.
		appName, err := names.UnitApplication(unit.Name())
		if err != nil {
			return false, errors.Trace(err)
		}
		agent = names.NewApplicationTag(appName).String()
	}
	status, err := c.Presence.AgentStatus(agent)
	return status == presence.Alive, err
}
