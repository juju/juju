// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationminion_test

import (
	"context"
	"reflect"
	"sync"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/retry"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/migrationminion"
	"github.com/juju/juju/rpc"
)

var (
	modelTag      = names.NewModelTag("model-uuid")
	addrs         = []string{"1.1.1.1:1111", "2.2.2.2:2222"}
	agentTag      = names.NewMachineTag("42")
	agentPassword = "sekret"
	caCert        = "cert"
)

type Suite struct {
	coretesting.BaseSuite
	config migrationminion.Config
	stub   *jujutesting.Stub
	client *stubMinionClient
	guard  *stubGuard
	agent  *stubAgent
	clock  *testclock.Clock
}

var _ = gc.Suite(&Suite{})

func (s *Suite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.stub = new(jujutesting.Stub)
	s.client = newStubMinionClient(s.stub)
	s.guard = newStubGuard(s.stub)
	s.agent = newStubAgent()
	s.clock = testclock.NewClock(time.Now())
	s.config = migrationminion.Config{
		Facade:  s.client,
		Guard:   s.guard,
		Agent:   s.agent,
		Clock:   s.clock,
		APIOpen: s.apiOpen,
		ValidateMigration: func(context.Context, base.APICaller) error {
			s.stub.AddCall("ValidateMigration")
			return nil
		},
		Logger: loggertesting.WrapCheckLog(c),
	}
}

func (s *Suite) apiOpen(ctx context.Context, info *api.Info, _ api.DialOpts) (api.Connection, error) {
	s.stub.AddCall("API open", info)
	return &stubConnection{stub: s.stub}, nil
}

func (s *Suite) TestStartAndStop(c *gc.C) {
	w, err := migrationminion.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CleanKill(c, w)
	s.stub.CheckCallNames(c, "Watch")
}

func (s *Suite) TestWatchFailure(c *gc.C) {
	s.client.watchErr = errors.New("boom")
	w, err := migrationminion.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	err = workertest.CheckKilled(c, w)
	c.Check(err, gc.ErrorMatches, "setting up watcher: boom")
}

func (s *Suite) TestClosedWatcherChannel(c *gc.C) {
	close(s.client.watcher.changes)
	w, err := migrationminion.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	err = workertest.CheckKilled(c, w)
	c.Check(err, gc.ErrorMatches, "watcher channel closed")
}

func (s *Suite) TestUnlockError(c *gc.C) {
	s.client.watcher.changes <- watcher.MigrationStatus{
		Phase: migration.NONE,
	}
	s.guard.unlockErr = errors.New("squish")
	w, err := migrationminion.New(s.config)
	c.Assert(err, jc.ErrorIsNil)

	err = workertest.CheckKilled(c, w)
	c.Check(err, gc.ErrorMatches, "squish")
	s.stub.CheckCallNames(c, "Watch", "Unlock")
}

func (s *Suite) TestLockdownError(c *gc.C) {
	s.client.watcher.changes <- watcher.MigrationStatus{
		Phase: migration.QUIESCE,
	}
	s.guard.lockdownErr = errors.New("squash")
	w, err := migrationminion.New(s.config)
	c.Assert(err, jc.ErrorIsNil)

	err = workertest.CheckKilled(c, w)
	c.Check(err, gc.ErrorMatches, "squash")
	s.stub.CheckCallNames(c, "Watch", "Lockdown")
}

func (s *Suite) TestNonRunningPhases(c *gc.C) {
	phases := []migration.Phase{
		migration.UNKNOWN,
		migration.NONE,
		migration.LOGTRANSFER,
		migration.REAP,
		migration.REAPFAILED,
		migration.DONE,
		migration.ABORT,
		migration.ABORTDONE,
	}
	for _, phase := range phases {
		s.checkNonRunningPhase(c, phase)
	}
}

func (s *Suite) checkNonRunningPhase(c *gc.C, phase migration.Phase) {
	c.Logf("checking %s", phase)
	s.stub.ResetCalls()
	s.client.watcher.changes <- watcher.MigrationStatus{Phase: phase}
	w, err := migrationminion.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CheckAlive(c, w)
	workertest.CleanKill(c, w)
	s.stub.CheckCallNames(c, "Watch", "Unlock")
}

func (s *Suite) TestQUIESCE(c *gc.C) {
	s.client.watcher.changes <- watcher.MigrationStatus{
		MigrationId: "id",
		Phase:       migration.QUIESCE,
	}
	w, err := migrationminion.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	s.waitForStubCalls(c, []string{
		"Watch",
		"Lockdown",
		"Report",
	})
	s.stub.CheckCall(c, 2, "Report", "id", migration.QUIESCE, true)
}

