// Copyright 2014, 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateenvirons

import (
	"github.com/juju/errors"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
)

// environStatePolicy implements state.Policy in
// terms of environs.Environ and related types.
type environStatePolicy struct {
	st         *state.State
	getEnviron NewEnvironFunc
	getBroker  NewCAASBrokerFunc
}

// GetNewPolicyFunc returns a state.NewPolicyFunc that will return
// a state.Policy implemented in terms of either environs.Environ
// or caas.Broker and related types.
func GetNewPolicyFunc() state.NewPolicyFunc {
	return func(st *state.State) state.Policy {
		return environStatePolicy{st, GetNewEnvironFunc(environs.New), GetNewCAASBrokerFunc(caas.New)}
	}
}

// Prechecker implements state.Policy.
func (p environStatePolicy) Prechecker() (environs.InstancePrechecker, error) {
	model, err := p.st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if model.Type() == state.ModelTypeIAAS {
		return p.getEnviron(model)
	}
	return p.getBroker(model)
}

// ConfigValidator implements state.Policy.
func (p environStatePolicy) ConfigValidator() (config.Validator, error) {
	model, err := p.st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	cloud, err := p.st.Cloud(model.CloudName())
	if err != nil {
		return nil, errors.Annotate(err, "getting cloud")
	}
	return environProvider(cloud.Type)
}

// ProviderConfigSchemaSource implements state.Policy.
func (p environStatePolicy) ProviderConfigSchemaSource(cloudName string) (config.ConfigSchemaSource, error) {
	cloud, err := p.st.Cloud(cloudName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	provider, err := environProvider(cloud.Type)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if cs, ok := provider.(config.ConfigSchemaSource); ok {
		return cs, nil
	}
	return nil, errors.NotImplementedf("config.ConfigSource")
}

// ConstraintsValidator implements state.Policy.
func (p environStatePolicy) ConstraintsValidator(ctx context.ProviderCallContext) (constraints.Validator, error) {
	model, err := p.st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	if model.Type() == state.ModelTypeIAAS {
		env, err := p.getEnviron(model)
		if err != nil {
			return nil, err
		}
		return env.ConstraintsValidator(ctx)
	}
	broker, err := p.getBroker(model)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return broker.ConstraintsValidator(ctx)
}

// InstanceDistributor implements state.Policy.
func (p environStatePolicy) InstanceDistributor() (context.Distributor, error) {
	model, err := p.st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if model.Type() != state.ModelTypeIAAS {
		// Only IAAS models support machines, hence distribution.
		return nil, errors.NotImplementedf("InstanceDistributor")
	}
	env, err := p.getEnviron(model)
	if err != nil {
		return nil, err
	}
	if p, ok := env.(context.Distributor); ok {
		return p, nil
	}
	return nil, errors.NotImplementedf("InstanceDistributor")
}

// StorageProviderRegistry implements state.Policy.
func (p environStatePolicy) StorageProviderRegistry() (storage.ProviderRegistry, error) {
	model, err := p.st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewStorageProviderRegistryForModel(model, p.getEnviron, p.getBroker)
}

// NewStorageProviderRegistryForModel returns a storage provider registry
// for the specified model.
func NewStorageProviderRegistryForModel(
	model *state.Model,
	newEnv NewEnvironFunc,
	newBroker NewCAASBrokerFunc,
) (_ storage.ProviderRegistry, err error) {
	var reg storage.ProviderRegistry
	if model.Type() == state.ModelTypeIAAS {
		if reg, err = newEnv(model); err != nil {
			return nil, errors.Trace(err)
		}
	} else {
		if reg, err = newBroker(model); err != nil {
			return nil, errors.Trace(err)
		}
	}
	return NewStorageProviderRegistry(reg), nil
}

// NewStorageProviderRegistry returns a storage.ProviderRegistry that chains
// the provided registry with the common storage providers.
func NewStorageProviderRegistry(reg storage.ProviderRegistry) storage.ProviderRegistry {
	return storage.ChainedProviderRegistry{reg, provider.CommonStorageProviders()}
}

func environProvider(cloudType string) (environs.EnvironProvider, error) {
	return environs.Provider(cloudType)
}
