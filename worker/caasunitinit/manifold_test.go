// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitinit_test

import (
	"os"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v3"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/dependency"
	dt "gopkg.in/juju/worker.v1/dependency/testing"
	"gopkg.in/juju/worker.v1/workertest"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider/exec"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/caasoperator"
	"github.com/juju/juju/worker/caasoperator/mocks"
	"github.com/juju/juju/worker/caasunitinit"
)

type ManifoldSuite struct {
	testing.IsolationSuite

	manifold  dependency.Manifold
	context   dependency.Context
	agent     fakeAgent
	apiCaller fakeAPICaller
	client    fakeClient
	clock     *testclock.Clock
	dataDir   string
	stub      testing.Stub
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	os.Setenv("JUJU_OPERATOR_SERVICE_IP", "127.0.0.1")
	os.Setenv("JUJU_OPERATOR_POD_IP", "127.0.0.2")

	s.dataDir = c.MkDir()
	s.agent = fakeAgent{
		config: fakeAgentConfig{
			tag:     names.NewApplicationTag("gitlab"),
			dataDir: s.dataDir,
		},
	}
	s.clock = testclock.NewClock(time.Time{})
	s.stub.ResetCalls()

	s.context = s.newContext(nil)
}

func (s *ManifoldSuite) TearDownTest(c *gc.C) {
	os.Setenv("JUJU_OPERATOR_SERVICE_IP", "")
	os.Setenv("JUJU_OPERATOR_POD_IP", "")

	s.IsolationSuite.TearDownTest(c)
}

func (s *ManifoldSuite) setupManifold(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.manifold = caasunitinit.Manifold(caasunitinit.ManifoldConfig{
		Logger:        loggo.GetLogger("test"),
		AgentName:     "agent",
		APICallerName: "api-caller",
		ClockName:     "clock",
		NewWorker:     s.newWorker,
		NewClient:     s.newClient,
		NewExecClient: func(modelName string) (exec.Executor, error) {
			return mocks.NewMockExecutor(ctrl), nil
		},
		LoadOperatorInfo: func(paths caasoperator.Paths) (*caas.OperatorInfo, error) {
			return &caas.OperatorInfo{
				CACert:     coretesting.CACert,
				Cert:       coretesting.ServerCert,
				PrivateKey: coretesting.ServerKey,
			}, nil
		},
	})
	return ctrl
}

func (s *ManifoldSuite) newContext(overlay map[string]interface{}) dependency.Context {
	resources := map[string]interface{}{
		"agent":      &s.agent,
		"api-caller": &s.apiCaller,
		"clock":      s.clock,
	}
	for k, v := range overlay {
		resources[k] = v
	}
	return dt.StubContext(nil, resources)
}

func (s *ManifoldSuite) newWorker(config caasunitinit.Config) (worker.Worker, error) {
	s.stub.MethodCall(s, "NewWorker", config)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	w := worker.NewRunner(worker.RunnerParams{})
	s.AddCleanup(func(c *gc.C) { workertest.DirtyKill(c, w) })
	return w, nil
}

func (s *ManifoldSuite) newClient(caller base.APICaller) caasunitinit.Client {
	s.stub.MethodCall(s, "NewClient", caller)
	return &s.client
}

var expectedInputs = []string{"agent", "api-caller", "clock"}

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	ctrl := s.setupManifold(c)
	defer ctrl.Finish()

	c.Assert(s.manifold.Inputs, jc.SameContents, expectedInputs)
}

func (s *ManifoldSuite) TestMissingInputs(c *gc.C) {
	ctrl := s.setupManifold(c)
	defer ctrl.Finish()

	for _, input := range expectedInputs {
		context := s.newContext(map[string]interface{}{
			input: dependency.ErrMissing,
		})
		_, err := s.manifold.Start(context)
		c.Assert(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	}
}

func (s *ManifoldSuite) TestStart(c *gc.C) {
	w := s.startWorkerClean(c)
	workertest.CleanKill(c, w)

	s.stub.CheckCallNames(c, "NewClient", "NewWorker")
	s.stub.CheckCall(c, 0, "NewClient", &s.apiCaller)

	args := s.stub.Calls()[1].Args
	c.Assert(args, gc.HasLen, 1)
	c.Assert(args[0], gc.FitsTypeOf, caasunitinit.Config{})
	config := args[0].(caasunitinit.Config)

	// Don't care about some helper funcs.
	c.Assert(config.UnitProviderIDFunc, gc.NotNil)
	config.UnitProviderIDFunc = nil
	c.Assert(config.Logger, gc.NotNil)
	config.Logger = nil
	c.Assert(config.ContainerStartWatcher, gc.NotNil)
	config.ContainerStartWatcher = nil
	c.Assert(config.NewExecClient, gc.NotNil)
	config.NewExecClient = nil
	c.Assert(config.InitializeUnit, gc.NotNil)
	config.InitializeUnit = nil
	c.Assert(config.Paths.ToolsDir, gc.Not(gc.Equals), "")
	c.Assert(config.Paths.State.BaseDir, gc.Not(gc.Equals), "")
	config.Paths = caasoperator.Paths{}

	c.Assert(config, jc.DeepEquals, caasunitinit.Config{
		Application: "gitlab",
		DataDir:     s.dataDir,
		Clock:       s.clock,
		OperatorInfo: caas.OperatorInfo{
			CACert:     coretesting.CACert,
			Cert:       coretesting.ServerCert,
			PrivateKey: coretesting.ServerKey,
		},
	})
}

func (s *ManifoldSuite) startWorkerClean(c *gc.C) worker.Worker {
	ctrl := s.setupManifold(c)
	defer ctrl.Finish()

	w, err := s.manifold.Start(s.context)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CheckAlive(c, w)
	return w
}
