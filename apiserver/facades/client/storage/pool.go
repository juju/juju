// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"context"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	coreversion "github.com/juju/juju/core/version"
	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// StoragePoolService defines the interface required by this API for managing
// storage pools within a model.
type StoragePoolService interface {
	// CreateStoragePool creates a new storage pool with the given name and
	// provider in the model. Returned is the unique uuid for the new storage
	// pool.
	//
	// The following errors may be returned:
	// - [github.com/juju/juju/domain/storage/errors.StoragePoolNameInvalid]
	// when the supplied storage pool name is considered invalid or empty.
	// - [github.com/juju/juju/domain/storage/errors.ProviderTypeInvalid] when
	// the supplied provider type value is invalid for further use.
	// - [github.com/juju/juju/domain/storage/errors.ProviderTypeNotFound] when
	// the supplied provider type is not known to the controller.
	// - [github.com/juju/juju/domain/storage/errors.StoragePoolAlreadyExists]
	// when a storage pool for the supplied name already exists in the model.
	// - [github.com/juju/juju/domain/storage/errors.StoragePoolAttributeInvalid]
	// when one of the supplied storage pool attributes is invalid.
	CreateStoragePool(
		context.Context,
		string,
		domainstorage.ProviderType,
		map[string]any,
	) (domainstorage.StoragePoolUUID, error)
}

// CreatePool creates a new storage pool in the model with specified parameters.
//
// If the user does not have write permission on the model then they can expect
// back an error with [params.CodeUnauthorized] and an empty
// [params.ErrorResult]. The request will not be entertained further.
//
// For each storage pool create arg supplied the caller can expected the
// following params error codes:
// - [params.CodeNotValid] when either the storage pool name is invalid, the
// provider type name supplied is invalid or an attribute value is invalid.
// - [params.CodeNotFound] when the supplied provider type does not exist in the
// model.
// - [params.CodeAlreadyExists] when a storage pool for the supplied name
// already exists in the model.
func (a *StorageAPI) CreatePool(
	ctx context.Context, p params.StoragePoolArgs,
) (params.ErrorResults, error) {
	if err := a.checkCanWrite(ctx); err != nil {
		return params.ErrorResults{}, err
	}

	results := params.ErrorResults{
		Results: make([]params.ErrorResult, 0, len(p.Pools)),
	}

	for _, poolToCreateArg := range p.Pools {
		createErr := a.createOneStoragePool(ctx, poolToCreateArg)
		results.Results = append(
			results.Results, params.ErrorResult{Error: createErr},
		)
	}

	return results, nil
}

// createOneStoragePool will create a new storage pool in the model for  single
// [params.StoragePool]. Any errors encountered during processing of the request
// will be handled and converted to [params.Error]s suitable for returning to
// the client. Should the pool be created successfully a nill [params.Error] is
// returned.
func (a *StorageAPI) createOneStoragePool(
	ctx context.Context, arg params.StoragePool,
) *params.Error {
	poolProviderType := domainstorage.ProviderType(arg.Provider)
	storagePoolUUID, err := a.storageService.CreateStoragePool(
		ctx, arg.Name, poolProviderType, arg.Attrs,
	)

	switch {
	case errors.Is(err, domainstorageerrors.StoragePoolNameInvalid):
		// We purposely don't echo out what the value is. It is an unknown
		// quantity and using the memory could be adverse.
		return apiservererrors.ParamsErrorf(
			params.CodeNotValid,
			"storage pool name is either not set or is not a valid name",
		)
	case errors.Is(err, domainstorageerrors.ProviderTypeInvalid):
		// We purposely don't echo out what the value is. It is an unknown
		// quantity and using the memory could be adverse.
		return apiservererrors.ParamsErrorf(
			params.CodeNotValid,
			"provider type is not a valid value that can be used for a new storage pool",
		)
	case errors.Is(err, domainstorageerrors.ProviderTypeNotFound):
		return apiservererrors.ParamsErrorf(
			params.CodeNotFound,
			"no storage provider %q exists in the current model",
			poolProviderType.String(),
		)
	case errors.Is(err, domainstorageerrors.StoragePoolAlreadyExists):
		return apiservererrors.ParamsErrorf(
			params.CodeAlreadyExists,
			"storage pool for name %q already exists in the model",
			arg.Name,
		)
	case errors.HasType[domainstorageerrors.StoragePoolAttributeInvalid](err):
		invalidErr, _ := errors.AsType[domainstorageerrors.StoragePoolAttributeInvalid](err)
		return apiservererrors.ParamsErrorf(
			params.CodeNotValid,
			"storage pool attribute %q is not valid: %s",
			invalidErr.Key, invalidErr.Message,
		)
	case err != nil:
		// We don't know what went wrong and so we tell the user that and log
		// the error. We don't send unknown quanities back to the caller.
		a.logger.Errorf(
			ctx,
			"creating new storage pool %q for provider %q: %s",
			arg.Name,
			poolProviderType.String(),
			err,
		)
		return apiservererrors.ParamsErrorf(
			"", "unable to create storage pool %q due to an unknown error",
			arg.Name,
		)
	}

	a.logger.Debugf(
		ctx,
		"new storage pool %q created with uuid %q",
		arg.Name,
		storagePoolUUID,
	)

	return nil
}

