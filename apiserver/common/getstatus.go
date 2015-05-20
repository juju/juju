// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"

	"github.com/juju/names"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

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

type ServiceStatusGetter struct {
	st           state.UnitFinder
	getcanAccess GetAuthFunc
}

func NewServiceStatusGetter(st state.UnitFinder, getcanAccess GetAuthFunc) *ServiceStatusGetter {
	return &ServiceStatusGetter{
		st:           st,
		getcanAccess: getcanAccess,
	}
}

// Status returns the status of each given entity.
func (s *ServiceStatusGetter) Status(args params.ServiceUnits) (params.ServiceStatusResults, error) {
	results := params.ServiceStatusResults{
		Results: make([]params.ServiceStatusResult, len(args.ServiceUnits)),
	}
	canAccess, err := s.getcanAccess()
	if err != nil {
		return params.ServiceStatusResults{}, err
	}

	for i, serviceUnit := range args.ServiceUnits {
		//TODO(perrito666) IsLeader check for unit.
		unit, err := s.st.Unit(serviceUnit.UnitName)
		if err != nil {
			results.Results[i].Error = ServerError(err)
			continue
		}

		if !canAccess(unit.Tag()) {
			results.Results[i].Error = ServerError(ErrPerm)
			continue
		}

		service, err := unit.Service()
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

		unitStatuses, err := service.MembersStatus()
		if err != nil {
			results.Results[i].Error = ServerError(err)
			continue
		}
		results.Results[i].Units.Results = make([]params.NamedStatusResult, len(unitStatuses))
		for ri, r := range unitStatuses {
			ur := params.NamedStatusResult{
				Tag: r.Tag,
				StatusResult: params.StatusResult{
					Status: params.Status(r.StatusInfo.Status),
					Info:   r.StatusInfo.Message,
					Data:   r.StatusInfo.Data,
					Since:  r.StatusInfo.Since,
				},
			}
			results.Results[i].Units.Results[ri] = ur
		}
	}
	return results, nil
}
