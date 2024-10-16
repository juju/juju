// Copyright 2014, 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateenvirons

import (
	"github.com/juju/errors"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/constraints"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
	"github.com/juju/juju/state"
)

// environStatePolicy implements state.Policy in
// terms of environs.Environ and related types.
type environStatePolicy struct {
	st                   *state.State
	storageServiceGetter storageServiceGetter
}

type storageServiceGetter func(modelUUID coremodel.UUID, registry storage.ProviderRegistry) state.StoragePoolGetter

// GetNewPolicyFunc returns a state.NewPolicyFunc that will return
// a state.Policy implemented in terms of either environs.Environ
// or caas.Broker and related types.
func GetNewPolicyFunc(
	storageServiceGetter storageServiceGetter,
) state.NewPolicyFunc {
	return func(st *state.State) state.Policy {
		return &environStatePolicy{
			st:                   st,
			storageServiceGetter: storageServiceGetter,
		}
	}
}

// ConstraintsValidator implements state.Policy.
func (p *environStatePolicy) ConstraintsValidator(ctx envcontext.ProviderCallContext) (constraints.Validator, error) {
	validator := constraints.NewValidator()
	validator.RegisterVocabulary(constraints.Arch, []string{arch.AMD64, arch.ARM64, arch.PPC64EL, arch.S390X, arch.RISCV64})
	return validator, nil
}

// StorageServices implements state.Policy.
func (p *environStatePolicy) StorageServices() (state.StoragePoolGetter, storage.ProviderRegistry, error) {
	model, err := p.st.Model()
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	registry := NewStorageProviderRegistry()
	storageService := p.storageServiceGetter(coremodel.UUID(model.UUID()), registry)
	return storageService, registry, nil
}

// NewStorageProviderRegistryForModel returns a storage provider registry
// for the specified model.
func NewStorageProviderRegistryForModel(
	model *state.Model,
	cloudService CloudService,
	credentialService CredentialService,
	modelConfigService ModelConfigService,
	newEnv NewEnvironFunc,
	newBroker NewCAASBrokerFunc,
) (_ storage.ProviderRegistry, err error) {
	return NewStorageProviderRegistry(), nil
}

// NewStorageProviderRegistry returns a storage.ProviderRegistry that chains
// the provided registry with the common storage providers.
func NewStorageProviderRegistry() storage.ProviderRegistry {
	return storage.ChainedProviderRegistry{provider.CommonStorageProviders()}
}
