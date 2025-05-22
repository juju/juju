// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater_test

import (
	"context"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/environs"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/instancemutater"
	"github.com/juju/juju/internal/worker/instancemutater/mocks"
)

type modelManifoldConfigSuite struct {
	testhelpers.IsolationSuite
}

func TestModelManifoldConfigSuite(t *stdtesting.T) {
	tc.Run(t, &modelManifoldConfigSuite{})
}
func (s *modelManifoldConfigSuite) TestInvalidConfigValidate(c *tc.C) {
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
				Logger: loggertesting.WrapCheckLog(c),
			},
			err: "nil NewWorker not valid",
		},
		{
			description: "Test no new client constructor",
			config: instancemutater.ModelManifoldConfig{
				Logger: loggertesting.WrapCheckLog(c),
				NewWorker: func(_ context.Context, cfg instancemutater.Config) (worker.Worker, error) {
					return mocks.NewMockWorker(ctrl), nil
				},
			},
			err: "nil NewClient not valid",
		},
		{
			description: "Test no agent name",
			config: instancemutater.ModelManifoldConfig{
				Logger: loggertesting.WrapCheckLog(c),
				NewWorker: func(_ context.Context, cfg instancemutater.Config) (worker.Worker, error) {
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
				Logger: loggertesting.WrapCheckLog(c),
				NewWorker: func(_ context.Context, cfg instancemutater.Config) (worker.Worker, error) {
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
				Logger: loggertesting.WrapCheckLog(c),
				NewWorker: func(_ context.Context, cfg instancemutater.Config) (worker.Worker, error) {
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
		c.Assert(err, tc.ErrorMatches, test.err)
	}
}

func (s *modelManifoldConfigSuite) TestValidConfigValidate(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	config := instancemutater.ModelManifoldConfig{
		Logger: loggertesting.WrapCheckLog(c),
		NewWorker: func(_ context.Context, cfg instancemutater.Config) (worker.Worker, error) {
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
	c.Assert(err, tc.IsNil)
}

type environAPIManifoldSuite struct {
	testhelpers.IsolationSuite

	getter    *mocks.MockGetter
	agent     *mocks.MockAgent
	environ   *mocks.MockEnviron
	apiCaller *mocks.MockAPICaller
	worker    *mocks.MockWorker
}

func TestEnvironAPIManifoldSuite(t *stdtesting.T) {
	tc.Run(t, &environAPIManifoldSuite{})
}

func (s *environAPIManifoldSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.getter = mocks.NewMockGetter(ctrl)
	s.agent = mocks.NewMockAgent(ctrl)
	s.environ = mocks.NewMockEnviron(ctrl)
	s.apiCaller = mocks.NewMockAPICaller(ctrl)
	s.worker = mocks.NewMockWorker(ctrl)

	return ctrl
}

func (s *environAPIManifoldSuite) TestStartReturnsWorker(c *tc.C) {
	defer s.setup(c).Finish()

	cExp := s.getter.EXPECT()
	cExp.Get("moon", gomock.Any()).SetArg(1, s.agent).Return(nil)
	cExp.Get("foobar", gomock.Any()).SetArg(1, s.environ).Return(nil)
	cExp.Get("baz", gomock.Any()).SetArg(1, s.apiCaller).Return(nil)

	config := instancemutater.EnvironAPIConfig{
		EnvironName:   "foobar",
		APICallerName: "baz",
		AgentName:     "moon",
	}
	manifold := instancemutater.EnvironAPIManifold(config, func(_ context.Context, environ environs.Environ, apiCaller base.APICaller, agent agent.Agent) (worker.Worker, error) {
		c.Assert(environ, tc.Equals, s.environ)
		c.Assert(apiCaller, tc.Equals, s.apiCaller)
		c.Assert(agent, tc.Equals, s.agent)

		return s.worker, nil
	})
	result, err := manifold.Start(c.Context(), s.getter)
	c.Assert(err, tc.IsNil)
	c.Assert(result, tc.Equals, s.worker)
}

func (s *environAPIManifoldSuite) TestMissingEnvironFromContext(c *tc.C) {
	defer s.setup(c).Finish()

	cExp := s.getter.EXPECT()
	cExp.Get("moon", gomock.Any()).SetArg(1, s.agent).Return(nil)
	cExp.Get("foobar", gomock.Any()).Return(errors.New("missing"))

	config := instancemutater.EnvironAPIConfig{
		EnvironName:   "foobar",
		APICallerName: "baz",
		AgentName:     "moon",
	}
	manifold := instancemutater.EnvironAPIManifold(config, func(context.Context, environs.Environ, base.APICaller, agent.Agent) (worker.Worker, error) {
		c.Fail()
		return nil, nil
	})
	_, err := manifold.Start(c.Context(), s.getter)
	c.Assert(err, tc.ErrorMatches, "missing")
}

func (s *environAPIManifoldSuite) TestMissingAPICallerFromContext(c *tc.C) {
	defer s.setup(c).Finish()

	cExp := s.getter.EXPECT()
	cExp.Get("moon", gomock.Any()).SetArg(1, s.agent).Return(nil)
	cExp.Get("foobar", gomock.Any()).SetArg(1, s.environ).Return(nil)
	cExp.Get("baz", gomock.Any()).Return(errors.New("missing"))

	config := instancemutater.EnvironAPIConfig{
		EnvironName:   "foobar",
		APICallerName: "baz",
		AgentName:     "moon",
	}
	manifold := instancemutater.EnvironAPIManifold(config, func(context.Context, environs.Environ, base.APICaller, agent.Agent) (worker.Worker, error) {
		c.Fail()
		return nil, nil
	})
	_, err := manifold.Start(c.Context(), s.getter)
	c.Assert(err, tc.ErrorMatches, "missing")
}

type modelManifoldSuite struct {
	testhelpers.IsolationSuite

	getter      *mocks.MockGetter
	agent       *mocks.MockAgent
	agentConfig *mocks.MockConfig
	environ     environShim
	apiCaller   *mocks.MockAPICaller
	worker      *mocks.MockWorker
	api         *mocks.MockInstanceMutaterAPI
}

func TestModelManifoldSuite(t *stdtesting.T) {
	tc.Run(t, &modelManifoldSuite{})
}

func (s *modelManifoldSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.getter = mocks.NewMockGetter(ctrl)
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

func (s *modelManifoldSuite) TestNewWorkerIsCalled(c *tc.C) {
	defer s.setup(c).Finish()

	s.behaviourContext()
	s.behaviourAgent()

	config := instancemutater.ModelManifoldConfig{
		EnvironName:   "foobar",
		APICallerName: "baz",
		AgentName:     "moon",
		Logger:        loggertesting.WrapCheckLog(c),
		NewWorker: func(_ context.Context, cfg instancemutater.Config) (worker.Worker, error) {
			return s.worker, nil
		},
		NewClient: func(base.APICaller) instancemutater.InstanceMutaterAPI {
			return s.api
		},
	}
	manifold := instancemutater.ModelManifold(config)
	result, err := manifold.Start(c.Context(), s.getter)
	c.Assert(err, tc.IsNil)
	c.Assert(result, tc.Equals, s.worker)
}

func (s *modelManifoldSuite) TestNewWorkerFromK8sController(c *tc.C) {
	defer s.setup(c).Finish()

	s.behaviourContext()
	s.behaviorK8sController()

	config := instancemutater.ModelManifoldConfig{
		EnvironName:   "foobar",
		APICallerName: "baz",
		AgentName:     "moon",
		Logger:        loggertesting.WrapCheckLog(c),
		NewWorker: func(_ context.Context, cfg instancemutater.Config) (worker.Worker, error) {
			return s.worker, nil
		},
		NewClient: func(base.APICaller) instancemutater.InstanceMutaterAPI {
			return s.api
		},
	}
	manifold := instancemutater.ModelManifold(config)
	result, err := manifold.Start(c.Context(), s.getter)
	c.Assert(err, tc.IsNil)
	c.Assert(result, tc.Equals, s.worker)
}

func (s *modelManifoldSuite) TestNewWorkerReturnsError(c *tc.C) {
	defer s.setup(c).Finish()

	s.behaviourContext()
	s.behaviourAgent()

	config := instancemutater.ModelManifoldConfig{
		EnvironName:   "foobar",
		APICallerName: "baz",
		AgentName:     "moon",
		Logger:        loggertesting.WrapCheckLog(c),
		NewWorker: func(_ context.Context, cfg instancemutater.Config) (worker.Worker, error) {
			return nil, errors.New("errored")
		},
		NewClient: func(base.APICaller) instancemutater.InstanceMutaterAPI {
			return s.api
		},
	}
	manifold := instancemutater.ModelManifold(config)
	_, err := manifold.Start(c.Context(), s.getter)
	c.Assert(err, tc.ErrorMatches, "cannot start model instance-mutater worker: errored")
}

func (s *modelManifoldSuite) TestConfigValidatesForMissingWorker(c *tc.C) {
	defer s.setup(c).Finish()

	s.behaviourContext()

	config := instancemutater.ModelManifoldConfig{
		EnvironName:   "foobar",
		APICallerName: "baz",
		AgentName:     "moon",
		Logger:        loggertesting.WrapCheckLog(c),
	}
	manifold := instancemutater.ModelManifold(config)
	_, err := manifold.Start(c.Context(), s.getter)
	c.Assert(err, tc.ErrorMatches, "nil NewWorker not valid")
}

func (s *modelManifoldSuite) TestConfigValidatesForMissingClient(c *tc.C) {
	defer s.setup(c).Finish()

	s.behaviourContext()

	config := instancemutater.ModelManifoldConfig{
		EnvironName:   "foobar",
		APICallerName: "baz",
		AgentName:     "moon",
		Logger:        loggertesting.WrapCheckLog(c),
		NewWorker: func(_ context.Context, cfg instancemutater.Config) (worker.Worker, error) {
			return s.worker, nil
		},
	}
	manifold := instancemutater.ModelManifold(config)
	_, err := manifold.Start(c.Context(), s.getter)
	c.Assert(err, tc.ErrorMatches, "nil NewClient not valid")
}

func (s *modelManifoldSuite) behaviourContext() {
	cExp := s.getter.EXPECT()
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
	testhelpers.IsolationSuite
}

func TestMachineManifoldConfigSuite(t *stdtesting.T) {
	tc.Run(t, &machineManifoldConfigSuite{})
}
func (s *machineManifoldConfigSuite) TestInvalidConfigValidate(c *tc.C) {
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
				Logger: loggertesting.WrapCheckLog(c),
			},
			err: "nil NewWorker not valid",
		},
		{
			description: "Test no new client constructor",
			config: instancemutater.MachineManifoldConfig{
				Logger: loggertesting.WrapCheckLog(c),
				NewWorker: func(_ context.Context, cfg instancemutater.Config) (worker.Worker, error) {
					return mocks.NewMockWorker(ctrl), nil
				},
			},
			err: "nil NewClient not valid",
		},
		{
			description: "Test no agent name",
			config: instancemutater.MachineManifoldConfig{
				Logger: loggertesting.WrapCheckLog(c),
				NewWorker: func(_ context.Context, cfg instancemutater.Config) (worker.Worker, error) {
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
				Logger: loggertesting.WrapCheckLog(c),
				NewWorker: func(_ context.Context, cfg instancemutater.Config) (worker.Worker, error) {
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
				Logger: loggertesting.WrapCheckLog(c),
				NewWorker: func(_ context.Context, cfg instancemutater.Config) (worker.Worker, error) {
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
		c.Assert(err, tc.ErrorMatches, test.err)
	}
}

func (s *machineManifoldConfigSuite) TestValidConfigValidate(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	config := instancemutater.MachineManifoldConfig{
		Logger: loggertesting.WrapCheckLog(c),
		NewWorker: func(_ context.Context, cfg instancemutater.Config) (worker.Worker, error) {
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
	c.Assert(err, tc.IsNil)
}

type brokerAPIManifoldSuite struct {
	testhelpers.IsolationSuite

	getter    *mocks.MockGetter
	agent     *mocks.MockAgent
	broker    *mocks.MockInstanceBroker
	apiCaller *mocks.MockAPICaller
	worker    *mocks.MockWorker
}

func TestBrokerAPIManifoldSuite(t *stdtesting.T) {
	tc.Run(t, &brokerAPIManifoldSuite{})
}

func (s *brokerAPIManifoldSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.getter = mocks.NewMockGetter(ctrl)
	s.agent = mocks.NewMockAgent(ctrl)
	s.broker = mocks.NewMockInstanceBroker(ctrl)
	s.apiCaller = mocks.NewMockAPICaller(ctrl)
	s.worker = mocks.NewMockWorker(ctrl)

	return ctrl
}

func (s *brokerAPIManifoldSuite) TestStartReturnsWorker(c *tc.C) {
	defer s.setup(c).Finish()

	cExp := s.getter.EXPECT()
	cExp.Get("moon", gomock.Any()).SetArg(1, s.agent).Return(nil)
	cExp.Get("foobar", gomock.Any()).SetArg(1, s.broker).Return(nil)
	cExp.Get("baz", gomock.Any()).SetArg(1, s.apiCaller).Return(nil)

	config := instancemutater.BrokerAPIConfig{
		BrokerName:    "foobar",
		APICallerName: "baz",
		AgentName:     "moon",
	}
	manifold := instancemutater.BrokerAPIManifold(config, func(_ context.Context, broker environs.InstanceBroker, apiCaller base.APICaller, agent agent.Agent) (worker.Worker, error) {
		c.Assert(broker, tc.Equals, s.broker)
		c.Assert(apiCaller, tc.Equals, s.apiCaller)
		c.Assert(agent, tc.Equals, s.agent)

		return s.worker, nil
	})
	result, err := manifold.Start(c.Context(), s.getter)
	c.Assert(err, tc.IsNil)
	c.Assert(result, tc.Equals, s.worker)
}

func (s *brokerAPIManifoldSuite) TestMissingBrokerFromContext(c *tc.C) {
	defer s.setup(c).Finish()

	cExp := s.getter.EXPECT()
	cExp.Get("moon", gomock.Any()).SetArg(1, s.agent).Return(nil)
	cExp.Get("foobar", gomock.Any()).Return(errors.New("missing"))

	config := instancemutater.BrokerAPIConfig{
		BrokerName:    "foobar",
		APICallerName: "baz",
		AgentName:     "moon",
	}
	manifold := instancemutater.BrokerAPIManifold(config, func(context.Context, environs.InstanceBroker, base.APICaller, agent.Agent) (worker.Worker, error) {
		c.Fail()
		return nil, nil
	})
	_, err := manifold.Start(c.Context(), s.getter)
	c.Assert(err, tc.ErrorMatches, "missing")
}

func (s *brokerAPIManifoldSuite) TestMissingAPICallerFromContext(c *tc.C) {
	defer s.setup(c).Finish()

	cExp := s.getter.EXPECT()
	cExp.Get("moon", gomock.Any()).SetArg(1, s.agent).Return(nil)
	cExp.Get("foobar", gomock.Any()).SetArg(1, s.broker).Return(nil)
	cExp.Get("baz", gomock.Any()).Return(errors.New("missing"))

	config := instancemutater.BrokerAPIConfig{
		BrokerName:    "foobar",
		APICallerName: "baz",
		AgentName:     "moon",
	}
	manifold := instancemutater.BrokerAPIManifold(config, func(context.Context, environs.InstanceBroker, base.APICaller, agent.Agent) (worker.Worker, error) {
		c.Fail()
		return nil, nil
	})
	_, err := manifold.Start(c.Context(), s.getter)
	c.Assert(err, tc.ErrorMatches, "missing")
}

type machineManifoldSuite struct {
	testhelpers.IsolationSuite

	getter      *mocks.MockGetter
	agent       *mocks.MockAgent
	agentConfig *mocks.MockConfig
	broker      brokerShim
	apiCaller   *mocks.MockAPICaller
	worker      *mocks.MockWorker
	api         *mocks.MockInstanceMutaterAPI
}

func TestMachineManifoldSuite(t *stdtesting.T) {
	tc.Run(t, &machineManifoldSuite{})
}

func (s *machineManifoldSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.getter = mocks.NewMockGetter(ctrl)
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

func (s *machineManifoldSuite) TestNewWorkerIsCalled(c *tc.C) {
	defer s.setup(c).Finish()

	s.behaviourContext()
	s.behaviourAgent()

	config := instancemutater.MachineManifoldConfig{
		BrokerName:    "foobar",
		APICallerName: "baz",
		AgentName:     "moon",
		Logger:        loggertesting.WrapCheckLog(c),
		NewWorker: func(_ context.Context, cfg instancemutater.Config) (worker.Worker, error) {
			return s.worker, nil
		},
		NewClient: func(base.APICaller) instancemutater.InstanceMutaterAPI {
			return s.api
		},
	}
	manifold := instancemutater.MachineManifold(config)
	result, err := manifold.Start(c.Context(), s.getter)
	c.Assert(err, tc.IsNil)
	c.Assert(result, tc.Equals, s.worker)
}

func (s *machineManifoldSuite) TestNewWorkerIsRejectedForK8sController(c *tc.C) {
	defer s.setup(c).Finish()

	s.behaviourContext()
	s.behaviorK8sController()

	config := instancemutater.MachineManifoldConfig{
		BrokerName:    "foobar",
		APICallerName: "baz",
		AgentName:     "moon",
		Logger:        loggertesting.WrapCheckLog(c),
		NewWorker: func(_ context.Context, cfg instancemutater.Config) (worker.Worker, error) {
			return s.worker, nil
		},
		NewClient: func(base.APICaller) instancemutater.InstanceMutaterAPI {
			return s.api
		},
	}
	manifold := instancemutater.MachineManifold(config)
	result, err := manifold.Start(c.Context(), s.getter)
	c.Assert(err, tc.Equals, dependency.ErrUninstall)
	c.Assert(result, tc.IsNil)
}

func (s *machineManifoldSuite) TestNewWorkerReturnsError(c *tc.C) {
	defer s.setup(c).Finish()

	s.behaviourContext()
	s.behaviourAgent()

	config := instancemutater.MachineManifoldConfig{
		BrokerName:    "foobar",
		APICallerName: "baz",
		AgentName:     "moon",
		Logger:        loggertesting.WrapCheckLog(c),
		NewWorker: func(_ context.Context, cfg instancemutater.Config) (worker.Worker, error) {
			return nil, errors.New("errored")
		},
		NewClient: func(base.APICaller) instancemutater.InstanceMutaterAPI {
			return s.api
		},
	}
	manifold := instancemutater.MachineManifold(config)
	_, err := manifold.Start(c.Context(), s.getter)
	c.Assert(err, tc.ErrorMatches, "cannot start machine instancemutater worker: errored")
}

func (s *machineManifoldSuite) TestConfigValidatesForMissingWorker(c *tc.C) {
	defer s.setup(c).Finish()

	s.behaviourContext()

	config := instancemutater.MachineManifoldConfig{
		BrokerName:    "foobar",
		APICallerName: "baz",
		AgentName:     "moon",
		Logger:        loggertesting.WrapCheckLog(c),
	}
	manifold := instancemutater.MachineManifold(config)
	_, err := manifold.Start(c.Context(), s.getter)
	c.Assert(err, tc.ErrorMatches, "nil NewWorker not valid")
}

func (s *machineManifoldSuite) TestConfigValidatesForMissingClient(c *tc.C) {
	defer s.setup(c).Finish()

	s.behaviourContext()

	config := instancemutater.MachineManifoldConfig{
		BrokerName:    "foobar",
		APICallerName: "baz",
		AgentName:     "moon",
		Logger:        loggertesting.WrapCheckLog(c),
		NewWorker: func(_ context.Context, cfg instancemutater.Config) (worker.Worker, error) {
			return s.worker, nil
		},
	}
	manifold := instancemutater.MachineManifold(config)
	_, err := manifold.Start(c.Context(), s.getter)
	c.Assert(err, tc.ErrorMatches, "nil NewClient not valid")
}

func (s *machineManifoldSuite) behaviourContext() {
	cExp := s.getter.EXPECT()
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
}

type brokerShim struct {
	*mocks.MockInstanceBroker
	*mocks.MockLXDProfiler
}
