// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
)

type ControllerConfigSuite struct {
	ConnSuite
}

var _ = gc.Suite(&ControllerConfigSuite{})

func (s *ControllerConfigSuite) TestControllerAndModelConfigInitialisation(c *gc.C) {
	controllerSettings, err := s.State.ReadSettings(state.ControllersC, "controllerSettings")
	c.Assert(err, jc.ErrorIsNil)
	modelSettings, err := s.State.ReadSettings(state.SettingsC, "model")
	c.Assert(err, jc.ErrorIsNil)

	for _, controllerAttr := range state.ControllerOnlyConfigAttributes {
		v, ok := controllerSettings.Get(controllerAttr)
		c.Assert(ok, jc.IsTrue)
		c.Assert(v, gc.Not(gc.Equals), "")

		_, ok = modelSettings.Get(controllerAttr)
		c.Assert(ok, jc.IsFalse)
	}

	for _, attr := range state.RetainModelConfigAttributes {
		cv, ok := controllerSettings.Get(attr)
		c.Assert(ok, jc.IsTrue)
		c.Assert(cv, gc.Not(gc.Equals), "")

		mv, ok := modelSettings.Get(attr)
		c.Assert(ok, jc.IsTrue)
		c.Assert(mv, gc.Equals, cv)
	}
}

func (s *ControllerConfigSuite) TestControllerConfig(c *gc.C) {
	cfg, err := s.State.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	m, err := s.State.ControllerModel()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg["uuid"], gc.Equals, m.ControllerUUID())
}
