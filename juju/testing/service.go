// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type service struct {
	env     *environ
	charm   *state.Charm
	service *state.Service
}

// AddService implements coretesting.Service.
func (svc *service) SetConfig(c *gc.C, settings map[string]string) {
	changes, err := svc.charm.Config().ParseSettingsStrings(settings)
	c.Assert(err, jc.ErrorIsNil)
	err = svc.service.UpdateConfigSettings(changes)
	c.Assert(err, jc.ErrorIsNil)
}

// Deploy implements coretesting.Service.
func (svc *service) Deploy(c *gc.C) coretesting.Unit {
	u, err := svc.service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)

	// TODO(ericsnow) Needs a machine/instance to which to deploy.
	err = svc.env.suite.State.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)

	// Wait until done....
	for i := 0; ; i++ {
		status, err := u.Status()
		c.Assert(err, jc.ErrorIsNil)

		switch status.Status {
		case state.StatusError:
			c.Errorf("unit in error state: %#v", status)
			c.Fail()
		case state.StatusActive:
			break
		}

		if i > 100 {
			panic("timed out")
		}
		// sleep...
	}

	return &unit{
		svc:  svc,
		unit: u,
	}
}
