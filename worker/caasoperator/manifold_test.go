// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/api/base"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/caasoperator"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
	"github.com/juju/juju/worker/workertest"
)

type ManifoldSuite struct {
	testing.IsolationSuite

	manifold        dependency.Manifold
	context         dependency.Context
	agent           fakeAgent
	apiCaller       fakeAPICaller
	charmDownloader fakeDownloader
	client          fakeClient
	clock           *testing.Clock
	dataDir         string
	stub            testing.Stub
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.dataDir = c.MkDir()
	s.agent = fakeAgent{
		config: fakeAgentConfig{
			tag:     names.NewApplicationTag("gitlab"),
			dataDir: s.dataDir,
		},
	}
	s.clock = testing.NewClock(time.Time{})
	s.stub.ResetCalls()

	s.context = s.newContext(nil)
	s.manifold = caasoperator.Manifold(caasoperator.ManifoldConfig{
		AgentName:          "agent",
		APICallerName:      "api-caller",
		ClockName:          "clock",
		NewWorker:          s.newWorker,
		NewClient:          s.newClient,
		NewCharmDownloader: s.newCharmDownloader,
	})
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

func (s *ManifoldSuite) newWorker(config caasoperator.Config) (worker.Worker, error) {
	s.stub.MethodCall(s, "NewWorker", config)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	w := worker.NewRunner(worker.RunnerParams{})
	s.AddCleanup(func(c *gc.C) { workertest.DirtyKill(c, w) })
	return w, nil
}

func (s *ManifoldSuite) newClient(caller base.APICaller) caasoperator.Client {
	s.stub.MethodCall(s, "NewClient", caller)
	return &s.client
}

func (s *ManifoldSuite) newCharmDownloader(caller base.APICaller) caasoperator.Downloader {
	s.stub.MethodCall(s, "NewCharmDownloader", caller)
	return &s.charmDownloader
}

var expectedInputs = []string{"agent", "api-caller", "clock"}

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	c.Assert(s.manifold.Inputs, jc.SameContents, expectedInputs)
}

func (s *ManifoldSuite) TestMissingInputs(c *gc.C) {
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

	s.stub.CheckCallNames(c, "NewClient", "NewCharmDownloader", "NewWorker")
	s.stub.CheckCall(c, 0, "NewClient", &s.apiCaller)
	s.stub.CheckCall(c, 1, "NewCharmDownloader", &s.apiCaller)

	args := s.stub.Calls()[2].Args
	c.Assert(args, gc.HasLen, 1)
	c.Assert(args[0], gc.FitsTypeOf, caasoperator.Config{})
	config := args[0].(caasoperator.Config)

	// Don't care about new runner factory func here.
	config.NewRunnerFactoryFunc = nil
	c.Assert(config, jc.DeepEquals, caasoperator.Config{
		ModelUUID:           coretesting.ModelTag.Id(),
		ModelName:           "gitlab-model",
		Application:         "gitlab",
		CharmConfigGetter:   &s.client,
		DataDir:             s.dataDir,
		CharmGetter:         &s.client,
		Clock:               s.clock,
		ContainerSpecSetter: &s.client,
		Downloader:          &s.charmDownloader,
		StatusSetter:        &s.client,
		APIAddressGetter:    &s.client,
		ProxySettingsGetter: &s.client,
	})
}

func (s *ManifoldSuite) startWorkerClean(c *gc.C) worker.Worker {
	w, err := s.manifold.Start(s.context)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CheckAlive(c, w)
	return w
}
