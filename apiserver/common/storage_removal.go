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
// required to classify the storage attached to a set of units being removed.
type StorageRemovalClassifier interface {
	// GetStorageClassificationForUnits returns,keyed by unit UUID,the
	// storage instances attached to the input units,along with the minimal
	// information needed to classify each as destroyed or detached when its
	// unit is removed.
	GetStorageClassificationForUnits(
		ctx context.Context, unitUUIDs []coreunit.UUID,
	) (map[coreunit.UUID][]domainstorage.StorageInstanceClassification, error)
}

// ClassifyStorageRemoval classifies the storage instances attached to the
// input units into those that will be destroyed and those that will be
// detached when the units are removed. If destroyStorage is true, every
// attached storage instance is classified as destroyed. Otherwise a
// storage instance is classified as detached when it is persistent,as
// its life cycle outlives the units it is attached to,and destroyed when it
// is not.
//
// Storage instances attached to more than one removed unit are only
// reported once. Units are processed in the input order,and the storage
// instances of each unit are reported in the deterministic order returned by
// the storage service. Returns empty results when no units are supplied..

func ClassifyStorageRemoval(
	ctx context.Context,
	storageService StorageRemovalClassifier,
	unitUUIDs []coreunit.UUID,
	destroyStorage bool,
) (destroyed, detached []params.Entity, _ error) {
	if len(unitUUIDs) == 0 {
		return nil, nil, nil
	}

	instancesByUnit, err := storageService.GetStorageClassificationForUnits(ctx, unitUUIDs)
	if err != nil {
		return nil, nil, errors.Errorf(
			"getting storage classification: %w", err,
		)
	}

	seen := make(map[string]bool)
	for _, unitUUID := range unitUUIDs {
		for _, instance := range instancesByUnit[unitUUID] {
			// Storage can be shared by multiple units,so we must only
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
