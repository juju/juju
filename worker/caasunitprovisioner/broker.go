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
	WatchUnits(appName string) (watcher.NotifyWatcher, error)
	Units(appName string) ([]caas.Unit, error)
	DeleteService(appName string) error
	UnexposeService(appName string) error
}

type ServiceBroker interface {
	Provider() caas.ContainerEnvironProvider
	EnsureService(appName string, params *caas.ServiceParams, numUnits int, config application.ConfigAttributes) error
	EnsureCrd(appName string, podSpec *caas.PodSpec) error
	Service(appName string) (*caas.Service, error)
	DeleteService(appName string) error
}
