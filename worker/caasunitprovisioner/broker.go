// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner

import (
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/watcher"
)

type ContainerBroker interface {
	Provider() caas.ContainerEnvironProvider
	EnsureUnit(appName, unitName string, spec *caas.PodSpec) error
	DeleteUnit(unitName string) error
	WatchUnits(appName string) (watcher.NotifyWatcher, error)
	Units(appName string) ([]caas.Unit, error)
	DeleteService(appName string) error
	UnexposeService(appName string) error
}

type ServiceBroker interface {
	Provider() caas.ContainerEnvironProvider
	EnsureService(appName string, unitSpec *caas.PodSpec, numUnits int, config application.ConfigAttributes) error
	Service(appName string) (*caas.Service, error)
	DeleteService(appName string) error
}
