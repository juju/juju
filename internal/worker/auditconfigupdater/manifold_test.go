// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package auditconfigupdater_test

import (
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	dt "github.com/juju/worker/v3/dependency/testing"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/auditlog"
	"github.com/juju/juju/internal/worker/auditconfigupdater"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher/watchertest"
)

type manifoldSuite struct {
	testing.IsolationSuite

	manifold     dependency.Manifold
	context      dependency.Context
	agent        *mockAgent
	stateTracker stubStateTracker

	cfgSource *configSource

	stub testing.Stub
}

var _ = gc.Suite(&manifoldSuite{})

func (s *manifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.agent = &mockAgent{}
	s.agent.conf.logDir = c.MkDir()

	configChanged := make(chan struct{}, 1)
	s.cfgSource = &configSource{
		watcher: watchertest.NewNotifyWatcher(configChanged),
		cfg:     makeControllerConfig(true, true, "This.Method"),
	}
	s.cfgSource.cfg["audit-log-max-size"] = "10M"
	s.cfgSource.cfg["audit-log-max-backups"] = 10
	s.PatchValue(&auditconfigupdater.ConfigSourceFromState, func(st *state.State) auditconfigupdater.ConfigSource {
		return s.cfgSource
	})

	s.stateTracker = stubStateTracker{}
	s.stub.ResetCalls()

	s.context = s.newContext(nil)

	s.manifold = auditconfigupdater.Manifold(auditconfigupdater.ManifoldConfig{
		AgentName: "agent",
		StateName: "state",
		NewWorker: s.newWorker,
	})
}

func (s *manifoldSuite) newContext(overlay map[string]interface{}) dependency.Context {
	resources := map[string]interface{}{
		"agent": s.agent,
		"state": &s.stateTracker,
	}
	for k, v := range overlay {
		resources[k] = v
	}
	return dt.StubContext(nil, resources)
}

func (s *manifoldSuite) newWorker(
	source auditconfigupdater.ConfigSource,
	initial auditlog.Config,
	factory auditconfigupdater.AuditLogFactory,
) (worker.Worker, error) {
	s.stub.MethodCall(s, "NewWorker", source, initial, factory)
	err := s.stub.NextErr()
	if err != nil {
		return nil, err
	}
	w := fakeWorker{config: initial}
	s.AddCleanup(func(c *gc.C) { workertest.DirtyKill(c, &w) })
	return &w, nil
}

var expectedInputs = []string{"agent", "state"}

func (s *manifoldSuite) TestInputs(c *gc.C) {
	c.Assert(s.manifold.Inputs, jc.SameContents, expectedInputs)
}

func (s *manifoldSuite) TestMissingInputs(c *gc.C) {
	for _, input := range expectedInputs {
		context := s.newContext(map[string]interface{}{
			input: dependency.ErrMissing,
		})
		_, err := s.manifold.Start(context)
		c.Assert(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	}
}

func (s *manifoldSuite) TestStart(c *gc.C) {
	w, err := s.manifold.Start(s.context)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	s.stub.CheckCallNames(c, "NewWorker")

	args := s.stub.Calls()[0].Args
	c.Assert(args, gc.HasLen, 3)
	c.Assert(args[0], gc.Equals, s.cfgSource)

	auditConfig := args[1].(auditlog.Config)
	target := auditConfig.Target
	c.Assert(target, gc.NotNil)
	defer target.Close()

	auditConfig.Target = nil
	c.Assert(auditConfig, gc.DeepEquals, auditlog.Config{
		Enabled:        true,
		CaptureAPIArgs: true,
		ExcludeMethods: set.NewStrings("This.Method"),
		MaxSizeMB:      10,
		MaxBackups:     10,
	})

	c.Assert(args[2], gc.NotNil)
}

func (s *manifoldSuite) TestStartWithAuditingDisabled(c *gc.C) {
	s.cfgSource.cfg["auditing-enabled"] = false
	w, err := s.manifold.Start(s.context)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	s.stub.CheckCallNames(c, "NewWorker")

	args := s.stub.Calls()[0].Args
	c.Assert(args, gc.HasLen, 3)
	c.Assert(args[0], gc.Equals, s.cfgSource)

	auditConfig := args[1].(auditlog.Config)
	c.Assert(auditConfig.Target, gc.IsNil)
}

func (s *manifoldSuite) TestOutput(c *gc.C) {
	w, err := s.manifold.Start(s.context)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	var getConfig func() auditlog.Config
	err = s.manifold.Output(w, &getConfig)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "NewWorker")

	args := s.stub.Calls()[0].Args
	c.Assert(args, gc.HasLen, 3)
	c.Assert(args[0], gc.Equals, s.cfgSource)

	auditConfig := args[1].(auditlog.Config)

	c.Assert(auditConfig, gc.DeepEquals, getConfig())
}

func (s *manifoldSuite) TestStopWorkerClosesState(c *gc.C) {
	w, err := s.manifold.Start(s.context)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	s.stateTracker.CheckCallNames(c, "Use")

	workertest.CleanKill(c, w)
	s.stateTracker.CheckCallNames(c, "Use", "Done")
}

func (s *manifoldSuite) TestClosesStateOnWorkerError(c *gc.C) {
	s.stub.SetErrors(errors.Errorf("splat"))
	w, err := s.manifold.Start(s.context)
	c.Assert(err, gc.ErrorMatches, "splat")
	c.Assert(w, gc.IsNil)

	s.stateTracker.CheckCallNames(c, "Use", "Done")
}

type mockAgent struct {
	agent.Agent
	conf mockAgentConfig
}

func (ma *mockAgent) CurrentConfig() agent.Config {
	return &ma.conf
}

type mockAgentConfig struct {
	agent.Config
	logDir string
}

func (c *mockAgentConfig) LogDir() string {
	return c.logDir
}

type stubStateTracker struct {
	testing.Stub
	pool  state.StatePool
	state state.State
}

func (s *stubStateTracker) Use() (*state.StatePool, *state.State, error) {
	s.MethodCall(s, "Use")
	return &s.pool, &s.state, s.NextErr()
}

func (s *stubStateTracker) Done() error {
	s.MethodCall(s, "Done")
	return s.NextErr()
}

func (s *stubStateTracker) Report() map[string]interface{} {
	s.MethodCall(s, "Report")
	return nil
}

type fakeWorker struct {
	config auditlog.Config
}

func (w *fakeWorker) Kill() {
}

func (w *fakeWorker) Wait() error {
	return nil
}

func (w *fakeWorker) CurrentConfig() auditlog.Config {
	return w.config
}
