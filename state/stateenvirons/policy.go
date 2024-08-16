// Copyright 2014, 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateenvirons

import (
	"sync"

	"github.com/juju/errors"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/constraints"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
	"github.com/juju/juju/state"
)

// environStatePolicy implements state.Policy in
// terms of environs.Environ and related types.
type environStatePolicy struct {
	st                   *state.State
	cloudService         CloudService
	credentialService    CredentialService
	getEnviron           NewEnvironFunc
	getBroker            NewCAASBrokerFunc
	checkerMu            sync.Mutex
	checker              deployChecker
	storageServiceGetter storageServiceGetter
}

// deployChecker is the subset of the Environ interface (common to Environ and
// Broker) that we need for pre-checking instances and validating constraints.
type deployChecker interface {
	environs.InstancePrechecker
	environs.ConstraintsChecker
}

type storageServiceGetter func(modelUUID coremodel.UUID, registry storage.ProviderRegistry) state.StoragePoolGetter

// GetNewPolicyFunc returns a state.NewPolicyFunc that will return
// a state.Policy implemented in terms of either environs.Environ
// or caas.Broker and related types.
func GetNewPolicyFunc(cloudService CloudService, credentialService CredentialService, storageServiceGetter storageServiceGetter) state.NewPolicyFunc {
	return func(st *state.State) state.Policy {
		return &environStatePolicy{
			st:                   st,
			cloudService:         cloudService,
			credentialService:    credentialService,
			getEnviron:           GetNewEnvironFunc(environs.New),
			getBroker:            GetNewCAASBrokerFunc(caas.New),
			storageServiceGetter: storageServiceGetter,
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

// ConstraintsValidator implements state.Policy.
func (p *environStatePolicy) ConstraintsValidator(ctx envcontext.ProviderCallContext) (constraints.Validator, error) {
	checker, err := p.getDeployChecker()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return checker.ConstraintsValidator(ctx)
}

// StorageServices implements state.Policy.
func (p *environStatePolicy) StorageServices() (state.StoragePoolGetter, storage.ProviderRegistry, error) {
	if p.credentialService == nil {
		return nil, nil, errors.NotSupportedf("StorageServices check without credential service")
	}
	if p.storageServiceGetter == nil {
		return nil, nil, errors.NotSupportedf("StorageServices check without storage pool getter")
	}

	model, err := p.st.Model()
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	// ProviderRegistry doesn't make any calls to fetch instance types,
	// so it doesn't help to use getDeployChecker() here.
	registry, err := newStorageProviderRegistryForModel(model, p.cloudService, p.credentialService, p.getEnviron, p.getBroker)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	storageService := p.storageServiceGetter(coremodel.UUID(model.UUID()), registry)
	return storageService, registry, nil
}

// newStorageProviderRegistryForModel returns a storage provider registry
// for the specified model.
func newStorageProviderRegistryForModel(
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
	return newStorageProviderRegistry(reg), nil
}

// newStorageProviderRegistry returns a storage.ProviderRegistry that chains
// the provided registry with the common storage providers.
func newStorageProviderRegistry(reg storage.ProviderRegistry) storage.ProviderRegistry {
	return storage.ChainedProviderRegistry{reg, provider.CommonStorageProviders()}
}
