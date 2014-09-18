// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/apt"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker/upgrader"
)

type UpgradeSuite struct {
	commonMachineSuite

	aptCmds         []*exec.Cmd
	oldVersion      version.Binary
	logWriter       loggo.TestWriter
	connectionDead  bool
	machineIsMaster bool
}

var _ = gc.Suite(&UpgradeSuite{})

type exposedAPI bool

var (
	FullAPIExposed       exposedAPI = true
	RestrictedAPIExposed exposedAPI = false
)

func (s *UpgradeSuite) SetUpTest(c *gc.C) {
	s.commonMachineSuite.SetUpTest(c)

	// Capture all apt commands.
	s.aptCmds = nil
	aptCmds := s.agentSuite.HookCommandOutput(&apt.CommandOutput, nil, nil)
	go func() {
		for cmd := range aptCmds {
			s.aptCmds = append(s.aptCmds, cmd)
		}
	}()

	s.oldVersion = version.Current
	s.oldVersion.Major = 1
	s.oldVersion.Minor = 16

	// Don't wait so long in tests.
	s.PatchValue(&upgradeStartTimeoutMaster, time.Duration(time.Millisecond*50))
	s.PatchValue(&upgradeStartTimeoutSecondary, time.Duration(time.Millisecond*60))

	// Allow tests to make the API connection appear to be dead.
	s.connectionDead = false
	s.PatchValue(&connectionIsDead, func(pinger) bool {
		return s.connectionDead
	})

	var fakeOpenStateForUpgrade = func(upgradingMachineAgent, agent.Config) (*state.State, error) {
		mongoInfo := s.State.MongoConnectionInfo()
		st, err := state.Open(mongoInfo, mongo.DefaultDialOpts(), environs.NewStatePolicy())
		c.Assert(err, gc.IsNil)
		return st, nil
	}
	s.PatchValue(&openStateForUpgrade, fakeOpenStateForUpgrade)

	s.machineIsMaster = true
	fakeIsMachineMaster := func(*state.State, string) (bool, error) {
		return s.machineIsMaster, nil
	}
	s.PatchValue(&isMachineMaster, fakeIsMachineMaster)
}

func (s *UpgradeSuite) captureLogs(c *gc.C) {
	c.Assert(loggo.RegisterWriter("upgrade-tests", &s.logWriter, loggo.INFO), gc.IsNil)
	s.AddCleanup(func(*gc.C) {
		loggo.RemoveWriter("upgrade-tests")
		s.logWriter.Clear()
	})
}

func (s *UpgradeSuite) countUpgradeAttempts(upgradeErr error) *int {
	count := 0
	s.PatchValue(&upgradesPerformUpgrade, func(version.Number, upgrades.Target, upgrades.Context) error {
		count++
		return upgradeErr
	})
	return &count
}

func (s *UpgradeSuite) TestContextInitializeWhenNoUpgradeRequired(c *gc.C) {
	// Set the agent's initial upgradedToVersion to almost the same as
	// the current version. We want it to be different to
	// version.Current (so that we can see it change) but not to
	// trigger upgrade steps.
	config := NewFakeConfigSetter(names.NewMachineTag("0"), makeBumpedCurrentVersion().Number)
	agent := NewFakeUpgradingMachineAgent(config)

	context := NewUpgradeWorkerContext()
	context.InitializeUsingAgent(agent)

	select {
	case <-context.UpgradeComplete:
		// Success
	default:
		c.Fatal("UpgradeComplete channel should be closed because no upgrade is required")
	}
	// The agent's version should have been updated.
	c.Assert(config.Version, gc.Equals, version.Current.Number)

}

func (s *UpgradeSuite) TestContextInitializeWhenUpgradeRequired(c *gc.C) {
	// Set the agent's upgradedToVersion so that upgrade steps are required.
	initialVersion := version.MustParse("1.16.0")
	config := NewFakeConfigSetter(names.NewMachineTag("0"), initialVersion)
	agent := NewFakeUpgradingMachineAgent(config)

	context := NewUpgradeWorkerContext()
	context.InitializeUsingAgent(agent)

	select {
	case <-context.UpgradeComplete:
		c.Fatal("UpgradeComplete channel shouldn't be closed because upgrade is required")
	default:
		// Success
	}
	// The agent's version should NOT have been updated.
	c.Assert(config.Version, gc.Equals, initialVersion)
}

