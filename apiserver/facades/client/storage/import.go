// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"context"

	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// StorageImportService defines the interface required by this API for importing
// storage.
type StorageImportService interface {
	// ImportFilesystem imports a filesystem by invoking the provider of the
	// given storage pool to identify the filesystem on the given natural entity
	// specified by the provider ID (e.g. a filesystem on a volume or a file-
	// system directly). The result of this call is the name of a new storage
	// instance using the given storage name.
	ImportFilesystem(
		ctx context.Context,
		storageName string,
		pool domainstorage.StoragePoolUUID,
		providerID string,
		force bool,
	) (string, error)

	// GetStoragePoolUUID returns the UUID of the storage pool for the specified
	// name.
	GetStoragePoolUUID(
		ctx context.Context, name string,
	) (domainstorage.StoragePoolUUID, error)
}

// Import imports existing storage into the model.
// A "CHANGE" block can block this operation.
func (a *StorageAPI) Import(
	ctx context.Context, args params.BulkImportStorageParamsV2,
) (params.ImportStorageResults, error) {
	err := a.checkCanWrite(ctx)
	if err != nil {
		return params.ImportStorageResults{}, err
	}

	one := func(
		arg params.ImportStorageParamsV2,
	) (params.ImportStorageDetails, error) {
		var details params.ImportStorageDetails

		poolUUID, err := a.storageService.GetStoragePoolUUID(ctx, arg.Pool)
		if errors.Is(err, domainstorageerrors.StoragePoolNameInvalid) {
			return details, apiservererrors.ParamsErrorf(
				params.CodeNotValid, "storage pool name is not valid",
			)
		} else if errors.Is(err, domainstorageerrors.StoragePoolNotFound) {
			return details, apiservererrors.ParamsErrorf(
				params.CodeNotFound, "storage pool not found",
			)
		} else if err != nil {
			return details, errors.Errorf(
				"getting storage pool uuid: %w", err,
			)
		}

		switch arg.Kind {
		case params.StorageKindFilesystem:
			inst, err := a.storageService.ImportFilesystem(
				ctx, arg.StorageName, poolUUID, arg.ProviderId, arg.Force)
			if errors.Is(err, domainstorageerrors.StoragePoolNotFound) {
				return details, apiservererrors.ParamsErrorf(
					params.CodeNotFound, "storage pool not found",
				)
			} else if errors.Is(err, domainstorageerrors.PooledStorageEntityNotFound) {
				return details, apiservererrors.ParamsErrorf(
					params.CodeNotFound, "storage entity not found in pool",
				)
			} else if err != nil {
				return details, errors.Errorf(
					"importing filesystem: %w", err,
				)
			}
			details.StorageTag = names.NewStorageTag(inst).String()
		case params.StorageKindBlock:
			return details, apiservererrors.ParamsErrorf(
				params.CodeNotSupported,
				"block device importing is not supported",
			)
		default:
			return details, apiservererrors.ParamsErrorf(
				params.CodeNotValid, "invalid storage kind",
			)
		}

		return details, nil
	}

	results := params.ImportStorageResults{
		Results: make([]params.ImportStorageResult, 0, len(args.Storage)),
	}
	for _, v := range args.Storage {
		var result params.ImportStorageResult
		res, err := one(v)
		if err != nil {
			result.Error = apiservererrors.ServerError(err)
		} else {
			result.Result = &res
		}
		results.Results = append(results.Results, result)
	}

	return results, nil
}

// Import imports existing storage into the model.
// A "CHANGE" block can block this operation.
func (a *StorageAPIv6) Import(
	ctx context.Context, args params.BulkImportStorageParams,
) (params.ImportStorageResults, error) {
	err := a.checkCanWrite(ctx)
	if err != nil {
		return params.ImportStorageResults{}, err
	}

	one := func(
		arg params.ImportStorageParams,
	) (params.ImportStorageDetails, error) {
		var details params.ImportStorageDetails

		poolUUID, err := a.storageService.GetStoragePoolUUID(ctx, arg.Pool)
		if errors.Is(err, domainstorageerrors.StoragePoolNameInvalid) {
			return details, apiservererrors.ParamsErrorf(
				params.CodeNotValid, "storage pool name is not valid",
			)
		} else if errors.Is(err, domainstorageerrors.StoragePoolNotFound) {
			return details, apiservererrors.ParamsErrorf(
				params.CodeNotFound, "storage pool not found",
			)
		} else if err != nil {
			return details, errors.Errorf(
				"getting storage pool uuid: %w", err,
			)
		}

		switch arg.Kind {
		case params.StorageKindFilesystem:
			inst, err := a.storageService.ImportFilesystem(
				ctx, arg.StorageName, poolUUID, arg.ProviderId, false)
			if errors.Is(err, domainstorageerrors.StoragePoolNotFound) {
				return details, apiservererrors.ParamsErrorf(
					params.CodeNotFound, "storage pool not found",
				)
			} else if errors.Is(err, domainstorageerrors.PooledStorageEntityNotFound) {
				return details, apiservererrors.ParamsErrorf(
					params.CodeNotFound, "storage entity not found in pool",
				)
			} else if err != nil {
				return details, errors.Errorf(
					"importing filesystem: %w", err,
				)
			}
			details.StorageTag = names.NewStorageTag(inst).String()
		case params.StorageKindBlock:
			return details, apiservererrors.ParamsErrorf(
				params.CodeNotSupported,
				"block device importing is not supported",
			)
		default:
			return details, apiservererrors.ParamsErrorf(
				params.CodeNotValid, "invalid storage kind",
			)
		}

		return details, nil
	}

	results := params.ImportStorageResults{
		Results: make([]params.ImportStorageResult, 0, len(args.Storage)),
	}
	for _, v := range args.Storage {
		var result params.ImportStorageResult
		res, err := one(v)
		if err != nil {
			result.Error = apiservererrors.ServerError(err)
		} else {
			result.Result = &res
		}
		results.Results = append(results.Results, result)
	}

	return results, nil
}
