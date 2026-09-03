// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"

	"github.com/juju/names/v6"

	coreunit "github.com/juju/juju/core/unit"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// StorageRemovalClassifier defines the subset of the storage service that is
// required to list the storage instances attached to the units being removed.
type StorageRemovalClassifier interface {
	// GetStorageInstancesForUnit returns the storage instances attached to the
	// unit with the input UUID.
	GetStorageInstancesForUnit(
		ctx context.Context, unitUUID coreunit.UUID,
	) ([]domainstorage.StorageInstanceInfo, error)
}

// ClassifyStorageRemoval classifies the storage instances attached to the
// input units into those that will be destroyed and those that will be
// detached when the units are removed.
//
// If destroyStorage is true, every attached storage instance is classified as
// destroyed. Otherwise a storage instance is classified as detached when it is
// persistent, as its life cycle outlives the units it is attached to, and
// destroyed when it is not.
//
// Storage instances attached to more than one of the removed units are only
// reported once. The returned entities preserve the order in which the storage
// instances are encountered.
func ClassifyStorageRemoval(
	ctx context.Context,
	storageService StorageRemovalClassifier,
	unitUUIDs []coreunit.UUID,
	destroyStorage bool,
) (destroyed, detached []params.Entity, _ error) {
	seen := make(map[string]bool)
	for _, unitUUID := range unitUUIDs {
		instances, err := storageService.GetStorageInstancesForUnit(ctx, unitUUID)
		if err != nil {
			return nil, nil, errors.Errorf(
				"getting storage instances for unit %q: %w", unitUUID, err,
			)
		}
		for _, instance := range instances {
			// Storage can be shared by multiple units, so we must only
			// report each storage instance once.
			if seen[instance.UUID.String()] {
				continue
			}
			seen[instance.UUID.String()] = true

			entity := params.Entity{Tag: names.NewStorageTag(instance.ID).String()}
			if destroyStorage || !instance.Persistent {
				destroyed = append(destroyed, entity)
			} else {
				detached = append(detached, entity)
			}
		}
	}
	return destroyed, detached, nil
}
