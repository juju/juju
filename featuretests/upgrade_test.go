// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// These tests check aspects of upgrade behaviour of the machine agent
// as a whole.

package featuretests

import (
	"strings"
	"time"

	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	pacman "github.com/juju/packaging/manager"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v2"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	agentcmd "github.com/juju/juju/cmd/jujud/agent"
	"github.com/juju/juju/cmd/jujud/agent/addons"
	"github.com/juju/juju/cmd/jujud/agent/agentconf"
	"github.com/juju/juju/cmd/jujud/agent/agenttest"
	agenterrors "github.com/juju/juju/cmd/jujud/agent/errors"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs/context"
	envtesting "github.com/juju/juju/environs/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"

	"github.com/juju/juju/state/watcher"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/upgrades"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/juju/worker/logsender"
	"github.com/juju/juju/worker/upgradesteps"
)

const (
	FullAPIExposed       = true
	RestrictedAPIExposed = false
)

// TODO(katco): 2016-08-09: lp:1611427
var ShortAttempt = &utils.AttemptStrategy{
	Total: time.Second * 10,
	Delay: time.Millisecond * 200,
}

type upgradeSuite struct {
	agenttest.AgentSuite
	oldVersion version.Binary
}

func (s *upgradeSuite) SetUpSuite(c *gc.C) {
	s.AgentSuite.SetUpSuite(c)
	// Speed up the watcher frequency to make the test much faster.
	s.PatchValue(&watcher.Period, 200*time.Millisecond)

	agenttest.InstallFakeEnsureMongo(s)
	s.PatchValue(&agentcmd.ProductionMongoWriteConcern, false)
}

func (s *upgradeSuite) SetUpTest(c *gc.C) {
	s.AgentSuite.SetUpTest(c)

	s.oldVersion = coretesting.CurrentVersion(c)
	s.oldVersion.Major--

	// Don't wait so long in tests.
	s.PatchValue(&upgradesteps.UpgradeStartTimeoutMaster, time.Millisecond*50)
	s.PatchValue(&upgradesteps.UpgradeStartTimeoutSecondary, time.Millisecond*60)

	// Ensure we don't fail disk space check.
	s.PatchValue(&upgrades.MinDiskSpaceMib, uint64(0))

	// Consume apt-get commands that get run before upgrades.
	aptCmds := s.AgentSuite.HookCommandOutput(&pacman.CommandOutput, nil, nil)
	go func() {
		for range aptCmds {
		}
	}()
}

