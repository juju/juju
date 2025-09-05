// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"context"

	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/domain/operation"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// ListOperations fetches the called actions for specified apps/units.
func (a *ActionAPI) ListOperations(ctx context.Context, arg params.OperationQueryArgs) (params.OperationResults, error) {
	if err := a.checkCanRead(ctx); err != nil {
		return params.OperationResults{}, errors.Capture(err)
	}

	args := operation.QueryArgs{
		Target:      makeOperationTarget(arg.Applications, arg.Machines, arg.Units),
		ActionNames: arg.ActionNames,
		Status:      arg.Status,
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
	// Map from operation tag string to all result slots (to support duplicate tags)
	tagToResults := make(map[string][]*params.OperationResult)

	operationIds := make([]string, 0, len(arg.Entities))
	for i, entity := range arg.Entities {
		operationTag, err := names.ParseOperationTag(entity.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		tagStr := operationTag.String()
		tagToResults[tagStr] = append(tagToResults[tagStr], &results.Results[i])
		operationIds = append(operationIds, operationTag.Id())
	}

	if len(operationIds) == 0 {
		return results, nil
	}

	result, err := a.operationService.GetOperationsByIDs(ctx, operationIds)
	if err != nil {
		return params.OperationResults{}, errors.Capture(err)
	}
	facadeResult := toOperationResults(result)
	// Fill in all matching slots for each returned operation
	for _, r := range facadeResult.Results {
		slots := tagToResults[r.OperationTag]
		if len(slots) == 0 {
			return params.OperationResults{}, errors.Errorf("unexpected result for %q", r.OperationTag)
		}
		for _, slot := range slots {
			*slot = r
		}
	}
	// Mark any slots that did not receive a result as NotFound
	for tag, slots := range tagToResults {
		for _, slot := range slots {
			if slot.OperationTag == "" && slot.Error == nil {
				slot.Error = apiservererrors.ServerError(errors.Errorf("missing result for %q: %w",
					tag, coreerrors.NotFound))
			}
		}
	}
	return results, nil
}
