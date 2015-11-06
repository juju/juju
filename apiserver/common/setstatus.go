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
// This is only slightly less evil than ServiceStatusGetter. We have
// StatusSetter already; all this does is set the status for the wrong
// entity, and render the auth so confused as to be ~worthless.
type ServiceStatusSetter struct {
	st           *state.State
	getCanModify GetAuthFunc
}

// NewServiceStatusSetter returns a ServiceStatusSetter.
func NewServiceStatusSetter(st *state.State, getCanModify GetAuthFunc) *ServiceStatusSetter {
	return &ServiceStatusSetter{
		st:           st,
		getCanModify: getCanModify,
	}
}

// SetStatus sets the status on the service given by the unit in args if the unit is the leader.
func (s *ServiceStatusSetter) SetStatus(args params.SetStatus) (params.ErrorResults, error) {
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
			result.Results[i].Error = ServerError(err)
			continue
		}
		if !canModify(tag) {
			result.Results[i].Error = ServerError(ErrPerm)
			continue
		}
		unitTag, ok := tag.(names.UnitTag)
		if !ok {
			// No matter what the canModify says, if this entity is not
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

		// ...and set the status, conditional on the unit being (and remaining)
		// service leader.
		checker := s.st.LeadershipChecker()
		token := checker.LeadershipCheck(serviceId, unitId)

		// TODO(fwereade) pass token into SetStatus instead of checking here.
		if err := token.Check(nil); err != nil {
			// TODO(fwereade) this should probably be ErrPerm is certain cases,
			// but I don't think I implemented an exported ErrNotLeader. I
			// should have done, though.
			result.Results[i].Error = ServerError(err)
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
	case *state.Service:
		return ErrPerm
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
			result.Results[i].Error = ServerError(err)
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
// TODO(fwereade): WTF. This method exists *only* for the convenience of the
// *client* API -- and is itself completely broken -- but we still expose it
// in every facade with a StatusSetter? FFS.
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
