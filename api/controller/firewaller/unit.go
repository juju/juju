// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"context"

	"github.com/juju/names/v6"

	"github.com/juju/juju/core/life"
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

// Life returns the unit's life cycle value.
func (u *Unit) Life() life.Value {
	return u.life
}

// Refresh updates the cached local copy of the unit's data.
func (u *Unit) Refresh(ctx context.Context) error {
	life, err := u.client.life(ctx, u.tag)
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