func (s *UpgradeSuite) TestRetryStrategy(c *gc.C) {
	retries := getUpgradeRetryStrategy()
	c.Assert(retries.Delay, gc.Equals, 2*time.Minute)
	c.Assert(retries.Min, gc.Equals, 5)
}

func (s *UpgradeSuite) TestIsUpgradeRunning(c *gc.C) {
	context := NewUpgradeWorkerContext()
	c.Assert(context.IsUpgradeRunning(), jc.IsTrue)

	close(context.UpgradeComplete)
	c.Assert(context.IsUpgradeRunning(), jc.IsFalse)
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

	workerErr, config, agent, context := s.runUpgradeWorker(c, params.JobHostUnits)

	// The worker shouldn't return an error so that the worker and
	// agent keep running.
	c.Check(workerErr, gc.IsNil)

	c.Check(*attemptsP, gc.Equals, maxUpgradeRetries)
	c.Check(config.Version, gc.Equals, s.oldVersion.Number) // Upgrade didn't finish
	c.Assert(agent.MachineStatusCalls, jc.DeepEquals, s.makeExpectedStatusCalls(maxUpgradeRetries))
	c.Assert(s.logWriter.Log(), jc.LogMatches, s.makeExpectedUpgradeLogs(maxUpgradeRetries, "hostMachine"))
	assertUpgradeNotComplete(c, context)
}

func (s *UpgradeSuite) TestUpgradeStepsRetries(c *gc.C) {
	// This test checks what happens when the first upgrade attempt
	// fails but the following on succeeds. The final state should be
	// the same as a successful upgrade which worked first go.
	attempts := 0
	fail := true
	fakePerformUpgrade := func(version.Number, upgrades.Target, upgrades.Context) error {
		attempts++
		if fail {
			fail = false
			return errors.New("boom")
		} else {
			return nil
		}
	}
	s.PatchValue(&upgradesPerformUpgrade, fakePerformUpgrade)
	s.captureLogs(c)

	workerErr, config, agent, context := s.runUpgradeWorker(c, params.JobHostUnits)

	c.Check(workerErr, gc.IsNil)
	c.Check(attempts, gc.Equals, 2)
	c.Check(config.Version, gc.Equals, version.Current.Number) // Upgrade finished
	c.Assert(agent.MachineStatusCalls, jc.DeepEquals, s.makeExpectedStatusCalls(1))
	c.Assert(s.logWriter.Log(), jc.LogMatches, s.makeExpectedUpgradeLogs(1, "hostMachine"))
	assertUpgradeComplete(c, context)
}

func (s *UpgradeSuite) TestApiConnectionFailure(c *gc.C) {
	// This test checks what happens when an upgrade fails because the
	// connection to mongo has gone away. This will happen when the
	// mongo master changes. In this case we want the upgrade worker
	// to return immediately without further retries. The error should
	// be returned by the worker so that the agent will restart.

	attemptsP := s.countUpgradeAttempts(errors.New("boom"))
	s.connectionDead = true // Make the connection to state appear to be dead
	s.captureLogs(c)

	workerErr, config, _, context := s.runUpgradeWorker(c, params.JobHostUnits)

	c.Check(workerErr, gc.ErrorMatches, "API connection lost during upgrade: boom")
	c.Check(*attemptsP, gc.Equals, 1)
	c.Check(config.Version, gc.Equals, s.oldVersion.Number) // Upgrade didn't finish
	assertUpgradeNotComplete(c, context)
}