func (s *Suite) TestVALIDATION(c *gc.C) {
	s.client.watcher.changes <- watcher.MigrationStatus{
		MigrationId:    "id",
		Phase:          migration.VALIDATION,
		TargetAPIAddrs: addrs,
		TargetCACert:   caCert,
	}
	w, err := migrationminion.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	s.waitForStubCalls(c, []string{
		"Watch",
		"Lockdown",
		"API open",
		"ValidateMigration",
		"API close",
		"Report",
	})
	s.stub.CheckCall(c, 2, "API open", &api.Info{
		ModelTag: modelTag,
		Tag:      agentTag,
		Password: agentPassword,
		Addrs:    addrs,
		CACert:   caCert,
	})
	s.stub.CheckCall(c, 5, "Report", "id", migration.VALIDATION, true)
}

func (s *Suite) TestVALIDATIONCanConnectButIsRepeatedlyCalled(c *gc.C) {
	s.client.watcher.changes <- watcher.MigrationStatus{
		MigrationId:    "id",
		Phase:          migration.VALIDATION,
		TargetAPIAddrs: addrs,
		TargetCACert:   caCert,
	}
	s.client.watcher.changes <- watcher.MigrationStatus{
		MigrationId:    "id",
		Phase:          migration.VALIDATION,
		TargetAPIAddrs: addrs,
		TargetCACert:   caCert,
	}
	w, err := migrationminion.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	s.waitForStubCalls(c, []string{
		"Watch",
		"Lockdown",
		"API open",
		"ValidateMigration",
		"API close",
		"Report",
	})

	s.stub.CheckCall(c, 2, "API open", &api.Info{
		ModelTag: modelTag,
		Tag:      agentTag,
		Password: agentPassword,
		Addrs:    addrs,
		CACert:   caCert,
	})
	s.stub.CheckCall(c, 5, "Report", "id", migration.VALIDATION, true)
}

func (s *Suite) TestVALIDATIONCantConnect(c *gc.C) {
	s.client.watcher.changes <- watcher.MigrationStatus{
		MigrationId: "id",
		Phase:       migration.VALIDATION,
	}
	s.config.APIOpen = func(context.Context, *api.Info, api.DialOpts) (api.Connection, error) {
		s.stub.AddCall("API open")
		return nil, errors.New("boom")
	}
	w, err := migrationminion.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	// Advance time enough for all of the retries to be exhausted.
	sleepTime := 100 * time.Millisecond
	for i := 0; i < 20; i++ {
		err := s.clock.WaitAdvance(sleepTime, coretesting.ShortWait, 1)
		c.Assert(err, jc.ErrorIsNil)
		sleepTime = calculateSleepTime(i)
	}

	s.waitForStubCalls(c, []string{
		"Watch",
		"Lockdown",
		"API open",
		"API open",
		"API open",
		"API open",
		"API open",
		"API open",
		"API open",
		"API open",
		"API open",
		"API open",
		"API open",
		"API open",
		"API open",
		"API open",
		"API open",
		"API open",
		"API open",
		"API open",
		"API open",
		"API open",
		"Report",
	})
	s.stub.CheckCall(c, 22, "Report", "id", migration.VALIDATION, false)
}

func (s *Suite) TestVALIDATIONCantConnectNotReportForTryAgainError(c *gc.C) {
	s.client.watcher.changes <- watcher.MigrationStatus{
		MigrationId: "id",
		Phase:       migration.VALIDATION,
	}
	s.config.APIOpen = func(context.Context, *api.Info, api.DialOpts) (api.Connection, error) {
		s.stub.AddCall("API open")
		return nil, apiservererrors.ErrTryAgain
	}
	w, err := migrationminion.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	// Advance time enough for all of the retries to be exhausted.
	sleepTime := 100 * time.Millisecond
	for i := 0; i < 20; i++ {
		err := s.clock.WaitAdvance(sleepTime, coretesting.ShortWait, 1)
		c.Assert(err, jc.ErrorIsNil)
		sleepTime = calculateSleepTime(i)
	}

	s.waitForStubCalls(c, []string{
		"Watch",
		"Lockdown",
		"API open",
		"API open",
		"API open",
		"API open",
		"API open",
		"API open",
		"API open",
		"API open",
		"API open",
		"API open",
		"API open",
		"API open",
		"API open",
		"API open",
		"API open",
		"API open",
		"API open",
		"API open",
		"API open",
		"API open",
	})
}

