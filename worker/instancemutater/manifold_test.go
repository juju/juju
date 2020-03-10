// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v3"
	worker "gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/worker/instancemutater"
	"github.com/juju/juju/worker/instancemutater/mocks"
)

type modelManifoldConfigSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&modelManifoldConfigSuite{})

func (s *modelManifoldConfigSuite) TestInvalidConfigValidate(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	testcases := []struct {
		description string
		config      instancemutater.ModelManifoldConfig
		err         string
	}{
		{
			description: "Test empty configuration",
			config:      instancemutater.ModelManifoldConfig{},
			err:         "nil Logger not valid",
		},
		{
			description: "Test no Logger",
			config:      instancemutater.ModelManifoldConfig{},
			err:         "nil Logger not valid",
		},
		{
			description: "Test no new worker constructor",
			config: instancemutater.ModelManifoldConfig{
				Logger: mocks.NewMockLogger(ctrl),
			},
			err: "nil NewWorker not valid",
		},
		{
			description: "Test no new client constructor",
			config: instancemutater.ModelManifoldConfig{
				Logger: mocks.NewMockLogger(ctrl),
				NewWorker: func(cfg instancemutater.Config) (worker.Worker, error) {
					return mocks.NewMockWorker(ctrl), nil
				},
			},
			err: "nil NewClient not valid",
		},
		{
			description: "Test no agent name",
			config: instancemutater.ModelManifoldConfig{
				Logger: mocks.NewMockLogger(ctrl),
				NewWorker: func(cfg instancemutater.Config) (worker.Worker, error) {
					return mocks.NewMockWorker(ctrl), nil
				},
				NewClient: func(base.APICaller) instancemutater.InstanceMutaterAPI {
					return mocks.NewMockInstanceMutaterAPI(ctrl)
				},
			},
			err: "empty AgentName not valid",
		},
		{
			description: "Test no environ name",
			config: instancemutater.ModelManifoldConfig{
				Logger: mocks.NewMockLogger(ctrl),
				NewWorker: func(cfg instancemutater.Config) (worker.Worker, error) {
					return mocks.NewMockWorker(ctrl), nil
				},
				NewClient: func(base.APICaller) instancemutater.InstanceMutaterAPI {
					return mocks.NewMockInstanceMutaterAPI(ctrl)
				},
				AgentName: "agent",
			},
			err: "empty EnvironName not valid",
		},
		{
			description: "Test no api caller name",
			config: instancemutater.ModelManifoldConfig{
				Logger: mocks.NewMockLogger(ctrl),
				NewWorker: func(cfg instancemutater.Config) (worker.Worker, error) {
					return mocks.NewMockWorker(ctrl), nil
				},
				NewClient: func(base.APICaller) instancemutater.InstanceMutaterAPI {
					return mocks.NewMockInstanceMutaterAPI(ctrl)
				},
				AgentName:   "agent",
				EnvironName: "environ",
			},
			err: "empty APICallerName not valid",
		},
	}
	for i, test := range testcases {
		c.Logf("%d %s", i, test.description)
		err := test.config.Validate()
		c.Assert(err, gc.ErrorMatches, test.err)
	}
}

func (s *modelManifoldConfigSuite) TestValidConfigValidate(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	config := instancemutater.ModelManifoldConfig{
		Logger: mocks.NewMockLogger(ctrl),
		NewWorker: func(cfg instancemutater.Config) (worker.Worker, error) {
			return mocks.NewMockWorker(ctrl), nil
		},
		NewClient: func(base.APICaller) instancemutater.InstanceMutaterAPI {
			return mocks.NewMockInstanceMutaterAPI(ctrl)
		},
		AgentName:     "agent",
		EnvironName:   "environ",
		APICallerName: "api-caller",
	}
	err := config.Validate()
	c.Assert(err, gc.IsNil)
}

type environAPIManifoldSuite struct {
	testing.IsolationSuite

	logger    *mocks.MockLogger
	context   *mocks.MockContext
	agent     *mocks.MockAgent
	environ   *mocks.MockEnviron
	apiCaller *mocks.MockAPICaller
	worker    *mocks.MockWorker
}

var _ = gc.Suite(&environAPIManifoldSuite{})

