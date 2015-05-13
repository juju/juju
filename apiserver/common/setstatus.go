// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

// ServiceStatusSetter implements a SetServiceStatus method to be
// used by facades that can change a service status.
type ServiceStatusSetter struct {
	st           *state.State
	getCanModify GetAuthFunc
}

func NewServiceStatusSetter(st *state.State, getCanModify GetAuthFunc) *ServiceStatusSetter {
	return &ServiceStatusSetter{
		st:           st,
		getCanModify: getCanModify,
	}
}

func (s *ServiceStatusSetter) SetStatus(args params.SetServiceStatus) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Services)),
	}
	if len(args.Services) == 0 {
		return result, nil
	}
	canModify, err := s.getCanModify()
	if err != nil {
		return params.ErrorResults{}, err
	}
	for i, arg := range args.Services {
		// TODO(perrito666) Check IsLeader for unitTag.
		unit, err := s.st.Unit(arg.UnitName)
		if err != nil {
			result.Results[i].Error = ServerError(err)
			continue
		}
		service, err := unit.Service()
		if err != nil {
			result.Results[i].Error = ServerError(err)
			continue
		}

		if !canModify(unit.Tag()) {
			result.Results[i].Error = ServerError(ErrPerm)
			continue
		}

		if err := service.SetStatus(state.Status(arg.Status), arg.Info, arg.Data); err != nil {
			result.Results[i].Error = ServerError(err)
		}

	}
	return result, nil

}

// StatusSetter implements a common SetStatus method for use by
// various facades.
type StatusSetter struct {
	st           state.EntityFinder
	getCanModify GetAuthFunc
}

// NewStatusSetter returns a new StatusSetter. The GetAuthFunc will be
// used on each invocation of SetStatus to determine current
// permissions.
func NewStatusSetter(st state.EntityFinder, getCanModify GetAuthFunc) *StatusSetter {
	return &StatusSetter{
		st:           st,
		getCanModify: getCanModify,
	}
}

func (s *StatusSetter) setEntityStatus(tag names.Tag, status params.Status, info string, data map[string]interface{}) error {
	entity, err := s.st.FindEntity(tag)
	if err != nil {
		return err
	}
	switch entity := entity.(type) {
	case state.StatusSetter:
		return entity.SetStatus(state.Status(status), info, data)
	default:
		return NotSupportedError(tag, fmt.Sprintf("setting status, %T", entity))
	}
}

// SetStatus sets the status of each given entity.
func (s *StatusSetter) SetStatus(args params.SetStatus) (params.ErrorResults, error) {
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
		tag, err := names.ParseTag(arg.Tag)
		if err != nil {
			result.Results[i].Error = ServerError(ErrPerm)
			continue
		}
		err = ErrPerm
		if canModify(tag) {
			err = s.setEntityStatus(tag, arg.Status, arg.Info, arg.Data)
		}
		result.Results[i].Error = ServerError(err)
	}
	return result, nil
}

func (s *StatusSetter) updateEntityStatusData(tag names.Tag, data map[string]interface{}) error {
	entity0, err := s.st.FindEntity(tag)
	if err != nil {
		return err
	}
	statusGetter, ok := entity0.(state.StatusGetter)
	if !ok {
		return NotSupportedError(tag, "getting status")
	}
	existingStatusInfo, err := statusGetter.Status()
	if err != nil {
		return err
	}
	newData := existingStatusInfo.Data
	if newData == nil {
		newData = data
	} else {
		for k, v := range data {
			newData[k] = v
		}
	}
	entity, ok := entity0.(state.StatusSetter)
	if !ok {
		return NotSupportedError(tag, "updating status")
	}
	if len(newData) > 0 && existingStatusInfo.Status != state.StatusError {
		return fmt.Errorf("%s is not in an error state", names.ReadableString(tag))
	}
	return entity.SetStatus(existingStatusInfo.Status, existingStatusInfo.Message, newData)
}

// UpdateStatus updates the status data of each given entity.
func (s *StatusSetter) UpdateStatus(args params.SetStatus) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	canModify, err := s.getCanModify()
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	for i, arg := range args.Entities {
		tag, err := names.ParseTag(arg.Tag)
		if err != nil {
			result.Results[i].Error = ServerError(ErrPerm)
			continue
		}
		err = ErrPerm
		if canModify(tag) {
			err = s.updateEntityStatusData(tag, arg.Data)
		}
		result.Results[i].Error = ServerError(err)
	}
	return result, nil
}
