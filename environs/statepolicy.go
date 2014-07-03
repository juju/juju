// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"github.com/juju/errors"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/policy"
	statePolicy "github.com/juju/juju/state/policy"
)

// environStatePolicy implements state.Policy in
// terms of environs.Environ and related types.
type environStatePolicy struct{}

var _ statePolicy.Policy = environStatePolicy{}

// NewStatePolicy returns a state.Policy that is
// implemented in terms of Environ and related
// types.
func NewStatePolicy() statePolicy.Policy {
	return environStatePolicy{}
}

func (environStatePolicy) Prechecker(cfg *config.Config) (policy.Prechecker, error) {
	// Environ implements state.Prechecker.
	return New(cfg)
}

func (environStatePolicy) ConfigValidator(providerType string) (policy.ConfigValidator, error) {
	// EnvironProvider implements state.ConfigValidator.
	return Provider(providerType)
}

func (environStatePolicy) EnvironCapability(cfg *config.Config) (policy.EnvironCapability, error) {
	// Environ implements state.EnvironCapability.
	return New(cfg)
}

func (environStatePolicy) ConstraintsValidator(cfg *config.Config) (constraints.Validator, error) {
	env, err := New(cfg)
	if err != nil {
		return nil, err
	}
	return env.ConstraintsValidator()
}

func (environStatePolicy) InstanceDistributor(cfg *config.Config) (policy.InstanceDistributor, error) {
	env, err := New(cfg)
	if err != nil {
		return nil, err
	}
	if p, ok := env.(policy.InstanceDistributor); ok {
		return p, nil
	}
	return nil, errors.NotImplementedf("InstanceDistributor")
}
