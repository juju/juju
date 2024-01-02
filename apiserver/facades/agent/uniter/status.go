// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// StatusAPI is the uniter part that deals with setting/getting
// status from different entities, this particular separation from
// base is because we have a shim to support unit/agent split.
type StatusAPI struct {
	st                *state.State
	model             CachedModel
	leadershipChecker leadership.Checker

	agentSetter       *common.StatusSetter
	unitSetter        *common.StatusSetter
	unitGetter        *common.StatusGetter
	applicationSetter *common.ApplicationStatusSetter
	getCanModify      common.GetAuthFunc
}

// CachedModel represents the methods that the StatusAPI needs on a
// model from the model cache.
type CachedModel interface {
	Application(string) (CachedApplication, error)
}

// CachedApplication represents the methods that the StatusAPI needs on
// an application from the model cache.
type CachedApplication interface {
	Status() status.StatusInfo
}

// NewStatusAPI creates a new server-side Status setter API facade.
func NewStatusAPI(st *state.State, model CachedModel, getCanModify common.GetAuthFunc, leadershipChecker leadership.Checker) *StatusAPI {
	// TODO(fwereade): so *all* of these have exactly the same auth
	// characteristics? I think not.
	unitSetter := common.NewStatusSetter(st, getCanModify)
	unitGetter := common.NewStatusGetter(st, getCanModify)
	applicationSetter := common.NewApplicationStatusSetter(st, getCanModify, leadershipChecker)
	agentSetter := common.NewStatusSetter(&common.UnitAgentFinder{st}, getCanModify)
	return &StatusAPI{
		st:                st,
		model:             model,
		leadershipChecker: leadershipChecker,
		agentSetter:       agentSetter,
		unitSetter:        unitSetter,
		unitGetter:        unitGetter,
		applicationSetter: applicationSetter,
		getCanModify:      getCanModify,
	}
}

// SetStatus will set status for a entities passed in args. If the entity is
// a Unit it will instead set status to its agent, to emulate backwards
// compatibility.
func (s *StatusAPI) SetStatus(args params.SetStatus) (params.ErrorResults, error) {
	return s.SetAgentStatus(args)
}

// SetAgentStatus will set status for agents of Units passed in args, if one
// of the args is not an Unit it will fail.
func (s *StatusAPI) SetAgentStatus(args params.SetStatus) (params.ErrorResults, error) {
	return s.agentSetter.SetStatus(args)
}

// SetUnitStatus sets status for all elements passed in args, the difference
// with SetStatus is that if an entity is a Unit it will set its status instead
// of its agent.
func (s *StatusAPI) SetUnitStatus(args params.SetStatus) (params.ErrorResults, error) {
	return s.unitSetter.SetStatus(args)
}

// SetApplicationStatus sets the status for all the Applications in args if the given Unit is
// the leader.
func (s *StatusAPI) SetApplicationStatus(args params.SetStatus) (params.ErrorResults, error) {
	return s.applicationSetter.SetStatus(args)
}

// UnitStatus returns the workload status information for the unit.
func (s *StatusAPI) UnitStatus(args params.Entities) (params.StatusResults, error) {
	return s.unitGetter.Status(args)
}

// ApplicationStatus returns the status of the Applications and its workloads
// if the given unit is the leader.
func (s *StatusAPI) ApplicationStatus(args params.Entities) (params.ApplicationStatusResults, error) {
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
		unitId := unitTag.Id()

		// Now we have the unit, we can get the application that should have been
		// specified in the first place...
		applicationId, err := names.UnitApplication(unitId)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		application, err := s.st.Application(applicationId)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		// ...so we can check the unit's application leadership...
		token := s.leadershipChecker.LeadershipCheck(applicationId, unitId)
		if err := token.Check(); err != nil {
			// TODO(fwereade) this should probably be ErrPerm in certain cases,
			// but I don't think I implemented an exported ErrNotLeader.
			// I should have done, though.
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		result.Results[i] = s.getAppAndUnitStatus(application)
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

func (s *StatusAPI) getAppAndUnitStatus(application *state.Application) params.ApplicationStatusResult {
	// If for some reason the application isn't yet in the cache, then
	// it has an unknown status.
	result := params.ApplicationStatusResult{
		Units: make(map[string]params.StatusResult),
	}
	appStatus := status.StatusInfo{
		Status: status.Unknown,
	}
	app, err := s.model.Application(application.Name())
	if err == nil {
		appStatus = app.Status()
	}
	result.Application = s.toStatusResult(appStatus)

	unitStatuses, err := application.UnitStatuses()
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
	} else {
		for name, status := range unitStatuses {
			result.Units[name] = s.toStatusResult(status)
		}
	}
	return result
}

type cacheShim struct {
	model *cache.Model
}

func (c cacheShim) Application(name string) (CachedApplication, error) {
	app, err := c.model.Application(name)
	if err != nil {
		return nil, err
	}
	return &app, nil
}
