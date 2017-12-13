// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner

import "github.com/juju/juju/core/application"

type ContainerBroker interface {
	EnsureUnit(appName, unitName, spec string) error
}

type ServiceBroker interface {
	EnsureService(appName, unitSpec string, numUnits int, config application.ConfigAttributes) error
	DeleteService(appName string) error
}
