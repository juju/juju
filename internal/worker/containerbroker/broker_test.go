// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package containerbroker_test

import (
	stdtesting "testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/api/agent/provisioner"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/container"
	"github.com/juju/juju/internal/container/broker"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/containerbroker"
	"github.com/juju/juju/internal/worker/containerbroker/mocks"
	"github.com/juju/juju/rpc/params"
)

type brokerConfigSuite struct {
	testhelpers.IsolationSuite
}

func TestBrokerConfigSuite(t *stdtesting.T) { tc.Run(t, &brokerConfigSuite{}) }
func (s *brokerConfigSuite) TestInvalidConfigValidate(c *tc.C) {
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
		c.Assert(err, tc.ErrorMatches, test.err)
	}
}

func (s *brokerConfigSuite) TestValidConfigValidate(c *tc.C) {
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
	c.Assert(err, tc.IsNil)
}

type trackerSuite struct {
	testhelpers.IsolationSuite

	apiCaller   *mocks.MockAPICaller
	agentConfig *mocks.MockConfig
	machineLock *mocks.MockLock
	broker      *mocks.MockInstanceBroker
	state       *mocks.MockState
	machine     *mocks.MockMachineProvisioner

	machineTag names.MachineTag
}

func TestTrackerSuite(t *stdtesting.T) { tc.Run(t, &trackerSuite{}) }
func (s *trackerSuite) setup(c *tc.C) *gomock.Controller {
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

func (s *trackerSuite) TestNewTracker(c *tc.C) {
	defer s.setup(c).Finish()

	_, err := s.withScenario(c,
		&broker.Config{
			Name:          "instance-broker",
			ContainerType: instance.LXD,
			ManagerConfig: map[string]string{
				container.ConfigAvailabilityZone: "0",
			},
			APICaller:    s.state,
			AgentConfig:  s.agentConfig,
			MachineTag:   s.machineTag,
			MachineLock:  s.machineLock,
			GetNetConfig: network.GetObservedNetworkConfig,
		},
		s.expectMachineTag,
		s.expectMachines,
		s.expectContainerConfig,
	)
	c.Assert(err, tc.IsNil)
}

func (s *trackerSuite) TestNewTrackerWithNoMachines(c *tc.C) {
	defer s.setup(c).Finish()

	_, err := s.withScenario(c,
		nil,
		s.expectMachineTag,
		s.expectNoMachines,
	)
	c.Assert(err, tc.ErrorMatches, "expected 1 result, got 0")
}

func (s *trackerSuite) TestNewTrackerWithDeadMachines(c *tc.C) {
	defer s.setup(c).Finish()

	_, err := s.withScenario(c,
		nil,
		s.expectMachineTag,
		s.expectDeadMachines,
	)
	c.Assert(err, tc.ErrorMatches, "resource permanently unavailable")
}

func (s *trackerSuite) TestNewTrackerWithNoContainers(c *tc.C) {
	defer s.setup(c).Finish()

	_, err := s.withScenario(c,
		nil,
		s.expectMachineTag,
		s.expectMachines,
		s.expectContainerConfig,
	)
	c.Assert(err, tc.IsNil)
}

func (s *trackerSuite) withScenario(c *tc.C, expected *broker.Config, behaviours ...func()) (*containerbroker.Tracker, error) {
	for _, b := range behaviours {
		b()
	}
	return containerbroker.NewTracker(c.Context(), containerbroker.Config{
		APICaller:   s.apiCaller,
		AgentConfig: s.agentConfig,
		MachineLock: s.machineLock,
		NewBrokerFunc: func(config broker.Config) (environs.InstanceBroker, error) {
			if expected != nil {
				c.Check(config.Name, tc.Equals, expected.Name)
				c.Check(config.ContainerType, tc.Equals, expected.ContainerType)
				c.Check(config.ManagerConfig, tc.DeepEquals, expected.ManagerConfig)
				c.Check(config.MachineTag, tc.Equals, expected.MachineTag)
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
	s.state.EXPECT().Machines(gomock.Any(), s.machineTag).Return([]provisioner.MachineResult{{
		Machine: s.machine,
	}}, nil)
	s.machine.EXPECT().Life().Return(life.Alive)
}

func (s *trackerSuite) expectNoMachines() {
	s.state.EXPECT().Machines(gomock.Any(), s.machineTag).Return([]provisioner.MachineResult{}, nil)
}

func (s *trackerSuite) expectDeadMachines() {
	s.state.EXPECT().Machines(gomock.Any(), s.machineTag).Return([]provisioner.MachineResult{{
		Machine: s.machine,
	}}, nil)
	s.machine.EXPECT().Life().Return(life.Dead)
}

func (s *trackerSuite) expectContainerConfig() {
	s.state.EXPECT().ContainerManagerConfig(gomock.Any(), params.ContainerManagerConfigParams{
		Type: instance.LXD,
	}).Return(params.ContainerManagerConfig{
		ManagerConfig: make(map[string]string),
	}, nil)
	s.machine.EXPECT().AvailabilityZone(gomock.Any()).Return("0", nil)
}
