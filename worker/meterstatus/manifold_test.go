// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package meterstatus_test

import (
	"time"

	"github.com/juju/names"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/fslock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	msapi "github.com/juju/juju/api/meterstatus"
	"github.com/juju/juju/api/watcher"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
	"github.com/juju/juju/worker/meterstatus"
	"github.com/juju/juju/worker/uniter/runner"
	"github.com/juju/juju/worker/uniter/runner/context"
)

type ManifoldSuite struct {
	coretesting.BaseSuite

	stub *testing.Stub

	dataDir  string
	oldLcAll string

	manifoldConfig meterstatus.ManifoldConfig
	manifold       dependency.Manifold
	dummyResources dt.StubResources
	getResource    dependency.GetResourceFunc
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.stub = &testing.Stub{}

	s.manifoldConfig = meterstatus.ManifoldConfig{
		AgentName:       "agent-name",
		APICallerName:   "apicaller-name",
		MachineLockName: "machine-lock-name",
	}
	s.manifold = meterstatus.Manifold(s.manifoldConfig)
	s.dataDir = c.MkDir()

	locksDir := c.MkDir()
	lock, err := fslock.NewLock(locksDir, "machine-lock")
	c.Assert(err, jc.ErrorIsNil)

	s.dummyResources = dt.StubResources{
		"agent-name":        dt.StubResource{Output: &dummyAgent{dataDir: s.dataDir}},
		"apicaller-name":    dt.StubResource{Output: &dummyAPICaller{}},
		"machine-lock-name": dt.StubResource{Output: lock},
	}
	s.getResource = dt.StubGetResource(s.dummyResources)
}

// TestInputs ensures the collect manifold has the expected defined inputs.
func (s *ManifoldSuite) TestInputs(c *gc.C) {
	c.Check(s.manifold.Inputs, jc.DeepEquals, []string{
		"agent-name", "apicaller-name", "machine-lock-name",
	})
}

// TestStartMissingDeps ensures that the manifold correctly handles a missing
// resource dependency.
func (s *ManifoldSuite) TestStartMissingDeps(c *gc.C) {
	for _, missingDep := range []string{
		"agent-name", "apicaller-name", "machine-lock-name",
	} {
		testResources := dt.StubResources{}
		for k, v := range s.dummyResources {
			if k == missingDep {
				testResources[k] = dt.StubResource{Error: dependency.ErrMissing}
			} else {
				testResources[k] = v
			}
		}
		getResource := dt.StubGetResource(testResources)
		worker, err := s.manifold.Start(getResource)
		c.Check(worker, gc.IsNil)
		c.Check(err, gc.Equals, dependency.ErrMissing)
	}
}

// TestStatusWorkerStarts ensures that the manifold correctly sets up the worker.
func (s *ManifoldSuite) TestStatusWorkerStarts(c *gc.C) {
	msClient := &stubMeterStatusClient{stub: s.stub, changes: make(chan struct{})}
	s.PatchValue(meterstatus.NewMeterStatusClient,
		func(_ base.APICaller, _ names.UnitTag) msapi.MeterStatusClient {
			return msClient
		})
	s.PatchValue(meterstatus.NewRunner,
		func(_ runner.Context, _ context.Paths) runner.Runner {
			return &stubRunner{stub: s.stub}
		})

	getResource := dt.StubGetResource(s.dummyResources)
	worker, err := s.manifold.Start(getResource)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(worker, gc.NotNil)
	worker.Kill()
	err = worker.Wait()
	c.Assert(err, jc.ErrorIsNil)
	s.stub.CheckCallNames(c, "MeterStatus", "RunHook", "WatchMeterStatus")
}

// TestStatusWorkerStarts ensures that the manifold correctly sets up the worker.
func (s *ManifoldSuite) TestStatusWorkerDoesNotRerunNoChange(c *gc.C) {
	msClient := &stubMeterStatusClient{stub: s.stub, changes: make(chan struct{})}
	s.PatchValue(meterstatus.NewMeterStatusClient,
		func(_ base.APICaller, _ names.UnitTag) msapi.MeterStatusClient {
			return msClient
		})
	s.PatchValue(meterstatus.NewRunner,
		func(_ runner.Context, _ context.Paths) runner.Runner {
			return &stubRunner{stub: s.stub}
		})

	getResource := dt.StubGetResource(s.dummyResources)
	worker, err := s.manifold.Start(getResource)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(worker, gc.NotNil)

	running := make(chan struct{})
	meterstatus.PatchInit(worker, func() { close(running) })

	select {
	case <-running:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for signal")
	}

	msClient.changes <- struct{}{}

	worker.Kill()
	err = worker.Wait()
	c.Assert(err, jc.ErrorIsNil)
	s.stub.CheckCallNames(c, "MeterStatus", "RunHook", "WatchMeterStatus", "MeterStatus")
}

