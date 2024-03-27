// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v5"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/leadership"
	corelogger "github.com/juju/juju/core/logger"
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

		// TODO(fwereade) pass token into SetStatus instead of checking here.
		if err := token.Check(); err != nil {
			// TODO(fwereade) this should probably be apiservererrors.ErrPerm in certain cases,
			// but I don't think I implemented an exported ErrNotLeader.
			// I should have done, though.
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
		if err := service.SetStatus(sInfo, nil); err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
		}

	}
	return result, nil
}

func NewStatusHistoryRecorder(sourceController string, logger corelogger.Logger) status.StatusHistoryRecorder {
	return func(statusKind string, statusId string, st status.Status, statusInfo string) {
		if statusKind == "machine-lxd-profile" && st != status.Idle {
			return
		}
		_, file, line, ok := runtime.Caller(1)
		if !ok {
			file = "???"
			line = 0
		}
		labels := map[string]string{
			"domain": "status",
			"kind":   statusKind,
			"id":     statusId,
			"value":  st.String(),
		}
		_ = logger.Log([]corelogger.LogRecord{{
			Time:     time.Now(),
			Level:    loggo.INFO,
			Module:   "juju.status",
			Location: fmt.Sprintf("%s:%d", filepath.Base(file), line),
			Entity:   sourceController,
			Message:  statusInfo,
			Labels:   labels,
		}})
	}

}

// StatusSetter implements a common SetStatus method for use by
// various facades.
type StatusSetter struct {
	st            state.EntityFinder
	getCanModify  GetAuthFunc
	recordHistory status.StatusHistoryRecorder
}

// NewStatusSetter returns a new StatusSetter. The GetAuthFunc will be
// used on each invocation of SetStatus to determine current
// permissions.
func NewStatusSetter(st state.EntityFinder, getCanModify GetAuthFunc, recordHistory status.StatusHistoryRecorder) *StatusSetter {
	return &StatusSetter{
		st:            st,
		getCanModify:  getCanModify,
		recordHistory: recordHistory,
	}
}

func (s *StatusSetter) setEntityStatus(tag names.Tag, entityStatus status.Status, info string, data map[string]interface{}, updated *time.Time) error {
	entity, err := s.st.FindEntity(tag)
	if err != nil {
		return err
	}
	switch entity := entity.(type) {
	case *state.Application:
		return apiservererrors.ErrPerm
	case status.StatusSetter:
		sInfo := status.StatusInfo{
			Status:  entityStatus,
			Message: info,
			Data:    data,
			Since:   updated,
		}
		return entity.SetStatus(sInfo, s.recordHistory)
	default:
		return apiservererrors.NotSupportedError(tag, fmt.Sprintf("setting status, %T", entity))
	}
}

// SetStatus sets the status of each given entity.
func (s *StatusSetter) SetStatus(ctx context.Context, args params.SetStatus) (params.ErrorResults, error) {
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
	// TODO(perrito666) 2016-05-02 lp:1558657
	now := time.Now()
	for i, arg := range args.Entities {
		tag, err := names.ParseTag(arg.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		err = apiservererrors.ErrPerm
		if canModify(tag) {
			err = s.setEntityStatus(tag, status.Status(arg.Status), arg.Info, arg.Data, &now)
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// UnitAgentFinder is a state.EntityFinder that finds unit agents.
type UnitAgentFinder struct {
	state.EntityFinder
}

// FindEntity implements state.EntityFinder and returns unit agents.
func (ua *UnitAgentFinder) FindEntity(tag names.Tag) (state.Entity, error) {
	_, ok := tag.(names.UnitTag)
	if !ok {
		return nil, errors.Errorf("unsupported tag %T", tag)
	}
	entity, err := ua.EntityFinder.FindEntity(tag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// this returns a state.Unit, but for testing we just cast to the minimal
	// interface we need.
	return entity.(hasAgent).Agent(), nil
}

type hasAgent interface {
	Agent() *state.UnitAgent
}
