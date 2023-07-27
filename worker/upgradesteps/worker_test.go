// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradesteps

import (
	"fmt"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/retry"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	agenterrors "github.com/juju/juju/agent/errors"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/juju/worker/gate"
)

// TODO(mjs) - these tests are too tightly coupled to the
// implementation. They needn't be internal tests.

type UpgradeSuite struct {
	coretesting.BaseSuite

	oldVersion      version.Binary
	logWriter       loggo.TestWriter
	connectionDead  bool
	preUpgradeError bool

	notify          chan struct{}
	upgradeErr      error
	controllersDone []string
	status          state.UpgradeStatus
}

var _ = gc.Suite(&UpgradeSuite{})

const fails = true
const succeeds = false

func (s *UpgradeSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.preUpgradeError = false
	// Most of these tests normally finish sub-second on a fast machine.
	// If any given test hits a minute, we have almost certainly become
	// wedged, so dump the logs.
	coretesting.DumpTestLogsAfter(time.Minute, c, s)

	s.oldVersion = coretesting.CurrentVersion()
	s.oldVersion.Major = 1
	s.oldVersion.Minor = 16

	// Don't wait so long in tests.
	s.PatchValue(&UpgradeStartTimeoutController, time.Millisecond*50)

	// Allow tests to make the API connection appear to be dead.
	s.connectionDead = false
	s.PatchValue(&agenterrors.ConnectionIsDead, func(agenterrors.Logger, agenterrors.Breakable) bool {
		return s.connectionDead
	})

	s.notify = make(chan struct{}, 1)
	s.upgradeErr = nil
	s.controllersDone = nil
}

func (s *UpgradeSuite) captureLogs(c *gc.C) {
	c.Assert(loggo.RegisterWriter("upgrade-tests", &s.logWriter), gc.IsNil)
	s.AddCleanup(func(*gc.C) {
		loggo.RemoveWriter("upgrade-tests")
		s.logWriter.Clear()
	})
}

func (s *UpgradeSuite) countUpgradeAttempts(upgradeErr error) *int {
	count := 0
	s.PatchValue(&PerformUpgrade, func(version.Number, []upgrades.Target, upgrades.Context) error {
		count++
		return upgradeErr
	})
	return &count
}

func (s *UpgradeSuite) TestNewChannelWhenNoUpgradeRequired(c *gc.C) {
	// Set the agent's upgradedToVersion to version.Current,
	// to simulate the upgrade steps having been run already.
	initialVersion := jujuversion.Current
	config := NewFakeConfigSetter(names.NewMachineTag("0"), initialVersion)

	lock := NewLock(config)

	// Upgrade steps have already been run.
	c.Assert(lock.IsUnlocked(), jc.IsTrue)
}

func (s *UpgradeSuite) TestNewChannelWhenUpgradeRequired(c *gc.C) {
	// Set the agent's upgradedToVersion so that upgrade steps are required.
	initialVersion := version.MustParse("1.16.0")
	config := NewFakeConfigSetter(names.NewMachineTag("0"), initialVersion)

	lock := NewLock(config)

	c.Assert(lock.IsUnlocked(), jc.IsFalse)
	// The agent's version should NOT have been updated.
	c.Assert(config.Version, gc.Equals, initialVersion)
}

func (s *UpgradeSuite) TestNoUpgradeNecessary(c *gc.C) {
	attemptsP := s.countUpgradeAttempts(nil)
	s.captureLogs(c)
	s.oldVersion.Number = jujuversion.Current // nothing to do

	workerErr, config, _, doneLock := s.runUpgradeWorker(c, false)

	c.Check(workerErr, gc.IsNil)
	c.Check(*attemptsP, gc.Equals, 0)
	c.Check(config.Version, gc.Equals, jujuversion.Current)
	c.Check(doneLock.IsUnlocked(), jc.IsTrue)
}

func (s *UpgradeSuite) TestNoUpgradeNecessaryIgnoresBuildNumbers(c *gc.C) {
	attemptsP := s.countUpgradeAttempts(nil)
	s.captureLogs(c)
	s.oldVersion.Number = jujuversion.Current
	s.oldVersion.Build = 1 // Ensure there's a build number mismatch.

	workerErr, config, _, doneLock := s.runUpgradeWorker(c, false)

	c.Check(workerErr, gc.IsNil)
	c.Check(*attemptsP, gc.Equals, 0)
	c.Check(config.Version, gc.Equals, s.oldVersion.Number)
	c.Check(doneLock.IsUnlocked(), jc.IsTrue)
}

