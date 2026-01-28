// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"context"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	coreversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/rpc/params"
)

// Import imports existing storage into the model.
// A "CHANGE" block can block this operation.
func (a *StorageAPI) Import(
	ctx context.Context, args params.BulkImportStorageParamsV2,
) (params.ImportStorageResults, error) {
	return params.ImportStorageResults{}, apiservererrors.ParamsErrorf(
		params.CodeNotYetAvailable, "not yet available in %s", coreversion.Current.String(),
	)
}

// Import imports existing storage into the model.
// A "CHANGE" block can block this operation.
func (a *StorageAPIv6) Import(
	ctx context.Context, args params.BulkImportStorageParams,
) (params.ImportStorageResults, error) {
	return params.ImportStorageResults{}, apiservererrors.ParamsErrorf(
		params.CodeNotYetAvailable, "not yet available in %s", coreversion.Current.String(),
	)
}
