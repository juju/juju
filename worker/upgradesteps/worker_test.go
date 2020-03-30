// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradesteps

import (
	"fmt"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/os/series"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/arch"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/agent"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/upgrades"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/juju/worker/gate"
)

// TODO(mjs) - these tests are too tightly coupled to the
// implementation. They needn't be internal tests.

type UpgradeSuite struct {
	statetesting.StateSuite

	oldVersion      version.Binary
	logWriter       loggo.TestWriter
	connectionDead  bool
	machineIsMaster bool
	preUpgradeError bool
}

var _ = gc.Suite(&UpgradeSuite{})

const fails = true
const succeeds = false

func (s *UpgradeSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)

	s.preUpgradeError = false
	// Most of these tests normally finish sub-second on a fast machine.
	// If any given test hits a minute, we have almost certainly become
	// wedged, so dump the logs.
	coretesting.DumpTestLogsAfter(time.Minute, c, s)

	s.oldVersion = version.Binary{
		Number: jujuversion.Current,
		Arch:   arch.HostArch(),
		Series: series.MustHostSeries(),
	}
	s.oldVersion.Major = 1
	s.oldVersion.Minor = 16

	// Don't wait so long in tests.
	s.PatchValue(&UpgradeStartTimeoutMaster, time.Millisecond*50)
	s.PatchValue(&UpgradeStartTimeoutSecondary, time.Millisecond*60)

	// Allow tests to make the API connection appear to be dead.
	s.connectionDead = false
	s.PatchValue(&cmdutil.ConnectionIsDead, func(loggo.Logger, cmdutil.Breakable) bool {
		return s.connectionDead
	})

	s.machineIsMaster = true
	fakeIsMachineMaster := func(*state.StatePool, string) (bool, error) {
		return s.machineIsMaster, nil
	}
	s.PatchValue(&IsMachineMaster, fakeIsMachineMaster)

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

