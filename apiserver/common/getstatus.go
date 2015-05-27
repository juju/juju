// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/leadership"
	"github.com/juju/juju/lease"
	"github.com/juju/juju/state"
)

var ErrIsNotLeader = errors.Errorf("this unit is not the leader")

// StatusGetter implements a common Status method for use by
// various facades.
type StatusGetter struct {
	st           state.EntityFinder
	getcanAccess GetAuthFunc
}

// NewStatusGetter returns a new StatusGetter. The GetAuthFunc will be
// used on each invocation of Status to determine current
// permissions.
func NewStatusGetter(st state.EntityFinder, getcanAccess GetAuthFunc) *StatusGetter {
	return &StatusGetter{
		st:           st,
		getcanAccess: getcanAccess,
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
	canAccess, err := s.getcanAccess()
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
type ServiceStatusGetter struct {
	st           state.EntityFinder
	getcanAccess GetAuthFunc
}

// NewServiceStatusGetter returns a ServiceStatusGetter.
func NewServiceStatusGetter(st state.EntityFinder, getcanAccess GetAuthFunc) *ServiceStatusGetter {
	return &ServiceStatusGetter{
		st:           st,
		getcanAccess: getcanAccess,
	}
}

// StatusService interface represents an Entity that can return Status for itself
// and its Units.
type StatusService interface {
	state.Entity
	Status() (state.StatusInfo, error)
	UnitsStatus() (map[string]state.StatusInfo, error)
	SetStatus(state.Status, string, map[string]interface{}) error
}

type serviceGetter func(state.EntityFinder, string) (StatusService, error)

type isLeaderFunc func(state.EntityFinder, string) (bool, error)

func getUnit(st state.EntityFinder, unitTag string) (*state.Unit, error) {
	tag, err := names.ParseUnitTag(unitTag)
	if err != nil {
		return nil, err
	}
	entity, err := st.FindEntity(tag)
	if err != nil {

		return nil, err
	}
	unit, ok := entity.(*state.Unit)
	if !ok {
		return nil, err
	}
	return unit, nil
}

func serviceFromUnitTag(st state.EntityFinder, unitTag string) (StatusService, error) {
	unit, err := getUnit(st, unitTag)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot obtain unit %q to obtain service", unitTag)
	}
	var service StatusService
	service, err = unit.Service()
	if err != nil {
		return nil, err
	}
	return service, nil
}

func isLeader(st state.EntityFinder, unitTag string) (bool, error) {
	unit, err := getUnit(st, unitTag)
	if err != nil {
		return false, errors.Annotatef(err, "cannot obtain unit %q to check leadership", unitTag)
	}
	service, err := unit.Service()
	if err != nil {
		return false, err
	}

	leaseManager := lease.Manager()
	leadershipManager := leadership.NewLeadershipManager(leaseManager)
	return leadershipManager.Leader(service.Name(), unit.Name()), nil
}

// Status returns the status of each given entity.
func (s *ServiceStatusGetter) Status(args params.Entities) (params.ServiceStatusResults, error) {
	return serviceStatus(s, args, serviceFromUnitTag, isLeader)
}

func serviceStatus(s *ServiceStatusGetter, args params.Entities, getService serviceGetter, isLeaderCheck isLeaderFunc) (params.ServiceStatusResults, error) {
	results := params.ServiceStatusResults{
		Results: make([]params.ServiceStatusResult, len(args.Entities)),
	}
	canAccess, err := s.getcanAccess()
	if err != nil {
		return params.ServiceStatusResults{}, err
	}

	for i, serviceUnit := range args.Entities {
		leader, err := isLeaderCheck(s.st, serviceUnit.Tag)
		if err != nil {
			results.Results[i].Error = ServerError(err)
			continue
		}
		if !leader {
			results.Results[i].Error = ServerError(ErrIsNotLeader)
			continue
		}
		var service StatusService
		service, err = getService(s.st, serviceUnit.Tag)
		if err != nil {
			results.Results[i].Error = ServerError(err)
			continue
		}
		if !canAccess(service.Tag()) {
			results.Results[i].Error = ServerError(ErrPerm)
			continue
		}

		serviceStatus, err := service.Status()
		if err != nil {
			results.Results[i].Service.Error = ServerError(err)
			results.Results[i].Error = ServerError(err)
			continue
		}
		results.Results[i].Service.Status = params.Status(serviceStatus.Status)
		results.Results[i].Service.Info = serviceStatus.Message
		results.Results[i].Service.Data = serviceStatus.Data
		results.Results[i].Service.Since = serviceStatus.Since

		unitStatuses, err := service.UnitsStatus()
		if err != nil {
			results.Results[i].Error = ServerError(err)
			continue
		}
		results.Results[i].Units = make(map[string]params.StatusResult, len(unitStatuses))
		for uTag, r := range unitStatuses {
			ur := params.StatusResult{
				Status: params.Status(r.Status),
				Info:   r.Message,
				Data:   r.Data,
				Since:  r.Since,
			}
			results.Results[i].Units[uTag] = ur
		}
	}
	return results, nil
}