func (s *upgradeSuite) TestLoginsDuringUpgrade(c *gc.C) {
	coretesting.SkipIfWindowsBug(c, "lp:1446885")

	// Create machine agent to upgrade
	machine, machine0Conf := s.makeStateAgentVersion(c, s.oldVersion)

	// Set up a second machine to log in as. API logins are tested
	// manually so there's no need to actually start this machine.
	machine1, password := s.Factory.MakeMachineReturningPassword(c, &factory.MachineParams{
		Nonce: agent.BootstrapNonce,
	})
	machine1Conf, _ := s.PrimeAgent(c, machine1.Tag(), password)

	// Mock out upgrade logic, using a channel so that the test knows
	// when upgrades have started and can control when upgrades
	// should finish.
	upgradeCh := make(chan bool)
	upgradeChClosed := false
	abort := make(chan bool)
	fakePerformUpgrade := func(version.Number, []upgrades.Target, upgrades.Context) error {
		// Signal that upgrade has started.
		select {
		case upgradeCh <- true:
		case <-abort:
			return nil
		}

		// Wait for signal that upgrades should finish.
		select {
		case <-upgradeCh:
		case <-abort:
			return nil
		}
		return nil
	}
	s.PatchValue(&upgradesteps.PerformUpgrade, fakePerformUpgrade)

	a := s.newAgent(c, machine)
	ctx := cmdtesting.Context(c)
	go func() { c.Check(a.Run(ctx), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()

	c.Assert(waitForUpgradeToStart(upgradeCh), jc.IsTrue)

	// The test will hang if there's a failure in the assertions below
	// and upgradeCh isn't closed.
	defer func() {
		if !upgradeChClosed {
			close(upgradeCh)
		}
	}()

	// Only user and local logins are allowed during upgrade. Users get a restricted API.
	s.checkLoginToAPIAsUser(c, machine0Conf, RestrictedAPIExposed)
	c.Assert(canLoginToAPIAsMachine(c, machine0Conf, machine0Conf), jc.IsTrue)
	c.Assert(canLoginToAPIAsMachine(c, machine1Conf, machine0Conf), jc.IsFalse)

	close(upgradeCh) // Allow upgrade to complete
	upgradeChClosed = true

	waitForUpgradeToFinish(c, machine0Conf)

	// All logins are allowed after upgrade
	s.checkLoginToAPIAsUser(c, machine0Conf, FullAPIExposed)
	c.Assert(canLoginToAPIAsMachine(c, machine0Conf, machine0Conf), jc.IsTrue)
	c.Assert(canLoginToAPIAsMachine(c, machine1Conf, machine0Conf), jc.IsTrue)
}

func (s *upgradeSuite) TestDowngradeOnMasterWhenOtherControllerDoesntStartUpgrade(c *gc.C) {
	coretesting.SkipIfWindowsBug(c, "lp:1446885")

	// This test checks that the master triggers a downgrade if one of
	// the other controller fails to signal it is ready for upgrade.
	//
	// This test is functional, ensuring that the upgrader worker
	// terminates the machine agent with the UpgradeReadyError which
	// makes the downgrade happen.

	// Provide (fake) tools so that the upgrader has something to downgrade to.
	envtesting.AssertUploadFakeToolsVersions(
		c, s.DefaultToolsStorage, s.Environ.Config().AgentStream(), s.Environ.Config().AgentStream(), s.oldVersion)

	// Create 3 controllers
	machineA, _ := s.makeStateAgentVersion(c, s.oldVersion)
	// We're not going to start the agents for machines A or B - we
	// need to make sure the API port is still set to the one picked
	// for this machine after we create the other machines.
	apiPort := s.ControllerConfig.APIPort()

	changes, err := s.State.EnableHA(3, constraints.Value{}, "quantal", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(changes.Added), gc.Equals, 2)
	machineB, _, _ := s.configureMachine(c, changes.Added[0], s.oldVersion)
	s.configureMachine(c, changes.Added[1], s.oldVersion)

	s.SetControllerConfigAPIPort(c, apiPort)

	// One of the other controllers is ready for upgrade (but machine C isn't).
	info, err := s.State.EnsureUpgradeInfo(machineB.Id(), s.oldVersion.Number, jujuversion.Current)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the agent will think it's the master controller.
	fakeIsMachineMaster := func(*state.StatePool, string) (bool, error) {
		return true, nil
	}
	s.PatchValue(&upgradesteps.IsMachineMaster, fakeIsMachineMaster)

	// Start the agent
	agent := s.newAgent(c, machineA)
	defer agent.Stop()
	agentDone := make(chan error)
	ctx := cmdtesting.Context(c)
	go func() {
		agentDone <- agent.Run(ctx)
	}()

	select {
	case agentErr := <-agentDone:
		upgradeReadyErr, ok := agentErr.(*agenterrors.UpgradeReadyError)
		if !ok {
			c.Fatalf("didn't see UpgradeReadyError, instead got: %v", agentErr)
		}
		// Confirm that the downgrade is back to the previous version.
		current := coretesting.CurrentVersion(c)
		c.Assert(upgradeReadyErr.OldTools, gc.Equals, current)
		c.Assert(upgradeReadyErr.NewTools, gc.Equals, s.oldVersion)

	case <-time.After(coretesting.LongWait):
		c.Fatal("machine agent did not exit as expected")
	}

	// UpgradeInfo doc should now be archived.
	err = info.Refresh()
	c.Assert(err, gc.ErrorMatches, "current upgrade info not found")
}

// TODO(mjs) - the following should maybe be part of AgentSuite
func (s *upgradeSuite) newAgent(c *gc.C, m *state.Machine) *agentcmd.MachineAgent {
	agentConf := agentconf.NewAgentConf(s.DataDir())
	agentConf.ReadConfig(m.Tag().String())
	logger := logsender.NewBufferedLogWriter(1024)
	s.AddCleanup(func(*gc.C) { logger.Close() })
	machineAgentFactory := agentcmd.MachineAgentFactoryFn(
		agentConf,
		logger,
		addons.DefaultIntrospectionSocketName,
		noPreUpgradeSteps,
		c.MkDir(),
	)
	a, err := machineAgentFactory(m.Tag(), false)
	c.Assert(err, jc.ErrorIsNil)
	return a
}

func noPreUpgradeSteps(_ *state.StatePool, _ agent.Config, isController, isMaster, isCaas bool) error {
	return nil
}

// TODO(mjs) - the following should maybe be part of AgentSuite
func (s *upgradeSuite) makeStateAgentVersion(c *gc.C, vers version.Binary) (*state.Machine, agent.ConfigSetterWriter) {
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Jobs:  []state.MachineJob{state.JobManageModel},
		Nonce: agent.BootstrapNonce,
	})
	_, config, _ := s.configureMachine(c, machine.Id(), vers)
	return machine, config
}

const initialMachinePassword = "machine-password-1234567890"

