// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller

import (
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/k8s"
)

// CAASBroker exposes CAAS broker functionality to a worker.
type CAASBroker interface {
	Application(string, k8s.WorkloadType) caas.Application
}

// PortMutator exposes CAAS application functionality to a worker.
type PortMutator interface {
	UpdatePorts(ports []caas.ServicePort, updateContainerPorts bool) error
}

// ServiceUpdater exposes CAAS application functionality to a worker.
type ServiceUpdater interface {
	UpdateService(caas.ServiceParam) error
}