func (s *UpgradeSuite) TestAbortWhenOtherStateServerDoesntStartUpgrade(c *gc.C) {
	// This test checks when a state server is upgrading and one of
	// the other state servers doesn't signal it is ready in time.

	// The master state server in this scenario is functionally tested
	// elsewhere in this suite.
	s.machineIsMaster = false

	s.createUpgradingStateServers(c)
	s.captureLogs(c)
	attemptsP := s.countUpgradeAttempts(nil)

	workerErr, config, agent, context := s.runUpgradeWorker(c, params.JobManageEnviron)

	c.Check(workerErr, gc.IsNil)
	c.Check(*attemptsP, gc.Equals, 0)
	c.Check(config.Version, gc.Equals, s.oldVersion.Number) // Upgrade didn't happen
	assertUpgradeNotComplete(c, context)

	// The environment agent-version should still be the new version.
	// It's up to the master to trigger the rollback.
	s.assertEnvironAgentVersion(c, version.Current.Number)

	causeMsg := " timed out after 60ms"
	c.Assert(s.logWriter.Log(), jc.LogMatches, []jc.SimpleMessage{
		{loggo.INFO, "waiting for other state servers to be ready for upgrade"},
		{loggo.ERROR, fmt.Sprintf(
			`other state servers failed to come up for upgrade to %s - aborting:`+causeMsg,
			version.Current.Number)},
	})
	c.Assert(agent.MachineStatusCalls, jc.DeepEquals, []MachineStatusCall{{
		params.StatusError,
		fmt.Sprintf(
			"upgrade to %s aborted while waiting for other state servers:"+causeMsg,
			version.Current.Number),
	}})
}

func (s *UpgradeSuite) TestWorkerAbortsIfAgentDies(c *gc.C) {
	s.machineIsMaster = false
	s.captureLogs(c)
	attemptsP := s.countUpgradeAttempts(nil)

	s.primeAgent(c, s.oldVersion, state.JobManageEnviron)

	config := s.makeFakeConfig()
	agent := NewFakeUpgradingMachineAgent(config)
	close(agent.DyingCh)
	workerErr, context := s.runUpgradeWorkerUsingAgent(c, params.JobManageEnviron, agent)

	c.Check(workerErr, gc.IsNil)
	c.Check(*attemptsP, gc.Equals, 0)
	c.Check(config.Version, gc.Equals, s.oldVersion.Number) // Upgrade didn't happen
	assertUpgradeNotComplete(c, context)
	c.Assert(s.logWriter.Log(), jc.LogMatches, []jc.SimpleMessage{
		{loggo.WARNING, "stopped waiting for other state servers: machine agent is terminating"},
	})
}

func (s *UpgradeSuite) TestSuccessMaster(c *gc.C) {
	// This test checks what happens when an upgrade works on the
	// first attempt on a master state server.
	s.machineIsMaster = true
	info := s.checkSuccess(c, "databaseMaster", func(*state.UpgradeInfo) {})
	c.Assert(info.Status(), gc.Equals, state.UpgradeFinishing)
}

func (s *UpgradeSuite) TestSuccessSecondary(c *gc.C) {
	// This test checks what happens when an upgrade works on the
	// first attempt on a secondary state server.
	s.machineIsMaster = false
	mungeInfo := func(info *state.UpgradeInfo) {
		// Indicate that the master is done
		err := info.SetStatus(state.UpgradeRunning)
		c.Assert(err, gc.IsNil)
		err = info.SetStatus(state.UpgradeFinishing)
		c.Assert(err, gc.IsNil)
	}
	s.checkSuccess(c, "stateServer", mungeInfo)
}

