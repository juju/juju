// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package containerbroker_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/api/provisioner"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/container"
	"github.com/juju/juju/container/broker"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/worker/containerbroker"
	"github.com/juju/juju/worker/containerbroker/mocks"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type brokerConfigSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&brokerConfigSuite{})

func (s *brokerConfigSuite) TestInvalidConfigValidate(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	testcases := []struct {
		description string
		config      containerbroker.Config
		err         string
	}{
		{
			description: "Test empty configuration",
			config:      containerbroker.Config{},
			err:         "nil APICaller not valid",
		},
		{
			description: "Test no api caller",
			config:      containerbroker.Config{},
			err:         "nil APICaller not valid",
		},
		{
			description: "Test no agent config",
			config: containerbroker.Config{
				APICaller: mocks.NewMockAPICaller(ctrl),
			},
			err: "nil AgentConfig not valid",
		},
		{
			description: "Test no machine lock",
			config: containerbroker.Config{
				APICaller:   mocks.NewMockAPICaller(ctrl),
				AgentConfig: mocks.NewMockConfig(ctrl),
			},
			err: "nil MachineLock not valid",
		},
		{
			description: "Test no broker func",
			config: containerbroker.Config{
				APICaller:   mocks.NewMockAPICaller(ctrl),
				AgentConfig: mocks.NewMockConfig(ctrl),
				MachineLock: mocks.NewMockLock(ctrl),
			},
			err: "nil NewBrokerFunc not valid",
		},
		{
			description: "Test no state func",
			config: containerbroker.Config{
				APICaller:   mocks.NewMockAPICaller(ctrl),
				AgentConfig: mocks.NewMockConfig(ctrl),
				MachineLock: mocks.NewMockLock(ctrl),
				NewBrokerFunc: func(broker.Config) (environs.InstanceBroker, error) {
					return mocks.NewMockInstanceBroker(ctrl), nil
				},
			},
			err: "nil NewStateFunc not valid",
		},
	}
	for i, test := range testcases {
		c.Logf("%d %s", i, test.description)
		err := test.config.Validate()
		c.Assert(err, gc.ErrorMatches, test.err)
	}
}

func (s *brokerConfigSuite) TestValidConfigValidate(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	config := containerbroker.Config{
		APICaller:   mocks.NewMockAPICaller(ctrl),
		AgentConfig: mocks.NewMockConfig(ctrl),
		MachineLock: mocks.NewMockLock(ctrl),
		NewBrokerFunc: func(broker.Config) (environs.InstanceBroker, error) {
			return mocks.NewMockInstanceBroker(ctrl), nil
		},
		NewStateFunc: func(base.APICaller) containerbroker.State {
			return mocks.NewMockState(ctrl)
		},
	}
	err := config.Validate()
	c.Assert(err, gc.IsNil)
}

type trackerSuite struct {
	testing.IsolationSuite

	apiCaller   *mocks.MockAPICaller
	agentConfig *mocks.MockConfig
	machineLock *mocks.MockLock
	broker      *mocks.MockInstanceBroker
	state       *mocks.MockState
	machine     *mocks.MockMachineProvisioner

	machineTag names.MachineTag
}

var _ = gc.Suite(&trackerSuite{})

func (s *trackerSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.apiCaller = mocks.NewMockAPICaller(ctrl)
	s.agentConfig = mocks.NewMockConfig(ctrl)
	s.machineLock = mocks.NewMockLock(ctrl)
	s.broker = mocks.NewMockInstanceBroker(ctrl)
	s.state = mocks.NewMockState(ctrl)
	s.machine = mocks.NewMockMachineProvisioner(ctrl)

	s.machineTag = names.NewMachineTag("machine-0")

	return ctrl
}

func (s *trackerSuite) TestNewTracker(c *gc.C) {
	defer s.setup(c).Finish()

	_, err := s.withScenario(c,
		&broker.Config{
			Name:          "instance-broker",
			ContainerType: instance.LXD,
			ManagerConfig: container.ManagerConfig(map[string]string{
				container.ConfigAvailabilityZone: "0",
			}),
			APICaller:    s.state,
			AgentConfig:  s.agentConfig,
			MachineTag:   s.machineTag,
			MachineLock:  s.machineLock,
			GetNetConfig: common.GetObservedNetworkConfig,
		},
		s.expectMachineTag,
		s.expectMachines,
		s.expectSupportedContainers,
		s.expectContainerConfig,
	)
	c.Assert(err, gc.IsNil)
}

