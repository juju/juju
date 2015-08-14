// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

// ErrIsNotLeader is an error for operations that require for a
// unit to be the leader but it was not the case.
// TODO(fwereade) why do we have an alternative implementation of ErrPerm
// that is exported (implying people will be able to meaningfully check it)
// but not actually handled anywhere or converted into an error code by the
// api server?
var ErrIsNotLeader = errors.Errorf("this unit is not the leader")

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
	case state.StatusGetter:
		statusInfo, err := getter.Status()
		result.Status = params.Status(statusInfo.Status)
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

// ServiceStatusGetter is a StatusGetter for combined service and unit statuses.
// TODO(fwereade) this is completely evil and should never have been created.
// We have a perfectly adequate StatusGetter already, that accepts bulk args;
// all this does is break the user model, break the api model, and lie about
// unit statuses).
type ServiceStatusGetter struct {
	st           *state.State
	getCanAccess GetAuthFunc
}

// NewServiceStatusGetter returns a ServiceStatusGetter.
func NewServiceStatusGetter(st *state.State, getCanAccess GetAuthFunc) *ServiceStatusGetter {
	return &ServiceStatusGetter{
		st:           st,
		getCanAccess: getCanAccess,
	}
}

// Status returns the status of the Service for each given Unit tag.
func (s *ServiceStatusGetter) Status(args params.Entities) (params.ServiceStatusResults, error) {
	result := params.ServiceStatusResults{
		Results: make([]params.ServiceStatusResult, len(args.Entities)),
	}
	canAccess, err := s.getCanAccess()
	if err != nil {
		return params.ServiceStatusResults{}, err
	}

	for i, arg := range args.Entities {
		// TODO(fwereade): the auth is basically nonsense, and basically only
		// works by coincidence (and is happening at the wrong layer anyway).
		// Read carefully.

		// We "know" that arg.Tag is either the calling unit or its service
		// (because getCanAccess is authUnitOrService, and we'll fail out if
		// it isn't); and, in practice, it's always going to be the calling
		// unit (because, /sigh, we don't actually use service tags to refer
		// to services in this method).
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

		// Now we have the unit, we can get the service that should have been
		// specified in the first place...
		serviceId, err := names.UnitService(unitId)
		if err != nil {
			result.Results[i].Error = ServerError(err)
			continue
		}
		service, err := s.st.Service(serviceId)
		if err != nil {
			result.Results[i].Error = ServerError(err)
			continue
		}

		// ...so we can check the unit's service leadership...
		checker := s.st.LeadershipChecker()
		token := checker.LeadershipCheck(serviceId, unitId)
		if err := token.Check(nil); err != nil {
			// TODO(fwereade) this should probably be ErrPerm is certain cases,
			// but I don't think I implemented an exported ErrNotLeader. I
			// should have done, though.
			result.Results[i].Error = ServerError(err)
			continue
		}

		// ...and collect the results.
		serviceStatus, unitStatuses, err := service.ServiceAndUnitsStatus()
		if err != nil {
			result.Results[i].Service.Error = ServerError(err)
			result.Results[i].Error = ServerError(err)
			continue
		}
		result.Results[i].Service.Status = params.Status(serviceStatus.Status)
		result.Results[i].Service.Info = serviceStatus.Message
		result.Results[i].Service.Data = serviceStatus.Data
		result.Results[i].Service.Since = serviceStatus.Since

		result.Results[i].Units = make(map[string]params.StatusResult, len(unitStatuses))
		for uTag, r := range unitStatuses {
			ur := params.StatusResult{
				Status: params.Status(r.Status),
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
func EntityStatusFromState(status state.StatusInfo) params.EntityStatus {
	return params.EntityStatus{
		params.Status(status.Status),
		status.Message,
		status.Data,
		status.Since,
	}
}
