// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	stdcontext "context"
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/constraints"
	coremodel "github.com/juju/juju/core/model"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
)

func (st *State) constraintsValidator() (constraints.Validator, error) {
	// Default behaviour is to simply use a standard validator with
	// no model specific behaviour built in.
	var validator constraints.Validator

	var err error
	validator, err = st.ConstraintsValidator(envcontext.WithoutCredentialInvalidator(stdcontext.Background()))
	if errors.Is(err, errors.NotImplemented) {
		validator = constraints.NewValidator()
	} else if err != nil {
		return nil, err
	} else if validator == nil {
		return nil, errors.New("policy returned nil constraints validator without an error")
	}

	return validator, nil
}

// ResolveConstraints combines the given constraints with the environ constraints to get
// a constraints which will be used to create a new instance.
func (st *State) ResolveConstraints(cons constraints.Value) (constraints.Value, error) {
	validator, err := st.constraintsValidator()
	if err != nil {
		return constraints.Value{}, err
	}
	modelCons, err := st.ModelConstraints()
	if err != nil {
		return constraints.Value{}, err
	}
	return validator.Merge(modelCons, cons)
}

// validateConstraints returns an error if the given constraints are not valid for the
// current model, and also any unsupported attributes.
func (st *State) validateConstraints(cons constraints.Value) ([]string, error) {
	validator, err := st.constraintsValidator()
	if err != nil {
		return nil, err
	}
	return validator.Validate(cons)
}

// Used for tests.
type noopStoragePoolGetter struct{}

func (noopStoragePoolGetter) GetStoragePoolByName(ctx stdcontext.Context, name string) (*storage.Config, error) {
	return nil, fmt.Errorf("storage pool %q not found%w", name, errors.Hide(storageerrors.PoolNotFoundError))
}

func (st *State) storageServices() (StoragePoolGetter, storage.ProviderRegistry, error) {
	if st.storageServiceGetter == nil {
		return noopStoragePoolGetter{}, storage.StaticProviderRegistry{}, nil
	}

	model, err := st.Model()
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	registry := provider.CommonStorageProviders()
	storageService := st.storageServiceGetter(coremodel.UUID(model.UUID()), registry)
	return storageService, registry, nil
}

// ConstraintsValidator implements state.Policy.
func (st *State) ConstraintsValidator(ctx envcontext.ProviderCallContext) (constraints.Validator, error) {
	validator := constraints.NewValidator()
	validator.RegisterVocabulary(constraints.Arch, []string{arch.AMD64, arch.ARM64, arch.PPC64EL, arch.S390X, arch.RISCV64})
	return validator, nil
}
