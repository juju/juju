// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

import "github.com/juju/juju/environs"

// NewContainerBrokerFunc returns a Container Broker.
type NewContainerBrokerFunc func(environs.CloudSpec) (Broker, error)

// NewOperatorConfigFunc functions return the agent config to use for
// a CAAS jujud operator.
type NewOperatorConfigFunc func(appName string) (*OperatorConfig, error)

// Broker instances interact with the CAAS substrate.
type Broker interface {
	// EnsureOperator creates an operator for running a charm for the specified application.
	// If the operator exists, this does nothing.
	EnsureOperator(appName, agentPath string, newConfig NewOperatorConfigFunc) error
}

// OperatorConfig is the config to use when creating an operator.
type OperatorConfig struct {
	// AgentConf is the contents of the agent.conf file.
	AgentConf []byte
}