func (s *environAPIManifoldSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.logger = mocks.NewMockLogger(ctrl)
	s.context = mocks.NewMockContext(ctrl)
	s.agent = mocks.NewMockAgent(ctrl)
	s.environ = mocks.NewMockEnviron(ctrl)
	s.apiCaller = mocks.NewMockAPICaller(ctrl)
	s.worker = mocks.NewMockWorker(ctrl)

	return ctrl
}

func (s *environAPIManifoldSuite) TestStartReturnsWorker(c *gc.C) {
	defer s.setup(c).Finish()

	cExp := s.context.EXPECT()
	cExp.Get("moon", gomock.Any()).SetArg(1, s.agent).Return(nil)
	cExp.Get("foobar", gomock.Any()).SetArg(1, s.environ).Return(nil)
	cExp.Get("baz", gomock.Any()).SetArg(1, s.apiCaller).Return(nil)

	config := instancemutater.EnvironAPIConfig{
		EnvironName:   "foobar",
		APICallerName: "baz",
		AgentName:     "moon",
	}
	manifold := instancemutater.EnvironAPIManifold(config, func(environ environs.Environ, apiCaller base.APICaller, agent agent.Agent) (worker.Worker, error) {
		c.Assert(environ, gc.Equals, s.environ)
		c.Assert(apiCaller, gc.Equals, s.apiCaller)
		c.Assert(agent, gc.Equals, s.agent)

		return s.worker, nil
	})
	result, err := manifold.Start(s.context)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.Equals, s.worker)
}

func (s *environAPIManifoldSuite) TestMissingEnvironFromContext(c *gc.C) {
	defer s.setup(c).Finish()

	cExp := s.context.EXPECT()
	cExp.Get("moon", gomock.Any()).SetArg(1, s.agent).Return(nil)
	cExp.Get("foobar", gomock.Any()).Return(errors.New("missing"))

	config := instancemutater.EnvironAPIConfig{
		EnvironName:   "foobar",
		APICallerName: "baz",
		AgentName:     "moon",
	}
	manifold := instancemutater.EnvironAPIManifold(config, func(environs.Environ, base.APICaller, agent.Agent) (worker.Worker, error) {
		c.Fail()
		return nil, nil
	})
	_, err := manifold.Start(s.context)
	c.Assert(err, gc.ErrorMatches, "missing")
}

func (s *environAPIManifoldSuite) TestMissingAPICallerFromContext(c *gc.C) {
	defer s.setup(c).Finish()

	cExp := s.context.EXPECT()
	cExp.Get("moon", gomock.Any()).SetArg(1, s.agent).Return(nil)
	cExp.Get("foobar", gomock.Any()).SetArg(1, s.environ).Return(nil)
	cExp.Get("baz", gomock.Any()).Return(errors.New("missing"))

	config := instancemutater.EnvironAPIConfig{
		EnvironName:   "foobar",
		APICallerName: "baz",
		AgentName:     "moon",
	}
	manifold := instancemutater.EnvironAPIManifold(config, func(environs.Environ, base.APICaller, agent.Agent) (worker.Worker, error) {
		c.Fail()
		return nil, nil
	})
	_, err := manifold.Start(s.context)
	c.Assert(err, gc.ErrorMatches, "missing")
}

type modelManifoldSuite struct {
	testing.IsolationSuite

	logger      *mocks.MockLogger
	context     *mocks.MockContext
	agent       *mocks.MockAgent
	agentConfig *mocks.MockConfig
	environ     environShim
	apiCaller   *mocks.MockAPICaller
	worker      *mocks.MockWorker
	api         *mocks.MockInstanceMutaterAPI
}

var _ = gc.Suite(&modelManifoldSuite{})

func (s *modelManifoldSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.logger = mocks.NewMockLogger(ctrl)
	s.context = mocks.NewMockContext(ctrl)
	s.agent = mocks.NewMockAgent(ctrl)
	s.agentConfig = mocks.NewMockConfig(ctrl)
	s.environ = environShim{
		MockEnviron:     mocks.NewMockEnviron(ctrl),
		MockLXDProfiler: mocks.NewMockLXDProfiler(ctrl),
	}
	s.apiCaller = mocks.NewMockAPICaller(ctrl)
	s.worker = mocks.NewMockWorker(ctrl)
	s.api = mocks.NewMockInstanceMutaterAPI(ctrl)

	return ctrl
}

