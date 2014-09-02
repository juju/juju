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

	aptCmds                     []*exec.Cmd
	machine0                    *state.Machine
	machine0Config              agent.Config
	oldVersion                  version.Binary
	upgradeToVersion            version.Binary
	logWriter                   loggo.TestWriter
	connectionDead              bool
	machineIsMaster             bool
	waitForOtherStateServersErr error
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

	// As Juju versions increase, update the version to which we are upgrading.
	s.upgradeToVersion = version.Current
	s.upgradeToVersion.Number.Minor++

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
	fakeIsMachineMaster := func(*state.State, names.MachineTag) (bool, error) {
		return s.machineIsMaster, nil
	}
	s.PatchValue(&isMachineMaster, fakeIsMachineMaster)

	s.waitForOtherStateServersErr = nil
	fakeWaitForOtherStateServers := func(*state.State, bool) error {
		return s.waitForOtherStateServersErr
	}
	s.PatchValue(&waitForOtherStateServers, fakeWaitForOtherStateServers)
}

func (s *UpgradeSuite) captureLogs(c *gc.C) {
	c.Assert(loggo.RegisterWriter("upgrade-tests", &s.logWriter, loggo.INFO), gc.IsNil)
	s.AddCleanup(func(*gc.C) { loggo.RemoveWriter("upgrade-tests") })
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

	attemptCount := 0
	fakePerformUpgrade := func(_ version.Number, _ upgrades.Target, _ upgrades.Context) error {
		attemptCount++
		return errors.New("boom")
	}
	s.PatchValue(&upgradesPerformUpgrade, fakePerformUpgrade)

	s.captureLogs(c)

	workerErr, config, agent, context := s.runUpgradeWorker(params.JobHostUnits)

	// The worker shouldn't return an error so that the worker and
	// agent keep running.
	c.Check(workerErr, gc.IsNil)

	c.Check(attemptCount, gc.Equals, numTestUpgradeRetries)
	c.Check(config.Version, gc.Equals, s.oldVersion.Number) // Upgrade didn't finish
	c.Assert(agent.MachineStatusCalls, jc.DeepEquals,
		s.generateExpectedStatusCalls(numTestUpgradeRetries))
	c.Assert(s.logWriter.Log(), jc.LogMatches,
		s.generateExpectedUpgradeLogs(numTestUpgradeRetries, "hostMachine"))
	assertUpgradeNotComplete(c, context)
}

func (s *UpgradeSuite) TestUpgradeStepsRetries(c *gc.C) {
	// This test checks what happens when the first upgrade attempt
	// fails but the following on succeeds. The final state should be
	// the same as a successful upgrade which worked first go.

	attemptCount := 0
	fail := true
	fakePerformUpgrade := func(_ version.Number, _ upgrades.Target, _ upgrades.Context) error {
		attemptCount++
		if fail {
			fail = false
			return errors.New("boom")
		} else {
			return nil
		}
	}
	s.PatchValue(&upgradesPerformUpgrade, fakePerformUpgrade)
	s.captureLogs(c)

	workerErr, config, agent, context := s.runUpgradeWorker(params.JobHostUnits)

	c.Check(workerErr, gc.IsNil)
	c.Check(attemptCount, gc.Equals, 2)
	c.Check(config.Version, gc.Equals, version.Current.Number) // Upgrade finished
	c.Assert(agent.MachineStatusCalls, jc.DeepEquals,
		s.generateExpectedStatusCalls(1))
	c.Assert(s.logWriter.Log(), jc.LogMatches,
		s.generateExpectedUpgradeLogs(1, "hostMachine"))
	assertUpgradeComplete(c, context)
}

func (s *UpgradeSuite) TestApiConnectionFailure(c *gc.C) {
	// This test checks what happens when an upgrade fails because the
	// connection to mongo has gone away. This will happen when the
	// mongo master changes. In this case we want the upgrade worker
	// to return immediately without further retries. The error should
	// be returned by the worker so that the agent will restart.

	attemptCount := 0
	fakePerformUpgrade := func(_ version.Number, _ upgrades.Target, _ upgrades.Context) error {
		attemptCount++
		return errors.New("boom")
	}
	s.PatchValue(&upgradesPerformUpgrade, fakePerformUpgrade)
	s.connectionDead = true // Make the connection to state appear to be dead
	s.captureLogs(c)

	workerErr, config, _, context := s.runUpgradeWorker(params.JobHostUnits)

	c.Check(workerErr, gc.ErrorMatches, "API connection lost during upgrade: boom")
	c.Check(attemptCount, gc.Equals, 1)
	c.Check(config.Version, gc.Equals, s.oldVersion.Number) // Upgrade didn't finish
	assertUpgradeNotComplete(c, context)
}

