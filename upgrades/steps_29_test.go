// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	"fmt"

	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	apicallermocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/service"
	"github.com/juju/juju/service/common"
	servicemocks "github.com/juju/juju/service/mocks"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
	"github.com/juju/juju/upgrades/mocks"
	configsettermocks "github.com/juju/juju/worker/upgradedatabase/mocks"
)

var v290 = version.MustParse("2.9.0")

type steps29Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps29Suite{})

func (s *steps29Suite) TestStoreDeployedUnitsInMachineAgentConf(c *gc.C) {
	step := findStep(c, v290, "store deployed units in machine agent.conf")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.HostMachine})
}

func (s *steps29Suite) TestAddCharmhubToModelConfig(c *gc.C) {
	step := findStateStep(c, v290, "add charm-hub-url to model config")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}

func (s *steps29Suite) TestRollUpAndConvertOpenedPortDocuments(c *gc.C) {
	step := findStateStep(c, v290, "roll up and convert opened port documents into the new endpoint-aware format")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}

func (s *steps29Suite) TestAddCharmOriginToApplications(c *gc.C) {
	step := findStateStep(c, v290, "add charm-origin to applications")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}

func (s *steps29Suite) TestAddAzureProviderNetworkConfig(c *gc.C) {
	step := findStateStep(c, v290, "add Azure provider network config")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}

type mergeAgents29Suite struct {
	testing.BaseSuite

	dataDir string
	machine agent.ConfigSetterWriter
	unitOne agent.ConfigSetterWriter
	unitTwo agent.ConfigSetterWriter

	machineTag names.MachineTag
	unitOneTag names.UnitTag
	unitTwoTag names.UnitTag

	mockCtx         *mocks.MockContext
	mockClient      *mocks.MockUpgradeStepsClient
	mockAgentConfig *configsettermocks.MockConfigSetter
	mockAPICaller   *apicallermocks.MockAPICaller
}

var _ = gc.Suite(&mergeAgents29Suite{})

func (s *mergeAgents29Suite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.dataDir = c.MkDir()

	s.machineTag = names.NewMachineTag("42")
	s.unitOneTag = names.NewUnitTag("principal/1")
	s.unitTwoTag = names.NewUnitTag("subordinate/2")

	s.machine = s.writeAgentConf(c, s.machineTag)
	s.unitOne = s.writeAgentConf(c, s.unitOneTag)
	s.unitTwo = s.writeAgentConf(c, s.unitTwoTag)

	s.PatchValue(upgrades.ServiceDiscovery, func(name string, _ common.Conf) (service.Service, error) {
		return nil, errors.NotFoundf(name)
	})
}

func (s *mergeAgents29Suite) writeAgentConf(c *gc.C, tag names.Tag) agent.ConfigSetterWriter {
	conf, err := agent.NewAgentConfig(
		agent.AgentConfigParams{
			Paths: agent.Paths{
				DataDir: s.dataDir,
			},
			Tag:               tag,
			Password:          "secret",
			Controller:        testing.ControllerTag,
			Model:             testing.ModelTag,
			APIAddresses:      []string{"localhost:4321"},
			CACert:            testing.CACert,
			UpgradedToVersion: version.MustParse("2.42.0"),
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(conf.Write(), jc.ErrorIsNil)
	return conf
}

func (s *mergeAgents29Suite) TestUnitAgentSuccess(c *gc.C) {
	// The upgrade step succeeds but does nothing for unit agents.
	defer s.setup(c).Finish()
	s.expectAgentConfigUnitTag()
	err := upgrades.StoreDeployedUnitsInMachineAgentConf(s.mockCtx)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *mergeAgents29Suite) TestGoodPath(c *gc.C) {
	mockController := s.setup(c)
	defer mockController.Finish()
	s.expectAgentConfigMachineTag()
	svc1 := s.expectService(mockController, s.unitOneTag)
	svc1.EXPECT().Installed().Return(true, nil)
	svc1.EXPECT().Stop().Return(nil)
	svc1.EXPECT().Remove().Return(nil)

	svc2 := s.expectService(mockController, s.unitTwoTag)
	svc2.EXPECT().Installed().Return(true, nil)
	svc2.EXPECT().Stop().Return(nil)
	svc2.EXPECT().Remove().Return(nil)

	s.mockAgentConfig.EXPECT().SetValue("deployed-units", "principal/1,subordinate/2")

	err := upgrades.StoreDeployedUnitsInMachineAgentConf(s.mockCtx)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *mergeAgents29Suite) TestServicesNotInstalled(c *gc.C) {
	// This also tests idempotency as the step may have been run before
	// which removed the services.
	mockController := s.setup(c)
	defer mockController.Finish()
	s.expectAgentConfigMachineTag()
	svc1 := s.expectService(mockController, s.unitOneTag)
	svc1.EXPECT().Installed().Return(false, nil)

	svc2 := s.expectService(mockController, s.unitTwoTag)
	svc2.EXPECT().Installed().Return(false, nil)

	s.mockAgentConfig.EXPECT().SetValue("deployed-units", "principal/1,subordinate/2")

	err := upgrades.StoreDeployedUnitsInMachineAgentConf(s.mockCtx)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *mergeAgents29Suite) TestServiceStopFailedStillCallsRemove(c *gc.C) {
	// This also tests idempotency as the step may have been run before
	// which removed the services.
	mockController := s.setup(c)
	defer mockController.Finish()
	s.expectAgentConfigMachineTag()

	svc := s.expectService(mockController, s.unitOneTag)
	svc.EXPECT().Installed().Return(true, nil)
	svc.EXPECT().Stop().Return(errors.New("boom"))
	svc.EXPECT().Remove().Return(nil)

	// Also happens to test the situation where service discovery for the
	// second unit returns an error.

	s.mockAgentConfig.EXPECT().SetValue("deployed-units", "principal/1,subordinate/2")

	err := upgrades.StoreDeployedUnitsInMachineAgentConf(s.mockCtx)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *mergeAgents29Suite) setup(c *gc.C) *gomock.Controller {
	ctlr := gomock.NewController(c)

	s.mockCtx = mocks.NewMockContext(ctlr)
	s.mockAgentConfig = configsettermocks.NewMockConfigSetter(ctlr)
	s.mockClient = mocks.NewMockUpgradeStepsClient(ctlr)

	s.expectAgentConfig()
	s.expectDataDir()
	return ctlr
}

func (s *mergeAgents29Suite) expectAgentConfig() {
	s.mockCtx.EXPECT().AgentConfig().Return(s.mockAgentConfig).AnyTimes()
}

func (s *mergeAgents29Suite) expectDataDir() {
	s.mockAgentConfig.EXPECT().DataDir().Return(s.dataDir).AnyTimes()
}

func (s *mergeAgents29Suite) expectAgentConfigMachineTag() {
	s.mockAgentConfig.EXPECT().Tag().Return(names.NewMachineTag("42")).AnyTimes()
}

func (s *mergeAgents29Suite) expectAgentConfigUnitTag() {
	s.mockAgentConfig.EXPECT().Tag().Return(names.NewUnitTag("principal/1"))
}

func (s *mergeAgents29Suite) expectService(ctrl *gomock.Controller, unit names.UnitTag) *servicemocks.MockService {
	orig := *upgrades.ServiceDiscovery
	svc := servicemocks.NewMockService(ctrl)
	// Chain up the service discovery.
	s.PatchValue(upgrades.ServiceDiscovery, func(name string, conf common.Conf) (service.Service, error) {
		if name == fmt.Sprintf("jujud-%s", unit) {
			return svc, nil
		}
		return orig(name, conf)
	})
	return svc
}
