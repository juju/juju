// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/apt"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/api"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
	"github.com/juju/juju/version"
)

type UpgradeSuite struct {
	commonMachineSuite

	aptCmds          []*exec.Cmd
	machine0         *state.Machine
	machine0Config   agent.Config
	upgradeToVersion version.Binary
}

var _ = gc.Suite(&UpgradeSuite{})

type exposedAPI bool

var (
	FullAPIExposed       exposedAPI = true
	RestrictedAPIExposed exposedAPI = false
)

func fakeRestart() error { return nil }

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

	// As Juju versions increase, update the version to which we are upgrading.
	s.upgradeToVersion = version.Current
	s.upgradeToVersion.Number.Minor++
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
	// Override the main upgrade entry point so that the test can
	// control when upgrades start and finish.
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

func waitForUpgradeToStart(upgradeCh chan bool) bool {
	select {
	case <-upgradeCh:
		return true
	case <-time.After(30 * time.Second):
		return false
	}
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
	s.agentSuite.PatchValue(&version.Current, s.upgradeToVersion)
	err := s.State.SetEnvironAgentVersion(s.upgradeToVersion.Number)
	c.Assert(err, gc.IsNil)

	oldVersion := s.upgradeToVersion
	oldVersion.Major = 1
	oldVersion.Minor = 16
	s.machine0, s.machine0Config, _ = s.primeAgent(c, oldVersion, job)

	a := s.newAgent(c, s.machine0)
	go func() { c.Check(a.Run(nil), gc.IsNil) }()
	return func() { c.Check(a.Stop(), gc.IsNil) }
}

func (s *UpgradeSuite) waitForUpgradeToFinish(c *gc.C) {
	success := false
	for attempt := coretesting.LongAttempt.Start(); attempt.Next(); {
		conf, err := agent.ReadConfig(agent.ConfigPath(
			s.machine0Config.DataDir(),
			s.machine0.Tag(),
		))
		c.Assert(err, gc.IsNil)
		success = conf.UpgradedToVersion() == s.upgradeToVersion.Number
		if success {
			break
		}
	}
	c.Assert(success, jc.IsTrue)
}

func (s *UpgradeSuite) checkLoginToAPIAsUser(c *gc.C, expectFullApi exposedAPI) {
	info := s.machine0Config.APIInfo()
	defaultInfo := s.APIInfo(c)
	info.Tag = defaultInfo.Tag
	info.Password = defaultInfo.Password
	info.Nonce = ""

	apiState, err := api.Open(info, upgradeTestDialOpts)
	c.Assert(err, gc.IsNil)
	defer apiState.Close()

	// this call should always work
	var result api.Status
	err = apiState.Call("Client", "", "FullStatus", nil, &result)
	c.Assert(err, gc.IsNil)

	// this call should only work if API is not restricted
	err = apiState.Call("Client", "", "DestroyEnvironment", nil, nil)
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

var upgradeTestDialOpts = api.DialOpts{
	DialAddressInterval: 50 * time.Millisecond,
	Timeout:             1 * time.Minute,
	RetryDelay:          250 * time.Millisecond,
}
