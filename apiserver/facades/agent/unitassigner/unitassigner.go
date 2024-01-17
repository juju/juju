// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitassigner

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

// assignerState defines the state methods this facade needs, so they can be mocked
// for testing.
type assignerState interface {
	WatchForUnitAssignment() state.StringsWatcher
	AssignStagedUnits(ids []string) ([]state.UnitAssignmentResult, error)
	AssignedMachineId(unit string) (string, error)
}

type statusSetter interface {
	SetStatus(context.Context, params.SetStatus) (params.ErrorResults, error)
}

type machineSaver interface {
	Save(context.Context, string) error
}

// API implements the functionality for assigning units to machines.
type API struct {
	st           assignerState
	res          facade.Resources
	statusSetter statusSetter
	machineSaver machineSaver
}

// AssignUnits assigns the units with the given ids to the correct machine. The
// error results are returned in the same order as the given entities.
func (a *API) AssignUnits(ctx context.Context, args params.Entities) (params.ErrorResults, error) {
	result := params.ErrorResults{}

	// state uses ids, but the API uses Tags, so we have to convert back and
	// forth (whee!).  The list of ids is (crucially) in the same order as the
	// list of tags.  This is the same order as the list of errors we return.
	ids := make([]string, len(args.Entities))
	for i, e := range args.Entities {
		tag, err := names.ParseUnitTag(e.Tag)
		if err != nil {
			return result, err
		}
		ids[i] = tag.Id()
	}

	res, err := a.st.AssignStagedUnits(ids)
	if err != nil {
		return result, apiservererrors.ServerError(err)
	}

	// The results come back from state in an undetermined order and do not
	// include results for units that were not found, so we have to make up for
	// that here.
	resultMap := make(map[string]error, len(ids))
	for _, r := range res {
		resultMap[r.Unit] = r.Error
		if r.Error != nil {
			continue
		}

		// Get assigned machine and ensure it exists in dqlite.
		machineId, err := a.st.AssignedMachineId(r.Unit)
		if err != nil {
			resultMap[r.Unit] = err
			continue
		}
		if err := a.saveMachineInfo(ctx, machineId); err != nil {
			resultMap[r.Unit] = err
		}
	}

	result.Results = make([]params.ErrorResult, len(args.Entities))
	for i, id := range ids {
		if err, ok := resultMap[id]; ok {
			result.Results[i].Error = apiservererrors.ServerError(err)
		} else {
			result.Results[i].Error =
				apiservererrors.ServerError(errors.NotFoundf("unit %q", args.Entities[i].Tag))
		}
	}

	return result, nil
}

func (a *API) saveMachineInfo(ctx context.Context, machineId string) error {
	// This is temporary - just insert the machine id all al the parent ones.
	for machineId != "" {
		if err := a.machineSaver.Save(ctx, machineId); err != nil {
			return errors.Annotatef(err, "saving info for machine %q", machineId)
		}
		parent := names.NewMachineTag(machineId).Parent()
		if parent == nil {
			break
		}
		machineId = parent.Id()
	}
	return nil
}

// WatchUnitAssignments returns a strings watcher that is notified when new unit
// assignments are added to the db.
func (a *API) WatchUnitAssignments(ctx context.Context) (params.StringsWatchResult, error) {
	watch := a.st.WatchForUnitAssignment()
	if changes, ok := <-watch.Changes(); ok {
		return params.StringsWatchResult{
			StringsWatcherId: a.res.Register(watch),
			Changes:          changes,
		}, nil
	}
	return params.StringsWatchResult{}, watcher.EnsureErr(watch)
}

// SetAgentStatus will set status for agents of Units passed in args, if one
// of the args is not an Unit it will fail.
func (a *API) SetAgentStatus(ctx context.Context, args params.SetStatus) (params.ErrorResults, error) {
	return a.statusSetter.SetStatus(ctx, args)
}