func (s *UpgradeSuite) TestUpgradeStepsFailure(c *gc.C) {
	// This test checks what happens when every upgrade attempt fails.
	// A number of retries should be observed and the agent should end
	// up in a state where it is is still running but is reporting an
	// error and the upgrade is not flagged as having completed (which
	// prevents most of the agent's workers from running and keeps the
	// API in restricted mode).

	attemptsP := s.countUpgradeAttempts(errors.New("boom"))
	s.captureLogs(c)

	workerErr, config, statusCalls, doneLock := s.runUpgradeWorker(c, false)

	// The worker shouldn't return an error so that the worker and
	// agent keep running.
	c.Check(workerErr, gc.IsNil)

	c.Check(*attemptsP, gc.Equals, maxUpgradeRetries)
	c.Check(config.Version, gc.Equals, s.oldVersion.Number) // Upgrade didn't finish
	c.Assert(statusCalls, jc.DeepEquals,
		s.makeExpectedStatusCalls(maxUpgradeRetries-1, fails, "boom"))
	c.Assert(s.logWriter.Log(), jc.LogMatches,
		s.makeExpectedUpgradeLogs(maxUpgradeRetries-1, "hostMachine", fails, "boom"))
	c.Assert(doneLock.IsUnlocked(), jc.IsFalse)
}

func (s *UpgradeSuite) TestUpgradeStepsRetries(c *gc.C) {
	// This test checks what happens when the first upgrade attempt
	// fails but the following on succeeds. The final state should be
	// the same as a successful upgrade which worked first go.
	attempts := 0
	fail := true
	fakePerformUpgrade := func(version.Number, []upgrades.Target, upgrades.Context) error {
		attempts++
		if fail {
			fail = false
			return errors.New("boom")
		} else {
			return nil
		}
	}
	s.PatchValue(&PerformUpgrade, fakePerformUpgrade)
	s.captureLogs(c)

	workerErr, config, statusCalls, doneLock := s.runUpgradeWorker(c, false)

	c.Check(workerErr, gc.IsNil)
	c.Check(attempts, gc.Equals, 2)
	c.Check(config.Version, gc.Equals, jujuversion.Current) // Upgrade finished
	c.Assert(statusCalls, jc.DeepEquals, s.makeExpectedStatusCalls(1, succeeds, "boom"))
	c.Assert(s.logWriter.Log(), jc.LogMatches, s.makeExpectedUpgradeLogs(1, "hostMachine", succeeds, "boom"))
	c.Check(doneLock.IsUnlocked(), jc.IsTrue)
}

func (s *UpgradeSuite) TestOtherUpgradeRunFailure(c *gc.C) {
	// This test checks what happens something other than the upgrade
	// steps themselves fails, ensuring the something is logged and
	// the agent status is updated.

	s.captureLogs(c)

	fakePerformUpgrade := func(version.Number, []upgrades.Target, upgrades.Context) error {
		s.upgradeErr = errors.New("cannot complete upgrade: upgrade has not yet run")
		return nil
	}
	s.PatchValue(&PerformUpgrade, fakePerformUpgrade)

	// Simulate the upgrade-database worker having run successfully.
	s.notify <- struct{}{}

	workerErr, config, statusCalls, doneLock := s.runUpgradeWorker(c, true)
	c.Check(workerErr, gc.IsNil)

	c.Check(config.Version, gc.Equals, jujuversion.Current) // Upgrade almost finished

	failReason := `upgrade done but failed to synchronise: cannot complete upgrade: upgrade has not yet run`
	c.Assert(statusCalls, jc.DeepEquals, s.makeExpectedStatusCalls(0, fails, failReason))
	c.Assert(s.logWriter.Log(), jc.LogMatches, s.makeExpectedUpgradeLogs(0, "databaseMaster", fails, failReason))
	c.Assert(doneLock.IsUnlocked(), jc.IsFalse)
}