// ListPools returns a list of pools.
// If filter is provided, returned list only contains pools that match
// the filter.
// Pools can be filtered on names and provider types.
// If both names and types are provided as filter,
// pools that match either are returned.
// This method lists union of pools and environment provider types.
// If no filter is provided, all pools are returned.
func (a *StorageAPI) ListPools(
	ctx context.Context,
	filters params.StoragePoolFilters,
) (params.StoragePoolsResults, error) {
	if err := a.checkCanRead(ctx); err != nil {
		return params.StoragePoolsResults{}, errors.Capture(err)
	}

	results := params.StoragePoolsResults{
		Results: make([]params.StoragePoolsResult, len(filters.Filters)),
	}
	for i, filter := range filters.Filters {
		pools, err := a.listPools(ctx, filter)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[i].Result = pools
	}
	return results, nil
}

func (a *StorageAPI) listPools(ctx context.Context, filter params.StoragePoolFilter) ([]params.StoragePool, error) {
	var (
		pools []domainstorage.StoragePool
		err   error
	)
	if len(filter.Names) == 0 && len(filter.Providers) == 0 {
		pools, err = a.storageService.ListStoragePools(ctx)
	} else if len(filter.Names) != 0 && len(filter.Providers) != 0 {
		pools, err = a.storageService.ListStoragePoolsByNamesAndProviders(ctx, filter.Names, filter.Providers)
	} else if len(filter.Names) != 0 {
		pools, err = a.storageService.ListStoragePoolsByNames(ctx, filter.Names)
	} else {
		pools, err = a.storageService.ListStoragePoolsByProviders(ctx, filter.Providers)
	}
	if err != nil {
		return nil, errors.Capture(err)
	}
	results := make([]params.StoragePool, len(pools))
	for i, p := range pools {
		pool := params.StoragePool{
			Name:     p.Name,
			Provider: p.Provider,
		}
		if len(p.Attrs) > 0 {
			pool.Attrs = make(map[string]any, len(p.Attrs))
			for k, v := range p.Attrs {
				pool.Attrs[k] = v
			}
		}
		results[i] = pool

	}
	return results, nil
}

// RemovePool deletes the named pool
func (a *StorageAPI) RemovePool(ctx context.Context, p params.StoragePoolDeleteArgs) (params.ErrorResults, error) {
	return params.ErrorResults{}, apiservererrors.ParamsErrorf(
		params.CodeNotYetAvailable, "not yet available in %s", coreversion.Current.String(),
	)
}

// UpdatePool deletes the named pool
func (a *StorageAPI) UpdatePool(ctx context.Context, p params.StoragePoolArgs) (params.ErrorResults, error) {
	return params.ErrorResults{}, apiservererrors.ParamsErrorf(
		params.CodeNotYetAvailable, "not yet available in %s", coreversion.Current.String(),
	)
}