func (s *modelManifoldSuite) TestNewWorkerIsCalled(c *gc.C) {
	defer s.setup(c).Finish()

	s.behaviourContext()
	s.behaviourAgent()

	config := instancemutater.ModelManifoldConfig{
		EnvironName:   "foobar",
		APICallerName: "baz",
		AgentName:     "moon",
		Logger:        s.logger,
		NewWorker: func(cfg instancemutater.Config) (worker.Worker, error) {
			return s.worker, nil
		},
		NewClient: func(base.APICaller) instancemutater.InstanceMutaterAPI {
			return s.api
		},
	}
	manifold := instancemutater.ModelManifold(config)
	result, err := manifold.Start(s.context)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.Equals, s.worker)
}

func (s *modelManifoldSuite) TestNewWorkerFromK8sController(c *gc.C) {
	defer s.setup(c).Finish()

	s.behaviourContext()
	s.behaviorK8sController()

	config := instancemutater.ModelManifoldConfig{
		EnvironName:   "foobar",
		APICallerName: "baz",
		AgentName:     "moon",
		Logger:        s.logger,
		NewWorker: func(cfg instancemutater.Config) (worker.Worker, error) {
			return s.worker, nil
		},
		NewClient: func(base.APICaller) instancemutater.InstanceMutaterAPI {
			return s.api
		},
	}
	manifold := instancemutater.ModelManifold(config)
	result, err := manifold.Start(s.context)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.Equals, s.worker)
}

func (s *modelManifoldSuite) TestNewWorkerReturnsError(c *gc.C) {
	defer s.setup(c).Finish()

	s.behaviourContext()
	s.behaviourAgent()

	config := instancemutater.ModelManifoldConfig{
		EnvironName:   "foobar",
		APICallerName: "baz",
		AgentName:     "moon",
		Logger:        s.logger,
		NewWorker: func(cfg instancemutater.Config) (worker.Worker, error) {
			return nil, errors.New("errored")
		},
		NewClient: func(base.APICaller) instancemutater.InstanceMutaterAPI {
			return s.api
		},
	}
	manifold := instancemutater.ModelManifold(config)
	_, err := manifold.Start(s.context)
	c.Assert(err, gc.ErrorMatches, "cannot start model instance-mutater worker: errored")
}

func (s *modelManifoldSuite) TestConfigValidatesForMissingWorker(c *gc.C) {
	defer s.setup(c).Finish()

	s.behaviourContext()

	config := instancemutater.ModelManifoldConfig{
		EnvironName:   "foobar",
		APICallerName: "baz",
		AgentName:     "moon",
		Logger:        s.logger,
	}
	manifold := instancemutater.ModelManifold(config)
	_, err := manifold.Start(s.context)
	c.Assert(err, gc.ErrorMatches, "nil NewWorker not valid")
}

func (s *modelManifoldSuite) TestConfigValidatesForMissingClient(c *gc.C) {
	defer s.setup(c).Finish()

	s.behaviourContext()

	config := instancemutater.ModelManifoldConfig{
		EnvironName:   "foobar",
		APICallerName: "baz",
		AgentName:     "moon",
		Logger:        s.logger,
		NewWorker: func(cfg instancemutater.Config) (worker.Worker, error) {
			return s.worker, nil
		},
	}
	manifold := instancemutater.ModelManifold(config)
	_, err := manifold.Start(s.context)
	c.Assert(err, gc.ErrorMatches, "nil NewClient not valid")
}

func (s *modelManifoldSuite) behaviourContext() {
	cExp := s.context.EXPECT()
	cExp.Get("moon", gomock.Any()).SetArg(1, s.agent).Return(nil)
	cExp.Get("foobar", gomock.Any()).SetArg(1, s.environ).Return(nil)
	cExp.Get("baz", gomock.Any()).SetArg(1, s.apiCaller).Return(nil)
}

func (s *modelManifoldSuite) behaviourAgent() {
	aExp := s.agent.EXPECT()
	aExp.CurrentConfig().Return(s.agentConfig)

	cExp := s.agentConfig.EXPECT()
	cExp.Tag().Return(names.MachineTag{})
}

func (s *modelManifoldSuite) behaviorK8sController() {
	aExp := s.agent.EXPECT()
	aExp.CurrentConfig().Return(s.agentConfig)

	cExp := s.agentConfig.EXPECT()
	cExp.Tag().Return(names.ControllerAgentTag{})
}

type environShim struct {
	*mocks.MockEnviron
	*mocks.MockLXDProfiler
}

type machineManifoldConfigSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&machineManifoldConfigSuite{})

