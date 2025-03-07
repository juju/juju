// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner

import (
	"github.com/juju/names/v5"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/config"
	"github.com/juju/juju/core/watcher"
)

type ContainerBroker interface {
	Provider() caas.ContainerEnvironProvider
	WatchOperator(string) (watcher.NotifyWatcher, error)
	Operator(string) (*caas.Operator, error)

	WatchUnits(appName string, mode caas.DeploymentMode) (watcher.NotifyWatcher, error)
	Units(appName string, mode caas.DeploymentMode) ([]caas.Unit, error)
	AnnotateUnit(appName string, mode caas.DeploymentMode, podName string, unit names.UnitTag) error
}

type ServiceBroker interface {
	Provider() caas.ContainerEnvironProvider
	EnsureService(appName string, statusCallback caas.StatusCallbackFunc, params *caas.ServiceParams, numUnits int, config config.ConfigAttributes) error
	DeleteService(appName string) error
	UnexposeService(appName string) error

	GetService(appName string, mode caas.DeploymentMode, includeClusterIP bool) (*caas.Service, error)
	WatchService(appName string, mode caas.DeploymentMode) (watcher.NotifyWatcher, error)
}
