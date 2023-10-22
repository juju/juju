// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/rpc/params"
)

// Unit represents a juju unit as seen by a firewaller worker.
type Unit struct {
	client *Client
	tag    names.UnitTag
	life   life.Value
}

// Name returns the name of the unit.
func (u *Unit) Name() string {
	return u.tag.Id()
}

// Tag returns the unit tag.
func (u *Unit) Tag() names.UnitTag {
	return u.tag
}

// Life returns the unit's life cycle value.
func (u *Unit) Life() life.Value {
	return u.life
}

// Refresh updates the cached local copy of the unit's data.
func (u *Unit) Refresh() error {
	life, err := u.client.life(u.tag)
	if err != nil {
		return err
	}
	u.life = life
	return nil
}

// Application returns the application.
func (u *Unit) Application() (*Application, error) {
	appName, err := names.UnitApplication(u.Name())
	if err != nil {
		return nil, err
	}
	applicationTag := names.NewApplicationTag(appName)
	app := &Application{
		client: u.client,
		tag:    applicationTag,
	}
	return app, nil
}

// AssignedMachine returns the tag of this unit's assigned machine (if
// any), or a CodeNotAssigned error.
func (u *Unit) AssignedMachine() (names.MachineTag, error) {
	var results params.StringResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	emptyTag := names.NewMachineTag("")
	err := u.client.facade.FacadeCall(context.TODO(), "GetAssignedMachine", args, &results)
	if err != nil {
		return emptyTag, err
	}
	if len(results.Results) != 1 {
		return emptyTag, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return emptyTag, result.Error
	}
	return names.ParseMachineTag(result.Result)
}