func (s *machineManifoldConfigSuite) TestInvalidConfigValidate(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	testcases := []struct {
		description string
		config      instancemutater.MachineManifoldConfig
		err         string
	}{
		{
			description: "Test empty configuration",
			config:      instancemutater.MachineManifoldConfig{},
			err:         "nil Logger not valid",
		},
		{
			description: "Test no Logger",
			config:      instancemutater.MachineManifoldConfig{},
			err:         "nil Logger not valid",
		},
		{
			description: "Test no new worker constructor",
			config: instancemutater.MachineManifoldConfig{
				Logger: mocks.NewMockLogger(ctrl),
			},
			err: "nil NewWorker not valid",
		},
		{
			description: "Test no new client constructor",
			config: instancemutater.MachineManifoldConfig{
				Logger: mocks.NewMockLogger(ctrl),
				NewWorker: func(cfg instancemutater.Config) (worker.Worker, error) {
					return mocks.NewMockWorker(ctrl), nil
				},
			},
			err: "nil NewClient not valid",
		},
		{
			description: "Test no agent name",
			config: instancemutater.MachineManifoldConfig{
				Logger: mocks.NewMockLogger(ctrl),
				NewWorker: func(cfg instancemutater.Config) (worker.Worker, error) {
					return mocks.NewMockWorker(ctrl), nil
				},
				NewClient: func(base.APICaller) instancemutater.InstanceMutaterAPI {
					return mocks.NewMockInstanceMutaterAPI(ctrl)
				},
			},
			err: "empty AgentName not valid",
		},
		{
			description: "Test no environ name",
			config: instancemutater.MachineManifoldConfig{
				Logger: mocks.NewMockLogger(ctrl),
				NewWorker: func(cfg instancemutater.Config) (worker.Worker, error) {
					return mocks.NewMockWorker(ctrl), nil
				},
				NewClient: func(base.APICaller) instancemutater.InstanceMutaterAPI {
					return mocks.NewMockInstanceMutaterAPI(ctrl)
				},
				AgentName: "agent",
			},
			err: "empty BrokerName not valid",
		},
		{
			description: "Test no api caller name",
			config: instancemutater.MachineManifoldConfig{
				Logger: mocks.NewMockLogger(ctrl),
				NewWorker: func(cfg instancemutater.Config) (worker.Worker, error) {
					return mocks.NewMockWorker(ctrl), nil
				},
				NewClient: func(base.APICaller) instancemutater.InstanceMutaterAPI {
					return mocks.NewMockInstanceMutaterAPI(ctrl)
				},
				AgentName:  "agent",
				BrokerName: "broker",
			},
			err: "empty APICallerName not valid",
		},
	}
	for i, test := range testcases {
		c.Logf("%d %s", i, test.description)
		err := test.config.Validate()
		c.Assert(err, gc.ErrorMatches, test.err)
	}
}

func (s *machineManifoldConfigSuite) TestValidConfigValidate(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	config := instancemutater.MachineManifoldConfig{
		Logger: mocks.NewMockLogger(ctrl),
		NewWorker: func(cfg instancemutater.Config) (worker.Worker, error) {
			return mocks.NewMockWorker(ctrl), nil
		},
		NewClient: func(base.APICaller) instancemutater.InstanceMutaterAPI {
			return mocks.NewMockInstanceMutaterAPI(ctrl)
		},
		AgentName:     "agent",
		BrokerName:    "broker",
		APICallerName: "api-caller",
	}
	err := config.Validate()
	c.Assert(err, gc.IsNil)
}

type brokerAPIManifoldSuite struct {
	testing.IsolationSuite

	logger    *mocks.MockLogger
	context   *mocks.MockContext
	agent     *mocks.MockAgent
	broker    *mocks.MockInstanceBroker
	apiCaller *mocks.MockAPICaller
	worker    *mocks.MockWorker
}

var _ = gc.Suite(&brokerAPIManifoldSuite{})

func (s *brokerAPIManifoldSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.logger = mocks.NewMockLogger(ctrl)
	s.context = mocks.NewMockContext(ctrl)
	s.agent = mocks.NewMockAgent(ctrl)
	s.broker = mocks.NewMockInstanceBroker(ctrl)
	s.apiCaller = mocks.NewMockAPICaller(ctrl)
	s.worker = mocks.NewMockWorker(ctrl)

	return ctrl
}

