// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
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
		controller.AgentRateLimitMax,
		controller.AgentRateLimitRate,
		controller.AllowModelAccessKey,
		controller.APIPortOpenDelay,
		controller.AuditLogExcludeMethods,
		controller.AutocertURLKey,
		controller.AutocertDNSNameKey,
		controller.CAASImageRepo,
		controller.CAASOperatorImagePath,
		controller.CharmStoreURL,
		controller.ControllerAPIPort,
		controller.ControllerName,
		controller.Features,
		controller.IdentityURL,
		controller.IdentityPublicKey,
		controller.JujuDBSnapChannel,
		controller.JujuHASpace,
		controller.JujuManagementSpace,
		controller.MaxDebugLogDuration,
		controller.MaxPruneTxnBatchSize,
		controller.MaxPruneTxnPasses,
		controller.MeteringURL,
		controller.ModelLogfileMaxBackups,
		controller.ModelLogfileMaxSize,
		controller.MongoMemoryProfile,
		controller.PruneTxnQueryCount,
		controller.PruneTxnSleepTime,
		controller.PublicDNSAddress,
		controller.MaxCharmStateSize,
		controller.MaxAgentStateSize,
		controller.NonSyncedWritesToRaftLog,
		controller.MigrationMinionWaitMax,
		controller.AgentLogfileMaxBackups,
		controller.AgentLogfileMaxSize,
		controller.ControllerResourceDownloadLimit,
		controller.ApplicationResourceDownloadLimit,
	)
	for _, controllerAttr := range controller.ControllerOnlyConfigAttributes {
		v, ok := controllerSettings.Get(controllerAttr)
		c.Logf(controllerAttr)
		if !optional.Contains(controllerAttr) {
			c.Check(ok, jc.IsTrue)
			c.Check(v, gc.Not(gc.Equals), "")
		}
	}
}

func (s *ControllerSuite) TestNewState(c *gc.C) {
	st, err := s.Controller.GetState(s.Model.ModelTag())
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
	c.Assert(cfg.PublicDNSAddress(), gc.Equals, "")

	err = s.State.UpdateControllerConfig(map[string]interface{}{
		controller.AuditingEnabled:     true,
		controller.AuditLogCaptureArgs: false,
		controller.PublicDNSAddress:    "controller.test.com:1234",
	}, nil)
	c.Assert(err, jc.ErrorIsNil)

	newCfg, err := s.State.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(newCfg.AuditingEnabled(), gc.Equals, true)
	c.Assert(newCfg.AuditLogCaptureArgs(), gc.Equals, false)
	c.Assert(newCfg.PublicDNSAddress(), gc.Equals, "controller.test.com:1234")
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
	_, err := s.State.AddSpace("mgmt-space", "", nil, false)
	c.Assert(err, jc.ErrorIsNil)

	m, err := s.State.AddMachine("quantal", state.JobManageModel, state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.SetMachineAddresses(network.NewSpaceAddress("192.168.9.9")), jc.ErrorIsNil)

	err = s.State.UpdateControllerConfig(map[string]interface{}{
		controller.JujuManagementSpace: "mgmt-space",
	}, nil)
	c.Assert(err, gc.ErrorMatches,
		`invalid config "juju-mgmt-space"="mgmt-space": machines with no addresses in this space: 0`)
}

func (s *ControllerSuite) TestUpdateControllerConfigAcceptsSpaceWithAddresses(c *gc.C) {
	sp, err := s.State.AddSpace("mgmt-space", "", nil, false)
	c.Assert(err, jc.ErrorIsNil)

	m, err := s.State.AddMachine("quantal", state.JobManageModel, state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	addr := network.NewSpaceAddress("192.168.9.9")
	addr.SpaceID = sp.Id()

	c.Assert(m.SetProviderAddresses(addr), jc.ErrorIsNil)

	err = s.State.UpdateControllerConfig(map[string]interface{}{
		controller.JujuManagementSpace: "mgmt-space",
	}, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ControllerSuite) TestControllerInfo(c *gc.C) {
	info, err := s.State.ControllerInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.CloudName, gc.Equals, "dummy")
	c.Assert(info.ModelTag, gc.Equals, s.modelTag)
	c.Assert(info.ControllerIds, gc.HasLen, 0)

	node, err := s.State.AddControllerNode()
	c.Assert(err, jc.ErrorIsNil)
	info, err = s.State.ControllerInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.ControllerIds, jc.DeepEquals, []string{node.Id()})
}

func (s *ControllerSuite) testOpenParams() state.OpenParams {
	return state.OpenParams{
		Clock:              clock.WallClock,
		ControllerTag:      s.State.ControllerTag(),
		ControllerModelTag: s.modelTag,
		MongoSession:       s.Session,
	}
}

func (s *ControllerSuite) TestReopenWithNoMachines(c *gc.C) {
	expected := &state.ControllerInfo{
		CloudName: "dummy",
		ModelTag:  s.modelTag,
	}
	info, err := s.State.ControllerInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, expected)

	controller, err := state.OpenController(s.testOpenParams())
	c.Assert(err, jc.ErrorIsNil)
	defer controller.Close()
	st := controller.SystemState()

	info, err = st.ControllerInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, expected)
}

func (s *ControllerSuite) TestStateServingInfo(c *gc.C) {
	_, err := s.State.StateServingInfo()
	c.Assert(err, gc.ErrorMatches, "state serving info not found")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	data := controller.StateServingInfo{
		APIPort:      69,
		StatePort:    80,
		Cert:         "Some cert",
		PrivateKey:   "Some key",
		SharedSecret: "Some Keyfile",
	}
	err = s.State.SetStateServingInfo(data)
	c.Assert(err, jc.ErrorIsNil)

	info, err := s.State.StateServingInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, data)
}

var setStateServingInfoWithInvalidInfoTests = []func(info *controller.StateServingInfo){
	func(info *controller.StateServingInfo) { info.APIPort = 0 },
	func(info *controller.StateServingInfo) { info.StatePort = 0 },
	func(info *controller.StateServingInfo) { info.Cert = "" },
	func(info *controller.StateServingInfo) { info.PrivateKey = "" },
}

func (s *ControllerSuite) TestSetStateServingInfoWithInvalidInfo(c *gc.C) {
	origData := controller.StateServingInfo{
		APIPort:      69,
		StatePort:    80,
		Cert:         "Some cert",
		PrivateKey:   "Some key",
		SharedSecret: "Some Keyfile",
	}
	for i, test := range setStateServingInfoWithInvalidInfoTests {
		c.Logf("test %d", i)
		data := origData
		test(&data)
		err := s.State.SetStateServingInfo(data)
		c.Assert(err, gc.ErrorMatches, "incomplete state serving info set in state")
	}
}
