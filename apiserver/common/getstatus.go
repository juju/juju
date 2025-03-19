// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"
	"fmt"

	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
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
		result.Error = apiservererrors.ServerError(err)
		return result
	}
	switch getter := entity.(type) {
	// Use UnitStatusSetter for getting unit status.
	case *state.Unit:
		return params.StatusResult{Error: apiservererrors.ServerError(apiservererrors.ErrPerm)}
	// Use the domain to get application statuses
	case *state.Application:
		return params.StatusResult{Error: apiservererrors.ServerError(apiservererrors.ErrPerm)}
	case status.StatusGetter:
		statusInfo, err := getter.Status()
		result.Status = statusInfo.Status.String()
		result.Info = statusInfo.Message
		result.Data = statusInfo.Data
		result.Since = statusInfo.Since
		result.Error = apiservererrors.ServerError(err)
	default:
		result.Error = apiservererrors.ServerError(apiservererrors.NotSupportedError(tag, fmt.Sprintf("getting status, %T", getter)))
	}
	return result
}

// Status returns the status of each given entity.
func (s *StatusGetter) Status(ctx context.Context, args params.Entities) (params.StatusResults, error) {
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
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if !canAccess(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		result.Results[i] = s.getEntityStatus(tag)
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
