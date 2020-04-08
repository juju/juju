// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

// UnitStateAPI provides common agent-side API functions to
// call into apiserver.common/UnitState
type UnitStateAPI struct {
	facade base.FacadeCaller
	tag    names.UnitTag
}

// NewUniterStateAPI creates a UnitStateAPI that uses the provided FacadeCaller
// for making calls.
func NewUniterStateAPI(facade base.FacadeCaller, tag names.UnitTag) *UnitStateAPI {
	return &UnitStateAPI{facade: facade, tag: tag}
}

// State returns the state persisted by the charm running in this unit
// and the state internal to the uniter for this unit.
func (u *UnitStateAPI) State() (params.UnitStateResult, error) {
	var results params.UnitStateResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	err := u.facade.FacadeCall("State", args, &results)
	if err != nil {
		return params.UnitStateResult{}, err
	}
	if len(results.Results) != 1 {
		return params.UnitStateResult{}, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return params.UnitStateResult{}, result.Error
	}
	return result, nil
}

// SetState sets the state persisted by the charm running in this unit
// and the state internal to the uniter for this unit.
func (u *UnitStateAPI) SetState(unitState params.SetUnitStateArg) error {
	unitState.Tag = u.tag.String()
	var results params.ErrorResults
	args := params.SetUnitStateArgs{
		Args: []params.SetUnitStateArg{unitState},
	}
	err := u.facade.FacadeCall("SetState", args, &results)
	if err != nil {
		return errors.Trace(err)
	}
	// Make sure we correctly decode quota-related errors.
	return maybeRestoreQuotaLimitError(results.OneError())
}

// maybeRestoreQuotaLimitError checks if the server emitted a quota limit
// exceeded error and restores it back to a typed error from juju/errors.
// Ideally, we would use apiserver/common.RestoreError but apparently, that
// package imports worker/uniter/{operation, remotestate} causing an import
// cycle when api/common is imported by api/uniter.
func maybeRestoreQuotaLimitError(err error) error {
	if params.IsCodeQuotaLimitExceeded(err) {
		return errors.NewQuotaLimitExceeded(nil, err.Error())
	}
	return err
}
