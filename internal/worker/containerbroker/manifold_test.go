// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package containerbroker_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/testing"
	"github.com/juju/worker/v4"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/container/broker"
	"github.com/juju/juju/internal/worker/containerbroker"
	"github.com/juju/juju/internal/worker/containerbroker/mocks"
)

type manifoldConfigSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&manifoldConfigSuite{})

func (s *manifoldConfigSuite) TestInvalidConfigValidate(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	testcases := []struct {
		description string
		config      containerbroker.ManifoldConfig
		err         string
	}{
		{
			description: "Test empty configuration",
			config:      containerbroker.ManifoldConfig{},
			err:         "empty AgentName not valid",
		},
		{
			description: "Test no agent name",
			config:      containerbroker.ManifoldConfig{},
			err:         "empty AgentName not valid",
		},
		{
			description: "Test no api caller name",
			config: containerbroker.ManifoldConfig{
				AgentName: "agent-name",
			},
			err: "empty APICallerName not valid",
		},
		{
			description: "Test no machine lock",
			config: containerbroker.ManifoldConfig{
				AgentName:     "agent-name",
				APICallerName: "api-caller-name",
			},
			err: "nil MachineLock not valid",
		},
		{
			description: "Test no broker func",
			config: containerbroker.ManifoldConfig{
				AgentName:     "agent-name",
				APICallerName: "api-caller-name",
				MachineLock:   mocks.NewMockLock(ctrl),
			},
			err: "nil NewBrokerFunc not valid",
		},
		{
			description: "Test no tracker func",
			config: containerbroker.ManifoldConfig{
				AgentName:     "agent-name",
				APICallerName: "api-caller-name",
				MachineLock:   mocks.NewMockLock(ctrl),
				NewBrokerFunc: func(broker.Config) (environs.InstanceBroker, error) {
					return mocks.NewMockInstanceBroker(ctrl), nil
				},
			},
			err: "nil NewTracker not valid",
		},
	}
	for i, test := range testcases {
		c.Logf("%d %s", i, test.description)
		err := test.config.Validate()
		c.Assert(err, tc.ErrorMatches, test.err)
	}
}

func (s *manifoldConfigSuite) TestValidConfigValidate(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	config := containerbroker.ManifoldConfig{
		AgentName:     "agent",
		APICallerName: "api-caller",
		MachineLock:   mocks.NewMockLock(ctrl),
		NewBrokerFunc: func(broker.Config) (environs.InstanceBroker, error) {
			return mocks.NewMockInstanceBroker(ctrl), nil
		},
		NewTracker: func(context.Context, containerbroker.Config) (worker.Worker, error) {
			return mocks.NewMockWorker(ctrl), nil
		},
	}
	err := config.Validate()
	c.Assert(err, tc.IsNil)
}

type manifoldSuite struct {
	testing.IsolationSuite

	getter      *mocks.MockGetter
	agent       *mocks.MockAgent
	agentConfig *mocks.MockConfig
	broker      *mocks.MockInstanceBroker
	apiCaller   *mocks.MockAPICaller
	worker      *mocks.MockWorker
	machineLock *mocks.MockLock
}

var _ = tc.Suite(&manifoldSuite{})

func (s *manifoldSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.getter = mocks.NewMockGetter(ctrl)
	s.agent = mocks.NewMockAgent(ctrl)
	s.agentConfig = mocks.NewMockConfig(ctrl)
	s.broker = mocks.NewMockInstanceBroker(ctrl)
	s.apiCaller = mocks.NewMockAPICaller(ctrl)
	s.worker = mocks.NewMockWorker(ctrl)
	s.machineLock = mocks.NewMockLock(ctrl)

	return ctrl
}

func (s *manifoldSuite) TestNewTrackerIsCalled(c *tc.C) {
	defer s.setup(c).Finish()

	s.behaviourContext()
	s.behaviourAgent()

	config := containerbroker.ManifoldConfig{
		APICallerName: "baz",
		AgentName:     "moon",
		MachineLock:   s.machineLock,
		NewBrokerFunc: func(broker.Config) (environs.InstanceBroker, error) {
			return s.broker, nil
		},
		NewTracker: func(_ context.Context, cfg containerbroker.Config) (worker.Worker, error) {
			return s.worker, nil
		},
	}
	manifold := containerbroker.Manifold(config)
	result, err := manifold.Start(context.Background(), s.getter)
	c.Assert(err, tc.IsNil)
	c.Assert(result, tc.Equals, s.worker)
}

func (s *manifoldSuite) TestNewTrackerReturnsError(c *tc.C) {
	defer s.setup(c).Finish()

	s.behaviourContext()
	s.behaviourAgent()

	config := containerbroker.ManifoldConfig{
		AgentName:     "moon",
		APICallerName: "baz",
		MachineLock:   s.machineLock,
		NewBrokerFunc: func(broker.Config) (environs.InstanceBroker, error) {
			return s.broker, nil
		},
		NewTracker: func(_ context.Context, cfg containerbroker.Config) (worker.Worker, error) {
			return nil, errors.New("errored")
		},
	}
	manifold := containerbroker.Manifold(config)
	_, err := manifold.Start(context.Background(), s.getter)
	c.Assert(err, tc.ErrorMatches, "errored")
}

func (s *manifoldSuite) behaviourContext() {
	cExp := s.getter.EXPECT()
	cExp.Get("moon", gomock.Any()).SetArg(1, s.agent).Return(nil)
	cExp.Get("baz", gomock.Any()).SetArg(1, s.apiCaller).Return(nil)
}

func (s *manifoldSuite) behaviourAgent() {
	aExp := s.agent.EXPECT()
	aExp.CurrentConfig().Return(s.agentConfig)
}
