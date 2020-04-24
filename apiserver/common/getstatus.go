// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
	"github.com/juju/names/v4"
)

// StatusGetter implements a common Status method for use by
// various facades.
type StatusGetter struct {
	st           state.EntityFinder
	getCanAccess GetAuthFunc
}

// NewStatusGetter returns a new StatusGetter. The GetAuthFunc will be
// used on each invocation of Status to determine current
// permissions.
func NewStatusGetter(st state.EntityFinder, getCanAccess GetAuthFunc) *StatusGetter {
	return &StatusGetter{
		st:           st,
		getCanAccess: getCanAccess,
	}
}

func (s *StatusGetter) getEntityStatus(tag names.Tag) params.StatusResult {
	var result params.StatusResult
	entity, err := s.st.FindEntity(tag)
	if err != nil {
		result.Error = ServerError(err)
		return result
	}
	switch getter := entity.(type) {
	case status.StatusGetter:
		statusInfo, err := getter.Status()
		result.Status = statusInfo.Status.String()
		result.Info = statusInfo.Message
		result.Data = statusInfo.Data
		result.Since = statusInfo.Since
		result.Error = ServerError(err)
	default:
		result.Error = ServerError(NotSupportedError(tag, fmt.Sprintf("getting status, %T", getter)))
	}
	return result
}

// Status returns the status of each given entity.
func (s *StatusGetter) Status(args params.Entities) (params.StatusResults, error) {
	result := params.StatusResults{
		Results: make([]params.StatusResult, len(args.Entities)),
	}
	canAccess, err := s.getCanAccess()
	if err != nil {
		return params.StatusResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = ServerError(err)
			continue
		}
		if !canAccess(tag) {
			result.Results[i].Error = ServerError(ErrPerm)
			continue
		}
		result.Results[i] = s.getEntityStatus(tag)
	}
	return result, nil
}

// ApplicationStatusGetter is a StatusGetter for combined application and unit statuses.
// TODO(fwereade) this is completely evil and should never have been created.
// We have a perfectly adequate StatusGetter already, that accepts bulk args;
// all this does is break the user model, break the api model, and lie about
// unit statuses).
type ApplicationStatusGetter struct {
	leadershipChecker leadership.Checker
	st                *state.State
	getCanAccess      GetAuthFunc
}

// NewApplicationStatusGetter returns a ApplicationStatusGetter.
func NewApplicationStatusGetter(st *state.State, getCanAccess GetAuthFunc, leadershipChecker leadership.Checker) *ApplicationStatusGetter {
	return &ApplicationStatusGetter{
		leadershipChecker: leadershipChecker,
		st:                st,
		getCanAccess:      getCanAccess,
	}
}

// Status returns the status of the Application for each given Unit tag.
func (s *ApplicationStatusGetter) Status(args params.Entities) (params.ApplicationStatusResults, error) {
	result := params.ApplicationStatusResults{
		Results: make([]params.ApplicationStatusResult, len(args.Entities)),
	}
	canAccess, err := s.getCanAccess()
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
			result.Results[i].Error = ServerError(err)
			continue
		}
		if !canAccess(tag) {
			result.Results[i].Error = ServerError(ErrPerm)
			continue
		}
		unitTag, ok := tag.(names.UnitTag)
		if !ok {
			// No matter what the canAccess says, if this entity is not
			// a unit, we say "NO".
			result.Results[i].Error = ServerError(ErrPerm)
			continue
		}
		unitId := unitTag.Id()

		// Now we have the unit, we can get the application that should have been
		// specified in the first place...
		applicationId, err := names.UnitApplication(unitId)
		if err != nil {
			result.Results[i].Error = ServerError(err)
			continue
		}
		application, err := s.st.Application(applicationId)
		if err != nil {
			result.Results[i].Error = ServerError(err)
			continue
		}

		// ...so we can check the unit's application leadership...
		token := s.leadershipChecker.LeadershipCheck(applicationId, unitId)
		if err := token.Check(0, nil); err != nil {
			// TODO(fwereade) this should probably be ErrPerm is certain cases,
			// but I don't think I implemented an exported ErrNotLeader. I
			// should have done, though.
			result.Results[i].Error = ServerError(err)
			continue
		}

		// ...and collect the results.
		applicationStatus, unitStatuses, err := application.ApplicationAndUnitsStatus()
		if err != nil {
			result.Results[i].Application.Error = ServerError(err)
			result.Results[i].Error = ServerError(err)
			continue
		}
		result.Results[i].Application.Status = applicationStatus.Status.String()
		result.Results[i].Application.Info = applicationStatus.Message
		result.Results[i].Application.Data = applicationStatus.Data
		result.Results[i].Application.Since = applicationStatus.Since

		result.Results[i].Units = make(map[string]params.StatusResult, len(unitStatuses))
		for uTag, r := range unitStatuses {
			ur := params.StatusResult{
				Status: r.Status.String(),
				Info:   r.Message,
				Data:   r.Data,
				Since:  r.Since,
			}
			result.Results[i].Units[uTag] = ur
		}
	}
	return result, nil
}

// EntityStatusFromState converts a state.StatusInfo into a params.EntityStatus.
func EntityStatusFromState(statusInfo status.StatusInfo) params.EntityStatus {
	return params.EntityStatus{
		Status: statusInfo.Status,
		Info:   statusInfo.Message,
		Data:   statusInfo.Data,
		Since:  statusInfo.Since,
	}
}