func (s *trackerSuite) TestNewTrackerWithNoMachines(c *gc.C) {
	defer s.setup(c).Finish()

	_, err := s.withScenario(c,
		nil,
		s.expectMachineTag,
		s.expectNoMachines,
	)
	c.Assert(err, gc.ErrorMatches, "expected 1 result, got 0")
}

func (s *trackerSuite) TestNewTrackerWithDeadMachines(c *gc.C) {
	defer s.setup(c).Finish()

	_, err := s.withScenario(c,
		nil,
		s.expectMachineTag,
		s.expectDeadMachines,
	)
	c.Assert(err, gc.ErrorMatches, "resource permanently unavailable")
}

func (s *trackerSuite) TestNewTrackerWithNoContainers(c *gc.C) {
	defer s.setup(c).Finish()

	_, err := s.withScenario(c,
		nil,
		s.expectMachineTag,
		s.expectMachines,
		s.expectNoSupportedContainers,
	)
	c.Assert(err, gc.ErrorMatches, "resource permanently unavailable")
}

func (s *trackerSuite) TestNewTrackerWithNoDeterminedContainers(c *gc.C) {
	defer s.setup(c).Finish()

	_, err := s.withScenario(c,
		nil,
		s.expectMachineTag,
		s.expectMachines,
		s.expectNoDeterminedSupportedContainers,
	)
	c.Assert(err, gc.ErrorMatches, "no container types determined")
}

func (s *trackerSuite) TestNewTrackerWithKVMContainers(c *gc.C) {
	defer s.setup(c).Finish()

	_, err := s.withScenario(c,
		nil,
		s.expectMachineTag,
		s.expectMachines,
		s.expectKVMSupportedContainers,
	)
	c.Assert(err, gc.ErrorMatches, "resource permanently unavailable")
}

func (s *trackerSuite) withScenario(c *gc.C, expected *broker.Config, behaviours ...func()) (*containerbroker.Tracker, error) {
	for _, b := range behaviours {
		b()
	}
	return containerbroker.NewTracker(containerbroker.Config{
		APICaller:   s.apiCaller,
		AgentConfig: s.agentConfig,
		MachineLock: s.machineLock,
		NewBrokerFunc: func(config broker.Config) (environs.InstanceBroker, error) {
			if expected != nil {
				c.Check(config.Name, gc.Equals, expected.Name)
				c.Check(config.ContainerType, gc.Equals, expected.ContainerType)
				c.Check(config.ManagerConfig, gc.DeepEquals, expected.ManagerConfig)
				c.Check(config.MachineTag, gc.Equals, expected.MachineTag)
			}
			return s.broker, nil
		},
		NewStateFunc: func(base.APICaller) containerbroker.State {
			return s.state
		},
	})
}

func (s *trackerSuite) expectMachineTag() {
	s.agentConfig.EXPECT().Tag().Return(s.machineTag)
}

func (s *trackerSuite) expectMachines() {
	s.state.EXPECT().Machines(s.machineTag).Return([]provisioner.MachineResult{{
		Machine: s.machine,
	}}, nil)
	s.machine.EXPECT().Life().Return(life.Alive)
}

func (s *trackerSuite) expectNoMachines() {
	s.state.EXPECT().Machines(s.machineTag).Return([]provisioner.MachineResult{}, nil)
}

func (s *trackerSuite) expectDeadMachines() {
	s.state.EXPECT().Machines(s.machineTag).Return([]provisioner.MachineResult{{
		Machine: s.machine,
	}}, nil)
	s.machine.EXPECT().Life().Return(life.Dead)
}

func (s *trackerSuite) expectSupportedContainers() {
	s.machine.EXPECT().SupportedContainers().Return([]instance.ContainerType{
		instance.LXD,
	}, true, nil)
}

func (s *trackerSuite) expectNoSupportedContainers() {
	s.machine.EXPECT().SupportedContainers().Return([]instance.ContainerType{}, true, nil)
}

func (s *trackerSuite) expectNoDeterminedSupportedContainers() {
	s.machine.EXPECT().SupportedContainers().Return([]instance.ContainerType{
		instance.LXD,
	}, false, nil)
}

func (s *trackerSuite) expectKVMSupportedContainers() {
	s.machine.EXPECT().SupportedContainers().Return([]instance.ContainerType{
		instance.KVM,
	}, true, nil)
}

func (s *trackerSuite) expectContainerConfig() {
	s.state.EXPECT().ContainerManagerConfig(params.ContainerManagerConfigParams{
		Type: instance.LXD,
	}).Return(params.ContainerManagerConfig{
		ManagerConfig: make(map[string]string),
	}, nil)
	s.machine.EXPECT().AvailabilityZone().Return("0", nil)
}