func (s *UpgradeSuite) checkSuccess(c *gc.C, target string, mungeInfo func(*state.UpgradeInfo)) *state.UpgradeInfo {
	_, machineIdB, machineIdC := s.createUpgradingStateServers(c)

	// Indicate that machine B and C are ready to upgrade
	vPrevious := s.oldVersion.Number
	vNext := version.Current.Number
	info, err := s.State.EnsureUpgradeInfo(machineIdB, vPrevious, vNext)
	c.Assert(err, gc.IsNil)
	_, err = s.State.EnsureUpgradeInfo(machineIdC, vPrevious, vNext)
	c.Assert(err, gc.IsNil)

	mungeInfo(info)

	attemptsP := s.countUpgradeAttempts(nil)
	s.captureLogs(c)

	workerErr, config, agent, context := s.runUpgradeWorker(c, params.JobManageEnviron)

	c.Check(workerErr, gc.IsNil)
	c.Check(*attemptsP, gc.Equals, 1)
	c.Check(config.Version, gc.Equals, version.Current.Number) // Upgrade finished
	c.Assert(agent.MachineStatusCalls, jc.DeepEquals, s.makeExpectedStatusCalls(0))
	c.Assert(s.logWriter.Log(), jc.LogMatches, s.makeExpectedUpgradeLogs(0, target))
	assertUpgradeComplete(c, context)

	err = info.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(info.StateServersDone(), jc.DeepEquals, []string{"0"})
	return info
}

func (s *UpgradeSuite) TestUpgradeStepsStateServer(c *gc.C) {
	s.assertUpgradeSteps(c, state.JobManageEnviron)
	s.assertStateServerUpgrades(c)
}

func (s *UpgradeSuite) TestUpgradeStepsHostMachine(c *gc.C) {
	// We need to first start up a state server that thinks it has already been upgraded.
	ss, _, _ := s.primeAgent(c, version.Current, state.JobManageEnviron)
	a := s.newAgent(c, ss)
	go func() { c.Check(a.Run(nil), gc.IsNil) }()
	defer func() { c.Check(a.Stop(), gc.IsNil) }()
	// Now run the test.
	s.assertUpgradeSteps(c, state.JobHostUnits)
	s.assertHostUpgrades(c)
}

func (s *UpgradeSuite) TestLoginsDuringUpgrade(c *gc.C) {
	// Most tests in this file are lightweight unit tests.
	//
	// This test however is a fairly heavyweight end-to-end functional
	// test that spins up machine agents and attempts actual logins
	// during and after the upgrade process. Please keep this as a
	// functional test so that there are at least some tests which
	// ensure that the various components involved with machine agent
	// upgrades hang together.
	upgradeCh := make(chan bool)
	fakePerformUpgrade := func(version.Number, upgrades.Target, upgrades.Context) error {
		upgradeCh <- true // signal that upgrade has started
		<-upgradeCh       // wait for signal that upgrades should finish
		return nil
	}
	s.PatchValue(&upgradesPerformUpgrade, fakePerformUpgrade)

	a, stopFunc := s.createAgentAndStartUpgrade(c, state.JobManageEnviron)
	defer func() {
		// stopFunc won't complete unless the upgrade is done
		select {
		case <-upgradeCh:
			break
		default:
			close(upgradeCh)
		}
		stopFunc()
	}()
	machine0Conf := a.CurrentConfig()

	// Set up a second machine to log in as.
	// API logins are tested manually so there's no need to actually
	// start this machine.
	var machine1Conf agent.Config
	_, machine1Conf, _ = s.primeAgent(c, version.Current, state.JobHostUnits)

	c.Assert(waitForUpgradeToStart(upgradeCh), gc.Equals, true)

	// Only user and local logins are allowed during upgrade. Users get a restricted API.
	checkLoginToAPIAsUser(c, machine0Conf, RestrictedAPIExposed)
	c.Assert(canLoginToAPIAsMachine(c, machine0Conf, machine0Conf), gc.Equals, true)
	c.Assert(canLoginToAPIAsMachine(c, machine1Conf, machine0Conf), gc.Equals, false)

	close(upgradeCh) // Allow upgrade to complete

	waitForUpgradeToFinish(c, machine0Conf)

	// All logins are allowed after upgrade
	checkLoginToAPIAsUser(c, machine0Conf, FullAPIExposed)
	c.Assert(canLoginToAPIAsMachine(c, machine0Conf, machine0Conf), gc.Equals, true)
	c.Assert(canLoginToAPIAsMachine(c, machine1Conf, machine0Conf), gc.Equals, true)
}

