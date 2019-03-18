// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
	names "gopkg.in/juju/names.v2"
	worker "gopkg.in/juju/worker.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/worker/instancemutater"
	"github.com/juju/juju/worker/instancemutater/mocks"
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
		config      instancemutater.ManifoldConfig
		err         string
	}{
		{
			description: "Test empty configuration",
			config:      instancemutater.ManifoldConfig{},
			err:         "nil Logger not valid",
		},
		{
			description: "Test no logger",
			config:      instancemutater.ManifoldConfig{},
			err:         "nil Logger not valid",
		},
		{
			description: "Test no new worker constructor",
			config: instancemutater.ManifoldConfig{
				Logger: mocks.NewMockLogger(ctrl),
			},
			err: "nil NewWorker not valid",
		},
		{
			description: "Test no new client constructor",
			config: instancemutater.ManifoldConfig{
				Logger: mocks.NewMockLogger(ctrl),
				NewWorker: func(cfg instancemutater.Config) (worker.Worker, error) {
					return mocks.NewMockWorker(ctrl), nil
				},
			},
			err: "nil NewClient not valid",
		},
		{
			description: "Test no agent name",
			config: instancemutater.ManifoldConfig{
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
			config: instancemutater.ManifoldConfig{
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
			config: instancemutater.ManifoldConfig{
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

func (s *manifoldConfigSuite) TestValidConfigValidate(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	config := instancemutater.ManifoldConfig{
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

type manifoldSuite struct {
	testing.IsolationSuite

	logger      *mocks.MockLogger
	context     *mocks.MockContext
	agent       *mocks.MockAgent
	agentConfig *mocks.MockConfig
	environ     *mocks.MockEnviron
	apiCaller   *mocks.MockAPICaller
	worker      *mocks.MockWorker
	api         *mocks.MockInstanceMutaterAPI
}

var _ = gc.Suite(&manifoldSuite{})

func (s *manifoldSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.logger = mocks.NewMockLogger(ctrl)
	s.context = mocks.NewMockContext(ctrl)
	s.agent = mocks.NewMockAgent(ctrl)
	s.agentConfig = mocks.NewMockConfig(ctrl)
	s.environ = mocks.NewMockEnviron(ctrl)
	s.apiCaller = mocks.NewMockAPICaller(ctrl)
	s.worker = mocks.NewMockWorker(ctrl)
	s.api = mocks.NewMockInstanceMutaterAPI(ctrl)

	return ctrl
}

func (s *manifoldSuite) TestNewWorkerIsCalled(c *gc.C) {
	defer s.setup(c).Finish()

	s.behaviourContext()
	s.behaviourAgent()

	config := instancemutater.ManifoldConfig{
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
	manifold := instancemutater.Manifold(config)
	result, err := manifold.Start(s.context)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.Equals, s.worker)
}

func (s *manifoldSuite) TestNewWorkerReturnsError(c *gc.C) {
	defer s.setup(c).Finish()

	s.behaviourContext()
	s.behaviourAgent()

	config := instancemutater.ManifoldConfig{
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
	manifold := instancemutater.Manifold(config)
	_, err := manifold.Start(s.context)
	c.Assert(err, gc.ErrorMatches, "cannot start machine instancemutater worker: errored")
}

func (s *manifoldSuite) TestConfigValidatesForMissingWorker(c *gc.C) {
	defer s.setup(c).Finish()

	s.behaviourContext()

	config := instancemutater.ManifoldConfig{
		EnvironName:   "foobar",
		APICallerName: "baz",
		AgentName:     "moon",
		Logger:        s.logger,
	}
	manifold := instancemutater.Manifold(config)
	_, err := manifold.Start(s.context)
	c.Assert(err, gc.ErrorMatches, "nil NewWorker not valid")
}

func (s *manifoldSuite) TestConfigValidatesForMissingClient(c *gc.C) {
	defer s.setup(c).Finish()

	s.behaviourContext()

	config := instancemutater.ManifoldConfig{
		EnvironName:   "foobar",
		APICallerName: "baz",
		AgentName:     "moon",
		Logger:        s.logger,
		NewWorker: func(cfg instancemutater.Config) (worker.Worker, error) {
			return s.worker, nil
		},
	}
	manifold := instancemutater.Manifold(config)
	_, err := manifold.Start(s.context)
	c.Assert(err, gc.ErrorMatches, "nil NewClient not valid")
}

func (s *manifoldSuite) behaviourContext() {
	cExp := s.context.EXPECT()
	cExp.Get("moon", gomock.Any()).SetArg(1, s.agent).Return(nil)
	cExp.Get("foobar", gomock.Any()).SetArg(1, s.environ).Return(nil)
	cExp.Get("baz", gomock.Any()).SetArg(1, s.apiCaller).Return(nil)
}

func (s *manifoldSuite) behaviourAgent() {
	aExp := s.agent.EXPECT()
	aExp.CurrentConfig().Return(s.agentConfig)

	cExp := s.agentConfig.EXPECT()
	cExp.Tag().Return(names.MachineTag{})
}