func (s *brokerAPIManifoldSuite) TestStartReturnsWorker(c *gc.C) {
	defer s.setup(c).Finish()

	cExp := s.context.EXPECT()
	cExp.Get("moon", gomock.Any()).SetArg(1, s.agent).Return(nil)
	cExp.Get("foobar", gomock.Any()).SetArg(1, s.broker).Return(nil)
	cExp.Get("baz", gomock.Any()).SetArg(1, s.apiCaller).Return(nil)

	config := instancemutater.BrokerAPIConfig{
		BrokerName:    "foobar",
		APICallerName: "baz",
		AgentName:     "moon",
	}
	manifold := instancemutater.BrokerAPIManifold(config, func(broker environs.InstanceBroker, apiCaller base.APICaller, agent agent.Agent) (worker.Worker, error) {
		c.Assert(broker, gc.Equals, s.broker)
		c.Assert(apiCaller, gc.Equals, s.apiCaller)
		c.Assert(agent, gc.Equals, s.agent)

		return s.worker, nil
	})
	result, err := manifold.Start(s.context)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.Equals, s.worker)
}

func (s *brokerAPIManifoldSuite) TestMissingBrokerFromContext(c *gc.C) {
	defer s.setup(c).Finish()

	cExp := s.context.EXPECT()
	cExp.Get("moon", gomock.Any()).SetArg(1, s.agent).Return(nil)
	cExp.Get("foobar", gomock.Any()).Return(errors.New("missing"))

	config := instancemutater.BrokerAPIConfig{
		BrokerName:    "foobar",
		APICallerName: "baz",
		AgentName:     "moon",
	}
	manifold := instancemutater.BrokerAPIManifold(config, func(environs.InstanceBroker, base.APICaller, agent.Agent) (worker.Worker, error) {
		c.Fail()
		return nil, nil
	})
	_, err := manifold.Start(s.context)
	c.Assert(err, gc.ErrorMatches, "missing")
}

func (s *brokerAPIManifoldSuite) TestMissingAPICallerFromContext(c *gc.C) {
	defer s.setup(c).Finish()

	cExp := s.context.EXPECT()
	cExp.Get("moon", gomock.Any()).SetArg(1, s.agent).Return(nil)
	cExp.Get("foobar", gomock.Any()).SetArg(1, s.broker).Return(nil)
	cExp.Get("baz", gomock.Any()).Return(errors.New("missing"))

	config := instancemutater.BrokerAPIConfig{
		BrokerName:    "foobar",
		APICallerName: "baz",
		AgentName:     "moon",
	}
	manifold := instancemutater.BrokerAPIManifold(config, func(environs.InstanceBroker, base.APICaller, agent.Agent) (worker.Worker, error) {
		c.Fail()
		return nil, nil
	})
	_, err := manifold.Start(s.context)
	c.Assert(err, gc.ErrorMatches, "missing")
}

type machineManifoldSuite struct {
	testing.IsolationSuite

	logger      *mocks.MockLogger
	context     *mocks.MockContext
	agent       *mocks.MockAgent
	agentConfig *mocks.MockConfig
	broker      brokerShim
	apiCaller   *mocks.MockAPICaller
	worker      *mocks.MockWorker
	api         *mocks.MockInstanceMutaterAPI
}

var _ = gc.Suite(&machineManifoldSuite{})

func (s *machineManifoldSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.logger = mocks.NewMockLogger(ctrl)
	s.context = mocks.NewMockContext(ctrl)
	s.agent = mocks.NewMockAgent(ctrl)
	s.agentConfig = mocks.NewMockConfig(ctrl)
	s.broker = brokerShim{
		MockInstanceBroker: mocks.NewMockInstanceBroker(ctrl),
		MockLXDProfiler:    mocks.NewMockLXDProfiler(ctrl),
	}
	s.apiCaller = mocks.NewMockAPICaller(ctrl)
	s.worker = mocks.NewMockWorker(ctrl)
	s.api = mocks.NewMockInstanceMutaterAPI(ctrl)

	return ctrl
}