func (s *UpgradeSuite) TestAbortWhenOtherStateServerDoesntStartUpgrade(c *gc.C) {
	// This test checks when a state server is upgrading and one of
	// the other state servers doesn't signal it is ready in time.

	// The master state server in this scenario is functionally tested
	// elsewhere in this suite.
	s.machineIsMaster = false

	attemptCount := 0
	fakePerformUpgrade := func(_ version.Number, _ upgrades.Target, _ upgrades.Context) error {
		attemptCount++
		return nil
	}
	s.PatchValue(&upgradesPerformUpgrade, fakePerformUpgrade)
	s.patchInTargetVersion(c)
	s.captureLogs(c)
	s.waitForOtherStateServersErr = errors.New("boom")

	workerErr, config, agent, context := s.runUpgradeWorker(params.JobManageEnviron)

	c.Check(workerErr, gc.IsNil)
	c.Check(attemptCount, gc.Equals, 0)
	c.Check(config.Version, gc.Equals, s.oldVersion.Number) // Upgrade didn't happen
	assertUpgradeNotComplete(c, context)

	// The environment agent-version should still be the new version.
	// It's up to the master to trigger the rollback.
	s.assertEnvironAgentVersion(c, s.upgradeToVersion.Number)

	c.Assert(s.logWriter.Log(), jc.LogMatches, []jc.SimpleMessage{{
		loggo.ERROR, fmt.Sprintf(
			`other state servers failed to come up for upgrade to %s - aborting: boom`,
			version.Current),
	}})
	c.Assert(agent.MachineStatusCalls, jc.DeepEquals, []MachineStatusCall{{
		params.StatusError,
		fmt.Sprintf(
			"upgrade to %s aborted while waiting for other state servers: boom",
			version.Current),
	}})
}

func (s *UpgradeSuite) TestSuccess(c *gc.C) {
	// This test checks what happens when an upgrade works on the
	// first attempt.

	attemptCount := 0
	fakePerformUpgrade := func(_ version.Number, _ upgrades.Target, _ upgrades.Context) error {
		attemptCount++
		return nil
	}
	s.PatchValue(&upgradesPerformUpgrade, fakePerformUpgrade)
	s.captureLogs(c)

	workerErr, config, agent, context := s.runUpgradeWorker(params.JobManageEnviron)

	c.Check(workerErr, gc.IsNil)
	c.Check(attemptCount, gc.Equals, 1)
	c.Check(config.Version, gc.Equals, version.Current.Number) // Upgrade finished
	c.Assert(agent.MachineStatusCalls, jc.DeepEquals,
		s.generateExpectedStatusCalls(0))
	c.Assert(s.logWriter.Log(), jc.LogMatches,
		s.generateExpectedUpgradeLogs(0, "databaseMaster"))
	assertUpgradeComplete(c, context)
}

func (s *UpgradeSuite) TestUpgradeStepsStateServer(c *gc.C) {
	s.assertUpgradeSteps(c, state.JobManageEnviron)
	s.assertStateServerUpgrades(c)
}

func (s *UpgradeSuite) TestUpgradeStepsHostMachine(c *gc.C) {
	// We need to first start up a state server that thinks it has already been upgraded.
	ss, _, _ := s.primeAgent(c, s.upgradeToVersion, state.JobManageEnviron)
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
	fakePerformUpgrade := func(_ version.Number, _ upgrades.Target, _ upgrades.Context) error {
		upgradeCh <- true // signal that upgrade has started
		<-upgradeCh       // wait for signal that upgrades should finish
		return nil
	}
	s.PatchValue(&upgradesPerformUpgrade, fakePerformUpgrade)

	stopFunc := s.createAgentAndStartUpgrade(c, state.JobManageEnviron)
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

	// Set up a second machine to log in as.
	// API logins are tested manually so there's no need to actually
	// start this machine.
	var machine1Config agent.Config
	_, machine1Config, _ = s.primeAgent(c, version.Current, state.JobHostUnits)

	c.Assert(waitForUpgradeToStart(upgradeCh), gc.Equals, true)

	// Only user and local logins are allowed during upgrade. Users get a restricted API.
	s.checkLoginToAPIAsUser(c, RestrictedAPIExposed)
	c.Assert(s.canLoginToAPIAsMachine(c, s.machine0Config), gc.Equals, true)
	c.Assert(s.canLoginToAPIAsMachine(c, machine1Config), gc.Equals, false)

	close(upgradeCh) // Allow upgrade to complete

	s.waitForUpgradeToFinish(c)

	// All logins are allowed after upgrade
	s.checkLoginToAPIAsUser(c, FullAPIExposed)
	c.Assert(s.canLoginToAPIAsMachine(c, s.machine0Config), gc.Equals, true)
	c.Assert(s.canLoginToAPIAsMachine(c, machine1Config), gc.Equals, true)
}