func (s *ManifoldSuite) TestStatusWorkerRunsHookOnChanges(c *gc.C) {
	msClient := &stubMeterStatusClient{stub: s.stub, changes: make(chan struct{})}
	s.PatchValue(meterstatus.NewMeterStatusClient,
		func(_ base.APICaller, _ names.UnitTag) msapi.MeterStatusClient {
			return msClient
		})
	s.PatchValue(meterstatus.NewRunner,
		func(_ runner.Context, _ context.Paths) runner.Runner {
			return &stubRunner{stub: s.stub}
		})

	getResource := dt.StubGetResource(s.dummyResources)

	worker, err := s.manifold.Start(getResource)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(worker, gc.NotNil)

	running := make(chan struct{})
	meterstatus.PatchInit(worker, func() { close(running) })

	select {
	case <-running:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for signal")
	}
	msClient.changes <- struct{}{}
	msClient.code = "RED"

	worker.Kill()
	err = worker.Wait()
	c.Assert(err, jc.ErrorIsNil)
	s.stub.CheckCallNames(c, "MeterStatus", "RunHook", "WatchMeterStatus", "MeterStatus", "RunHook")

}

func (s *ManifoldSuite) TestStatusWorkerDoesNotRerunAfterRestart(c *gc.C) {
	msClient := &stubMeterStatusClient{stub: s.stub, changes: make(chan struct{})}
	s.PatchValue(meterstatus.NewMeterStatusClient,
		func(_ base.APICaller, _ names.UnitTag) msapi.MeterStatusClient {
			return msClient
		})
	s.PatchValue(meterstatus.NewRunner,
		func(_ runner.Context, _ context.Paths) runner.Runner {
			return &stubRunner{stub: s.stub}
		})

	getResource := dt.StubGetResource(s.dummyResources)
	worker, err := s.manifold.Start(getResource)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(worker, gc.NotNil)

	msClient.changes <- struct{}{}

	// Kill worker.
	worker.Kill()
	err = worker.Wait()
	c.Assert(err, jc.ErrorIsNil)

	// Restart it.
	worker, err = s.manifold.Start(getResource)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(worker, gc.NotNil)

	running := make(chan struct{})
	meterstatus.PatchInit(worker, func() { close(running) })

	select {
	case <-running:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for signal")
	}

	worker.Kill()
	err = worker.Wait()

	s.stub.CheckCallNames(c, "MeterStatus", "RunHook", "WatchMeterStatus", "MeterStatus", "MeterStatus", "WatchMeterStatus")
	c.Assert(err, jc.ErrorIsNil)
}

type dummyAgent struct {
	agent.Agent
	dataDir string
}

func (a dummyAgent) CurrentConfig() agent.Config {
	return &dummyAgentConfig{dataDir: a.dataDir}
}

type dummyAgentConfig struct {
	agent.Config
	dataDir string
}

// Tag implements agent.AgentConfig.
func (ac dummyAgentConfig) Tag() names.Tag {
	return names.NewUnitTag("u/0")
}

// DataDir implements agent.AgentConfig.
func (ac dummyAgentConfig) DataDir() string {
	return ac.dataDir
}

type dummyAPICaller struct {
	base.APICaller
}

func (dummyAPICaller) BestFacadeVersion(facade string) int {
	return 42
}

type stubMeterStatusClient struct {
	stub    *testing.Stub
	changes chan struct{}
	code    string
}

func (s *stubMeterStatusClient) MeterStatus() (string, string, error) {
	s.stub.MethodCall(s, "MeterStatus")
	if s.code == "" {
		return "GREEN", "", nil
	} else {
		return s.code, "", nil
	}

}

func (s *stubMeterStatusClient) WatchMeterStatus() (watcher.NotifyWatcher, error) {
	s.stub.MethodCall(s, "WatchMeterStatus")
	return s, nil
}

func (s *stubMeterStatusClient) Changes() <-chan struct{} {
	return s.changes
}

func (s *stubMeterStatusClient) Stop() error {
	return nil
}

func (s *stubMeterStatusClient) Err() error {
	return nil
}

type stubRunner struct {
	runner.Runner
	stub *testing.Stub
}

func (r *stubRunner) RunHook(name string) error {
	r.stub.MethodCall(r, "RunHook", name)
	return r.stub.NextErr()
}