func (s *UpgradeSuite) TestUpgradeSkippedIfNoUpgradeRequired(c *gc.C) {
	attempts := 0
	upgradeCh := make(chan bool)
	fakePerformUpgrade := func(version.Number, upgrades.Target, upgrades.Context) error {
		// Note: this shouldn't run.
		attempts++
		// If execution ends up here, wait so it can be detected (by
		// checking for restricted API
		<-upgradeCh
		return nil
	}
	s.PatchValue(&upgradesPerformUpgrade, fakePerformUpgrade)

	// Set up machine agent running the current version.
	//
	// Set the agent's initial upgradedToVersion to be almost the same
	// as version.Current but not quite. We want it to be different to
	// version.Current (so that we can see it change) but not to
	// trigger upgrade steps.
	initialVersion := makeBumpedCurrentVersion()
	machine, agentConf, _ := s.primeAgent(c, initialVersion, state.JobManageEnviron)
	a := s.newAgent(c, machine)
	go func() { c.Check(a.Run(nil), gc.IsNil) }()
	defer func() {
		close(upgradeCh)
		c.Check(a.Stop(), gc.IsNil)
	}()

	// Test that unrestricted API logins are possible (i.e. no
	// "upgrade mode" in force)
	checkLoginToAPIAsUser(c, agentConf, FullAPIExposed)
	c.Assert(attempts, gc.Equals, 0) // There should have been no attempt to upgrade.

	// Even though no upgrade was done upgradedToVersion should have been updated.
	c.Assert(a.CurrentConfig().UpgradedToVersion(), gc.Equals, version.Current.Number)
}

func (s *UpgradeSuite) TestDowngradeOnMasterWhenOtherStateServerDoesntStartUpgrade(c *gc.C) {
	// This test checks that the master triggers a downgrade if one of
	// the other state server fails to signal it is ready for upgrade.
	//
	// This test is functional, ensuring that the upgrader worker
	// terminates the machine agent with the UpgradeReadyError which
	// makes the downgrade happen.

	// Speed up the watcher frequency to make the test much faster.
	s.PatchValue(&watcher.Period, 200*time.Millisecond)

	// Provide (fake) tools so that the upgrader has something to downgrade to.
	envtesting.AssertUploadFakeToolsVersions(c, s.Environ.Storage(), s.oldVersion)

	// Only the first machine is going to be ready for upgrade.
	machineIdA, machineIdB, _ := s.createUpgradingStateServers(c)

	// One of the other state servers is ready for upgrade (but machine C doesn't).
	info, err := s.State.EnsureUpgradeInfo(machineIdB, s.oldVersion.Number, version.Current.Number)
	c.Assert(err, gc.IsNil)

	agent := s.newAgentFromMachineId(c, machineIdA)
	defer agent.Stop()

	s.machineIsMaster = true

	var agentErr error
	agentDone := make(chan bool)
	go func() {
		agentErr = agent.Run(nil)
		close(agentDone)
	}()

	select {
	case <-agentDone:
		upgradeReadyErr, ok := agentErr.(*upgrader.UpgradeReadyError)
		if !ok {
			c.Fatalf("didn't see UpgradeReadyError, instead got: %v", agentErr)
		}
		// Confirm that the downgrade is back to the previous version.
		c.Assert(upgradeReadyErr.OldTools, gc.Equals, version.Current)
		c.Assert(upgradeReadyErr.NewTools, gc.Equals, s.oldVersion)

	case <-time.After(coretesting.LongWait):
		c.Fatal("machine agent did not exit as expected")
	}

	// UpgradeInfo doc should now be archived.
	err = info.Refresh()
	c.Assert(err, gc.ErrorMatches, "current upgrade info not found")
}

// Run just the upgrade-steps worker with a fake machine agent and
// fake agent config.
func (s *UpgradeSuite) runUpgradeWorker(c *gc.C, job params.MachineJob) (
	error, *fakeConfigSetter, *fakeUpgradingMachineAgent, *upgradeWorkerContext,
) {
	config := s.makeFakeConfig()
	agent := NewFakeUpgradingMachineAgent(config)
	err, context := s.runUpgradeWorkerUsingAgent(c, job, agent)
	return err, config, agent, context
}

