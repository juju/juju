// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package policy

import (
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/mongo"
)

// Info encapsulates information about cluster of
// servers holding juju state and can be used to make a
// connection to that cluster.
type Info struct {
	mongo.Info
	// Tag holds the name of the entity that is connecting.
	// It should be empty when connecting as an administrator.
	Tag string

	// Password holds the password for the connecting entity.
	Password string
}

// EnvironCapability implements access to metadata about the capabilities
// of an environment.
type EnvironCapability interface {
	// SupportedArchitectures returns the image architectures which can
	// be hosted by this environment.
	SupportedArchitectures() ([]string, error)

	// SupportNetworks returns whether the environment has support to
	// specify networks for services and machines.
	SupportNetworks() bool

	// SupportsUnitAssignment returns an error which, if non-nil, indicates
	// that the environment does not support unit placement. If the environment
	// does not support unit placement, then machines may not be created
	// without units, and units cannot be placed explcitly.
	SupportsUnitPlacement() error
}

// Prechecker is a policy interface that is provided to State
// to perform pre-flight checking of instance creation.
type Prechecker interface {
	// PrecheckInstance performs a preflight check on the specified
	// series and constraints, ensuring that they are possibly valid for
	// creating an instance in this environment.
	//
	// PrecheckInstance is best effort, and not guaranteed to eliminate
	// all invalid parameters. If PrecheckInstance returns nil, it is not
	// guaranteed that the constraints are valid; if a non-nil error is
	// returned, then the constraints are definitely invalid.
	PrecheckInstance(series string, cons constraints.Value, placement string) error
}

// ConfigValidator is a policy interface that is provided to State
// to check validity of new configuration attributes before applying them to state.
type ConfigValidator interface {
	Validate(cfg, old *config.Config) (valid *config.Config, err error)
}

// InstanceDistributor is a policy interface that is provided
// to State to perform distribution of units across instances
// for high availability.
type InstanceDistributor interface {
	// DistributeInstance takes a set of clean, empty
	// instances, and a distribution group, and returns
	// the subset of candidates which the policy will
	// allow entry into the distribution group.
	//
	// The AssignClean and AssignCleanEmpty unit
	// assignment policies will attempt to assign a
	// unit to each of the resulting instances until
	// one is successful. If no instances can be assigned
	// to (e.g. because of concurrent deployments), then
	// a new machine will be allocated.
	DistributeInstances(candidates, distributionGroup []instance.Id) ([]instance.Id, error)
}
