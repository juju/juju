// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package reboot

import (
	"github.com/juju/os/series"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/container"
	"github.com/juju/juju/container/factory"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/service"
	"github.com/juju/juju/service/common"
)

// rebootWaiterShim wraps the functions required by RebootWaiter
// to facilitate mock testing.
type rebootWaiterShim struct {
}

// HostSeries returns the series of the current host.
func (r rebootWaiterShim) HostSeries() (string, error) {
	return series.HostSeries()
}

// ListServices returns a list of names of services running
// on the current host.
func (r rebootWaiterShim) ListServices() ([]string, error) {
	return service.ListServices()
}

// NewService returns a new juju service object.
func (r rebootWaiterShim) NewService(name string, conf common.Conf, series string) (Service, error) {
	return service.NewService(name, conf, series)
}

// NewContainerManager return an object implementing Manager.
func (r rebootWaiterShim) NewContainerManager(containerType instance.ContainerType, conf container.ManagerConfig) (Manager, error) {
	return factory.NewContainerManager(containerType, conf)
}

// ScheduleAction schedules the reboot action based on the
// current operating system.
func (r rebootWaiterShim) ScheduleAction(action params.RebootAction, after int) error {
	return scheduleAction(action, after)
}

// agentConfigShim wraps the method required by a Model in
// the RebootWaiter.
type agentConfigShim struct {
	aCfg agent.Config
}

// Model return an object implementing Model.
func (a *agentConfigShim) Model() Model {
	return a.aCfg.Model()
}
