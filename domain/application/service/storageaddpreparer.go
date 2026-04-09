// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"math"

	corestorage "github.com/juju/juju/core/storage"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/service/storage"
	internalcharm "github.com/juju/juju/domain/deployment/charm"
	"github.com/juju/juju/internal/errors"
)

// UnitStorageAddPreparer prepares the arguments needed to add storage to a
// unit while preserving the existing validation and concurrency checks.
type UnitStorageAddPreparer interface {
	PrepareUnitAddStorage(
		ctx context.Context,
		storageName corestorage.Name,
		unitUUID coreunit.UUID,
		addCount uint32,
		arg AddUnitStorageOverride,
	) (UnitAddStorageArg, error)
}

type unitStorageAddPreparerState interface {
	// GetCharmStorageAndInstanceCountByUnitUUID returns the metadata and how
	// many storage instances exist for the named storage on the specified unit.
	GetCharmStorageAndInstanceCountByUnitUUID(
		ctx context.Context, unitUUID coreunit.UUID, storageName corestorage.Name,
	) (internalcharm.Storage, uint32, error)
}

type unitStorageAddPreparerStorageService interface {
	// GetUnitStorageDirectiveByName returns the named storage directive for the
	// unit.
	GetUnitStorageDirectiveByName(
		ctx context.Context, uuid coreunit.UUID, storageName corestorage.Name,
	) (application.StorageDirective, error)

	// MakeUnitAddStorageArgs creates the storage arguments required to add
	// storage to a unit.
	MakeUnitAddStorageArgs(
		ctx context.Context, unitUUID coreunit.UUID, addCount uint32, sd application.StorageDirective,
	) (UnitAddStorageArg, error)

	// ValidateApplicationStorageDirectiveOverrides checks a set of storage
	// directive overrides against the charm storage definitions.
	ValidateApplicationStorageDirectiveOverrides(
		ctx context.Context,
		charmStorageDefs map[string]internalcharm.Storage,
		overrides map[string]storage.StorageDirectiveOverride,
	) error
}

type unitStorageAddPreparer struct {
	st             unitStorageAddPreparerState
	storageService unitStorageAddPreparerStorageService
}

// NewUnitStorageAddPreparer returns a helper that prepares unit storage add
// arguments for callers in other domains.
func NewUnitStorageAddPreparer(
	st unitStorageAddPreparerState, storageService unitStorageAddPreparerStorageService,
) UnitStorageAddPreparer {
	return &unitStorageAddPreparer{
		st:             st,
		storageService: storageService,
	}
}

func (s *unitStorageAddPreparer) PrepareUnitAddStorage(
	ctx context.Context,
	storageName corestorage.Name,
	unitUUID coreunit.UUID,
	addCount uint32,
	arg AddUnitStorageOverride,
) (UnitAddStorageArg, error) {
	unitStorageDirective, err := s.storageService.GetUnitStorageDirectiveByName(
		ctx,
		unitUUID,
		storageName,
	)
	if err != nil {
		return UnitAddStorageArg{}, errors.Errorf(
			"getting unit %q storage directive: %w",
			unitUUID, err,
		)
	}

	charmStorage, existingCount, err := s.st.GetCharmStorageAndInstanceCountByUnitUUID(
		ctx,
		unitUUID,
		storageName,
	)
	if err != nil {
		return UnitAddStorageArg{}, errors.Errorf(
			"getting unit %q charm storage %q and count: %w",
			unitUUID, storageName, err,
		)
	}

	// We only care about a subset of the attributes for validation.
	charmStorageDefs := map[string]internalcharm.Storage{
		storageName.String(): {
			Name:        charmStorage.Name,
			Type:        charmStorage.Type,
			CountMin:    charmStorage.CountMin,
			CountMax:    charmStorage.CountMax,
			MinimumSize: charmStorage.MinimumSize,
		},
	}

	storageDirective := unitStorageDirective
	if arg.StoragePoolUUID != nil {
		storageDirective.PoolUUID = *arg.StoragePoolUUID
	}
	if arg.SizeMiB != nil {
		storageDirective.Size = *arg.SizeMiB
	}

	wantCount := addCount + existingCount
	toCheck := map[string]storage.StorageDirectiveOverride{
		storageName.String(): {
			Count:    &wantCount,
			PoolUUID: &storageDirective.PoolUUID,
			Size:     &storageDirective.Size,
		},
	}
	err = s.storageService.ValidateApplicationStorageDirectiveOverrides(
		ctx,
		charmStorageDefs,
		toCheck,
	)
	if err != nil {
		return UnitAddStorageArg{}, errors.Capture(err)
	}

	args, err := s.storageService.MakeUnitAddStorageArgs(
		ctx,
		unitUUID,
		addCount,
		storageDirective,
	)
	if err != nil {
		return UnitAddStorageArg{}, errors.Capture(err)
	}

	// Record the max allowed count precondition.
	// This will be checked inside the transaction.
	args.CountLessThanEqual = uint32(math.MaxUint32)
	if charmStorage.CountMax > 0 {
		args.CountLessThanEqual = uint32(charmStorage.CountMax) - addCount
	}
	return args, nil
}