// Run just the upgrade-steps worker with the fake machine agent
// provided.
func (s *UpgradeSuite) runUpgradeWorkerUsingAgent(
	c *gc.C,
	job params.MachineJob,
	agent *fakeUpgradingMachineAgent,
) (error, *upgradeWorkerContext) {
	context := NewUpgradeWorkerContext()
	worker := context.Worker(agent, nil, []params.MachineJob{job})
	s.setInstantRetryStrategy()
	return worker.Wait(), context
}

func (s *UpgradeSuite) makeFakeConfig() *fakeConfigSetter {
	return NewFakeConfigSetter(names.NewMachineTag("0"), s.oldVersion.Number)
}

// Create 3 configured state servers that appear to be running tools
// with version s.oldVersion and return their ids.
func (s *UpgradeSuite) createUpgradingStateServers(c *gc.C) (machineIdA, machineIdB, machineIdC string) {
	machine0, _, _ := s.primeAgent(c, s.oldVersion, state.JobManageEnviron)
	machineIdA = machine0.Id()

	changes, err := s.State.EnsureAvailability(3, constraints.Value{}, "quantal", nil)
	c.Assert(err, gc.IsNil)
	c.Assert(len(changes.Added), gc.Equals, 2)
	machineIdB = changes.Added[0]
	s.configureMachine(c, machineIdB, s.oldVersion)
	machineIdC = changes.Added[1]
	s.configureMachine(c, machineIdC, s.oldVersion)

	return
}

func (s *UpgradeSuite) newAgentFromMachineId(c *gc.C, machineId string) *MachineAgent {
	machine, err := s.State.Machine(machineId)
	c.Assert(err, gc.IsNil)
	return s.newAgent(c, machine)
}

// Return a version the same as the current software version, but with
// the build number bumped.
func makeBumpedCurrentVersion() version.Binary {
	v := version.Current
	v.Build++
	return v
}

func waitForUpgradeToStart(upgradeCh chan bool) bool {
	select {
	case <-upgradeCh:
		return true
	case <-time.After(coretesting.LongWait):
		return false
	}
}

const maxUpgradeRetries = 3

func (s *UpgradeSuite) setInstantRetryStrategy() {
	s.PatchValue(&getUpgradeRetryStrategy, func() utils.AttemptStrategy {
		return utils.AttemptStrategy{
			Delay: 0,
			Min:   maxUpgradeRetries,
		}
	})
}

func (s *UpgradeSuite) makeExpectedStatusCalls(failCount int) []MachineStatusCall {
	calls := []MachineStatusCall{{
		params.StatusStarted,
		fmt.Sprintf("upgrading to %s", version.Current.Number),
	}}
	for i := 0; i < calcNumRetries(failCount); i++ {
		calls = append(calls, MachineStatusCall{
			params.StatusError,
			fmt.Sprintf("upgrade to %s failed (will retry): boom", version.Current.Number),
		})
	}
	if failCount >= maxUpgradeRetries {
		calls = append(calls, MachineStatusCall{
			params.StatusError,
			fmt.Sprintf("upgrade to %s failed (giving up): boom", version.Current.Number),
		})
	} else {
		calls = append(calls, MachineStatusCall{params.StatusStarted, ""})
	}
	return calls
}

