// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package containerbroker_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/juju/container/broker"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/worker/containerbroker"
	"github.com/juju/juju/worker/containerbroker/mocks"
	"github.com/juju/testing"
	worker "github.com/juju/worker/v3"
	gc "gopkg.in/check.v1"
)

type manifoldConfigSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&manifoldConfigSuite{})

func (s *manifoldConfigSuite) TestInvalidConfigValidate(c *gc.C) {
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
		c.Assert(err, gc.ErrorMatches, test.err)
	}
}

func (s *manifoldConfigSuite) TestValidConfigValidate(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	config := containerbroker.ManifoldConfig{
		AgentName:     "agent",
		APICallerName: "api-caller",
		MachineLock:   mocks.NewMockLock(ctrl),
		NewBrokerFunc: func(broker.Config) (environs.InstanceBroker, error) {
			return mocks.NewMockInstanceBroker(ctrl), nil
		},
		NewTracker: func(containerbroker.Config) (worker.Worker, error) {
			return mocks.NewMockWorker(ctrl), nil
		},
	}
	err := config.Validate()
	c.Assert(err, gc.IsNil)
}

type manifoldSuite struct {
	testing.IsolationSuite

	context     *mocks.MockContext
	agent       *mocks.MockAgent
	agentConfig *mocks.MockConfig
	broker      *mocks.MockInstanceBroker
	apiCaller   *mocks.MockAPICaller
	worker      *mocks.MockWorker
	machineLock *mocks.MockLock
}

var _ = gc.Suite(&manifoldSuite{})

func (s *manifoldSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.context = mocks.NewMockContext(ctrl)
	s.agent = mocks.NewMockAgent(ctrl)
	s.agentConfig = mocks.NewMockConfig(ctrl)
	s.broker = mocks.NewMockInstanceBroker(ctrl)
	s.apiCaller = mocks.NewMockAPICaller(ctrl)
	s.worker = mocks.NewMockWorker(ctrl)
	s.machineLock = mocks.NewMockLock(ctrl)

	return ctrl
}

func (s *manifoldSuite) TestNewTrackerIsCalled(c *gc.C) {
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
		NewTracker: func(cfg containerbroker.Config) (worker.Worker, error) {
			return s.worker, nil
		},
	}
	manifold := containerbroker.Manifold(config)
	result, err := manifold.Start(s.context)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.Equals, s.worker)
}

func (s *manifoldSuite) TestNewTrackerReturnsError(c *gc.C) {
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
		NewTracker: func(cfg containerbroker.Config) (worker.Worker, error) {
			return nil, errors.New("errored")
		},
	}
	manifold := containerbroker.Manifold(config)
	_, err := manifold.Start(s.context)
	c.Assert(err, gc.ErrorMatches, "errored")
}

func (s *manifoldSuite) behaviourContext() {
	cExp := s.context.EXPECT()
	cExp.Get("moon", gomock.Any()).SetArg(1, s.agent).Return(nil)
	cExp.Get("baz", gomock.Any()).SetArg(1, s.apiCaller).Return(nil)
}

func (s *manifoldSuite) behaviourAgent() {
	aExp := s.agent.EXPECT()
	aExp.CurrentConfig().Return(s.agentConfig)
}