func (s *UpgradeSuite) TestAPIConnectionFailure(c *gc.C) {
	// This test checks what happens when an upgrade fails because the
	// connection to mongo has gone away. This will happen when the
	// mongo master changes. In this case we want the upgrade worker
	// to return immediately without further retries. The error should
	// be returned by the worker so that the agent will restart.

	attemptsP := s.countUpgradeAttempts(errors.New("boom"))
	s.connectionDead = true // Make the connection to state appear to be dead
	s.captureLogs(c)

	workerErr, config, _, doneLock := s.runUpgradeWorker(c, false)

	c.Check(workerErr, gc.ErrorMatches, "API connection lost during upgrade: boom")
	c.Check(*attemptsP, gc.Equals, 1)
	c.Check(config.Version, gc.Equals, s.oldVersion.Number) // Upgrade didn't finish
	c.Assert(doneLock.IsUnlocked(), jc.IsFalse)
}

func (s *UpgradeSuite) TestAbortWhenOtherControllerDoesNotStartUpgrade(c *gc.C) {
	// This test checks when a controller is upgrading and one of
	// the other controllers doesn't signal it is ready in time.

	s.captureLogs(c)
	attemptsP := s.countUpgradeAttempts(nil)

	workerErr, config, statusCalls, doneLock := s.runUpgradeWorker(c, true)

	c.Check(workerErr, gc.IsNil)
	c.Check(*attemptsP, gc.Equals, 0)
	c.Check(config.Version, gc.Equals, s.oldVersion.Number) // Upgrade didn't happen
	c.Assert(doneLock.IsUnlocked(), jc.IsFalse)

	causeMsg := " timed out after 50ms"
	c.Assert(s.logWriter.Log(), jc.LogMatches, []jc.SimpleMessage{
		{loggo.INFO, "waiting for other controllers to be ready for upgrade"},
		{loggo.ERROR, "aborted wait for other controllers: timed out after 50ms"},
		{loggo.ERROR, `upgrade from .+ to .+ for "machine-0" failed \(giving up\): ` +
			"aborted wait for other controllers:" + causeMsg},
	})
	c.Assert(statusCalls, jc.DeepEquals, []StatusCall{{
		status.Error,
		fmt.Sprintf(
			"upgrade to %s failed (giving up): aborted wait for other controllers:"+causeMsg,
			jujuversion.Current),
	}})
}

func (s *UpgradeSuite) TestSuccessLeadingController(c *gc.C) {
	// This test checks what happens when an upgrade works on the first
	// attempt, on the first controller to set the status to "running".
	info := s.checkSuccess(c, "databaseMaster", func(i UpgradeInfo) {
		err := i.SetStatus(state.UpgradeDBComplete)
		c.Assert(err, jc.ErrorIsNil)
	})
	c.Assert(info.Status(), gc.Equals, state.UpgradeRunning)
}

func (s *UpgradeSuite) TestSuccessFollowingController(c *gc.C) {
	// This test checks what happens when an upgrade works on the a controller
	// following a controller having already set the status to "running".
	s.checkSuccess(c, "controller", func(info UpgradeInfo) {
		// Indicate that the master is done
		err := info.SetStatus(state.UpgradeDBComplete)
		c.Assert(err, jc.ErrorIsNil)
		err = info.SetStatus(state.UpgradeRunning)
		c.Assert(err, jc.ErrorIsNil)
	})
}

func (s *UpgradeSuite) checkSuccess(c *gc.C, target string, mungeInfo func(UpgradeInfo)) UpgradeInfo {
	info := &fakeUpgradeInfo{s}
	mungeInfo(info)

	s.notify <- struct{}{}

	attemptsP := s.countUpgradeAttempts(nil)
	s.captureLogs(c)

	workerErr, config, statusCalls, doneLock := s.runUpgradeWorker(c, true)

	c.Check(workerErr, gc.IsNil)
	c.Check(*attemptsP, gc.Equals, 1)
	c.Check(config.Version, gc.Equals, jujuversion.Current) // Upgrade finished
	c.Assert(statusCalls, jc.DeepEquals, s.makeExpectedStatusCalls(0, succeeds, ""))
	c.Assert(s.logWriter.Log(), jc.LogMatches, s.makeExpectedUpgradeLogs(0, target, succeeds, ""))
	c.Check(doneLock.IsUnlocked(), jc.IsTrue)

	c.Assert(s.controllersDone, jc.SameContents, []string{"0"})
	return info
}