func (s *UpgradeSuite) TestRetryStrategy(c *gc.C) {
	retries := getUpgradeRetryStrategy()
	c.Assert(retries.Delay, gc.Equals, 2*time.Minute)
	c.Assert(retries.Min, gc.Equals, 5)
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

	fakePerformUpgrade := func(version.Number, []upgrades.Target, upgrades.Context) error {
		// Delete UpgradeInfo for the upgrade so that finaliseUpgrade() will fail
		return s.State.ClearUpgradeInfo()
	}
	s.PatchValue(&PerformUpgrade, fakePerformUpgrade)
	m := s.Factory.MakeMachine(c, &factory.MachineParams{
		Jobs: []state.MachineJob{state.JobManageModel},
	})
	s.captureLogs(c)

	// Simulate the upgrade-database worker having run successfully.
	info, err := s.State.EnsureUpgradeInfo(m.Id(), s.oldVersion.Number, jujuversion.Current)
	c.Assert(err, jc.ErrorIsNil)
	err = info.SetStatus(state.UpgradeDBComplete)
	c.Assert(err, jc.ErrorIsNil)

	workerErr, config, statusCalls, doneLock := s.runUpgradeWorker(c, true)

	c.Check(workerErr, gc.IsNil)
	c.Check(config.Version, gc.Equals, jujuversion.Current) // Upgrade almost finished
	failReason := `upgrade done but: cannot set upgrade status to "finishing": ` +
		`Another status change may have occurred concurrently`
	c.Assert(statusCalls, jc.DeepEquals,
		s.makeExpectedStatusCalls(0, fails, failReason))
	c.Assert(s.logWriter.Log(), jc.LogMatches,
		s.makeExpectedUpgradeLogs(0, "databaseMaster", fails, failReason))
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

func (s *UpgradeSuite) TestAbortWhenOtherControllerDoesntStartUpgrade(c *gc.C) {
	// This test checks when a controller is upgrading and one of
	// the other controllers doesn't signal it is ready in time.

	err := s.State.SetModelAgentVersion(jujuversion.Current, false)
	c.Assert(err, jc.ErrorIsNil)

	// The master controller in this scenario is functionally tested
	// elsewhere.
	s.machineIsMaster = false

	s.create3Controllers(c)
	s.captureLogs(c)
	attemptsP := s.countUpgradeAttempts(nil)

	workerErr, config, statusCalls, doneLock := s.runUpgradeWorker(c, true)

	c.Check(workerErr, gc.IsNil)
	c.Check(*attemptsP, gc.Equals, 0)
	c.Check(config.Version, gc.Equals, s.oldVersion.Number) // Upgrade didn't happen
	c.Assert(doneLock.IsUnlocked(), jc.IsFalse)

	// The environment agent-version should still be the new version.
	// It's up to the master to trigger the rollback.
	s.assertEnvironAgentVersion(c, jujuversion.Current)

	causeMsg := " timed out after 60ms"
	c.Assert(s.logWriter.Log(), jc.LogMatches, []jc.SimpleMessage{
		{loggo.INFO, "waiting for other controllers to be ready for upgrade"},
		{loggo.ERROR, "aborted wait for other controllers: timed out after 60ms"},
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

func (s *UpgradeSuite) TestSuccessMaster(c *gc.C) {
	// This test checks what happens when an upgrade works on the
	// first attempt on a master controller.
	s.machineIsMaster = true
	info := s.checkSuccess(c, "databaseMaster", func(i *state.UpgradeInfo) {
		err := i.SetStatus(state.UpgradeDBComplete)
		c.Assert(err, jc.ErrorIsNil)
	})
	c.Assert(info.Status(), gc.Equals, state.UpgradeFinishing)
}

func (s *UpgradeSuite) TestSuccessSecondary(c *gc.C) {
	// This test checks what happens when an upgrade works on the
	// first attempt on a secondary controller.
	s.machineIsMaster = false
	mungeInfo := func(info *state.UpgradeInfo) {
		// Indicate that the master is done
		err := info.SetStatus(state.UpgradeDBComplete)
		c.Assert(err, jc.ErrorIsNil)
		err = info.SetStatus(state.UpgradeRunning)
		c.Assert(err, jc.ErrorIsNil)
		err = info.SetStatus(state.UpgradeFinishing)
		c.Assert(err, jc.ErrorIsNil)
	}
	s.checkSuccess(c, "controller", mungeInfo)
}

func (s *UpgradeSuite) checkSuccess(c *gc.C, target string, mungeInfo func(*state.UpgradeInfo)) *state.UpgradeInfo {
	_, machineIdB, machineIdC := s.create3Controllers(c)

	// Indicate that machine B and C are ready to upgrade
	vPrevious := s.oldVersion.Number
	vNext := jujuversion.Current
	info, err := s.State.EnsureUpgradeInfo(machineIdB, vPrevious, vNext)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.EnsureUpgradeInfo(machineIdC, vPrevious, vNext)
	c.Assert(err, jc.ErrorIsNil)

	mungeInfo(info)

	attemptsP := s.countUpgradeAttempts(nil)
	s.captureLogs(c)

	workerErr, config, statusCalls, doneLock := s.runUpgradeWorker(c, true)

	c.Check(workerErr, gc.IsNil)
	c.Check(*attemptsP, gc.Equals, 1)
	c.Check(config.Version, gc.Equals, jujuversion.Current) // Upgrade finished
	c.Assert(statusCalls, jc.DeepEquals, s.makeExpectedStatusCalls(0, succeeds, ""))
	c.Assert(s.logWriter.Log(), jc.LogMatches, s.makeExpectedUpgradeLogs(0, target, succeeds, ""))
	c.Check(doneLock.IsUnlocked(), jc.IsTrue)

	err = info.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.ControllersDone(), jc.DeepEquals, []string{"0"})
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

// Run just the upgradesteps worker with a fake machine agent and
// fake agent config.
func (s *UpgradeSuite) runUpgradeWorker(c *gc.C, isController bool) (
	error, *fakeConfigSetter, []StatusCall, gate.Lock,
) {
	s.setInstantRetryStrategy(c)
	config := s.makeFakeConfig()
	agent := NewFakeAgent(config)
	doneLock := NewLock(config)
	machineStatus := &testStatusSetter{}
	worker, err := NewWorker(
		doneLock,
		agent,
		nil,
		isController,
		s.openStateForUpgrade,
		s.preUpgradeSteps,
		machineStatus,
		false,
	)
	c.Assert(err, jc.ErrorIsNil)
	return worker.Wait(), config, machineStatus.Calls, doneLock
}

func (s *UpgradeSuite) openStateForUpgrade() (*state.StatePool, error) {
	newPolicy := stateenvirons.GetNewPolicyFunc()
	pool, err := state.OpenStatePool(state.OpenParams{
		Clock:              clock.WallClock,
		ControllerTag:      s.State.ControllerTag(),
		ControllerModelTag: s.Model.ModelTag(),
		MongoSession:       s.State.MongoSession(),
		NewPolicy:          newPolicy,
	})
	if err != nil {
		return nil, err
	}
	return pool, nil
}

func (s *UpgradeSuite) preUpgradeSteps(pool *state.StatePool, agentConf agent.Config, isController, isMaster, isCaas bool) error {
	if s.preUpgradeError {
		return errors.New("preupgrade error")
	}
	return nil
}

func (s *UpgradeSuite) makeFakeConfig() *fakeConfigSetter {
	return NewFakeConfigSetter(names.NewMachineTag("0"), s.oldVersion.Number)
}

func (s *UpgradeSuite) create3Controllers(c *gc.C) (machineIdA, machineIdB, machineIdC string) {
	machine0 := s.Factory.MakeMachine(c, &factory.MachineParams{
		Jobs: []state.MachineJob{state.JobManageModel},
	})
	machineIdA = machine0.Id()

	changes, err := s.State.EnableHA(3, constraints.Value{}, "quantal", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(changes.Added), gc.Equals, 2)

	machineIdB = changes.Added[0]
	s.setMachineProvisioned(c, machineIdB)

	machineIdC = changes.Added[1]
	s.setMachineProvisioned(c, machineIdC)

	return
}

func (s *UpgradeSuite) setMachineProvisioned(c *gc.C, id string) {
	machine, err := s.State.Machine(id)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned(instance.Id(id+"-inst"), "", "nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
}

const maxUpgradeRetries = 3

func (s *UpgradeSuite) setInstantRetryStrategy(c *gc.C) {
	// TODO(katco): 2016-08-09: lp:1611427
	s.PatchValue(&getUpgradeRetryStrategy, func() utils.AttemptStrategy {
		c.Logf("setting instant retry strategy for upgrade: retries=%d", maxUpgradeRetries)
		return utils.AttemptStrategy{
			Delay: 0,
			Min:   maxUpgradeRetries,
		}
	})
}

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
			loggo.INFO, "waiting for other controllers to be ready for upgrade",
		})
		var waitMsg string
		switch target {
		case "databaseMaster":
			waitMsg = "all controllers are ready to run upgrade steps"
		case "controller":
			waitMsg = "the master has completed its upgrade steps"
		}
		outLogs = append(outLogs, jc.SimpleMessage{loggo.INFO, "finished waiting - " + waitMsg})
	}

	outLogs = append(outLogs, jc.SimpleMessage{
		loggo.INFO, fmt.Sprintf(
			`starting upgrade from %s to %s for "machine-0"`,
			s.oldVersion.Number, jujuversion.Current),
	})

	failMessage := fmt.Sprintf(
		`upgrade from %s to %s for "machine-0" failed \(%%s\): %s`,
		s.oldVersion.Number, jujuversion.Current, failReason)

	for i := 0; i < retryCount; i++ {
		outLogs = append(outLogs, jc.SimpleMessage{loggo.ERROR, fmt.Sprintf(failMessage, "will retry")})
	}
	if expectFail {
		outLogs = append(outLogs, jc.SimpleMessage{loggo.ERROR, fmt.Sprintf(failMessage, "giving up")})
	} else {
		outLogs = append(outLogs, jc.SimpleMessage{loggo.INFO,
			fmt.Sprintf(`upgrade to %s completed successfully.`, jujuversion.Current)})
	}
	return outLogs
}

func (s *UpgradeSuite) assertEnvironAgentVersion(c *gc.C, expected version.Number) {
	envConfig, err := s.Model.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	agentVersion, ok := envConfig.AgentVersion()
	c.Assert(ok, jc.IsTrue)
	c.Assert(agentVersion, gc.Equals, expected)
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
