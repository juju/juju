// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner

import (
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/watcher"
)

type ContainerBroker interface {
	Provider() caas.ContainerEnvironProvider
	WatchUnits(appName string) (watcher.NotifyWatcher, error)
	Units(appName string) ([]caas.Unit, error)
	WatchOperator(string) (watcher.NotifyWatcher, error)
	Operator(string) (*caas.Operator, error)
}

type ServiceBroker interface {
	Provider() caas.ContainerEnvironProvider
	EnsureService(appName string, statusCallback caas.StatusCallbackFunc, params *caas.ServiceParams, numUnits int, config application.ConfigAttributes) error
	EnsureCustomResourceDefinition(appName string, podSpec *caas.PodSpec) error
	GetService(appName string, includeClusterIP bool) (*caas.Service, error)
	DeleteService(appName string) error
	UnexposeService(appName string) error
	WatchService(appName string) (watcher.NotifyWatcher, error)
}
