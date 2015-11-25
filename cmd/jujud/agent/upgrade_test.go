// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/series"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	agenttesting "github.com/juju/juju/cmd/jujud/agent/testing"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/upgrades"
	"github.com/juju/juju/version"
)

type UpgradeSuite struct {
	agenttesting.AgentSuite

	oldVersion      version.Binary
	logWriter       loggo.TestWriter
	connectionDead  bool
	machineIsMaster bool
}

var _ = gc.Suite(&UpgradeSuite{})

const fails = true
const succeeds = false

func (s *UpgradeSuite) SetUpTest(c *gc.C) {
	s.AgentSuite.SetUpTest(c)

	s.oldVersion = version.Binary{
		Number: version.Current,
		Arch:   arch.HostArch(),
		Series: series.HostSeries(),
	}
	s.oldVersion.Major = 1
	s.oldVersion.Minor = 16

	// Don't wait so long in tests.
	s.PatchValue(&upgradeStartTimeoutMaster, time.Duration(time.Millisecond*50))
	s.PatchValue(&upgradeStartTimeoutSecondary, time.Duration(time.Millisecond*60))

	// Allow tests to make the API connection appear to be dead.
	s.connectionDead = false
	s.PatchValue(&cmdutil.ConnectionIsDead, func(loggo.Logger, cmdutil.Pinger) bool {
		return s.connectionDead
	})

	var fakeOpenStateForUpgrade = func(upgradingMachineAgent, agent.Config) (*state.State, error) {
		mongoInfo := s.State.MongoConnectionInfo()
		st, err := state.Open(s.State.EnvironTag(), mongoInfo, mongo.DefaultDialOpts(), environs.NewStatePolicy())
		c.Assert(err, jc.ErrorIsNil)
		return st, nil
	}
	s.PatchValue(&openStateForUpgrade, fakeOpenStateForUpgrade)

	s.machineIsMaster = true
	fakeIsMachineMaster := func(*state.State, string) (bool, error) {
		return s.machineIsMaster, nil
	}
	s.PatchValue(&isMachineMaster, fakeIsMachineMaster)
	// Most of these tests normally finish sub-second on a fast machine.
	// If any given test hits a minute, we have almost certainly become
	// wedged, so dump the logs.
	coretesting.DumpTestLogsAfter(time.Minute, c, s)
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
	s.PatchValue(&upgradesPerformUpgrade, func(version.Number, []upgrades.Target, upgrades.Context) error {
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
	c.Assert(config.Version, gc.Equals, version.Current)

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

func (s *UpgradeSuite) TestNoUpgradeNecessary(c *gc.C) {
	attemptsP := s.countUpgradeAttempts(nil)
	s.captureLogs(c)
	s.oldVersion.Number = version.Current // nothing to do

	workerErr, config, _, context := s.runUpgradeWorker(c, multiwatcher.JobHostUnits)

	c.Check(workerErr, gc.IsNil)
	c.Check(*attemptsP, gc.Equals, 0)
	c.Check(config.Version, gc.Equals, version.Current)
	assertUpgradeComplete(c, context)
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

	workerErr, config, agent, context := s.runUpgradeWorker(c, multiwatcher.JobHostUnits)

	// The worker shouldn't return an error so that the worker and
	// agent keep running.
	c.Check(workerErr, gc.IsNil)

	c.Check(*attemptsP, gc.Equals, maxUpgradeRetries)
	c.Check(config.Version, gc.Equals, s.oldVersion.Number) // Upgrade didn't finish
	c.Assert(agent.MachineStatusCalls, jc.DeepEquals,
		s.makeExpectedStatusCalls(maxUpgradeRetries-1, fails, "boom"))
	c.Assert(s.logWriter.Log(), jc.LogMatches,
		s.makeExpectedUpgradeLogs(maxUpgradeRetries-1, "hostMachine", fails, "boom"))
	assertUpgradeNotComplete(c, context)
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
	s.PatchValue(&upgradesPerformUpgrade, fakePerformUpgrade)
	s.captureLogs(c)

	workerErr, config, agent, context := s.runUpgradeWorker(c, multiwatcher.JobHostUnits)

	c.Check(workerErr, gc.IsNil)
	c.Check(attempts, gc.Equals, 2)
	c.Check(config.Version, gc.Equals, version.Current) // Upgrade finished
	c.Assert(agent.MachineStatusCalls, jc.DeepEquals, s.makeExpectedStatusCalls(1, succeeds, "boom"))
	c.Assert(s.logWriter.Log(), jc.LogMatches, s.makeExpectedUpgradeLogs(1, "hostMachine", succeeds, "boom"))
	assertUpgradeComplete(c, context)
}

func (s *UpgradeSuite) TestOtherUpgradeRunFailure(c *gc.C) {
	// This test checks what happens something other than the upgrade
	// steps themselves fails, ensuring the something is logged and
	// the agent status is updated.

	fakePerformUpgrade := func(version.Number, []upgrades.Target, upgrades.Context) error {
		// Delete UpgradeInfo for the upgrade so that finaliseUpgrade() will fail
		s.State.ClearUpgradeInfo()
		return nil
	}
	s.PatchValue(&upgradesPerformUpgrade, fakePerformUpgrade)
	s.Factory.MakeMachine(c, &factory.MachineParams{
		Jobs: []state.MachineJob{state.JobManageEnviron},
	})
	s.captureLogs(c)

	workerErr, config, agent, context := s.runUpgradeWorker(c, multiwatcher.JobManageEnviron)

	c.Check(workerErr, gc.IsNil)
	c.Check(config.Version, gc.Equals, version.Current) // Upgrade almost finished
	failReason := `upgrade done but: cannot set upgrade status to "finishing": ` +
		`Another status change may have occurred concurrently`
	c.Assert(agent.MachineStatusCalls, jc.DeepEquals,
		s.makeExpectedStatusCalls(0, fails, failReason))
	c.Assert(s.logWriter.Log(), jc.LogMatches,
		s.makeExpectedUpgradeLogs(0, "databaseMaster", fails, failReason))
	assertUpgradeNotComplete(c, context)
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

	workerErr, config, _, context := s.runUpgradeWorker(c, multiwatcher.JobHostUnits)

	c.Check(workerErr, gc.ErrorMatches, "API connection lost during upgrade: boom")
	c.Check(*attemptsP, gc.Equals, 1)
	c.Check(config.Version, gc.Equals, s.oldVersion.Number) // Upgrade didn't finish
	assertUpgradeNotComplete(c, context)
}

func (s *UpgradeSuite) TestAbortWhenOtherStateServerDoesntStartUpgrade(c *gc.C) {
	// This test checks when a state server is upgrading and one of
	// the other state servers doesn't signal it is ready in time.

	// The master state server in this scenario is functionally tested
	// elsewhere.
	s.machineIsMaster = false

	s.createUpgradingStateServers(c)
	s.captureLogs(c)
	attemptsP := s.countUpgradeAttempts(nil)

	workerErr, config, agent, context := s.runUpgradeWorker(c, multiwatcher.JobManageEnviron)

	c.Check(workerErr, gc.IsNil)
	c.Check(*attemptsP, gc.Equals, 0)
	c.Check(config.Version, gc.Equals, s.oldVersion.Number) // Upgrade didn't happen
	assertUpgradeNotComplete(c, context)

	// The environment agent-version should still be the new version.
	// It's up to the master to trigger the rollback.
	s.assertEnvironAgentVersion(c, version.Current)

	causeMsg := " timed out after 60ms"
	c.Assert(s.logWriter.Log(), jc.LogMatches, []jc.SimpleMessage{
		{loggo.INFO, "waiting for other state servers to be ready for upgrade"},
		{loggo.ERROR, "aborted wait for other state servers: timed out after 60ms"},
		{loggo.ERROR, `upgrade from .+ to .+ for "machine-0" failed \(giving up\): ` +
			"aborted wait for other state servers:" + causeMsg},
	})
	c.Assert(agent.MachineStatusCalls, jc.DeepEquals, []MachineStatusCall{{
		params.StatusError,
		fmt.Sprintf(
			"upgrade to %s failed (giving up): aborted wait for other state servers:"+causeMsg,
			version.Current),
	}})
}

func (s *UpgradeSuite) TestWorkerAbortsIfAgentDies(c *gc.C) {
	s.machineIsMaster = false
	s.captureLogs(c)
	attemptsP := s.countUpgradeAttempts(nil)

	s.Factory.MakeMachine(c, &factory.MachineParams{
		Jobs: []state.MachineJob{state.JobManageEnviron},
	})
	config := s.makeFakeConfig()
	agent := NewFakeUpgradingMachineAgent(config)
	close(agent.DyingCh)
	workerErr, context := s.runUpgradeWorkerUsingAgent(c, agent, multiwatcher.JobManageEnviron)

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
		c.Assert(err, jc.ErrorIsNil)
		err = info.SetStatus(state.UpgradeFinishing)
		c.Assert(err, jc.ErrorIsNil)
	}
	s.checkSuccess(c, "stateServer", mungeInfo)
}

func (s *UpgradeSuite) checkSuccess(c *gc.C, target string, mungeInfo func(*state.UpgradeInfo)) *state.UpgradeInfo {
	_, machineIdB, machineIdC := s.createUpgradingStateServers(c)

	// Indicate that machine B and C are ready to upgrade
	vPrevious := s.oldVersion.Number
	vNext := version.Current
	info, err := s.State.EnsureUpgradeInfo(machineIdB, vPrevious, vNext)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.EnsureUpgradeInfo(machineIdC, vPrevious, vNext)
	c.Assert(err, jc.ErrorIsNil)

	mungeInfo(info)

	attemptsP := s.countUpgradeAttempts(nil)
	s.captureLogs(c)

	workerErr, config, agent, context := s.runUpgradeWorker(c, multiwatcher.JobManageEnviron)

	c.Check(workerErr, gc.IsNil)
	c.Check(*attemptsP, gc.Equals, 1)
	c.Check(config.Version, gc.Equals, version.Current) // Upgrade finished
	c.Assert(agent.MachineStatusCalls, jc.DeepEquals, s.makeExpectedStatusCalls(0, succeeds, ""))
	c.Assert(s.logWriter.Log(), jc.LogMatches, s.makeExpectedUpgradeLogs(0, target, succeeds, ""))
	assertUpgradeComplete(c, context)

	err = info.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.StateServersDone(), jc.DeepEquals, []string{"0"})
	return info
}

func (s *UpgradeSuite) TestJobsToTargets(c *gc.C) {
	check := func(jobs []multiwatcher.MachineJob, isMaster bool, expectedTargets ...upgrades.Target) {
		c.Assert(jobsToTargets(jobs, isMaster), jc.SameContents, expectedTargets)
	}

	check([]multiwatcher.MachineJob{multiwatcher.JobHostUnits}, false, upgrades.HostMachine)
	check([]multiwatcher.MachineJob{multiwatcher.JobManageEnviron}, false, upgrades.StateServer)
	check([]multiwatcher.MachineJob{multiwatcher.JobManageEnviron}, true,
		upgrades.StateServer, upgrades.DatabaseMaster)
	check([]multiwatcher.MachineJob{multiwatcher.JobManageEnviron, multiwatcher.JobHostUnits}, false,
		upgrades.StateServer, upgrades.HostMachine)
	check([]multiwatcher.MachineJob{multiwatcher.JobManageEnviron, multiwatcher.JobHostUnits}, true,
		upgrades.StateServer, upgrades.DatabaseMaster, upgrades.HostMachine)
}

// Run just the upgrade-steps worker with a fake machine agent and
// fake agent config.
func (s *UpgradeSuite) runUpgradeWorker(c *gc.C, jobs ...multiwatcher.MachineJob) (
	error, *fakeConfigSetter, *fakeUpgradingMachineAgent, *upgradeWorkerContext,
) {
	config := s.makeFakeConfig()
	agent := NewFakeUpgradingMachineAgent(config)
	err, context := s.runUpgradeWorkerUsingAgent(c, agent, jobs...)
	return err, config, agent, context
}

// Run just the upgrade-steps worker with the fake machine agent
// provided.
func (s *UpgradeSuite) runUpgradeWorkerUsingAgent(
	c *gc.C,
	agent *fakeUpgradingMachineAgent,
	jobs ...multiwatcher.MachineJob,
) (error, *upgradeWorkerContext) {
	s.setInstantRetryStrategy(c)
	context := NewUpgradeWorkerContext()
	worker := context.Worker(agent, nil, jobs)
	return worker.Wait(), context
}

func (s *UpgradeSuite) makeFakeConfig() *fakeConfigSetter {
	return NewFakeConfigSetter(names.NewMachineTag("0"), s.oldVersion.Number)
}

func (s *UpgradeSuite) createUpgradingStateServers(c *gc.C) (machineIdA, machineIdB, machineIdC string) {
	machine0 := s.Factory.MakeMachine(c, &factory.MachineParams{
		Jobs: []state.MachineJob{state.JobManageEnviron},
	})
	machineIdA = machine0.Id()
	s.setMachineAlive(c, machineIdA)

	changes, err := s.State.EnsureAvailability(3, constraints.Value{}, "quantal", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(changes.Added), gc.Equals, 2)

	machineIdB = changes.Added[0]
	s.setMachineProvisioned(c, machineIdB)
	s.setMachineAlive(c, machineIdB)

	machineIdC = changes.Added[1]
	s.setMachineProvisioned(c, machineIdC)
	s.setMachineAlive(c, machineIdC)

	return
}

func (s *UpgradeSuite) setMachineProvisioned(c *gc.C, id string) {
	machine, err := s.State.Machine(id)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned(instance.Id(id+"-inst"), "nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UpgradeSuite) setMachineAlive(c *gc.C, id string) {
	machine, err := s.State.Machine(id)
	c.Assert(err, jc.ErrorIsNil)
	pinger, err := machine.SetAgentPresence()
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) { pinger.Stop() })
}

