// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplication

import (
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
)

// ControllerState provides the subset of controller state
// required by the CAAS application facade.
type ControllerState interface {
	APIHostPortsForAgents(controller.Config) ([]network.SpaceHostPorts, error)
}
