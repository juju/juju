// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

import "github.com/juju/juju/environs"

// NewContainerBrokerFunc returns a Container Broker.
type NewContainerBrokerFunc func(environs.CloudSpec) (Broker, error)

// Broker instances interact with the CAAS substrate.
type Broker interface {
	// EnsureOperator creates or updates an operator pod for running
	// a charm for the specified application.
	EnsureOperator(appName, agentPath string, config *OperatorConfig) error

	// EnsureService creates or updates a service for pods with the given spec.
	EnsureService(appName, unitSpec string, numUnits int, config ResourceConfig) error

	// DeleteService deletes the specified service.
	DeleteService(appName string) error

	// ExposeService sets up external access to the specified service.
	ExposeService(appName string, config ResourceConfig) error

	// UnexposeService removes external access to the specified service.
	UnexposeService(appName string) error

	// EnsureUnit creates or updates a pod with the given spec.
	EnsureUnit(appName, unitName, spec string) error
}

// OperatorConfig is the config to use when creating an operator.
type OperatorConfig struct {
	// AgentConf is the contents of the agent.conf file.
	AgentConf []byte
}
