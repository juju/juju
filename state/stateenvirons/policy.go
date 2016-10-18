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
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
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

// Prechecker implements state.Policy.
func (p environStatePolicy) Prechecker() (state.Prechecker, error) {
	// Environ implements state.Prechecker.
	return p.getEnviron(p.st)
}

// ConfigValidator implements state.Policy.
func (p environStatePolicy) ConfigValidator() (config.Validator, error) {
	return environProvider(p.st)
}

// ProviderConfigSchemaSource implements state.Policy.
func (p environStatePolicy) ProviderConfigSchemaSource() (config.ConfigSchemaSource, error) {
	provider, err := environProvider(p.st)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if cs, ok := provider.(config.ConfigSchemaSource); ok {
		return cs, nil
	}
	return nil, errors.NotImplementedf("config.ConfigSource")
}

// ConstraintsValidator implements state.Policy.
func (p environStatePolicy) ConstraintsValidator() (constraints.Validator, error) {
	env, err := p.getEnviron(p.st)
	if err != nil {
		return nil, err
	}
	return env.ConstraintsValidator()
}

// InstanceDistributor implements state.Policy.
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

// StorageProviderRegistry implements state.Policy.
func (p environStatePolicy) StorageProviderRegistry() (storage.ProviderRegistry, error) {
	env, err := p.getEnviron(p.st)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewStorageProviderRegistry(env), nil
}

// NewStorageProviderRegistry returns a storage.ProviderRegistry that chains
// the provided Environ with the common storage providers.
func NewStorageProviderRegistry(env environs.Environ) storage.ProviderRegistry {
	return storage.ChainedProviderRegistry{env, provider.CommonStorageProviders()}
}

func environProvider(st *state.State) (environs.EnvironProvider, error) {
	model, err := st.Model()
	if err != nil {
		return nil, errors.Annotate(err, "getting model")
	}
	cloud, err := st.Cloud(model.Cloud())
	if err != nil {
		return nil, errors.Annotate(err, "getting cloud")
	}
	// EnvironProvider implements state.ConfigValidator.
	return environs.Provider(cloud.Type)
}
