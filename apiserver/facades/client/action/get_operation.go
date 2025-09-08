// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"context"

	"github.com/juju/collections/transform"
	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/operation"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// ListOperations fetches the called operations for specified apps/units.
func (a *ActionAPI) ListOperations(ctx context.Context, arg params.OperationQueryArgs) (params.OperationResults, error) {
	if err := a.checkCanRead(ctx); err != nil {
		return params.OperationResults{}, errors.Capture(err)
	}

	var status []corestatus.Status
	if arg.Status != nil {
		status = transform.Slice(arg.Status, func(f string) corestatus.Status {
			return corestatus.Status(f)
		})
	}
	args := operation.QueryArgs{
		Receivers:   makeOperationReceivers(arg.Applications, arg.Machines, arg.Units),
		ActionNames: arg.ActionNames,
		Status:      status,
		Limit:       arg.Limit,
		Offset:      arg.Offset,
	}
	result, err := a.operationService.GetOperations(ctx, args)
	if err != nil {
		return params.OperationResults{}, errors.Capture(err)
	}

	return toOperationResults(result), errors.Capture(err)
}

// Operations fetches the specified operation ids.
func (a *ActionAPI) Operations(ctx context.Context, arg params.Entities) (params.OperationResults, error) {
	if err := a.checkCanRead(ctx); err != nil {
		return params.OperationResults{}, errors.Capture(err)
	}

	results := params.OperationResults{Results: make([]params.OperationResult, len(arg.Entities))}

	for i, entity := range arg.Entities {
		operationTag, err := names.ParseOperationTag(entity.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		result, err := a.operationService.GetOperationByID(ctx, operationTag.Id())
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[i] = toOperationResult(result)
	}
	return results, nil
}