func (s *UpgradeSuite) TestUpgradeSkippedIfNoUpgradeRequired(c *gc.C) {
	attemptCount := 0
	upgradeCh := make(chan bool)
	fakePerformUpgrade := func(_ version.Number, _ upgrades.Target, _ upgrades.Context) error {
		// Note: this shouldn't run.
		attemptCount++
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
	s.machine0, s.machine0Config, _ = s.primeAgent(c, initialVersion, state.JobManageEnviron)
	a := s.newAgent(c, s.machine0)
	go func() { c.Check(a.Run(nil), gc.IsNil) }()
	defer func() {
		close(upgradeCh)
		c.Check(a.Stop(), gc.IsNil)
	}()

	// Test that unrestricted API logins are possible (i.e. no
	// "upgrade mode" in force)
	s.checkLoginToAPIAsUser(c, FullAPIExposed)
	// There should have been no attempt to upgrade.
	c.Assert(attemptCount, gc.Equals, 0)

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

	s.machineIsMaster = true

	// Signal that some state servers didn't come up for upgrade. This
	// should trigger a rollback to the previous agent version.
	s.waitForOtherStateServersErr = errors.New("boom")

	// Provide (fake) tools so that the upgrader has something to downgrade to.
	envtesting.AssertUploadFakeToolsVersions(c, s.Environ.Storage(), s.oldVersion)

	agent := s.createUpgradingAgent(c, state.JobManageEnviron)
	defer agent.Stop()

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
		c.Assert(upgradeReadyErr.OldTools, gc.Equals, s.upgradeToVersion)
		c.Assert(upgradeReadyErr.NewTools, gc.Equals, s.oldVersion)

	case <-time.After(coretesting.LongWait):
		c.Fatal("machine agent did not exit as expected")
	}
}