func (s *Suite) TestVALIDATIONFail(c *gc.C) {
	s.client.watcher.changes <- watcher.MigrationStatus{
		MigrationId: "id",
		Phase:       migration.VALIDATION,
	}
	s.config.ValidateMigration = func(context.Context, base.APICaller) error {
		s.stub.AddCall("ValidateMigration")
		return errors.New("boom")
	}
	w, err := migrationminion.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	// Advance time enough for all of the retries to be exhausted.
	sleepTime := 100 * time.Millisecond
	for i := 0; i < 20; i++ {
		err := s.clock.WaitAdvance(sleepTime, coretesting.ShortWait, 1)
		c.Assert(err, jc.ErrorIsNil)
		sleepTime = calculateSleepTime(i)
	}

	expectedCalls := []string{"Watch", "Lockdown"}
	for i := 0; i < 20; i++ {
		expectedCalls = append(expectedCalls, "API open", "ValidateMigration", "API close")
	}
	expectedCalls = append(expectedCalls, "Report")
	s.waitForStubCalls(c, expectedCalls)
	s.stub.CheckCall(c, 62, "Report", "id", migration.VALIDATION, false)
}

func (s *Suite) TestVALIDATIONRetrySucceed(c *gc.C) {
	s.client.watcher.changes <- watcher.MigrationStatus{
		MigrationId: "id",
		Phase:       migration.VALIDATION,
	}
	var stub jujutesting.Stub
	stub.SetErrors(errors.New("nope"), errors.New("not yet"), nil)
	s.config.ValidateMigration = func(context.Context, base.APICaller) error {
		stub.AddCall("ValidateMigration")
		return stub.NextErr()
	}

	w, err := migrationminion.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	waitForStubCalls(c, &stub, "ValidateMigration")

	err = s.clock.WaitAdvance(160*time.Millisecond, coretesting.LongWait, 1)
	c.Assert(err, jc.ErrorIsNil)

	waitForStubCalls(c, &stub, "ValidateMigration", "ValidateMigration")

	err = s.clock.WaitAdvance(256*time.Millisecond, coretesting.LongWait, 1)
	c.Assert(err, jc.ErrorIsNil)

	s.waitForStubCalls(c, []string{
		"Watch",
		"Lockdown",
		"API open",
		"API close",
		"API open",
		"API close",
		"API open",
		"API close",
		"Report",
	})
	s.stub.CheckCall(c, 8, "Report", "id", migration.VALIDATION, true)
}

func (s *Suite) TestSUCCESS(c *gc.C) {
	s.client.watcher.changes <- watcher.MigrationStatus{
		MigrationId:    "id",
		Phase:          migration.SUCCESS,
		TargetAPIAddrs: addrs,
		TargetCACert:   caCert,
	}
	w, err := migrationminion.New(s.config)
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-s.agent.configChanged:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out")
	}
	workertest.CleanKill(c, w)
	c.Assert(s.agent.conf.addrs, gc.DeepEquals, addrs)
	c.Assert(s.agent.conf.caCert, gc.DeepEquals, caCert)
	s.stub.CheckCallNames(c, "Watch", "Lockdown", "Report")
	s.stub.CheckCall(c, 2, "Report", "id", migration.SUCCESS, true)
}

func (s *Suite) TestSUCCESSCantConnectNotReportForTryAgainError(c *gc.C) {
	s.client.watcher.changes <- watcher.MigrationStatus{
		MigrationId: "id",
		Phase:       migration.SUCCESS,
	}
	s.agent.conf.tag = names.NewUnitTag("app/0")
	s.agent.conf.dir = "/var/lib/juju/agents/unit-app-0"
	s.config.APIOpen = func(context.Context, *api.Info, api.DialOpts) (api.Connection, error) {
		s.stub.AddCall("API open")
		return nil, apiservererrors.ErrTryAgain
	}
	s.stub.SetErrors(rpc.ErrShutdown)
	w, err := migrationminion.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	// Advance time enough for all of the retries to be exhausted.
	sleepTime := 100 * time.Millisecond
	for i := 0; i < 9; i++ {
		err := s.clock.WaitAdvance(sleepTime, coretesting.ShortWait, 1)
		c.Assert(err, jc.ErrorIsNil)
		sleepTime = sleepTime * 2
	}

	s.waitForStubCalls(c, []string{
		"Watch",
		"Lockdown",
		"Report",
		"API open",
		"API open",
		"API open",
		"API open",
		"API open",
		"API open",
		"API open",
		"API open",
		"API open",
		"API open",
	})
}

