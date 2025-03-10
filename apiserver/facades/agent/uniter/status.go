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
	coreapplication "github.com/juju/juju/core/application"
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
	leadershipChecker leadership.Checker

	applicationService ApplicationService

	unitSetter        *common.UnitStatusSetter
	unitGetter        *common.UnitStatusGetter
	applicationSetter *common.ApplicationStatusSetter
	getCanModify      common.GetAuthFunc
}

// NewStatusAPI creates a new server-side Status setter API facade.
func NewStatusAPI(st *state.State, applicationService ApplicationService, getCanModify common.GetAuthFunc, leadershipChecker leadership.Checker, clock clock.Clock) *StatusAPI {
	// TODO(fwereade): so *all* of these have exactly the same auth
	// characteristics? I think not.
	unitSetter := common.NewUnitStatusSetter(applicationService, clock, getCanModify)
	unitGetter := common.NewUnitStatusGetter(applicationService, clock, getCanModify)
	applicationSetter := common.NewApplicationStatusSetter(st, getCanModify, leadershipChecker)
	return &StatusAPI{
		leadershipChecker:  leadershipChecker,
		applicationService: applicationService,
		unitSetter:         unitSetter,
		unitGetter:         unitGetter,
		applicationSetter:  applicationSetter,
		getCanModify:       getCanModify,
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

		if err := s.applicationService.SetUnitAgentStatus(ctx, unit.Name(tag.Id()), &status.StatusInfo{
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

		// We "know" that arg.Tag is either the calling unit or its application
		// (because getCanAccess is authUnitOrApplication, and we'll fail out if
		// it isn't); and, in practice, it's always going to be the calling
		// unit (because, /sigh, we don't actually use application tags to refer
		// to applications in this method).
		tag, err := names.ParseTag(arg.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if !canAccess(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		unitTag, ok := tag.(names.UnitTag)
		if !ok {
			// No matter what the canAccess says, if this entity is not
			// a unit, we say "NO".
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		unitName := unitTag.Id()

		// Now we have the unit, we can get the application that should have been
		// specified in the first place...
		applicationName, err := names.UnitApplication(unitName)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		// ...so we can check the unit's application leadership...
		token := s.leadershipChecker.LeadershipCheck(applicationName, unitName)
		if err := token.Check(); err != nil {
			// TODO(fwereade) this should probably be ErrPerm in certain cases,
			// but I don't think I implemented an exported ErrNotLeader.
			// I should have done, though.
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		applicationID, err := s.applicationService.GetApplicationIDByName(ctx, applicationName)
		if errors.Is(err, applicationerrors.ApplicationNotFound) {
			result.Results[i].Error = apiservererrors.ServerError(errors.NotFoundf("application %q", applicationName))
			continue
		} else if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		result.Results[i] = s.getAppAndUnitStatus(ctx, applicationID)
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

func (s *StatusAPI) getAppAndUnitStatus(ctx context.Context, applicationId coreapplication.ID) params.ApplicationStatusResult {
	result := params.ApplicationStatusResult{
		Units: make(map[string]params.StatusResult),
	}
	appStatus := status.StatusInfo{Status: status.Unknown}
	aStatus, err := s.applicationService.GetApplicationDisplayStatus(ctx, applicationId)
	if err == nil && aStatus != nil {
		appStatus = *aStatus
	}
	result.Application = s.toStatusResult(appStatus)

	unitStatuses, err := s.applicationService.GetUnitWorkloadStatusesForApplication(ctx, applicationId)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		result.Error = apiservererrors.ServerError(errors.NotFoundf("application %q", applicationId))
	} else if err != nil {
		result.Error = apiservererrors.ServerError(err)
	} else {
		for name, status := range unitStatuses {
			result.Units[name.String()] = s.toStatusResult(status)
		}
	}
	return result
}
