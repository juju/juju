// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
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
		controller.AgentLogfileMaxBackups,
		controller.AgentLogfileMaxSize,
		controller.AgentRateLimitMax,
		controller.AgentRateLimitRate,
		controller.AllowModelAccessKey,
		controller.APIPortOpenDelay,
		controller.ApplicationResourceDownloadLimit,
		controller.AuditLogExcludeMethods,
		controller.AutocertDNSNameKey,
		controller.AutocertURLKey,
		controller.CAASImageRepo,
		controller.CAASOperatorImagePath,
		controller.ControllerAPIPort,
		controller.ControllerName,
		controller.ControllerResourceDownloadLimit,
		controller.Features,
		controller.IdentityPublicKey,
		controller.IdentityURL,
		controller.JujuDBSnapChannel,
		controller.JujuHASpace,
		controller.JujuManagementSpace,
		controller.LoginTokenRefreshURL,
		controller.MaxAgentStateSize,
		controller.MaxCharmStateSize,
		controller.MaxDebugLogDuration,
		controller.MaxPruneTxnBatchSize,
		controller.MaxPruneTxnPasses,
		controller.MeteringURL,
		controller.MigrationMinionWaitMax,
		controller.ModelLogfileMaxBackups,
		controller.ModelLogfileMaxSize,
		controller.MongoMemoryProfile,
		controller.ObjectStoreType,
		controller.ObjectStoreS3Endpoint,
		controller.ObjectStoreS3StaticKey,
		controller.ObjectStoreS3StaticSecret,
		controller.ObjectStoreS3StaticSession,
		controller.OpenTelemetryEnabled,
		controller.OpenTelemetryEndpoint,
		controller.OpenTelemetryInsecure,
		controller.OpenTelemetrySampleRatio,
		controller.OpenTelemetryStackTraces,
		controller.PruneTxnQueryCount,
		controller.PruneTxnSleepTime,
		controller.PublicDNSAddress,
		controller.QueryTracingEnabled,
		controller.QueryTracingThreshold,
		controller.SystemSSHKeys,
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

func (s *ControllerSuite) TestSetMachineAddressesControllerCharm(c *gc.C) {
	controller, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobManageModel, state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	worker, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	controllerApp := s.AddTestingApplication(c, "controller", s.AddTestingCharm(c, "juju-controller"))
	s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: controllerApp,
		Machine:     controller,
	})

	controllerConfig := testing.FakeControllerConfig()

	addresses := network.NewSpaceAddresses("10.0.0.1")
	err = controller.SetMachineAddresses(controllerConfig, addresses...)
	c.Assert(err, jc.ErrorIsNil)

	// Updating a worker machine does not affect charm config.
	addresses = network.NewSpaceAddresses("10.0.0.2")
	err = worker.SetMachineAddresses(controllerConfig, addresses...)
	c.Assert(err, jc.ErrorIsNil)

	cfg, err := controllerApp.CharmConfig(model.GenerationMaster)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg["controller-url"], gc.Equals, "wss://10.0.0.1:17777/api")
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
	st, err := controller.SystemState()
	c.Assert(err, jc.ErrorIsNil)

	info, err = st.ControllerInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, expected)
}

func (s *ControllerSuite) TestStateServingInfo(c *gc.C) {
	_, err := s.State.StateServingInfo()
	c.Assert(err, gc.ErrorMatches, "state serving info not found")
	c.Assert(err, jc.ErrorIs, errors.NotFound)

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
