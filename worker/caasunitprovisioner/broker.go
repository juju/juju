// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner

import "github.com/juju/juju/caas"

type ContainerBroker interface {
	EnsureUnit(appName, unitName, spec string) error
}

type ServiceBroker interface {
	EnsureService(appName, unitSpec string, numUnits int, config caas.ResourceConfig) error
	DeleteService(appName string) error
}

// TODO(caas) - move to a firewaller worker
type ServiceExposer interface {
	ExposeService(appName string, config caas.ResourceConfig) error
}
