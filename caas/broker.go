// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

import (
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/network"
	"github.com/juju/juju/status"
	"github.com/juju/juju/watcher"
)

// NewContainerBrokerFunc returns a Container Broker.
type NewContainerBrokerFunc func(environs.CloudSpec) (Broker, error)

// Broker instances interact with the CAAS substrate.
type Broker interface {
	// EnsureOperator creates or updates an operator pod for running
	// a charm for the specified application.
	EnsureOperator(appName, agentPath string, config *OperatorConfig) error

	// DeleteOperator deletes the specified operator.
	DeleteOperator(appName string) error

	// EnsureService creates or updates a service for pods with the given spec.
	EnsureService(appName string, spec *ContainerSpec, numUnits int, config application.ConfigAttributes) error

	// Service returns the service for the specified application.
	Service(appName string) (*Service, error)

	// DeleteService deletes the specified service.
	DeleteService(appName string) error

	// ExposeService sets up external access to the specified service.
	ExposeService(appName string, config application.ConfigAttributes) error

	// UnexposeService removes external access to the specified service.
	UnexposeService(appName string) error

	// EnsureUnit creates or updates a pod with the given spec.
	EnsureUnit(appName, unitName string, spec *ContainerSpec) error

	// DeleteUnit deletes a unit pod with the given unit name.
	DeleteUnit(unitName string) error

	// WatchUnits returns a watcher which notifies when there
	// are changes to units of the specified application.
	WatchUnits(appName string) (watcher.NotifyWatcher, error)

	// Units returns all units of the specified application.
	Units(appName string) ([]Unit, error)
}

// Service represents information about the status of a caas service entity.
type Service struct {
	Id        string
	Addresses []network.Address
}

// Unit represents information about the status of a "pod".
type Unit struct {
	Id      string
	UnitTag string
	Address string
	Ports   []string
	Dying   bool
	Status  status.StatusInfo
}

// OperatorConfig is the config to use when creating an operator.
type OperatorConfig struct {
	// AgentConf is the contents of the agent.conf file.
	AgentConf []byte
}