func (s *UpgradeSuite) TestJobsToTargets(c *gc.C) {
	c.Assert(upgradeTargets(false), jc.DeepEquals, []upgrades.Target{upgrades.HostMachine})
	c.Assert(upgradeTargets(true), jc.SameContents, []upgrades.Target{upgrades.HostMachine, upgrades.Controller})
}

func (s *UpgradeSuite) TestPreUpgradeFail(c *gc.C) {
	s.preUpgradeError = true
	s.captureLogs(c)

	workerErr, config, statusCalls, doneLock := s.runUpgradeWorker(c, false)

	c.Check(workerErr, jc.ErrorIsNil)
	c.Check(config.Version, gc.Equals, s.oldVersion.Number) // Upgrade didn't finish
	c.Assert(doneLock.IsUnlocked(), jc.IsFalse)

	causeMessage := `machine 0 cannot be upgraded: preupgrade error`
	failMessage := fmt.Sprintf(
		`upgrade from %s to %s for "machine-0" failed \(giving up\): %s`,
		s.oldVersion.Number, jujuversion.Current, causeMessage)
	c.Assert(s.logWriter.Log(), jc.LogMatches, []jc.SimpleMessage{
		{loggo.INFO, "checking that upgrade can proceed"},
		{loggo.ERROR, failMessage},
	})

	statusMessage := fmt.Sprintf(
		`upgrade to %s failed (giving up): %s`, jujuversion.Current, causeMessage)
	c.Assert(statusCalls, jc.DeepEquals, []StatusCall{{
		status.Error, statusMessage,
	}})
}

// Run just the upgradeSteps worker with a fake machine agent and
// fake agent config.
func (s *UpgradeSuite) runUpgradeWorker(c *gc.C, isController bool) (
	error, *fakeConfigSetter, []StatusCall, gate.Lock,
) {
	config := s.makeFakeConfig()
	agent := NewFakeAgent(config)
	doneLock := NewLock(config)
	machineStatus := &testStatusSetter{}
	testRetryStrategy := retry.CallArgs{
		Clock:    clock.WallClock,
		Delay:    time.Millisecond,
		Attempts: maxUpgradeRetries,
	}
	worker, err := NewWorker(
		doneLock,
		agent,
		nil,
		isController,
		s.openStateForUpgrade,
		s.preUpgradeSteps,
		testRetryStrategy,
		machineStatus,
	)
	c.Assert(err, jc.ErrorIsNil)
	return worker.Wait(), config, machineStatus.Calls, doneLock
}

func (s *UpgradeSuite) openStateForUpgrade() (*state.StatePool, SystemState, error) {
	return &state.StatePool{}, &fakeSystemState{s}, nil
}

func (s *UpgradeSuite) preUpgradeSteps(_ agent.Config, _, _ bool) error {
	if s.preUpgradeError {
		return errors.New("preupgrade error")
	}
	return nil
}

func (s *UpgradeSuite) makeFakeConfig() *fakeConfigSetter {
	return NewFakeConfigSetter(names.NewMachineTag("0"), s.oldVersion.Number)
}

const maxUpgradeRetries = 3

func (s *UpgradeSuite) makeExpectedStatusCalls(retryCount int, expectFail bool, failReason string) []StatusCall {
	calls := []StatusCall{{
		status.Started,
		fmt.Sprintf("upgrading to %s", jujuversion.Current),
	}}
	for i := 0; i < retryCount; i++ {
		calls = append(calls, StatusCall{
			status.Error,
			fmt.Sprintf("upgrade to %s failed (will retry): %s", jujuversion.Current, failReason),
		})
	}
	if expectFail {
		calls = append(calls, StatusCall{
			status.Error,
			fmt.Sprintf("upgrade to %s failed (giving up): %s", jujuversion.Current, failReason),
		})
	} else {
		calls = append(calls, StatusCall{status.Started, ""})
	}
	return calls
}

