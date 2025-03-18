// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// StatusAPI is the uniter part that deals with setting/getting
// status from different entities, this particular separation from
// base is because we have a shim to support unit/agent split.
type StatusAPI struct {
	statusService StatusService

	unitSetter        *common.UnitStatusSetter
	unitGetter        *common.UnitStatusGetter
	applicationSetter *common.ApplicationStatusSetter
	getCanModify      common.GetAuthFunc
}

// NewStatusAPI creates a new server-side Status setter API facade.
func NewStatusAPI(st *state.State, statusService StatusService, getCanModify common.GetAuthFunc, leadershipChecker leadership.Checker, clock clock.Clock) *StatusAPI {
	// TODO(fwereade): so *all* of these have exactly the same auth
	// characteristics? I think not.
	unitSetter := common.NewUnitStatusSetter(statusService, clock, getCanModify)
	unitGetter := common.NewUnitStatusGetter(statusService, clock, getCanModify)
	applicationSetter := common.NewApplicationStatusSetter(st, getCanModify, leadershipChecker)
	return &StatusAPI{
		statusService:     statusService,
		unitSetter:        unitSetter,
		unitGetter:        unitGetter,
		applicationSetter: applicationSetter,
		getCanModify:      getCanModify,
	}
}

// SetStatus will set status for a entities passed in args. If the entity is
// a Unit it will instead set status to its agent, to emulate backwards
// compatibility.
func (s *StatusAPI) SetStatus(ctx context.Context, args params.SetStatus) (params.ErrorResults, error) {
	return s.SetAgentStatus(ctx, args)
}

// SetAgentStatus will set status for agents of Units passed in args, if one
// of the args is not an Unit it will fail.
func (s *StatusAPI) SetAgentStatus(ctx context.Context, args params.SetStatus) (params.ErrorResults, error) {
	canModify, err := s.getCanModify()
	if err != nil {
		return params.ErrorResults{}, err
	}

	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		tag, err := names.ParseUnitTag(arg.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		if !canModify(tag) {
			results.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		if err := s.statusService.SetUnitAgentStatus(ctx, unit.Name(tag.Id()), &status.StatusInfo{
			Status:  status.Status(arg.Status),
			Message: arg.Info,
			Data:    arg.Data,
		}); errors.Is(err, applicationerrors.UnitNotFound) {
			results.Results[i].Error = apiservererrors.ServerError(errors.NotFoundf("unit %q", tag.Id()))
		} else if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
	}
	return results, nil
}

// SetUnitStatus sets status for all elements passed in args, the difference
// with SetStatus is that if an entity is a Unit it will set its status instead
// of its agent.
func (s *StatusAPI) SetUnitStatus(ctx context.Context, args params.SetStatus) (params.ErrorResults, error) {
	return s.unitSetter.SetStatus(ctx, args)
}

// SetApplicationStatus sets the status for all the Applications in args if the given Unit is
// the leader.
func (s *StatusAPI) SetApplicationStatus(ctx context.Context, args params.SetStatus) (params.ErrorResults, error) {
	return s.applicationSetter.SetStatus(ctx, args)
}

// UnitStatus returns the workload status information for the unit.
func (s *StatusAPI) UnitStatus(ctx context.Context, args params.Entities) (params.StatusResults, error) {
	return s.unitGetter.Status(ctx, args)
}

// ApplicationStatus returns the status of the Applications and its workloads
// if the given unit is the leader.
func (s *StatusAPI) ApplicationStatus(ctx context.Context, args params.Entities) (params.ApplicationStatusResults, error) {
	result := params.ApplicationStatusResults{
		Results: make([]params.ApplicationStatusResult, len(args.Entities)),
	}
	canAccess, err := s.getCanModify()
	if err != nil {
		return params.ApplicationStatusResults{}, err
	}

	for i, arg := range args.Entities {
		// TODO(fwereade): the auth is basically nonsense, and basically only
		// works by coincidence (and is happening at the wrong layer anyway).
		// Read carefully.
		unitTag, err := names.ParseUnitTag(arg.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if !canAccess(unitTag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		unitName, err := unit.NewName(unitTag.Id())
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		appStatus, unitStatuses, err := s.statusService.GetApplicationAndUnitStatusesForUnitWithLeader(ctx, unitName)
		if errors.Is(err, applicationerrors.ApplicationNotFound) {
			result.Results[i].Error = apiservererrors.ServerError(errors.NotFoundf("application %q", unitName.Application()))
			continue
		} else if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		res := params.ApplicationStatusResult{
			Application: s.toStatusResult(*appStatus),
			Units:       make(map[string]params.StatusResult),
		}
		for name, status := range unitStatuses {
			res.Units[name.String()] = s.toStatusResult(status)
		}
		result.Results[i] = res
	}
	return result, nil
}

func (s *StatusAPI) toStatusResult(i status.StatusInfo) params.StatusResult {
	return params.StatusResult{
		Status: i.Status.String(),
		Info:   i.Message,
		Data:   i.Data,
		Since:  i.Since,
	}
}
