// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"github.com/juju/names"

	"github.com/juju/juju/api/common"
	"github.com/juju/juju/apiserver/params"
)

// Unit represents a juju unit as seen by the deployer worker.
type Unit struct {
	tag  names.UnitTag
	life params.Life
	st   *State
}

// Tag returns the unit's tag.
func (u *Unit) Tag() string {
	return u.tag.String()
}

// Name returns the unit's name.
func (u *Unit) Name() string {
	return u.tag.Id()
}

// Life returns the unit's lifecycle value.
func (u *Unit) Life() params.Life {
	return u.life
}

// Refresh updates the cached local copy of the unit's data.
func (u *Unit) Refresh() error {
	life, err := common.Life(u.st.facade, u.tag)
	if err != nil {
		return err
	}
	u.life = life
	return nil
}

// Remove removes the unit from state, calling EnsureDead first, then Remove.
// It will fail if the unit is not present.
func (u *Unit) Remove() error {
	var result params.ErrorResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	err := u.st.facade.FacadeCall("Remove", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// SetPassword sets the unit's password.
func (u *Unit) SetPassword(password string) error {
	var result params.ErrorResults
	args := params.EntityPasswords{
		Changes: []params.EntityPassword{
			{Tag: u.tag.String(), Password: password},
		},
	}
	err := u.st.facade.FacadeCall("SetPasswords", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}