func (s *UpgradeSuite) runUpgradeWorker(job params.MachineJob) (
	error, *fakeConfigSetter, *fakeUpgradingMachineAgent, *upgradeWorkerContext,
) {
	config := NewFakeConfigSetter(names.NewMachineTag("0"), s.oldVersion.Number)
	agent := NewFakeUpgradingMachineAgent(config)
	context := NewUpgradeWorkerContext()
	worker := context.Worker(agent, nil, []params.MachineJob{job})
	s.setInstantRetryStrategy()
	return worker.Wait(), config, agent, context
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

const numTestUpgradeRetries = 3

func (s *UpgradeSuite) setInstantRetryStrategy() {
	s.PatchValue(&getUpgradeRetryStrategy, func() utils.AttemptStrategy {
		return utils.AttemptStrategy{
			Delay: 0,
			Min:   numTestUpgradeRetries,
		}
	})
}

func (s *UpgradeSuite) generateExpectedStatusCalls(failCount int) []MachineStatusCall {
	calls := []MachineStatusCall{{
		params.StatusStarted,
		fmt.Sprintf("upgrading to %s", version.Current),
	}}
	for i := 0; i < calcNumRetries(failCount); i++ {
		calls = append(calls, MachineStatusCall{
			params.StatusError,
			fmt.Sprintf("upgrade to %s failed (will retry): boom", version.Current),
		})
	}
	if failCount >= numTestUpgradeRetries {
		calls = append(calls, MachineStatusCall{
			params.StatusError,
			fmt.Sprintf("upgrade to %s failed (giving up): boom", version.Current),
		})
	} else {
		calls = append(calls, MachineStatusCall{params.StatusStarted, ""})
	}
	return calls
}

func (s *UpgradeSuite) generateExpectedUpgradeLogs(failCount int, target string) []jc.SimpleMessage {
	outLogs := []jc.SimpleMessage{
		{loggo.INFO, fmt.Sprintf(
			`starting upgrade from %s to %s for %s "machine-0"`,
			s.oldVersion, version.Current, target)},
	}
	failMessage := fmt.Sprintf(
		`upgrade from %s to %s for %s "machine-0" failed \(%%s\): boom`,
		s.oldVersion, version.Current, target)

	for i := 0; i < calcNumRetries(failCount); i++ {
		outLogs = append(outLogs, jc.SimpleMessage{loggo.ERROR, fmt.Sprintf(failMessage, "will retry")})
	}
	if failCount >= numTestUpgradeRetries {
		outLogs = append(outLogs, jc.SimpleMessage{loggo.ERROR, fmt.Sprintf(failMessage, "giving up")})
		outLogs = append(outLogs, jc.SimpleMessage{loggo.ERROR,
			fmt.Sprintf(`upgrade to %s failed.`, version.Current)})
	} else {
		outLogs = append(outLogs, jc.SimpleMessage{loggo.INFO,
			fmt.Sprintf(`upgrade to %s completed successfully.`, version.Current)})
	}
	return outLogs
}

func calcNumRetries(failCount int) int {
	n := failCount
	if failCount >= numTestUpgradeRetries {
		n--
	}
	return n
}

func (s *UpgradeSuite) assertUpgradeSteps(c *gc.C, job state.MachineJob) {
	stopFunc := s.createAgentAndStartUpgrade(c, job)
	defer stopFunc()
	s.waitForUpgradeToFinish(c)
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

func (s *UpgradeSuite) createAgentAndStartUpgrade(c *gc.C, job state.MachineJob) func() {
	a := s.createUpgradingAgent(c, job)
	go func() { c.Check(a.Run(nil), gc.IsNil) }()
	return func() { c.Check(a.Stop(), gc.IsNil) }
}

func (s *UpgradeSuite) createUpgradingAgent(c *gc.C, job state.MachineJob) *MachineAgent {
	s.patchInTargetVersion(c)
	s.machine0, s.machine0Config, _ = s.primeAgent(c, s.oldVersion, job)
	return s.newAgent(c, s.machine0)
}

func (s *UpgradeSuite) patchInTargetVersion(c *gc.C) {
	s.PatchValue(&version.Current, s.upgradeToVersion)
	err := s.State.SetEnvironAgentVersion(s.upgradeToVersion.Number)
	c.Assert(err, gc.IsNil)
}

func (s *UpgradeSuite) waitForUpgradeToFinish(c *gc.C) {
	success := false
	for attempt := coretesting.LongAttempt.Start(); attempt.Next(); {
		conf := s.getMachine0Config(c)
		success = conf.UpgradedToVersion() == s.upgradeToVersion.Number
		if success {
			break
		}
	}
	c.Assert(success, jc.IsTrue)
}

func (s *UpgradeSuite) getMachine0Config(c *gc.C) agent.Config {
	conf, err := agent.ReadConfig(agent.ConfigPath(
		s.machine0Config.DataDir(),
		s.machine0.Tag(),
	))
	c.Assert(err, gc.IsNil)
	return conf
}

func (s *UpgradeSuite) checkLoginToAPIAsUser(c *gc.C, expectFullApi exposedAPI) {
	info := s.machine0Config.APIInfo()
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

func (s *UpgradeSuite) canLoginToAPIAsMachine(c *gc.C, config agent.Config) bool {
	// Ensure logins are always to the API server (machine-0)
	info := config.APIInfo()
	info.Addrs = s.machine0Config.APIInfo().Addrs
	apiState, err := api.Open(info, upgradeTestDialOpts)
	if apiState != nil {
		apiState.Close()
	}
	if apiState != nil && err == nil {
		return true
	}
	return false
}

func (s *UpgradeSuite) assertEnvironAgentVersion(c *gc.C, expected version.Number) {
	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	agentVersion, ok := envConfig.AgentVersion()
	c.Assert(ok, jc.IsTrue)
	c.Assert(agentVersion, gc.Equals, expected)
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
		config: confSetter,
	}
}

type fakeUpgradingMachineAgent struct {
	config             agent.ConfigSetter
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
