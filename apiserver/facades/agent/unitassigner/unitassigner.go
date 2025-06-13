// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitassigner

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

// assignerState defines the state methods this facade needs, so they can be mocked
// for testing.
type assignerState interface {
	WatchForUnitAssignment() state.StringsWatcher
	AssignStagedUnits(allSpaces network.SpaceInfos, ids []string) ([]state.UnitAssignmentResult, error)
	AssignedMachineId(unit string) (string, error)
}

// MachineService is the interface that is used to interact with the machine
// domain.
type MachineService interface {
	CreateMachine(context.Context, machine.Name, *string) (machine.UUID, error)
}

// NetworkService is the interface that is used to interact with the
// network spaces/subnets.
type NetworkService interface {
	// GetAllSpaces returns all spaces for the model.
	GetAllSpaces(ctx context.Context) (network.SpaceInfos, error)
}

// ApplicationService is the interface that is used to interact with the
// application domain.
type StatusService interface {
	// SetUnitAgentStatus sets the status of the agent of the given unit.
	SetUnitAgentStatus(ctx context.Context, name unit.Name, status status.StatusInfo) error
}

// API implements the functionality for assigning units to machines.
type API struct {
	st             assignerState
	machineService MachineService
	networkService NetworkService
	statusService  StatusService
	clock          clock.Clock
	res            facade.Resources
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

	allSpaces, err := a.networkService.GetAllSpaces(ctx)
	if err != nil {
		return result, errors.Trace(err)
	}

	res, err := a.st.AssignStagedUnits(allSpaces, ids)
	if err != nil {
		return result, apiservererrors.ServerError(err)
	}

	machineToUnitMap := make(map[string][]unit.Name)

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
		machineToUnitMap[machineId] = append(machineToUnitMap[machineId], unit.Name(r.Unit))
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

func (a *API) saveMachineInfo(ctx context.Context, machineName string) error {
	// This is temporary - just insert the machine id and all the parent ones.
	for machineName != "" {
		_, err := a.machineService.CreateMachine(ctx, machine.Name(machineName), nil)
		// The machine might already exist e.g. if we are adding a subordinate
		// unit to an already existing machine. In this case, just continue
		// without error.
		if err != nil && !errors.Is(err, machineerrors.MachineAlreadyExists) {
			return errors.Annotatef(err, "saving info for machine %q", machineName)
		}
		parent := names.NewMachineTag(machineName).Parent()
		if parent == nil {
			break
		}
		machineName = parent.Id()
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
// of the args is not an Unit it will add the error to the result and continue,
// until all the args are processed.
func (a *API) SetAgentStatus(ctx context.Context, args params.SetStatus) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}

	for i, arg := range args.Entities {
		tag, err := names.ParseUnitTag(arg.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		if err := a.statusService.SetUnitAgentStatus(ctx, unit.Name(tag.Id()), status.StatusInfo{
			Status:  status.Status(arg.Status),
			Message: arg.Info,
			Data:    arg.Data,
			Since:   ptr(a.clock.Now()),
		}); errors.Is(err, applicationerrors.UnitNotFound) {
			results.Results[i].Error = apiservererrors.ServerError(errors.NotFoundf("unit %q", tag.Id()))
		} else if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
		}
	}

	return results, nil
}

func ptr[T any](v T) *T {
	return &v
}