func (s *UpgradeSuite) makeExpectedUpgradeLogs(failCount int, target string) []jc.SimpleMessage {
	outLogs := []jc.SimpleMessage{}

	if target == "databaseMaster" || target == "stateServer" {
		outLogs = append(outLogs, jc.SimpleMessage{
			loggo.INFO, "waiting for other state servers to be ready for upgrade",
		})
	}

	outLogs = append(outLogs, jc.SimpleMessage{
		loggo.INFO, fmt.Sprintf(
			`starting upgrade from %s to %s for %s "machine-0"`,
			s.oldVersion.Number, version.Current.Number, target),
	})

	failMessage := fmt.Sprintf(
		`upgrade from %s to %s for %s "machine-0" failed \(%%s\): boom`,
		s.oldVersion.Number, version.Current.Number, target)

	for i := 0; i < calcNumRetries(failCount); i++ {
		outLogs = append(outLogs, jc.SimpleMessage{loggo.ERROR, fmt.Sprintf(failMessage, "will retry")})
	}
	if failCount >= maxUpgradeRetries {
		outLogs = append(outLogs, jc.SimpleMessage{loggo.ERROR, fmt.Sprintf(failMessage, "giving up")})
		outLogs = append(outLogs, jc.SimpleMessage{loggo.ERROR,
			fmt.Sprintf(`upgrade to %s failed.`, version.Current.Number)})
	} else {
		outLogs = append(outLogs, jc.SimpleMessage{loggo.INFO,
			fmt.Sprintf(`upgrade to %s completed successfully.`, version.Current.Number)})
	}
	return outLogs
}

func calcNumRetries(failCount int) int {
	n := failCount
	if failCount >= maxUpgradeRetries {
		n--
	}
	return n
}

func (s *UpgradeSuite) assertUpgradeSteps(c *gc.C, job state.MachineJob) {
	agent, stopFunc := s.createAgentAndStartUpgrade(c, job)
	defer stopFunc()
	waitForUpgradeToFinish(c, agent.CurrentConfig())
}

func (s *UpgradeSuite) keyFile() string {
	return filepath.Join(s.DataDir(), "system-identity")
}

func (s *UpgradeSuite) assertCommonUpgrades(c *gc.C) {
	// rsyslog-gnutls should have been installed.
	c.Assert(s.aptCmds, gc.HasLen, 1)
	args := s.aptCmds[0].Args
	c.Assert(len(args), jc.GreaterThan, 1)
	c.Assert(args[0], gc.Equals, "apt-get")
	c.Assert(args[len(args)-1], gc.Equals, "rsyslog-gnutls")
}

func (s *UpgradeSuite) assertStateServerUpgrades(c *gc.C) {
	s.assertCommonUpgrades(c)
	// System SSH key
	c.Assert(s.keyFile(), jc.IsNonEmptyFile)
	// Syslog port should have been updated
	cfg, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	c.Assert(cfg.SyslogPort(), gc.Equals, config.DefaultSyslogPort)
	// Deprecated attributes should have been deleted - just test a couple.
	allAttrs := cfg.AllAttrs()
	_, ok := allAttrs["public-bucket"]
	c.Assert(ok, jc.IsFalse)
	_, ok = allAttrs["public-bucket-region"]
	c.Assert(ok, jc.IsFalse)
}

func (s *UpgradeSuite) assertHostUpgrades(c *gc.C) {
	s.assertCommonUpgrades(c)
	// Lock directory
	lockdir := filepath.Join(s.DataDir(), "locks")
	c.Assert(lockdir, jc.IsDirectory)
	// SSH key file should not be generated for hosts.
	_, err := os.Stat(s.keyFile())
	c.Assert(err, jc.Satisfies, os.IsNotExist)
	// Syslog port should not have been updated
	cfg, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	c.Assert(cfg.SyslogPort(), gc.Not(gc.Equals), config.DefaultSyslogPort)
	// Add other checks as needed...
}

func (s *UpgradeSuite) createAgentAndStartUpgrade(c *gc.C, job state.MachineJob) (*MachineAgent, func()) {
	machine, _, _ := s.primeAgent(c, s.oldVersion, job)
	a := s.newAgent(c, machine)
	go func() { c.Check(a.Run(nil), gc.IsNil) }()
	return a, func() { c.Check(a.Stop(), gc.IsNil) }
}

func (s *UpgradeSuite) assertEnvironAgentVersion(c *gc.C, expected version.Number) {
	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	agentVersion, ok := envConfig.AgentVersion()
	c.Assert(ok, jc.IsTrue)
	c.Assert(agentVersion, gc.Equals, expected)
}

