// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/collections/set"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/storage"
)

// Attrs defines storage attributes.
type Attrs map[string]string

// StoragePool represents a storage pool in Juju.
// It contains the name of the pool, the provider type, and any attributes
type StoragePool struct {
	UUID     string
	Name     string
	Provider string
	Attrs    Attrs
	OriginID int
}

// These type aliases are used to specify filter terms.
type (
	Names     []string
	Providers []string
)

func deduplicateNamesOrProviders[T ~[]string](namesOrProviders T) T {
	if len(namesOrProviders) == 0 {
		return nil
	}
	// Ensure uniqueness and no empty values.
	result := set.NewStrings()
	for _, v := range namesOrProviders {
		if v != "" {
			result.Add(v)
		}
	}
	if result.IsEmpty() {
		return nil
	}
	return T(result.Values())
}

// Values returns the unique values of the Names.
func (n Names) Values() []string {
	return deduplicateNamesOrProviders(n)
}

// Values returns the unique values of the Providers.
func (p Providers) Values() []string {
	return deduplicateNamesOrProviders(p)
}

// FilesystemInfo describes information about a filesystem.
type FilesystemInfo struct {
	storage.FilesystemInfo
	Pool          string
	BackingVolume *storage.VolumeInfo
}

// RecommendedStoragePoolArg represents a recommended storage pool assignment
// for the state layer to accept.
type RecommendedStoragePoolArg struct {
	StoragePoolUUID StoragePoolUUID
	StorageKind     StorageKind
}

// RecommendedStoragePoolParams represents a recommended storage pool assignment
// at the service layer boundary. It is accepted by services and translated into
// state-layer arguments before being persisted.
type RecommendedStoragePoolParams struct {
	StoragePoolUUID StoragePoolUUID
	StorageKind     StorageKind
}

// ImportStoragePoolParams represents a storage pool definition used when importing
// storage pools into the model.
type ImportStoragePoolParams struct {
	UUID   StoragePoolUUID
	Name   string
	Origin StoragePoolOrigin
	Type   string
	Attrs  map[string]any
}

// ImportStorageInstanceParams represents data to import a storage instance
// and its owner.
type ImportStorageInstanceParams struct {
	StorageName      string
	StorageKind      string
	StorageID        string
	RequestedSizeMiB uint64
	PoolName         string
	UnitName         string
}

// Validate returns NotValid if the params have an empty StorageID or
// PoolName or RequestedSizeMiB.
func (i ImportStorageInstanceParams) Validate() error {
	if i.PoolName == "" || i.RequestedSizeMiB == 0 || i.StorageID == "" {
		return errors.New("empty PoolName, RequestedSizeMiB, or StorageID not valid").Add(coreerrors.NotValid)
	}
	return nil
}
