// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"context"

	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/storage"
)

const (
	// ErrStorageRegistryDying is used to indicate to *third parties* that the
	// storage registry worker is dying, instead of catacomb.ErrDying, which is
	// unsuitable for propagating inter-worker.
	// This error indicates to consuming workers that their dependency has
	// become unmet and a restart by the dependency engine is imminent.
	ErrStorageRegistryDying = errors.ConstError("storage registry worker is dying")
)

// StorageRegistryGetter is the interface that is used to get a storage
// registry.
type StorageRegistryGetter interface {
	// GetStorageRegistry returns a storage registry for the given namespace.
	GetStorageRegistry(context.Context, string) (storage.ProviderRegistry, error)
}

// ModelStorageRegistryGetter is the interface that is used to get a storage
// registry.
type ModelStorageRegistryGetter interface {
	// GetStorageRegistry returns a storage registry for the given namespace.
	GetStorageRegistry(context.Context) (storage.ProviderRegistry, error)
}

// ConstModelStorageRegistry is a function that returns the same storage
// registry every time it is called.
type ConstModelStorageRegistry func() storage.ProviderRegistry

// GetStorageRegistry returns a storage registry for the given namespace.
func (c ConstModelStorageRegistry) GetStorageRegistry(ctx context.Context) (storage.ProviderRegistry, error) {
	return c(), nil
}
