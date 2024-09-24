// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	stdcontext "context"
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/core/constraints"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/internal/storage"
)

// NewPolicyFunc is the type of a function that,
// given a *State, returns a Policy for that State.
type NewPolicyFunc func(*State) Policy

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
	// ConstraintsValidator returns a constraints.Validator or an error.
	ConstraintsValidator(envcontext.ProviderCallContext) (constraints.Validator, error)

	// StorageServices returns a StoragePoolGetter, storage.ProviderRegistry or an error.
	StorageServices() (StoragePoolGetter, storage.ProviderRegistry, error)
}

func (st *State) constraintsValidator() (constraints.Validator, error) {
	// Default behaviour is to simply use a standard validator with
	// no model specific behaviour built in.
	var validator constraints.Validator
	if st.policy != nil {
		var err error
		validator, err = st.policy.ConstraintsValidator(envcontext.WithoutCredentialInvalidator(stdcontext.Background()))
		if errors.Is(err, errors.NotImplemented) {
			validator = constraints.NewValidator()
		} else if err != nil {
			return nil, err
		} else if validator == nil {
			return nil, errors.New("policy returned nil constraints validator without an error")
		}
	} else {
		validator = constraints.NewValidator()
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
	if st.policy == nil {
		return noopStoragePoolGetter{}, storage.StaticProviderRegistry{}, nil
	}
	return st.policy.StorageServices()
}
