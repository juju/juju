// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/state"
)

type ControllerConfigSuite struct {
	ConnSuite
}

var _ = gc.Suite(&ControllerConfigSuite{})

func (s *ControllerConfigSuite) TestControllerAndModelConfigInitialisation(c *gc.C) {
	// Test setup has created model using a fully populated environs.Config.
	// This test ensure that the controller specific attributes have been separated out.
	controllerSettings, err := s.State.ReadSettings(state.ControllersC, "controllerSettings")
	c.Assert(err, jc.ErrorIsNil)
	modelSettings, err := s.State.ReadSettings(state.SettingsC, "e")
	c.Assert(err, jc.ErrorIsNil)

	optional := func(attr string) bool {
		return attr == controller.IdentityURL || attr == controller.IdentityPublicKey
	}
	for _, controllerAttr := range controller.ControllerOnlyConfigAttributes {
		v, ok := controllerSettings.Get(controllerAttr)
		if !optional(controllerAttr) && controllerAttr != controller.CAPrivateKey {
			c.Assert(ok, jc.IsTrue)
			c.Assert(v, gc.Not(gc.Equals), "")
		}

		// TODO(wallyworld) - we want to remove controller uuid from model config
		// but it needs to be there for now
		_, ok = modelSettings.Get(controllerAttr)
		c.Assert(ok, gc.Equals, controllerAttr == controller.ControllerUUIDKey)
	}
}

func (s *ControllerConfigSuite) TestControllerConfig(c *gc.C) {
	cfg, err := s.State.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	m, err := s.State.ControllerModel()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg["controller-uuid"], gc.Equals, m.ControllerUUID())
}
