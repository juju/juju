// Copyright 2014, 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateenvirons

import (
	"github.com/juju/errors"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/state"
)

// environStatePolicy implements state.Policy in
// terms of environs.Environ and related types.
type environStatePolicy struct {
	st         *state.State
	getEnviron func(*state.State) (environs.Environ, error)
}

// GetNewPolicyFunc returns a state.NewPolicyFunc that will return
// a state.Policy implemented in terms of environs.Environ and
// related types. The provided function will be used to construct
// environs.Environs given a state.State.
func GetNewPolicyFunc(getEnviron func(*state.State) (environs.Environ, error)) state.NewPolicyFunc {
	return func(st *state.State) state.Policy {
		return environStatePolicy{st, getEnviron}
	}
}

// Prechecker implements state.Prechecker.
func (p environStatePolicy) Prechecker() (state.Prechecker, error) {
	// Environ implements state.Prechecker.
	return p.getEnviron(p.st)
}

// ConfigValidator implements state.Prechecker.
func (p environStatePolicy) ConfigValidator() (config.Validator, error) {
	model, err := p.st.Model()
	if err != nil {
		return nil, errors.Annotate(err, "getting model")
	}
	cloud, err := p.st.Cloud(model.Cloud())
	if err != nil {
		return nil, errors.Annotate(err, "getting cloud")
	}
	// EnvironProvider implements state.ConfigValidator.
	return environs.Provider(cloud.Type)
}

// ConstraintsValidator implements state.Prechecker.
func (p environStatePolicy) ConstraintsValidator() (constraints.Validator, error) {
	env, err := p.getEnviron(p.st)
	if err != nil {
		return nil, err
	}
	return env.ConstraintsValidator()
}

// InstanceDistributor implements state.Prechecker.
func (p environStatePolicy) InstanceDistributor() (instance.Distributor, error) {
	env, err := p.getEnviron(p.st)
	if err != nil {
		return nil, err
	}
	if p, ok := env.(instance.Distributor); ok {
		return p, nil
	}
	return nil, errors.NotImplementedf("InstanceDistributor")
}
