// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package meterstatus_test

import (
	"sync"
	"time"

	"github.com/juju/names"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/fslock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	msapi "github.com/juju/juju/api/meterstatus"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
	"github.com/juju/juju/worker/meterstatus"
	"github.com/juju/juju/worker/uniter/runner"
)

type ManifoldSuite struct {
	coretesting.BaseSuite

	stub *testing.Stub

	dataDir string

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
		AgentName:               "agent-name",
		APICallerName:           "apicaller-name",
		MachineLockName:         "machine-lock-name",
		NewHookRunner:           meterstatus.NewHookRunner,
		NewMeterStatusAPIClient: msapi.NewClient,

		NewConnectedStatusWorker: meterstatus.NewConnectedStatusWorker,
		NewIsolatedStatusWorker:  meterstatus.NewIsolatedStatusWorker,
	}
	s.manifold = meterstatus.Manifold(s.manifoldConfig)
	s.dataDir = c.MkDir()

	locksDir := c.MkDir()
	lock, err := fslock.NewLock(locksDir, "machine-lock", fslock.Defaults())
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
		"agent-name", "machine-lock-name",
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

type PatchedManifoldSuite struct {
	coretesting.BaseSuite
	msClient       *stubMeterStatusClient
	manifoldConfig meterstatus.ManifoldConfig
	stub           *testing.Stub
	dummyResources dt.StubResources
}

func (s *PatchedManifoldSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.msClient = &stubMeterStatusClient{stub: s.stub, changes: make(chan struct{})}
	newMSClient := func(_ base.APICaller, _ names.UnitTag) msapi.MeterStatusClient {
		return s.msClient
	}
	newHookRunner := func(_ names.UnitTag, _ *fslock.Lock, _ agent.Config) meterstatus.HookRunner {
		return &stubRunner{stub: s.stub}
	}

	s.manifoldConfig = meterstatus.ManifoldConfig{
		AgentName:               "agent-name",
		APICallerName:           "apicaller-name",
		MachineLockName:         "machine-lock-name",
		NewHookRunner:           newHookRunner,
		NewMeterStatusAPIClient: newMSClient,
	}
}

// TestStatusWorkerStarts ensures that the manifold correctly sets up the connected worker.
func (s *PatchedManifoldSuite) TestStatusWorkerStarts(c *gc.C) {
	var called bool
	s.manifoldConfig.NewConnectedStatusWorker = func(cfg meterstatus.ConnectedConfig) (worker.Worker, error) {
		called = true
		return meterstatus.NewConnectedStatusWorker(cfg)
	}
	manifold := meterstatus.Manifold(s.manifoldConfig)
	getResource := dt.StubGetResource(s.dummyResources)
	worker, err := manifold.Start(getResource)
	c.Assert(called, jc.IsTrue)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(worker, gc.NotNil)
	worker.Kill()
	err = worker.Wait()
	c.Assert(err, jc.ErrorIsNil)
	s.stub.CheckCallNames(c, "MeterStatus", "RunHook", "WatchMeterStatus")
}

// TestInactiveWorker ensures that the manifold correctly sets up the isolated worker.
func (s *PatchedManifoldSuite) TestIsolatedWorker(c *gc.C) {
	delete(s.dummyResources, "apicaller-name")
	var called bool
	s.manifoldConfig.NewIsolatedStatusWorker = func(cfg meterstatus.IsolatedConfig) (worker.Worker, error) {
		called = true
		return meterstatus.NewIsolatedStatusWorker(cfg)
	}
	manifold := meterstatus.Manifold(s.manifoldConfig)
	getResource := dt.StubGetResource(s.dummyResources)
	worker, err := manifold.Start(getResource)
	c.Assert(called, jc.IsTrue)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(worker, gc.NotNil)
	worker.Kill()
	err = worker.Wait()
	c.Assert(err, jc.ErrorIsNil)
	s.stub.CheckCallNames(c, "MeterStatus", "RunHook", "WatchMeterStatus")
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
	sync.RWMutex
	stub    *testing.Stub
	changes chan struct{}
	code    string
}

func newStubMeterStatusClient(stub *testing.Stub) *stubMeterStatusClient {
	changes := make(chan struct{})
	return &stubMeterStatusClient{stub: stub, changes: changes}
}

func (s *stubMeterStatusClient) SignalStatus(codes ...string) {
	if len(codes) == 0 {
		codes = []string{s.code}
	}
	for _, code := range codes {
		s.SetStatus(code)
		select {
		case s.changes <- struct{}{}:
		case <-time.After(coretesting.LongWait):
			panic("timed out signaling meter status change")
		}
	}
}

func (s *stubMeterStatusClient) SetStatus(code string) {
	s.Lock()
	defer s.Unlock()
	s.code = code
}

func (s *stubMeterStatusClient) MeterStatus() (string, string, error) {
	s.RLock()
	defer s.RUnlock()
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

func (s *stubMeterStatusClient) Changes() watcher.NotifyChannel {
	return s.changes
}

func (s *stubMeterStatusClient) Kill() {
}

func (s *stubMeterStatusClient) Wait() error {
	return nil
}

type stubRunner struct {
	runner.Runner
	stub *testing.Stub
	ran  chan struct{}
}

func (r *stubRunner) RunHook(code, info string, abort <-chan struct{}) error {
	r.stub.MethodCall(r, "RunHook", code, info)
	if r.ran != nil {
		select {
		case r.ran <- struct{}{}:
		case <-time.After(coretesting.LongWait):
			panic("timed out signaling hook run")
		}
	}
	return r.stub.NextErr()
}
