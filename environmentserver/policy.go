// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environmentserver

import (
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
)

// Policy is an interface provided to State that may
// be consulted by State to validate or modify the
// behaviour of certain operations.
//
// If a Policy implementation does not implement one
// of the methods, it must return an error that
// satisfies errors.IsNotImplemented, and will thus
// be ignored. Any other error will cause an error
// in the use of the policy.
type Policy interface {
	// Prechecker takes a *config.Config and returns a Prechecker or an error.
	Prechecker(*config.Config) (Prechecker, error)

	// ConfigValidator takes a provider type name and returns a ConfigValidator
	// or an error.
	ConfigValidator(providerType string) (ConfigValidator, error)

	// InstanceDistributor takes a *config.Config and returns an
	// InstanceDistributor or an error.
	InstanceDistributor(*config.Config) (InstanceDistributor, error)
}

type Capability interface {
	// EnvironCapability takes a *config.Config and returns an EnvironCapability
	// or an error.
	EnvironCapability(*config.Config) (EnvironCapability, error)
}

type Constraints interface {
	// ConstraintsValidator takes a *config.Config and returns a
	// constraints.Validator or an error.
	ConstraintsValidator() (constraints.Validator, error)
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

// precheckInstance calls the state's assigned policy, if non-nil, to obtain
// a Prechecker, and calls PrecheckInstance if a non-nil Prechecker is returned.
func (d *deployer) PrecheckInstance(series string, cons constraints.Value, placement string) error {
	cfg, err := d.state.EnvironConfig()
	if err != nil {
		return err
	}

	env, err := environs.New(cfg)
	if err != nil {
		return err
	}

	prechecker := Prechecker(env)

	if errors.IsNotImplemented(err) {
		return nil
	} else if err != nil {
		return err
	}

	if prechecker == nil {
		return fmt.Errorf("policy returned nil prechecker without an error")
	}

	return prechecker.PrecheckInstance(series, cons, placement)
}

func (d *deployer) ConstraintsValidator() (constraints.Validator, error) {
	// Default behaviour is to simply use a standard validator with
	// no environment specific behaviour built in.
	defaultValidator := constraints.NewValidator()

	cfg, err := d.state.EnvironConfig()
	if err != nil {
		return nil, err
	}
	env, err := environs.New(cfg)
	if err != nil {
		return nil, err
	}

	validator, ok := env.(Constraints)
	if !ok {
		return defaultValidator, nil
	}

	if validator == nil {
		return nil, fmt.Errorf("policy returned nil constraints validator without an error")
	}
	return validator.ConstraintsValidator()
}

// resolveConstraints combines the given constraints with the environ constraints to get
// a constraints which will be used to create a new instance.
func (d *deployer) ResolveConstraints(cons constraints.Value) (constraints.Value, error) {
	validator, err := d.ConstraintsValidator()
	if err != nil {
		return constraints.Value{}, err
	}
	envCons, err := d.state.EnvironConstraints()
	if err != nil {
		return constraints.Value{}, err
	}
	return validator.Merge(envCons, cons)
}

// validateConstraints returns an error if the given constraints are not valid for the
// current environment, and also any unsupported attributes.
func (d *deployer) ValidateConstraints(cons constraints.Value) ([]string, error) {
	validator, err := d.ConstraintsValidator()
	if err != nil {
		return nil, err
	}
	return validator.Validate(cons)
}

// validate calls the state's assigned policy, if non-nil, to obtain
// a ConfigValidator, and calls Validate if a non-nil ConfigValidator is
// returned.
func (d *deployer) Validate(cfg, old *config.Config) (valid *config.Config, err error) {
	currentConfig, err := d.state.EnvironConfig()
	if err != nil {
		return nil, err
	}

	env, err := environs.New(currentConfig)
	if err != nil {
		return nil, err
	}

	configValidator, ok := env.(ConfigValidator)
	if !ok {
		return cfg, nil
	} else if err != nil {
		return nil, err
	}

	if configValidator == nil {
		return nil, fmt.Errorf("policy returned nil configValidator without an error")
	}
	return configValidator.Validate(cfg, old)
}

// supportsUnitPlacement calls the state's assigned policy, if non-nil,
// to obtain an EnvironCapability, and calls SupportsUnitPlacement if a
// non-nil EnvironCapability is returned.
func (d *deployer) SupportsUnitPlacement() error {
	cfg, err := d.state.EnvironConfig()
	if err != nil {
		return err
	}

	env, err := environs.New(cfg)
	if err != nil {
		return err
	}

	capability, ok := env.(EnvironCapability)
	if !ok {
		return nil
	}
	if capability == nil {
		return fmt.Errorf("policy returned nil EnvironCapability without an error")
	}
	return capability.SupportsUnitPlacement()
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