func (s *UpgradeSuite) makeExpectedUpgradeLogs(retryCount int, target string, expectFail bool, failReason string) []jc.SimpleMessage {
	outLogs := []jc.SimpleMessage{}

	if target == "databaseMaster" || target == "controller" {
		outLogs = append(outLogs, jc.SimpleMessage{
			Level: loggo.INFO, Message: "waiting for other controllers to be ready for upgrade",
		})
		outLogs = append(outLogs, jc.SimpleMessage{
			Level:   loggo.INFO,
			Message: "finished waiting - all controllers are ready to run upgrade steps",
		})
	}

	outLogs = append(outLogs, jc.SimpleMessage{
		Level: loggo.INFO, Message: fmt.Sprintf(
			`starting upgrade from %s to %s for "machine-0"`,
			s.oldVersion.Number, jujuversion.Current),
	})

	failMessage := fmt.Sprintf(
		`upgrade from %s to %s for "machine-0" failed \(%%s\): %s`,
		s.oldVersion.Number, jujuversion.Current, failReason)

	for i := 0; i < retryCount; i++ {
		outLogs = append(outLogs, jc.SimpleMessage{Level: loggo.ERROR, Message: fmt.Sprintf(failMessage, "will retry")})
	}
	if expectFail {
		outLogs = append(outLogs, jc.SimpleMessage{Level: loggo.ERROR, Message: fmt.Sprintf(failMessage, "giving up")})
	} else {
		outLogs = append(outLogs, jc.SimpleMessage{Level: loggo.INFO,
			Message: fmt.Sprintf(`upgrade to %s completed successfully.`, jujuversion.Current)})
	}
	return outLogs
}

type fakeUpgradeInfo struct {
	s *UpgradeSuite
}

func (u *fakeUpgradeInfo) SetControllerDone(controllerId string) error {
	u.s.controllersDone = append(u.s.controllersDone, controllerId)
	return u.s.upgradeErr
}

func (u *fakeUpgradeInfo) Watch() state.NotifyWatcher {
	return watchertest.NewMockNotifyWatcher(u.s.notify)
}

func (u *fakeUpgradeInfo) Refresh() error {
	return nil
}

func (u *fakeUpgradeInfo) AllProvisionedControllersReady() (bool, error) {
	return true, nil
}

func (u *fakeUpgradeInfo) SetStatus(status state.UpgradeStatus) error {
	u.s.status = status
	return nil
}

func (u *fakeUpgradeInfo) Status() state.UpgradeStatus {
	return u.s.status
}

func (u *fakeUpgradeInfo) Abort() error {
	return nil
}

type fakeSystemState struct {
	s *UpgradeSuite
}

func (*fakeSystemState) ModelType() (state.ModelType, error) {
	return state.ModelTypeIAAS, nil
}

func (s *fakeSystemState) EnsureUpgradeInfo(controllerId string, previousVersion, targetVersion version.Number) (UpgradeInfo, error) {
	return &fakeUpgradeInfo{s.s}, nil
}

// NewFakeConfigSetter returns a fakeConfigSetter which implements
// just enough of the agent.ConfigSetter interface to keep the upgrade
// steps worker happy.
func NewFakeConfigSetter(agentTag names.Tag, initialVersion version.Number) *fakeConfigSetter {
	return &fakeConfigSetter{
		AgentTag: agentTag,
		Version:  initialVersion,
	}
}

type fakeConfigSetter struct {
	agent.ConfigSetter
	AgentTag names.Tag
	Version  version.Number
}

func (s *fakeConfigSetter) Tag() names.Tag {
	return s.AgentTag
}

func (s *fakeConfigSetter) UpgradedToVersion() version.Number {
	return s.Version
}

func (s *fakeConfigSetter) SetUpgradedToVersion(newVersion version.Number) {
	s.Version = newVersion
}

// NewFakeAgent returns a fakeAgent which implements the agent.Agent
// interface. This provides enough MachineAgent functionality to
// support upgrades.
func NewFakeAgent(confSetter agent.ConfigSetter) *fakeAgent {
	return &fakeAgent{
		config: confSetter,
	}
}

type fakeAgent struct {
	config agent.ConfigSetter
}

func (a *fakeAgent) CurrentConfig() agent.Config {
	return a.config
}

func (a *fakeAgent) ChangeConfig(mutate agent.ConfigMutator) error {
	return mutate(a.config)
}

type StatusCall struct {
	Status status.Status
	Info   string
}

type testStatusSetter struct {
	Calls []StatusCall
}

func (s *testStatusSetter) SetStatus(status status.Status, info string, _ map[string]interface{}) error {
	s.Calls = append(s.Calls, StatusCall{status, info})
	return nil
}