// Return a version the same as the current software version, but with
// the build number bumped.
//
// The version Tag is also cleared so that upgrades.PerformUpgrade
// doesn't think it needs to run upgrade steps unnecessarily.
func makeBumpedCurrentVersion() version.Binary {
	v := version.Binary{
		Number: version.Current,
		Arch:   arch.HostArch(),
		Series: series.HostSeries(),
	}
	v.Build++
	v.Tag = ""
	return v
}

const maxUpgradeRetries = 3

func (s *UpgradeSuite) setInstantRetryStrategy(c *gc.C) {
	s.PatchValue(&getUpgradeRetryStrategy, func() utils.AttemptStrategy {
		c.Logf("setting instant retry strategy for upgrade: retries=%d", maxUpgradeRetries)
		return utils.AttemptStrategy{
			Delay: 0,
			Min:   maxUpgradeRetries,
		}
	})
}

func (s *UpgradeSuite) makeExpectedStatusCalls(retryCount int, expectFail bool, failReason string) []MachineStatusCall {
	calls := []MachineStatusCall{{
		params.StatusStarted,
		fmt.Sprintf("upgrading to %s", version.Current),
	}}
	for i := 0; i < retryCount; i++ {
		calls = append(calls, MachineStatusCall{
			params.StatusError,
			fmt.Sprintf("upgrade to %s failed (will retry): %s", version.Current, failReason),
		})
	}
	if expectFail {
		calls = append(calls, MachineStatusCall{
			params.StatusError,
			fmt.Sprintf("upgrade to %s failed (giving up): %s", version.Current, failReason),
		})
	} else {
		calls = append(calls, MachineStatusCall{params.StatusStarted, ""})
	}
	return calls
}

