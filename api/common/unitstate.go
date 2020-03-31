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

// NewUpgradeSeriesAPI creates a UpgradeSeriesAPI on the specified facade,
// and uses this name when calling through the caller.
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
	return results.OneError()
}
