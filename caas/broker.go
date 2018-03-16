// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

import (
	"github.com/juju/errors"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/network"
	"github.com/juju/juju/status"
	"github.com/juju/juju/watcher"
)

// ContainerEnvironProvider represents a computing and storage provider
// for a container runtime.
type ContainerEnvironProvider interface {
	environs.EnvironProvider

	// Open opens the broker and returns it. The configuration must
	// have passed through PrepareConfig at some point in its lifecycle.
	//
	// Open should not perform any expensive operations, such as querying
	// the cloud API, as it will be called frequently.
	Open(args environs.OpenParams) (Broker, error)

	// ParsePodSpec unmarshalls the given YAML pod spec.
	ParsePodSpec(in string) (*PodSpec, error)
}

// New returns a new broker based on the provided configuration.
func New(args environs.OpenParams) (Broker, error) {
	p, err := environs.Provider(args.Cloud.Type)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return Open(p, args)
}

// Open creates a Broker instance and errors if the provider is not for
// a container substrate.
func Open(p environs.EnvironProvider, args environs.OpenParams) (Broker, error) {
	if envProvider, ok := p.(ContainerEnvironProvider); !ok {
		return nil, errors.NotValidf("container environ provider %T", p)
	} else {
		return envProvider.Open(args)
	}
}

// NewContainerBrokerFunc returns a Container Broker.
type NewContainerBrokerFunc func(args environs.OpenParams) (Broker, error)

// Broker instances interact with the CAAS substrate.
type Broker interface {
	// Provider returns the ContainerEnvironProvider that created this Broker.
	Provider() ContainerEnvironProvider

	// EnsureNamespace ensures this broker's namespace is created.
	EnsureNamespace() error

	// DeleteNamespace deletes this broker's namespace.
	DeleteNamespace() error

	// EnsureOperator creates or updates an operator pod for running
	// a charm for the specified application.
	EnsureOperator(appName, agentPath string, config *OperatorConfig) error

	// DeleteOperator deletes the specified operator.
	DeleteOperator(appName string) error

	// EnsureService creates or updates a service for pods with the given spec.
	EnsureService(appName string, spec *PodSpec, numUnits int, config application.ConfigAttributes) error

	// Service returns the service for the specified application.
	Service(appName string) (*Service, error)

	// DeleteService deletes the specified service.
	DeleteService(appName string) error

	// ExposeService sets up external access to the specified service.
	ExposeService(appName string, config application.ConfigAttributes) error

	// UnexposeService removes external access to the specified service.
	UnexposeService(appName string) error

	// EnsureUnit creates or updates a pod with the given spec.
	EnsureUnit(appName, unitName string, spec *PodSpec) error

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