// TODO(mjs) - the following should maybe be part of AgentSuite
func (s *upgradeSuite) configureMachine(c *gc.C, machineId string, vers version.Binary) (
	machine *state.Machine, agentConfig agent.ConfigSetterWriter, tools *tools.Tools,
) {
	m, err := s.State.Machine(machineId)
	c.Assert(err, jc.ErrorIsNil)

	// Provision the machine if it isn't already
	if _, err := m.InstanceId(); err != nil {
		inst, md := jujutesting.AssertStartInstance(c, s.Environ, context.NewCloudCallContext(), s.ControllerConfig.ControllerUUID(), machineId)
		c.Assert(m.SetProvisioned(inst.Id(), "", agent.BootstrapNonce, md), jc.ErrorIsNil)
	}

	// Set up the new machine.
	err = m.SetAgentVersion(vers)
	c.Assert(err, jc.ErrorIsNil)
	err = m.SetPassword(initialMachinePassword)
	c.Assert(err, jc.ErrorIsNil)
	tag := m.Tag()
	if m.IsManager() {
		err = m.SetMongoPassword(initialMachinePassword)
		c.Assert(err, jc.ErrorIsNil)
		agentConfig, tools = s.PrimeStateAgentVersion(c, tag, initialMachinePassword, vers)
		info, ok := agentConfig.StateServingInfo()
		c.Assert(ok, jc.IsTrue)
		err = s.State.SetStateServingInfo(info)
		c.Assert(err, jc.ErrorIsNil)
	} else {
		agentConfig, tools = s.PrimeAgentVersion(c, tag, initialMachinePassword, vers)
	}
	err = agentConfig.Write()
	c.Assert(err, jc.ErrorIsNil)
	return m, agentConfig, tools
}

func canLoginToAPIAsMachine(c *gc.C, fromConf, toConf agent.Config) bool {
	fromInfo, ok := fromConf.APIInfo()
	c.Assert(ok, jc.IsTrue)
	toInfo, ok := toConf.APIInfo()
	c.Assert(ok, jc.IsTrue)
	fromInfo.Addrs = toInfo.Addrs
	var err error
	var apiState api.Connection
	for a := ShortAttempt.Start(); a.Next(); {
		apiState, err = api.Open(fromInfo, upgradeTestDialOpts)
		// If space discovery is still in progress we retry.
		if err != nil && strings.Contains(err.Error(), "spaces are still being discovered") {
			if !a.HasNext() {
				return false
			}
			continue
		}
		if apiState != nil {
			apiState.Close()
		}
		break
	}
	return apiState != nil && err == nil
}

func (s *upgradeSuite) checkLoginToAPIAsUser(c *gc.C, conf agent.Config, expectFullAPI bool) {
	var err error
	// Multiple attempts may be necessary because there is a small gap
	// between the post-upgrade version being written to the agent's
	// config (as observed by waitForUpgradeToFinish) and the end of
	// "upgrade mode" (i.e. when the agent's UpgradeComplete channel
	// is closed). Without this tests that call checkLoginToAPIAsUser
	// can occasionally fail.
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		err = s.attemptRestrictedAPIAsUser(c, conf)
		switch expectFullAPI {
		case FullAPIExposed:
			if err == nil {
				return
			}
		case RestrictedAPIExposed:
			if params.IsCodeUpgradeInProgress(err) {
				return
			}
		}
	}
	c.Fatalf("timed out waiting for expected API behaviour. last error was: %v", err)
}

func (s *upgradeSuite) attemptRestrictedAPIAsUser(c *gc.C, conf agent.Config) error {
	info, ok := conf.APIInfo()
	c.Assert(ok, jc.IsTrue)
	info.Tag = s.AdminUserTag(c)
	info.Password = "dummy-secret"
	info.Nonce = ""

	apiState, err := api.Open(info, upgradeTestDialOpts)
	if err != nil {
		// If space discovery is in progress we'll get an error here
		// and need to retry.
		return err
	}
	defer apiState.Close()

	// This call should always work, but might fail if the apiserver
	// is restarting. If it fails just return the error so retries
	// can continue.
	err = apiState.APICall("Client", 1, "", "FullStatus", nil, new(params.FullStatus))
	if err != nil {
		return errors.Annotate(err, "FullStatus call")
	}

	// this call should only work if API is not restricted
	err = apiState.APICall("Client", 1, "", "WatchAll", nil, nil)
	return errors.Annotate(err, "WatchAll call")
}

var upgradeTestDialOpts = api.DialOpts{
	Timeout:             2 * time.Minute,
	RetryDelay:          250 * time.Millisecond,
	DialAddressInterval: 50 * time.Millisecond,
}

func waitForUpgradeToStart(upgradeCh chan bool) bool {
	select {
	case <-upgradeCh:
		return true
	case <-time.After(coretesting.LongWait):
		return false
	}
}

func waitForUpgradeToFinish(c *gc.C, conf agent.Config) {
	success := false
	for attempt := coretesting.LongAttempt.Start(); attempt.Next(); {
		diskConf := readConfigFromDisk(c, conf.DataDir(), conf.Tag())
		success = diskConf.UpgradedToVersion() == jujuversion.Current
		if success {
			break
		}
	}
	c.Assert(success, jc.IsTrue)
}

func readConfigFromDisk(c *gc.C, dir string, tag names.Tag) agent.Config {
	conf, err := agent.ReadConfig(agent.ConfigPath(dir, tag))
	c.Assert(err, jc.ErrorIsNil)
	return conf
}