func (s *UpgradeSuite) makeExpectedUpgradeLogs(retryCount int, target string, expectFail bool, failReason string) []jc.SimpleMessage {
	outLogs := []jc.SimpleMessage{}

	if target == "databaseMaster" || target == "stateServer" {
		outLogs = append(outLogs, jc.SimpleMessage{
			loggo.INFO, "waiting for other state servers to be ready for upgrade",
		})
		var waitMsg string
		switch target {
		case "databaseMaster":
			waitMsg = "all state servers are ready to run upgrade steps"
		case "stateServer":
			waitMsg = "the master has completed its upgrade steps"
		}
		outLogs = append(outLogs, jc.SimpleMessage{loggo.INFO, "finished waiting - " + waitMsg})
	}

	outLogs = append(outLogs, jc.SimpleMessage{
		loggo.INFO, fmt.Sprintf(
			`starting upgrade from %s to %s for "machine-0"`,
			s.oldVersion.Number, version.Current),
	})

	failMessage := fmt.Sprintf(
		`upgrade from %s to %s for "machine-0" failed \(%%s\): %s`,
		s.oldVersion.Number, version.Current, failReason)

	for i := 0; i < retryCount; i++ {
		outLogs = append(outLogs, jc.SimpleMessage{loggo.ERROR, fmt.Sprintf(failMessage, "will retry")})
	}
	if expectFail {
		outLogs = append(outLogs, jc.SimpleMessage{loggo.ERROR, fmt.Sprintf(failMessage, "giving up")})
	} else {
		outLogs = append(outLogs, jc.SimpleMessage{loggo.INFO,
			fmt.Sprintf(`upgrade to %s completed successfully.`, version.Current)})
	}
	return outLogs
}

func (s *UpgradeSuite) assertEnvironAgentVersion(c *gc.C, expected version.Number) {
	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	agentVersion, ok := envConfig.AgentVersion()
	c.Assert(ok, jc.IsTrue)
	c.Assert(agentVersion, gc.Equals, expected)
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

func (a *fakeUpgradingMachineAgent) setMachineStatus(_ api.Connection, status params.Status, info string) error {
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

func (a *fakeUpgradingMachineAgent) ChangeConfig(mutate agent.ConfigMutator) error {
	return mutate(a.config)
}

func (a *fakeUpgradingMachineAgent) Dying() <-chan struct{} {
	return a.DyingCh
}
