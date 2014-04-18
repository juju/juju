// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/errors"
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
	// Prechecker takes a *config.Config and returns
	// a (possibly nil) Prechecker or an error.
	Prechecker(*config.Config) (Prechecker, error)

	// ConfigValidator takes a provider type name and returns
	// a (possibly nil) ConfigValidator or an error.
	ConfigValidator(providerType string) (ConfigValidator, error)

	// EnvironCapability takes a *config.Config and returns
	// a (possibly nil) EnvironCapability or an error.
	EnvironCapability(*config.Config) (EnvironCapability, error)
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
	PrecheckInstance(series string, cons constraints.Value) error
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
func (st *State) precheckInstance(series string, cons constraints.Value) error {
	if st.policy == nil {
		return nil
	}
	cfg, err := st.EnvironConfig()
	if err != nil {
		return err
	}
	prechecker, err := st.policy.Prechecker(cfg)
	if errors.IsNotImplemented(err) {
		return nil
	} else if err != nil {
		return err
	}
	if prechecker == nil {
		return fmt.Errorf("policy returned nil prechecker without an error")
	}
	return prechecker.PrecheckInstance(series, cons)
}

// validate calls the state's assigned policy, if non-nil, to obtain
// a ConfigValidator, and calls Validate if a non-nil ConfigValidator is
// returned.
func (st *State) validate(cfg, old *config.Config) (valid *config.Config, err error) {
	if st.policy == nil {
		return cfg, nil
	}
	configValidator, err := st.policy.ConfigValidator(cfg.Type())
	if errors.IsNotImplemented(err) {
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
func (st *State) supportsUnitPlacement() error {
	if st.policy == nil {
		return nil
	}
	cfg, err := st.EnvironConfig()
	if err != nil {
		return err
	}
	capability, err := st.policy.EnvironCapability(cfg)
	if errors.IsNotImplemented(err) {
		return nil
	} else if err != nil {
		return err
	}
	if capability == nil {
		return fmt.Errorf("policy returned nil EnvironCapability without an error")
	}
	return capability.SupportsUnitPlacement()
}
