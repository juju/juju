// Copyright 2014, 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateenvirons

import (
	"context"
	stdcontext "context"
	"sync"

	"github.com/juju/errors"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
	"github.com/juju/juju/state"
)

// environStatePolicy implements state.Policy in
// terms of environs.Environ and related types.
type environStatePolicy struct {
	st                *state.State
	cloudService      CloudService
	credentialService CredentialService
	getEnviron        NewEnvironFunc
	getBroker         NewCAASBrokerFunc
	checkerMu         sync.Mutex
	checker           deployChecker
}

// deployChecker is the subset of the Environ interface (common to Environ and
// Broker) that we need for pre-checking instances and validating constraints.
type deployChecker interface {
	environs.InstancePrechecker
	environs.ConstraintsChecker
}

// NewInstancePrechecker returns a new instance prechecker that uses the
// specified cloudService and credentialService to create the underlying
// deployChecker.
func NewInstancePrechecker(st *state.State, cloudService CloudService, credentialService CredentialService) (environs.InstancePrechecker, error) {
	policy := &environStatePolicy{
		st:                st,
		cloudService:      cloudService,
		credentialService: credentialService,
		getEnviron:        GetNewEnvironFunc(environs.New),
		getBroker:         GetNewCAASBrokerFunc(caas.New),
	}
	return policy.Prechecker()
}

// ProviderConfigSchemaSource returns a function that can be used to
// get a config.ConfigSchemaSource for the specified cloud.
func ProviderConfigSchemaSource(cloudService CloudService) config.ConfigSchemaSourceGetter {
	return func(ctx context.Context, cloudName string) (config.ConfigSchemaSource, error) {
		cloud, err := cloudService.Get(ctx, cloudName)
		if err != nil {
			return nil, errors.Trace(err)
		}
		provider, err := environs.Provider(cloud.Type)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if cs, ok := provider.(config.ConfigSchemaSource); ok {
			return cs, nil
		}
		return nil, errors.NotImplementedf("config.ConfigSource")
	}
}

// GetNewPolicyFunc returns a state.NewPolicyFunc that will return
// a state.Policy implemented in terms of either environs.Environ
// or caas.Broker and related types.
func GetNewPolicyFunc(cloudService CloudService, credentialService CredentialService) state.NewPolicyFunc {
	return func(st *state.State) state.Policy {
		return &environStatePolicy{
			st:                st,
			cloudService:      cloudService,
			credentialService: credentialService,
			getEnviron:        GetNewEnvironFunc(environs.New),
			getBroker:         GetNewCAASBrokerFunc(caas.New),
		}
	}
}

// getDeployChecker returns the cached deployChecker instance, or creates a
// new one if it hasn't yet been created and cached.
func (p *environStatePolicy) getDeployChecker() (deployChecker, error) {
	p.checkerMu.Lock()
	defer p.checkerMu.Unlock()

	if p.credentialService == nil {
		return nil, errors.NotSupportedf("deploy check without credential service")
	}
	if p.checker != nil {
		return p.checker, nil
	}

	model, err := p.st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if model.Type() == state.ModelTypeIAAS {
		p.checker, err = p.getEnviron(model, p.cloudService, p.credentialService)
	} else {
		p.checker, err = p.getBroker(model, p.cloudService, p.credentialService)
	}
	return p.checker, err
}

// Prechecker implements state.Policy.
func (p *environStatePolicy) Prechecker() (environs.InstancePrechecker, error) {
	return p.getDeployChecker()
}

// ConfigValidator implements state.Policy.
func (p *environStatePolicy) ConfigValidator() (config.Validator, error) {
	model, err := p.st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	cloud, err := p.cloudService.Get(stdcontext.Background(), model.CloudName())
	if err != nil {
		return nil, errors.Annotate(err, "getting cloud")
	}
	return environProvider(cloud.Type)
}

// ConstraintsValidator implements state.Policy.
func (p *environStatePolicy) ConstraintsValidator(ctx envcontext.ProviderCallContext) (constraints.Validator, error) {
	checker, err := p.getDeployChecker()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return checker.ConstraintsValidator(ctx)
}

// StorageProviderRegistry implements state.Policy.
func (p *environStatePolicy) StorageProviderRegistry() (storage.ProviderRegistry, error) {
	if p.credentialService == nil {
		return nil, errors.NotSupportedf("StorageProviderRegistry check without credential service")
	}

	model, err := p.st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	// ProviderRegistry doesn't make any calls to fetch instance types,
	// so it doesn't help to use getDeployChecker() here.
	return NewStorageProviderRegistryForModel(model, p.cloudService, p.credentialService, p.getEnviron, p.getBroker)
}

// NewStorageProviderRegistryForModel returns a storage provider registry
// for the specified model.
func NewStorageProviderRegistryForModel(
	model *state.Model,
	cloudService CloudService,
	credentialService CredentialService,
	newEnv NewEnvironFunc,
	newBroker NewCAASBrokerFunc,
) (_ storage.ProviderRegistry, err error) {
	var reg storage.ProviderRegistry
	if model.Type() == state.ModelTypeIAAS {
		if reg, err = newEnv(model, cloudService, credentialService); err != nil {
			return nil, errors.Trace(err)
		}
	} else {
		if reg, err = newBroker(model, cloudService, credentialService); err != nil {
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
