// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	coreunit "github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/rpc/params"
)

// EnqueueOperation takes a list of Actions and queues them up to be executed as
// an operation, each action running as a task on the designated ActionReceiver.
// We return the ID of the overall operation and each individual task.
func (a *ActionAPI) EnqueueOperation(ctx context.Context, arg params.Actions) (params.EnqueuedActions, error) {
	operationId, actionResults, err := a.enqueue(ctx, arg)
	if err != nil {
		return params.EnqueuedActions{}, err
	}
	results := params.EnqueuedActions{
		OperationTag: names.NewOperationTag(operationId).String(),
		Actions:      actionResults.Results,
	}
	return results, nil
}

func (a *ActionAPI) enqueue(ctx context.Context, arg params.Actions) (string, params.ActionResults, error) {
	if err := a.checkCanWrite(ctx); err != nil {
		return "", params.ActionResults{}, errors.Trace(err)
	}

	return "", params.ActionResults{}, errors.NotSupportedf("actions in Dqlite")
}

// ListOperations fetches the called actions for specified apps/units.
func (a *ActionAPI) ListOperations(ctx context.Context, arg params.OperationQueryArgs) (params.OperationResults, error) {
	if err := a.checkCanRead(ctx); err != nil {
		return params.OperationResults{}, errors.Trace(err)
	}

	var receiverTags []names.Tag
	for _, name := range arg.Units {
		receiverTags = append(receiverTags, names.NewUnitTag(name))
	}
	for _, id := range arg.Machines {
		receiverTags = append(receiverTags, names.NewMachineTag(id))
	}

	var unitNames []coreunit.Name
	if len(arg.ActionNames) == 0 && len(arg.Applications) == 0 && len(receiverTags) == 0 {
		var err error
		unitNames, err = a.applicationService.GetAllUnitNames(ctx)
		if err != nil {
			return params.OperationResults{}, errors.Trace(err)
		}
	} else {
		for _, aName := range arg.Applications {
			appUnitName, err := a.applicationService.GetUnitNamesForApplication(ctx, aName)
			if errors.Is(err, applicationerrors.ApplicationNotFound) {
				return params.OperationResults{}, errors.NotFoundf("application %q", aName)
			} else if err != nil {
				return params.OperationResults{}, errors.Trace(err)
			}
			unitNames = append(unitNames, appUnitName...)
		}
	}
	for _, unitName := range unitNames {
		tag := names.NewUnitTag(unitName.String())
		receiverTags = append(receiverTags, tag)
	}

	return params.OperationResults{}, errors.NotSupportedf("actions in Dqlite")
}

// Operations fetches the specified operation ids.
func (a *ActionAPI) Operations(ctx context.Context, arg params.Entities) (params.OperationResults, error) {
	if err := a.checkCanRead(ctx); err != nil {
		return params.OperationResults{}, errors.Trace(err)
	}
	results := params.OperationResults{Results: make([]params.OperationResult, len(arg.Entities))}

	for i, entity := range arg.Entities {
		_, err := names.ParseOperationTag(entity.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		results.Results[i].Error = apiservererrors.ServerError(errors.NotSupportedf("actions in Dqlite"))
	}
	return results, nil
}