func (s *Suite) TestSUCCESSRetryReport(c *gc.C) {
	s.client.watcher.changes <- watcher.MigrationStatus{
		MigrationId: "id",
		Phase:       migration.SUCCESS,
	}
	s.agent.conf.tag = names.NewUnitTag("app/0")
	s.agent.conf.dir = "/var/lib/juju/agents/unit-app-0"
	s.config.NewFacade = func(a base.APICaller) (migrationminion.Facade, error) {
		return s.config.Facade, nil
	}

	s.stub.SetErrors(rpc.ErrShutdown)
	w, err := migrationminion.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	s.waitForStubCalls(c, []string{
		"Watch",
		"Lockdown",
		"Report",
		"API open",
		"Report",
		"API close",
	})
}

func (s *Suite) waitForStubCalls(c *gc.C, expectedCallNames []string) {
	waitForStubCalls(c, s.stub, expectedCallNames...)
}

func waitForStubCalls(c *gc.C, stub *jujutesting.Stub, expectedCallNames ...string) {
	var callNames []string
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		callNames = stubCallNames(stub)
		if reflect.DeepEqual(callNames, expectedCallNames) {
			return
		}
	}
	c.Fatalf("failed to see expected calls. saw: %v", callNames)
}

// Make this a feature of stub
func stubCallNames(stub *jujutesting.Stub) []string {
	var out []string
	for _, call := range stub.Calls() {
		out = append(out, call.FuncName)
	}
	return out
}

func calculateSleepTime(i int) time.Duration {
	// These numbers correspond to the retry strategy used in the
	// migration minion.
	return retry.ExpBackoff(100*time.Millisecond, 25*time.Second, 1.6, false)(0, i+1)
}

func newStubGuard(stub *jujutesting.Stub) *stubGuard {
	return &stubGuard{stub: stub}
}

type stubGuard struct {
	stub        *jujutesting.Stub
	unlockErr   error
	lockdownErr error
}

func (g *stubGuard) Lockdown(ctx context.Context) error {
	g.stub.AddCall("Lockdown")
	return g.lockdownErr
}

func (g *stubGuard) Unlock(ctx context.Context) error {
	g.stub.AddCall("Unlock")
	return g.unlockErr
}

func newStubMinionClient(stub *jujutesting.Stub) *stubMinionClient {
	return &stubMinionClient{
		stub:    stub,
		watcher: newStubWatcher(),
	}
}

type stubMinionClient struct {
	stub     *jujutesting.Stub
	watcher  *stubWatcher
	watchErr error
}

func (c *stubMinionClient) Watch(ctx context.Context) (watcher.MigrationStatusWatcher, error) {
	c.stub.MethodCall(c, "Watch")
	if c.watchErr != nil {
		return nil, c.watchErr
	}
	return c.watcher, nil
}

func (c *stubMinionClient) Report(ctx context.Context, id string, phase migration.Phase, success bool) error {
	c.stub.MethodCall(c, "Report", id, phase, success)
	return c.stub.NextErr()
}

func newStubWatcher() *stubWatcher {
	return &stubWatcher{
		Worker:  workertest.NewErrorWorker(nil),
		changes: make(chan watcher.MigrationStatus, 2),
	}
}

type stubWatcher struct {
	worker.Worker
	changes chan watcher.MigrationStatus
}

func (w *stubWatcher) Changes() <-chan watcher.MigrationStatus {
	return w.changes
}

func newStubAgent() *stubAgent {
	return &stubAgent{
		configChanged: make(chan bool),
	}
}

type stubAgent struct {
	agent.Agent
	configChanged chan bool
	conf          stubAgentConfig
}

func (ma *stubAgent) CurrentConfig() agent.Config {
	return &ma.conf
}

func (ma *stubAgent) ChangeConfig(f agent.ConfigMutator) error {
	defer close(ma.configChanged)
	return f(&ma.conf)
}

type stubAgentConfig struct {
	agent.ConfigSetter

	tag names.Tag
	dir string

	mu     sync.Mutex
	addrs  []string
	caCert string
}

func (mc *stubAgentConfig) SetAPIHostPorts(servers []network.HostPorts) error {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.addrs = nil
	for _, hps := range servers {
		for _, hp := range hps {
			mc.addrs = append(mc.addrs, network.DialAddress(hp))
		}
	}
	return nil
}

func (mc *stubAgentConfig) SetCACert(cert string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.caCert = cert
}

func (mc *stubAgentConfig) APIInfo() (*api.Info, bool) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	return &api.Info{
		Addrs:    mc.addrs,
		CACert:   mc.caCert,
		ModelTag: modelTag,
		Tag:      agentTag,
		Password: agentPassword,
	}, true
}

func (mc *stubAgentConfig) Tag() names.Tag {
	return mc.tag
}

func (mc *stubAgentConfig) Dir() string {
	return mc.dir
}

type stubConnection struct {
	api.Connection
	stub *jujutesting.Stub
}

func (c *stubConnection) Close() error {
	c.stub.AddCall("API close")
	return nil
}