func (s *machineManifoldSuite) TestNewWorkerIsCalled(c *gc.C) {
	defer s.setup(c).Finish()

	s.behaviourContext()
	s.behaviourAgent()

	config := instancemutater.MachineManifoldConfig{
		BrokerName:    "foobar",
		APICallerName: "baz",
		AgentName:     "moon",
		Logger:        s.logger,
		NewWorker: func(cfg instancemutater.Config) (worker.Worker, error) {
			return s.worker, nil
		},
		NewClient: func(base.APICaller) instancemutater.InstanceMutaterAPI {
			return s.api
		},
	}
	manifold := instancemutater.MachineManifold(config)
	result, err := manifold.Start(s.context)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.Equals, s.worker)
}

func (s *machineManifoldSuite) TestNewWorkerIsRejectedForK8sController(c *gc.C) {
	defer s.setup(c).Finish()

	s.behaviourContext()
	s.behaviorK8sController()

	config := instancemutater.MachineManifoldConfig{
		BrokerName:    "foobar",
		APICallerName: "baz",
		AgentName:     "moon",
		Logger:        s.logger,
		NewWorker: func(cfg instancemutater.Config) (worker.Worker, error) {
			return s.worker, nil
		},
		NewClient: func(base.APICaller) instancemutater.InstanceMutaterAPI {
			return s.api
		},
	}
	manifold := instancemutater.MachineManifold(config)
	result, err := manifold.Start(s.context)
	c.Assert(err, gc.Equals, dependency.ErrUninstall)
	c.Assert(result, gc.IsNil)
}

func (s *machineManifoldSuite) TestNewWorkerReturnsError(c *gc.C) {
	defer s.setup(c).Finish()

	s.behaviourContext()
	s.behaviourAgent()

	config := instancemutater.MachineManifoldConfig{
		BrokerName:    "foobar",
		APICallerName: "baz",
		AgentName:     "moon",
		Logger:        s.logger,
		NewWorker: func(cfg instancemutater.Config) (worker.Worker, error) {
			return nil, errors.New("errored")
		},
		NewClient: func(base.APICaller) instancemutater.InstanceMutaterAPI {
			return s.api
		},
	}
	manifold := instancemutater.MachineManifold(config)
	_, err := manifold.Start(s.context)
	c.Assert(err, gc.ErrorMatches, "cannot start machine instancemutater worker: errored")
}

func (s *machineManifoldSuite) TestConfigValidatesForMissingWorker(c *gc.C) {
	defer s.setup(c).Finish()

	s.behaviourContext()

	config := instancemutater.MachineManifoldConfig{
		BrokerName:    "foobar",
		APICallerName: "baz",
		AgentName:     "moon",
		Logger:        s.logger,
	}
	manifold := instancemutater.MachineManifold(config)
	_, err := manifold.Start(s.context)
	c.Assert(err, gc.ErrorMatches, "nil NewWorker not valid")
}

func (s *machineManifoldSuite) TestConfigValidatesForMissingClient(c *gc.C) {
	defer s.setup(c).Finish()

	s.behaviourContext()

	config := instancemutater.MachineManifoldConfig{
		BrokerName:    "foobar",
		APICallerName: "baz",
		AgentName:     "moon",
		Logger:        s.logger,
		NewWorker: func(cfg instancemutater.Config) (worker.Worker, error) {
			return s.worker, nil
		},
	}
	manifold := instancemutater.MachineManifold(config)
	_, err := manifold.Start(s.context)
	c.Assert(err, gc.ErrorMatches, "nil NewClient not valid")
}

func (s *machineManifoldSuite) behaviourContext() {
	cExp := s.context.EXPECT()
	cExp.Get("moon", gomock.Any()).SetArg(1, s.agent).Return(nil)
	cExp.Get("foobar", gomock.Any()).SetArg(1, s.broker).Return(nil)
	cExp.Get("baz", gomock.Any()).SetArg(1, s.apiCaller).Return(nil)
}

func (s *machineManifoldSuite) behaviourAgent() {
	aExp := s.agent.EXPECT()
	aExp.CurrentConfig().Return(s.agentConfig)

	cExp := s.agentConfig.EXPECT()
	cExp.Tag().Return(names.MachineTag{})
}

func (s *machineManifoldSuite) behaviorK8sController() {
	aExp := s.agent.EXPECT()
	aExp.CurrentConfig().Return(s.agentConfig)

	cExp := s.agentConfig.EXPECT()
	cExp.Tag().Return(names.ControllerAgentTag{})

	lExp := s.logger.EXPECT()
	lExp.Warningf(gomock.Any(), "controller")
}

type brokerShim struct {
	*mocks.MockInstanceBroker
	*mocks.MockLXDProfiler
}
