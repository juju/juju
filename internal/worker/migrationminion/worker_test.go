// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationminion_test

import (
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	"github.com/juju/retry"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/worker/fortress"
	"github.com/juju/juju/internal/worker/migrationminion"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
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
		ValidateMigration: func(base.APICaller) error {
			s.stub.AddCall("ValidateMigration")
			return nil
		},
		NewFacade: func(base.APICaller) (migrationminion.Facade, error) {
			return s.client, nil
		},
		Logger: loggo.GetLogger("test"),
	}
}

func (s *Suite) apiOpen(info *api.Info, _ api.DialOpts) (api.Connection, error) {
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
		migration.ABORT,
		migration.ABORTDONE,
	}
	for _, phase := range phases {
		s.checkNonRunningPhase(c, phase)
	}
}

func (s *Suite) TestPostSuccessPhasesUpdateAgentConfig(c *gc.C) {
	phases := []migration.Phase{
		migration.LOGTRANSFER,
		migration.REAP,
		migration.REAPFAILED,
		migration.DONE,
	}
	for _, phase := range phases {
		s.checkPostSuccessPhase(c, phase)
	}
}

func (s *Suite) checkPostSuccessPhase(c *gc.C, phase migration.Phase) {
	c.Logf("checking %s", phase)
	s.stub.ResetCalls()
	s.agent = newStubAgent()
	s.config.Agent = s.agent
	s.client.watcher.changes <- watcher.MigrationStatus{
		MigrationId:    "id",
		Phase:          phase,
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
	s.stub.CheckCallNames(c, "Watch", "API open", "API close", "Unlock")
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
	s.config.APIOpen = func(*api.Info, api.DialOpts) (api.Connection, error) {
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

func (s *Suite) TestVALIDATIONCantConnectReportsFailureForTryAgainError(c *gc.C) {
	s.client.watcher.changes <- watcher.MigrationStatus{
		MigrationId: "id",
		Phase:       migration.VALIDATION,
	}
	s.config.APIOpen = func(*api.Info, api.DialOpts) (api.Connection, error) {
		s.stub.AddCall("API open")
		return nil, &rpc.RequestError{Message: "try again", Code: params.CodeTryAgain}
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

	// The rate-limit retries are exhausted, so the failure must be reported
	// immediately rather than leaving the migration master to wait for the
	// minion report timeout.
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

func (s *Suite) TestVALIDATIONRetryStopsWhenKilledNoReport(c *gc.C) {
	s.client.watcher.changes <- watcher.MigrationStatus{
		MigrationId: "id",
		Phase:       migration.VALIDATION,
	}
	s.config.APIOpen = func(*api.Info, api.DialOpts) (api.Connection, error) {
		s.stub.AddCall("API open")
		return nil, &rpc.RequestError{Message: "try again", Code: params.CodeTryAgain}
	}
	w, err := migrationminion.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	// Wait for the first retry to enter its backoff sleep, then kill the
	// worker. A shutdown must not report validation failure.
	select {
	case <-s.clock.Alarms():
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for retry backoff")
	}
	w.Kill()

	err = workertest.CheckKilled(c, w)
	c.Check(err, jc.ErrorIsNil)
	s.stub.CheckCallNames(c, "Watch", "Lockdown", "API open")
}

func (s *Suite) TestVALIDATIONFail(c *gc.C) {
	s.client.watcher.changes <- watcher.MigrationStatus{
		MigrationId: "id",
		Phase:       migration.VALIDATION,
	}
	s.config.ValidateMigration = func(base.APICaller) error {
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
	s.config.ValidateMigration = func(base.APICaller) error {
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
	s.stub.CheckCallNames(c, "Watch", "Lockdown", "API open", "API close", "Report")
	s.stub.CheckCall(c, 4, "Report", "id", migration.SUCCESS, true)
}

// TestSUCCESSReportFailureStillWritesConfig covers the wedge seen in the
// integration tests: a report RPC that dies mid-flight (context canceled,
// connection torn down by a bounce) must not prevent the agent config
// from being updated, otherwise the unit points at a controller whose
// model is gone forever.
func (s *Suite) TestSUCCESSReportFailureStillWritesConfig(c *gc.C) {
	s.agent.conf.tag = names.NewUnitTag("app/0")
	s.agent.conf.dir = "/var/lib/juju/agents/unit-app-0"

	// The first report attempt fails, and every fallback attempt fails
	// too, so robustReport returns an error; doSUCCESS must still update
	// the agent config instead of aborting.
	s.stub.SetErrors(errors.New("report boom"))
	s.config.NewFacade = func(base.APICaller) (migrationminion.Facade, error) {
		return &failingReportFacade{}, nil
	}

	s.client.watcher.changes <- watcher.MigrationStatus{
		MigrationId:    "id",
		Phase:          migration.SUCCESS,
		TargetAPIAddrs: addrs,
		TargetCACert:   caCert,
	}
	w, err := migrationminion.New(s.config)
	c.Assert(err, jc.ErrorIsNil)

	// The fallback retry is bounded by the same duration as the
	// validation retry (5m); advance past it so the fallback exhausts
	// and the config is written.
	err = s.clock.WaitAdvance(6*time.Minute, coretesting.LongWait, 1)
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-s.agent.configChanged:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for agent config change")
	}
	c.Assert(s.agent.conf.addrs, gc.DeepEquals, addrs)

	// The config write succeeded, so the worker must still bounce with
	// the report error and replay the report on the next worker start.
	err = w.Wait()
	c.Assert(err, gc.ErrorMatches, "reporting migration status: cannot report migration status .*: max duration exceeded: fallback report boom")
}

// TestSUCCESSReportAndConfigFailureBothReported covers the case where both
// the SUCCESS report and the agent config write fail: the config error is the
// one that must be acted on, but the report failure must not be lost.
func (s *Suite) TestSUCCESSReportAndConfigFailureBothReported(c *gc.C) {
	s.agent.conf.tag = names.NewUnitTag("app/0")
	s.agent.conf.dir = "/var/lib/juju/agents/unit-app-0"

	s.stub.SetErrors(errors.New("report boom"))
	s.config.NewFacade = func(base.APICaller) (migrationminion.Facade, error) {
		return &failingReportFacade{}, nil
	}

	s.client.watcher.changes <- watcher.MigrationStatus{
		MigrationId: "id",
		Phase:       migration.SUCCESS,
		// An address without a port makes the agent config update fail.
		TargetAPIAddrs: []string{"foo"},
		TargetCACert:   caCert,
		SourceAPIAddrs: []string{"3.3.3.3:3333"},
		SourceCACert:   caCert,
	}
	w, err := migrationminion.New(s.config)
	c.Assert(err, jc.ErrorIsNil)

	// Advance past the fallback retry envelope so the report gives up and
	// the config update is attempted.
	err = s.clock.WaitAdvance(6*time.Minute, coretesting.LongWait, 1)
	c.Assert(err, jc.ErrorIsNil)

	err = workertest.CheckKilled(c, w)
	c.Check(err, gc.ErrorMatches, `.*reporting migration status failed: .*report boom.*converting API addresses: .*`)
}

// TestSUCCESSFallbackReportCompletesDuringTeardown covers the race where the
// worker is torn down while the SUCCESS report is in flight: the fallback
// direct dial runs after the catacomb is already dying, and must not abort
// on it - otherwise the report is lost and the migration master waits
// forever.
func (s *Suite) TestSUCCESSFallbackReportCompletesDuringTeardown(c *gc.C) {
	s.agent.conf.tag = names.NewUnitTag("app/0")
	s.agent.conf.dir = "/var/lib/juju/agents/unit-app-0"

	facade := &blockingReportFacade{
		Facade:  s.client,
		started: make(chan struct{}),
		proceed: make(chan struct{}),
	}
	s.config.Facade = facade

	reported := make(chan struct{})
	s.config.NewFacade = func(base.APICaller) (migrationminion.Facade, error) {
		return &reportingFacade{reported: reported}, nil
	}

	s.client.watcher.changes <- watcher.MigrationStatus{
		MigrationId:    "id",
		Phase:          migration.SUCCESS,
		TargetAPIAddrs: addrs,
		TargetCACert:   caCert,
		SourceAPIAddrs: []string{"3.3.3.3:3333"},
		SourceCACert:   caCert,
	}
	w, err := migrationminion.New(s.config)
	c.Assert(err, jc.ErrorIsNil)

	// Wait for the in-flight report, then kill the worker so the fallback
	// dial runs with the catacomb already dying.
	select {
	case <-facade.started:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for in-flight report")
	}
	w.Kill()
	close(facade.proceed)

	// The fallback dial and report must complete despite the teardown, and
	// the agent config must still be written.
	select {
	case <-reported:
	case <-time.After(coretesting.LongWait):
		c.Fatal("fallback report did not complete during teardown")
	}
	select {
	case <-s.agent.configChanged:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for agent config change")
	}
	workertest.CleanKill(c, w)
}

// TestSUCCESSFallbackReportStopsRetryingDuringTeardown covers the other
// half of the teardown race: if the worker is killed and the fallback
// report still fails, the retry loop must not keep retrying into the
// teardown. The worker must die promptly.
func (s *Suite) TestSUCCESSFallbackReportStopsRetryingDuringTeardown(c *gc.C) {
	s.agent.conf.tag = names.NewUnitTag("app/0")
	s.agent.conf.dir = "/var/lib/juju/agents/unit-app-0"

	facade := &blockingReportFacade{
		Facade:  s.client,
		started: make(chan struct{}),
		proceed: make(chan struct{}),
	}
	s.config.Facade = facade

	s.config.NewFacade = func(base.APICaller) (migrationminion.Facade, error) {
		return &failingReportFacade{}, nil
	}

	s.client.watcher.changes <- watcher.MigrationStatus{
		MigrationId:    "id",
		Phase:          migration.SUCCESS,
		TargetAPIAddrs: addrs,
		TargetCACert:   caCert,
		SourceAPIAddrs: []string{"3.3.3.3:3333"},
		SourceCACert:   caCert,
	}
	w, err := migrationminion.New(s.config)
	c.Assert(err, jc.ErrorIsNil)

	// Wait for the in-flight report, then kill the worker; the failing
	// fallback report must not retry: the worker dies without any clock
	// advance.
	select {
	case <-facade.started:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for in-flight report")
	}
	w.Kill()
	close(facade.proceed)

	err = workertest.CheckKilled(c, w)
	c.Check(err, jc.ErrorIsNil)
}

// TestSUCCESSFallbackReportKilledDuringBackoffSleep covers the case where the
// worker is killed while the fallback report is sleeping between attempts.
// retry.Call reports that as a stopped retry rather than errWorkerDying, but
// the worker must still die cleanly - the config has already been written, so
// the report will replay on the next worker start.
func (s *Suite) TestSUCCESSFallbackReportKilledDuringBackoffSleep(c *gc.C) {
	s.agent.conf.tag = names.NewUnitTag("app/0")
	s.agent.conf.dir = "/var/lib/juju/agents/unit-app-0"

	// The in-flight report fails, forcing the fallback path.
	s.stub.SetErrors(rpc.ErrShutdown)

	attempted := make(chan struct{}, 1)
	s.config.NewFacade = func(base.APICaller) (migrationminion.Facade, error) {
		return &signallingFailingFacade{attempted: attempted}, nil
	}

	s.client.watcher.changes <- watcher.MigrationStatus{
		MigrationId:    "id",
		Phase:          migration.SUCCESS,
		TargetAPIAddrs: addrs,
		TargetCACert:   caCert,
		SourceAPIAddrs: []string{"3.3.3.3:3333"},
		SourceCACert:   caCert,
	}
	w, err := migrationminion.New(s.config)
	c.Assert(err, jc.ErrorIsNil)

	// Wait for the first fallback attempt to fail, then for the retry loop
	// to schedule its backoff sleep.
	select {
	case <-attempted:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for fallback report attempt")
	}
	select {
	case <-s.clock.Alarms():
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for retry backoff")
	}

	// Killing during the sleep must be treated like dying mid-attempt.
	w.Kill()
	err = workertest.CheckKilled(c, w)
	c.Check(err, jc.ErrorIsNil)
}

func (s *Suite) TestSUCCESSCantConnectNotReportForTryAgainError(c *gc.C) {
	s.client.watcher.changes <- watcher.MigrationStatus{
		MigrationId:    "id",
		Phase:          migration.SUCCESS,
		TargetAPIAddrs: []string{"target-controller:1234"},
		SourceAPIAddrs: []string{"source-controller:1234"},
	}
	s.agent.conf.tag = names.NewUnitTag("app/0")
	s.agent.conf.dir = "/var/lib/juju/agents/unit-app-0"
	s.config.APIOpen = func(info *api.Info, opts api.DialOpts) (api.Connection, error) {
		s.stub.AddCall("API open")
		if info.Addrs[0] == "target-controller:1234" {
			return &stubConnection{stub: s.stub}, nil
		}
		return nil, apiservererrors.ErrTryAgain
	}
	s.stub.SetErrors(rpc.ErrShutdown)
	w, err := migrationminion.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	// The in-flight report dies on the shut down connection, the fallback
	// dial to the source gets ErrTryAgain, and the bounded retry keeps
	// dialling while the phase replays.
	//
	// Each fallback attempt waits on the bounded backoff delay; advance
	// the clock past one delay at a time.
	for _, expected := range [][]string{
		{"Watch", "Lockdown", "API open", "API close", "Report", "API open"},
		{"Watch", "Lockdown", "API open", "API close", "Report", "API open", "API open"},
	} {
		err = s.clock.WaitAdvance(100*time.Millisecond, coretesting.ShortWait, 1)
		c.Assert(err, jc.ErrorIsNil)
		s.waitForStubCalls(c, expected)
	}
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
		"API open",
		"API close",
		"Report",
		"API open",
		"Report",
		"API close",
	})
}

func (s *Suite) TestSUCCESSTargetDialRetriesTryAgain(c *gc.C) {
	s.client.watcher.changes <- watcher.MigrationStatus{
		MigrationId:    "id",
		Phase:          migration.SUCCESS,
		TargetAPIAddrs: addrs,
		TargetCACert:   caCert,
	}
	var attempts int
	s.config.APIOpen = func(*api.Info, api.DialOpts) (api.Connection, error) {
		s.stub.AddCall("API open")
		attempts++
		if attempts <= 2 {
			return nil, &rpc.RequestError{Message: "try again", Code: params.CodeTryAgain}
		}
		return &stubConnection{stub: s.stub}, nil
	}
	w, err := migrationminion.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	// Advance the clock through the retry backoff delays.
	for i := 0; i < 2; i++ {
		err = s.clock.WaitAdvance(calculateSleepTime(i), coretesting.LongWait, 1)
		c.Assert(err, jc.ErrorIsNil)
	}

	select {
	case <-s.agent.configChanged:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for agent config change")
	}
	c.Assert(s.agent.conf.addrs, gc.DeepEquals, addrs)
	c.Assert(s.agent.conf.caCert, gc.DeepEquals, caCert)
	c.Assert(attempts, gc.Equals, 3)
}

func (s *Suite) TestPostSuccessTargetDialRetriesTryAgain(c *gc.C) {
	s.client.watcher.changes <- watcher.MigrationStatus{
		MigrationId:    "id",
		Phase:          migration.REAP,
		TargetAPIAddrs: addrs,
		TargetCACert:   caCert,
	}
	var attempts int
	s.config.APIOpen = func(*api.Info, api.DialOpts) (api.Connection, error) {
		s.stub.AddCall("API open")
		attempts++
		if attempts <= 2 {
			return nil, &rpc.RequestError{Message: "try again", Code: params.CodeTryAgain}
		}
		return &stubConnection{stub: s.stub}, nil
	}
	w, err := migrationminion.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	// Advance the clock through the retry backoff delays.
	for i := 0; i < 2; i++ {
		err = s.clock.WaitAdvance(calculateSleepTime(i), coretesting.LongWait, 1)
		c.Assert(err, jc.ErrorIsNil)
	}

	select {
	case <-s.agent.configChanged:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for agent config change")
	}
	c.Assert(s.agent.conf.addrs, gc.DeepEquals, addrs)
	c.Assert(s.agent.conf.caCert, gc.DeepEquals, caCert)
	c.Assert(attempts, gc.Equals, 3)
}

func (s *Suite) TestSUCCESSTargetDialFatalErrorNotRetried(c *gc.C) {
	s.client.watcher.changes <- watcher.MigrationStatus{
		MigrationId:    "id",
		Phase:          migration.SUCCESS,
		TargetAPIAddrs: addrs,
		TargetCACert:   caCert,
	}
	s.config.APIOpen = func(*api.Info, api.DialOpts) (api.Connection, error) {
		s.stub.AddCall("API open")
		return nil, errors.New("boom")
	}
	w, err := migrationminion.New(s.config)
	c.Assert(err, jc.ErrorIsNil)

	err = workertest.CheckKilled(c, w)
	c.Check(err, gc.ErrorMatches, `.*failed to open API to target controller: boom`)
	s.stub.CheckCallNames(c, "Watch", "Lockdown", "API open")
}

func (s *Suite) TestSUCCESSTargetDialFatalErrorAfterTryAgainNotRetried(c *gc.C) {
	s.client.watcher.changes <- watcher.MigrationStatus{
		MigrationId:    "id",
		Phase:          migration.SUCCESS,
		TargetAPIAddrs: addrs,
		TargetCACert:   caCert,
	}
	var attempts int
	s.config.APIOpen = func(*api.Info, api.DialOpts) (api.Connection, error) {
		s.stub.AddCall("API open")
		attempts++
		if attempts == 1 {
			return nil, &rpc.RequestError{Message: "try again", Code: params.CodeTryAgain}
		}
		return nil, errors.New("boom")
	}
	w, err := migrationminion.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	// Advance past the first retry delay so the fatal error is attempted.
	err = s.clock.WaitAdvance(calculateSleepTime(0), coretesting.LongWait, 1)
	c.Assert(err, jc.ErrorIsNil)

	err = workertest.CheckKilled(c, w)
	c.Check(err, gc.ErrorMatches, `.*failed to open API to target controller: boom`)
	c.Check(attempts, gc.Equals, 2)
}

func (s *Suite) TestSUCCESSTargetDialTryAgainExhausted(c *gc.C) {
	tw := &loggo.TestWriter{}
	c.Assert(loggo.RegisterWriter("target-dial-exhausted-test", loggo.NewMinimumLevelWriter(tw, loggo.CRITICAL)), jc.ErrorIsNil)
	defer func() {
		_, err := loggo.RemoveWriter("target-dial-exhausted-test")
		c.Check(err, jc.ErrorIsNil)
	}()

	s.client.watcher.changes <- watcher.MigrationStatus{
		MigrationId:    "id",
		Phase:          migration.SUCCESS,
		TargetAPIAddrs: addrs,
		TargetCACert:   caCert,
	}
	s.config.APIOpen = func(*api.Info, api.DialOpts) (api.Connection, error) {
		s.stub.AddCall("API open")
		return nil, &rpc.RequestError{Message: "try again", Code: params.CodeTryAgain}
	}
	w, err := migrationminion.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	// Advance time enough for all of the retries to be exhausted.
	sleepTime := 100 * time.Millisecond
	for i := 0; i < 20; i++ {
		err = s.clock.WaitAdvance(sleepTime, coretesting.ShortWait, 1)
		c.Assert(err, jc.ErrorIsNil)
		sleepTime = calculateSleepTime(i)
	}

	err = workertest.CheckKilled(c, w)
	c.Check(err, gc.ErrorMatches, `.*try changing "agent-ratelimit-max" and "agent-ratelimit-rate".*`)

	criticalFound := false
	for _, entry := range tw.Log() {
		if strings.Contains(entry.Message, "agent.conf left unchanged") &&
			strings.Contains(entry.Message, "agent-ratelimit-max") {
			criticalFound = true
			break
		}
	}
	c.Check(criticalFound, jc.IsTrue, gc.Commentf("critical log not found in %v", tw.Log()))
}

func (s *Suite) TestPostSuccessTargetDialTryAgainExhausted(c *gc.C) {
	s.client.watcher.changes <- watcher.MigrationStatus{
		MigrationId:    "id",
		Phase:          migration.REAP,
		TargetAPIAddrs: addrs,
		TargetCACert:   caCert,
	}
	s.config.APIOpen = func(*api.Info, api.DialOpts) (api.Connection, error) {
		s.stub.AddCall("API open")
		return nil, &rpc.RequestError{Message: "try again", Code: params.CodeTryAgain}
	}
	w, err := migrationminion.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	// Advance time enough for all of the retries to be exhausted.
	sleepTime := 100 * time.Millisecond
	for i := 0; i < 20; i++ {
		err = s.clock.WaitAdvance(sleepTime, coretesting.ShortWait, 1)
		c.Assert(err, jc.ErrorIsNil)
		sleepTime = calculateSleepTime(i)
	}

	err = workertest.CheckKilled(c, w)
	c.Check(err, gc.ErrorMatches, `.*try changing "agent-ratelimit-max" and "agent-ratelimit-rate".*`)
}

func (s *Suite) TestSUCCESSTargetDialRetryStopsWhenKilled(c *gc.C) {
	s.client.watcher.changes <- watcher.MigrationStatus{
		MigrationId:    "id",
		Phase:          migration.SUCCESS,
		TargetAPIAddrs: addrs,
		TargetCACert:   caCert,
	}
	s.config.APIOpen = func(*api.Info, api.DialOpts) (api.Connection, error) {
		s.stub.AddCall("API open")
		return nil, apiservererrors.ErrTryAgain
	}
	w, err := migrationminion.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	// Wait for the first retry to enter its backoff sleep, then kill the
	// worker. The retry must stop promptly rather than run to exhaustion.
	select {
	case <-s.clock.Alarms():
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for retry backoff")
	}
	w.Kill()

	err = workertest.CheckKilled(c, w)
	c.Check(err, jc.ErrorIsNil)
}

func (s *Suite) TestPostSuccessTargetDialKilledDuringBackoffNoCritical(c *gc.C) {
	tw := &loggo.TestWriter{}
	c.Assert(loggo.RegisterWriter("target-dial-test", loggo.NewMinimumLevelWriter(tw, loggo.CRITICAL)), jc.ErrorIsNil)
	defer func() {
		_, err := loggo.RemoveWriter("target-dial-test")
		c.Check(err, jc.ErrorIsNil)
	}()

	s.client.watcher.changes <- watcher.MigrationStatus{
		MigrationId:    "id",
		Phase:          migration.REAP,
		TargetAPIAddrs: addrs,
		TargetCACert:   caCert,
	}
	s.config.APIOpen = func(*api.Info, api.DialOpts) (api.Connection, error) {
		s.stub.AddCall("API open")
		return nil, apiservererrors.ErrTryAgain
	}
	w, err := migrationminion.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	// Wait for the first retry to enter its backoff sleep, then kill the
	// worker. The teardown must not be reported as a migration failure.
	select {
	case <-s.clock.Alarms():
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for retry backoff")
	}
	w.Kill()

	err = workertest.CheckKilled(c, w)
	c.Check(err, jc.ErrorIsNil)
	for _, entry := range tw.Log() {
		c.Check(entry.Message, gc.Not(gc.Matches), `.*agent.conf left unchanged and pointing to source controller.*`)
	}
}

func (s *Suite) TestSuccessAfterValidateWithRedirect(c *gc.C) {
	finalAddress := []string{"7.7.7.7:1234"}
	finalCACert := "final-cert"
	s.client.watcher.changes <- watcher.MigrationStatus{
		MigrationId:    "id",
		Phase:          migration.VALIDATION,
		TargetAPIAddrs: addrs,
		TargetCACert:   caCert,
	}
	s.client.watcher.changes <- watcher.MigrationStatus{
		MigrationId:    "id",
		Phase:          migration.SUCCESS,
		TargetAPIAddrs: addrs,
		TargetCACert:   caCert,
	}

	s.config.APIOpen = func(info *api.Info, _ api.DialOpts) (api.Connection, error) {
		addressCopy := make([]string, len(info.Addrs))
		copy(addressCopy, info.Addrs)
		s.stub.AddCall("API open addresses", addressCopy)
		if slices.Equal(info.Addrs, addrs) {
			return nil, &api.RedirectError{
				Servers: []network.MachineHostPorts{network.NewMachineHostPorts(1234, "7.7.7.7")},
				CACert:  finalCACert,
			}
		}
		if slices.Equal(info.Addrs, finalAddress) {
			return &stubConnection{stub: s.stub}, nil
		}
		return nil, errors.Errorf("unexpected address %q", info.Addrs[0])
	}

	w, err := migrationminion.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	select {
	case <-s.agent.configChanged:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out")
	}
	workertest.CleanKill(c, w)
	c.Assert(s.agent.conf.addrs, gc.DeepEquals, finalAddress)
	c.Assert(s.agent.conf.caCert, gc.DeepEquals, finalCACert)
	s.stub.CheckCall(c, 2, "API open addresses", addrs)
	s.stub.CheckCall(c, 3, "API open addresses", finalAddress)
}

func (s *Suite) TestValidateFailsWithRedirectLoop(c *gc.C) {
	s.client.watcher.changes <- watcher.MigrationStatus{
		MigrationId:    "id",
		Phase:          migration.VALIDATION,
		TargetAPIAddrs: addrs,
		TargetCACert:   caCert,
	}

	s.config.APIOpen = func(info *api.Info, _ api.DialOpts) (api.Connection, error) {
		s.stub.AddCall("API open")
		return nil, &api.RedirectError{
			Servers: []network.MachineHostPorts{network.NewMachineHostPorts(1234, "7.7.7.7")},
			CACert:  caCert,
		}
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
	// maxRetries=20 and maxRedirects=5
	for i := 0; i < 20*5; i++ {
		expectedCalls = append(expectedCalls, "API open")
	}
	expectedCalls = append(expectedCalls, "Report")
	s.waitForStubCalls(c, expectedCalls)
	s.stub.CheckCall(c, len(expectedCalls)-1, "Report", "id", migration.VALIDATION, false)
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

func (g *stubGuard) Lockdown(fortress.Abort) error {
	g.stub.AddCall("Lockdown")
	return g.lockdownErr
}

func (g *stubGuard) Unlock() error {
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

func (c *stubMinionClient) Watch() (watcher.MigrationStatusWatcher, error) {
	c.stub.MethodCall(c, "Watch")
	if c.watchErr != nil {
		return nil, c.watchErr
	}
	return c.watcher, nil
}

func (c *stubMinionClient) Report(id string, phase migration.Phase, success bool) error {
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

// blockingReportFacade blocks Report until the test releases it, so a test
// can tear the worker down while a report is in flight.
type blockingReportFacade struct {
	migrationminion.Facade
	started chan struct{}
	proceed chan struct{}
}

func (f *blockingReportFacade) Report(string, migration.Phase, bool) error {
	close(f.started)
	<-f.proceed
	return errors.New("report boom")
}

// failingReportFacade always fails Report, so a test can exhaust the fallback
// report retries.
type failingReportFacade struct {
	migrationminion.Facade
}

func (f *failingReportFacade) Report(string, migration.Phase, bool) error {
	return errors.New("fallback report boom")
}

// signallingFailingFacade fails Report and signals each attempt, so a test can
// wait for the retry loop to enter its backoff sleep.
type signallingFailingFacade struct {
	migrationminion.Facade
	attempted chan<- struct{}
}

func (f *signallingFailingFacade) Report(string, migration.Phase, bool) error {
	select {
	case f.attempted <- struct{}{}:
	default:
	}
	return errors.New("fallback report boom")
}

// reportingFacade reports successfully, signalling when it has done so.
type reportingFacade struct {
	migrationminion.Facade
	reported chan<- struct{}
}

func (f *reportingFacade) Report(string, migration.Phase, bool) error {
	close(f.reported)
	return nil
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
