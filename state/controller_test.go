// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/state"
)

type ControllerSuite struct {
	ConnSuite
}

var _ = gc.Suite(&ControllerSuite{})

func (s *ControllerSuite) TestControllerAndModelConfigInitialisation(c *gc.C) {
	// Test setup has created model using a fully populated environs.Config.
	// This test ensure that the controller specific attributes have been separated out.
	controllerSettings, err := s.State.ReadSettings(state.ControllersC, "controllerSettings")
	c.Assert(err, jc.ErrorIsNil)

	optional := map[string]bool{
		controller.IdentityURL:         true,
		controller.IdentityPublicKey:   true,
		controller.AutocertURLKey:      true,
		controller.AutocertDNSNameKey:  true,
		controller.AllowModelAccessKey: true,
		controller.MongoMemoryProfile:  true,
	}
	for _, controllerAttr := range controller.ControllerOnlyConfigAttributes {
		v, ok := controllerSettings.Get(controllerAttr)
		if !optional[controllerAttr] {
			c.Assert(ok, jc.IsTrue)
			c.Assert(v, gc.Not(gc.Equals), "")
		}
	}
}

func (s *ControllerSuite) TestNewState(c *gc.C) {
	st, err := s.Controller.NewState(s.State.ModelTag())
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()
	c.Check(st.ModelUUID(), gc.Equals, s.State.ModelUUID())
	c.Check(st, gc.Not(gc.Equals), s.State)
}

func (s *ControllerSuite) TestControllerConfig(c *gc.C) {
	cfg, err := s.State.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	m, err := s.State.ControllerModel()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg["controller-uuid"], gc.Equals, m.ControllerUUID())
}

func (s *ControllerSuite) TestPing(c *gc.C) {
	c.Assert(s.Controller.Ping(), gc.IsNil)
	gitjujutesting.MgoServer.Restart()
	c.Assert(s.Controller.Ping(), gc.NotNil)
}
