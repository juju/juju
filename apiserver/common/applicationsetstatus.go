// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"
	"time"

	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// ApplicationStatusSetter implements a SetApplicationStatus method to be
// used by facades that can change an application status.
// This is only slightly less evil than ApplicationStatusGetter. We have
// StatusSetter already; all this does is set the status for the wrong
// entity, and render the auth so confused as to be ~worthless.
type ApplicationStatusSetter struct {
	leadershipChecker leadership.Checker
	st                *state.State
	getCanModify      GetAuthFunc
}

// NewApplicationStatusSetter returns a ServiceStatusSetter.
func NewApplicationStatusSetter(st *state.State, getCanModify GetAuthFunc, leadershipChecker leadership.Checker) *ApplicationStatusSetter {
	return &ApplicationStatusSetter{
		leadershipChecker: leadershipChecker,
		st:                st,
		getCanModify:      getCanModify,
	}
}

// SetStatus sets the status on the service given by the unit in args if the unit is the leader.
func (s *ApplicationStatusSetter) SetStatus(ctx context.Context, args params.SetStatus) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}

	canModify, err := s.getCanModify()
	if err != nil {
		return params.ErrorResults{}, err
	}

	for i, arg := range args.Entities {

		// TODO(fwereade): the auth is basically nonsense, and basically only
		// works by coincidence. Read carefully.

		// We "know" that arg.Tag is either the calling unit or its service
		// (because getCanModify is authUnitOrService, and we'll fail out if
		// it isn't); and, in practice, it's always going to be the calling
		// unit (because, /sigh, we don't actually use service tags to refer
		// to services in this method).
		tag, err := names.ParseTag(arg.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if !canModify(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		unitTag, ok := tag.(names.UnitTag)
		if !ok {
			// No matter what the canModify says, if this entity is not
			// a unit, we say "NO".
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		unitId := unitTag.Id()

		// Now we have the unit, we can get the service that should have been
		// specified in the first place...
		serviceId, err := names.UnitApplication(unitId)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		service, err := s.st.Application(serviceId)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		// ...and set the status, conditional on the unit being (and remaining)
		// service leader.
		token := s.leadershipChecker.LeadershipCheck(serviceId, unitId)

		if err := token.Check(); err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		// TODO(perrito666) 2016-05-02 lp:1558657
		now := time.Now()
		sInfo := status.StatusInfo{
			Status:  status.Status(arg.Status),
			Message: arg.Info,
			Data:    arg.Data,
			Since:   &now,
		}
		if err := service.SetStatus(sInfo); err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
		}

	}
	return result, nil
}
