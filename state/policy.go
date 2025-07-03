// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"

	"github.com/juju/errors"

	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
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
	// StorageServices returns a StoragePoolGetter, storage.ProviderRegistry or an error.
	StorageServices() (StoragePoolGetter, error)
}

// Used for tests.
type noopStoragePoolGetter struct{}

func (noopStoragePoolGetter) GetStorageRegistry(ctx context.Context) (storage.ProviderRegistry, error) {
	return storage.StaticProviderRegistry{}, nil
}

func (noopStoragePoolGetter) GetStoragePoolByName(ctx context.Context, name string) (domainstorage.StoragePool, error) {
	return domainstorage.StoragePool{}, fmt.Errorf(
		"storage pool %q not found%w", name, errors.Hide(storageerrors.PoolNotFoundError),
	)
}

func (st *State) storageServices() (StoragePoolGetter, error) {
	if st.policy == nil {
		return noopStoragePoolGetter{}, nil
	}
	return st.policy.StorageServices()
}
