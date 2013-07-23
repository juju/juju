// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"strings"

	"launchpad.net/juju-core/state/api/params"
)

// Unit represents a juju unit as seen by the deployer worker.
type Unit struct {
	tag  string
	life params.Life
	st   *State
}

// Tag returns the unit's tag.
func (u *Unit) Tag() string {
	return u.tag
}

const unitTagPrefix = "unit-"

// Name returns the unit's name.
func (u *Unit) Name() string {
	if !strings.HasPrefix(u.tag, unitTagPrefix) {
		return ""
	}
	// Strip off the "unit-" prefix.
	name := u.tag[len(unitTagPrefix):]
	// Put the slashes back.
	name = strings.Replace(name, "-", "/", -1)
	return name
}

// Life returns the unit's lifecycle value.
func (u *Unit) Life() params.Life {
	return u.life
}

// Refresh updates the cached local copy of the unit's data.
func (u *Unit) Refresh() error {
	life, err := u.st.unitLife(u.tag)
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
		Entities: []params.Entity{{Tag: u.tag}},
	}
	err := u.st.caller.Call("Deployer", "", "Remove", args, &result)
	if err != nil {
		return err
	}
	if len(result.Errors) > 0 && result.Errors[0] != nil {
		return result.Errors[0]
	}
	return nil
}

// SetPassword changes the unit's password.
func (u *Unit) SetPassword(password string) error {
	var result params.ErrorResults
	args := params.PasswordChanges{
		Changes: []params.PasswordChange{
			{Tag: u.tag, Password: password},
		},
	}
	err := u.st.caller.Call("Deployer", "", "SetPasswords", args, &result)
	if err != nil {
		return err
	}
	if len(result.Errors) > 0 && result.Errors[0] != nil {
		return result.Errors[0]
	}
	return nil
}

// AssignedMachineTag returns the tag of the machine this unit is
// assigned to.
func (u *Unit) AssignedMachineTag() (string, error) {
	var result params.EntityResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag}},
	}
	err := u.st.caller.Call("Deployer", "", "AssignedMachineTag", args, &result)
	if err != nil {
		return "", err
	}
	if len(result.Results) > 0 && result.Results[0].Error != nil {
		return "", result.Results[0].Error
	}
	return result.Results[0].Tag, nil
}
