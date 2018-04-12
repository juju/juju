// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/network"
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

	optional := set.NewStrings(
		controller.IdentityURL,
		controller.IdentityPublicKey,
		controller.AutocertURLKey,
		controller.AutocertDNSNameKey,
		controller.AllowModelAccessKey,
		controller.MongoMemoryProfile,
		controller.JujuHASpace,
		controller.JujuManagementSpace,
		controller.AuditLogExcludeMethods,
	)
	for _, controllerAttr := range controller.ControllerOnlyConfigAttributes {
		v, ok := controllerSettings.Get(controllerAttr)
		c.Logf(controllerAttr)
		if !optional.Contains(controllerAttr) {
			c.Assert(ok, jc.IsTrue)
			c.Assert(v, gc.Not(gc.Equals), "")
		}
	}
}

func (s *ControllerSuite) TestNewState(c *gc.C) {
	st, err := s.Controller.NewState(s.IAASModel.ModelTag())
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()
	c.Check(st.ModelUUID(), gc.Equals, s.State.ModelUUID())
	c.Check(st, gc.Not(gc.Equals), s.State)
}

func (s *ControllerSuite) TestControllerConfig(c *gc.C) {
	cfg, err := s.State.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg["controller-uuid"], gc.Equals, s.State.ControllerUUID())
}

func (s *ControllerSuite) TestPing(c *gc.C) {
	c.Assert(s.Controller.Ping(), gc.IsNil)
	gitjujutesting.MgoServer.Restart()
	c.Assert(s.Controller.Ping(), gc.NotNil)
}

func (s *ControllerSuite) TestUpdateControllerConfig(c *gc.C) {
	cfg, err := s.State.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	// Sanity check.
	c.Check(cfg.AuditingEnabled(), gc.Equals, false)
	c.Check(cfg.AuditLogCaptureArgs(), gc.Equals, true)

	err = s.State.UpdateControllerConfig(map[string]interface{}{
		controller.AuditingEnabled:     true,
		controller.AuditLogCaptureArgs: false,
	}, nil)
	c.Assert(err, jc.ErrorIsNil)

	newCfg, err := s.State.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(newCfg.AuditingEnabled(), gc.Equals, true)
	c.Assert(newCfg.AuditLogCaptureArgs(), gc.Equals, false)
}

func (s *ControllerSuite) TestUpdateControllerConfigRemoveYieldsDefaults(c *gc.C) {
	err := s.State.UpdateControllerConfig(map[string]interface{}{
		controller.AuditingEnabled:     true,
		controller.AuditLogCaptureArgs: true,
	}, nil)
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.UpdateControllerConfig(nil, []string{
		controller.AuditLogCaptureArgs,
	})
	c.Assert(err, jc.ErrorIsNil)

	newCfg, err := s.State.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(newCfg.AuditLogCaptureArgs(), gc.Equals, false)
}

func (s *ControllerSuite) TestUpdateControllerConfigRejectsDisallowedUpdates(c *gc.C) {
	// Sanity check.
	c.Assert(controller.AllowedUpdateConfigAttributes.Contains(controller.APIPort), jc.IsFalse)

	err := s.State.UpdateControllerConfig(map[string]interface{}{
		controller.APIPort: 1234,
	}, nil)
	c.Assert(err, gc.ErrorMatches, `can't change "api-port" after bootstrap`)

	err = s.State.UpdateControllerConfig(nil, []string{controller.APIPort})
	c.Assert(err, gc.ErrorMatches, `can't change "api-port" after bootstrap`)
}

func (s *ControllerSuite) TestUpdateControllerConfigChecksSchema(c *gc.C) {
	err := s.State.UpdateControllerConfig(map[string]interface{}{
		controller.AuditLogExcludeMethods: []int{1, 2, 3},
	}, nil)
	c.Assert(err, gc.ErrorMatches, `audit-log-exclude-methods\[0\]: expected string, got int\(1\)`)
}

func (s *ControllerSuite) TestUpdateControllerConfigValidates(c *gc.C) {
	err := s.State.UpdateControllerConfig(map[string]interface{}{
		controller.AuditLogExcludeMethods: []string{"thing"},
	}, nil)
	c.Assert(err, gc.ErrorMatches, `invalid audit log exclude methods: should be a list of "Facade.Method" names \(or "ReadOnlyMethods"\), got "thing" at position 1`)
}

func (s *ControllerSuite) TestUpdatingUnknownName(c *gc.C) {
	err := s.State.UpdateControllerConfig(map[string]interface{}{
		"ana-ng": "majestic",
	}, nil)
	c.Assert(err, gc.ErrorMatches, `unknown controller config setting "ana-ng"`)
}

func (s *ControllerSuite) TestRemovingUnknownName(c *gc.C) {
	err := s.State.UpdateControllerConfig(nil, []string{"dr-worm"})
	c.Assert(err, gc.ErrorMatches, `unknown controller config setting "dr-worm"`)
}

func (s *ControllerSuite) TestUpdateControllerConfigRejectsSpaceWithoutAddresses(c *gc.C) {
	m, err := s.State.AddMachine("quantal", state.JobManageModel, state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.SetMachineAddresses(network.NewAddress("192.168.9.9")), jc.ErrorIsNil)

	err = s.State.UpdateControllerConfig(map[string]interface{}{
		controller.JujuManagementSpace: "mgmt-space",
	}, nil)
	c.Assert(err, gc.ErrorMatches,
		`invalid config "juju-mgmt-space"="mgmt-space": machines with no addresses in this space: 0`)
}

func (s *ControllerSuite) TestUpdateControllerConfigAcceptsSpaceWithAddresses(c *gc.C) {
	m, err := s.State.AddMachine("quantal", state.JobManageModel, state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.SetProviderAddresses(network.NewAddressOnSpace("mgmt-space", "192.168.9.9")), jc.ErrorIsNil)

	err = s.State.UpdateControllerConfig(map[string]interface{}{
		controller.JujuManagementSpace: "mgmt-space",
	}, nil)
	c.Assert(err, jc.ErrorIsNil)
}
