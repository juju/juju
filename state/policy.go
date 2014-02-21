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
	if errors.IsNotImplementedError(err) {
		return nil
	} else if err != nil {
		return err
	}
	if prechecker == nil {
		return fmt.Errorf("policy returned nil prechecker without an error")
	}
	return prechecker.PrecheckInstance(series, cons)
}
