// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries

import (
	"github.com/juju/juju/service"
	"github.com/juju/juju/service/common"
)

// AgentService is a service managed by the local init system, for running a
// unit agent.
type AgentService interface {
	Running() (bool, error)
	Start() error
	Stop() error
}

// ServiceAccess describes methods for interacting with the local init system.
type ServiceAccess interface {
	// ListServices returns a slice of service
	// names known by the local init system.
	ListServices() ([]string, error)

	// DiscoverService returns a service implementation
	// based on the input service name and config.
	DiscoverService(string) (AgentService, error)
}

// serviceAccess is the default implementation of ServiceAccess.
// It wraps methods from the service package.
type serviceAccess struct{}

var _ ServiceAccess = &serviceAccess{}

// ListServices lists all the running services on a machine.
func (s *serviceAccess) ListServices() ([]string, error) {
	return service.ListServices()
}

// DiscoverService returns the interface for a service running on a the machine.
func (s *serviceAccess) DiscoverService(name string) (AgentService, error) {
	return service.DiscoverService(name, common.Conf{})
}