func waitForUpgradeToFinish(c *gc.C, conf agent.Config) {
	success := false
	for attempt := coretesting.LongAttempt.Start(); attempt.Next(); {
		diskConf := readConfigFromDisk(c, conf.DataDir(), conf.Tag())
		success = diskConf.UpgradedToVersion() == version.Current.Number
		if success {
			break
		}
	}
	c.Assert(success, jc.IsTrue)
}

func readConfigFromDisk(c *gc.C, dir string, tag names.Tag) agent.Config {
	conf, err := agent.ReadConfig(agent.ConfigPath(dir, tag))
	c.Assert(err, gc.IsNil)
	return conf
}

func checkLoginToAPIAsUser(c *gc.C, conf agent.Config, expectFullApi exposedAPI) {
	info := conf.APIInfo()
	info.Tag = names.NewUserTag("admin")
	info.Password = "dummy-secret"
	info.Nonce = ""

	apiState, err := api.Open(info, upgradeTestDialOpts)
	c.Assert(err, gc.IsNil)
	defer apiState.Close()

	// this call should always work
	var result api.Status
	err = apiState.APICall("Client", 0, "", "FullStatus", nil, &result)
	c.Assert(err, gc.IsNil)

	// this call should only work if API is not restricted
	err = apiState.APICall("Client", 0, "", "DestroyEnvironment", nil, nil)
	if expectFullApi {
		c.Assert(err, gc.IsNil)
	} else {
		c.Assert(err, gc.ErrorMatches, "upgrade in progress .+")
	}
}

func canLoginToAPIAsMachine(c *gc.C, fromConf, toConf agent.Config) bool {
	info := fromConf.APIInfo()
	info.Addrs = toConf.APIInfo().Addrs
	apiState, err := api.Open(info, upgradeTestDialOpts)
	if apiState != nil {
		apiState.Close()
	}
	return apiState != nil && err == nil
}

var upgradeTestDialOpts = api.DialOpts{
	DialAddressInterval: 50 * time.Millisecond,
	Timeout:             1 * time.Minute,
	RetryDelay:          250 * time.Millisecond,
}

func assertUpgradeComplete(c *gc.C, context *upgradeWorkerContext) {
	select {
	case <-context.UpgradeComplete:
	default:
		c.Error("UpgradeComplete channel is open but shouldn't be")
	}
}

func assertUpgradeNotComplete(c *gc.C, context *upgradeWorkerContext) {
	select {
	case <-context.UpgradeComplete:
		c.Error("UpgradeComplete channel is closed but shouldn't be")
	default:
	}
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

// NewFakeUpgradingMachineAgent returns a fakeUpgradingMachineAgent which implements
// the upgradingMachineAgent interface. This provides enough
// MachineAgent functionality to support upgrades.
func NewFakeUpgradingMachineAgent(confSetter agent.ConfigSetter) *fakeUpgradingMachineAgent {
	return &fakeUpgradingMachineAgent{
		config:  confSetter,
		DyingCh: make(chan struct{}),
	}
}

type fakeUpgradingMachineAgent struct {
	config             agent.ConfigSetter
	DyingCh            chan struct{}
	MachineStatusCalls []MachineStatusCall
}

type MachineStatusCall struct {
	Status params.Status
	Info   string
}

func (a *fakeUpgradingMachineAgent) setMachineStatus(_ *api.State, status params.Status, info string) error {
	// Record setMachineStatus calls for later inspection.
	a.MachineStatusCalls = append(a.MachineStatusCalls, MachineStatusCall{status, info})
	return nil
}

func (a *fakeUpgradingMachineAgent) ensureMongoServer(agent.Config) error {
	return nil
}

func (a *fakeUpgradingMachineAgent) CurrentConfig() agent.Config {
	return a.config
}

func (a *fakeUpgradingMachineAgent) ChangeConfig(mutate AgentConfigMutator) error {
	return mutate(a.config)
}

func (a *fakeUpgradingMachineAgent) Dying() <-chan struct{} {
	return a.DyingCh
}
