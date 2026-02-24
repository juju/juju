// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller

import (
	"github.com/juju/juju/caas"
)

// CAASBroker exposes CAAS broker functionality to a worker.
type CAASBroker interface {
	Application(string, caas.DeploymentType) caas.Application
}

// PortMutator describes the required interface for mutating the ports of an
// application.
type PortMutator interface {
	// UpdatePorts ensures that the applications currently available ports
	// matches the supplied values.
	UpdatePorts(ports []caas.ServicePort, updateContainerPorts bool) error
}
