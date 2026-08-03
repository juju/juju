// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package reboot

import (
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/internal/container"
	"github.com/juju/juju/rpc/params"
)

// RebootWaiter describes the functions required by Reboot.
// Added for use in mocked tests.
type RebootWaiter interface {
	ListServices() ([]string, error)
	NewServiceReference(string) (Service, error)
	NewContainerManager(instance.ContainerType, container.ManagerConfig) (Manager, error)
	ScheduleAction(action params.RebootAction, after int) error
}

// Service describes the method required for a Service in Reboot.
type Service interface {
	Stop() error
}

// Manager describes the method required for a ContainerManager
// in Reboot.
type Manager interface {
	IsInitialized() bool
	ListContainers() ([]instances.Instance, error)
}

// AgentConfig describes the method required for a AgentConfig
// in Reboot.
type AgentConfig interface {
	Model() Model
}

// Model describes the method required for a Model
// in Reboot.
type Model interface {
	Id() string
}
