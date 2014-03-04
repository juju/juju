// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"os"
	"os/exec"
	"path/filepath"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/version"
)

type UpgradeSuite struct {
	commonMachineSuite

	aptCmds          []*exec.Cmd
	machine          *state.Machine
	upgradeToVersion version.Binary
}

var _ = gc.Suite(&UpgradeSuite{})

func fakeRestart() error { return nil }

func (s *UpgradeSuite) SetUpTest(c *gc.C) {
	s.commonMachineSuite.SetUpTest(c)

	// Capture all apt commands.
	s.aptCmds = nil
	aptCmds := s.HookCommandOutput(&utils.AptCommandOutput, nil, nil)
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

func (s *UpgradeSuite) assertUpgradeSteps(c *gc.C, job state.MachineJob) {
	s.PatchValue(&version.Current, s.upgradeToVersion)
	err := s.State.SetEnvironAgentVersion(s.upgradeToVersion.Number)
	c.Assert(err, gc.IsNil)

	oldVersion := s.upgradeToVersion
	oldVersion.Major = 1
	oldVersion.Minor = 16
	var oldConfig agent.Config
	s.machine, oldConfig, _ = s.primeAgent(c, oldVersion, job)

	a := s.newAgent(c, s.machine)
	go func() { c.Check(a.Run(nil), gc.IsNil) }()
	defer func() { c.Check(a.Stop(), gc.IsNil) }()

	// Wait for upgrade steps to run.
	success := false
	for attempt := coretesting.LongAttempt.Start(); attempt.Next(); {
		conf, err := agent.ReadConf(agent.ConfigPath(oldConfig.DataDir(), s.machine.Tag()))
		c.Assert(err, gc.IsNil)
		success = conf.UpgradedToVersion() == s.upgradeToVersion.Number
		if success {
			break
		}
	}
	// Upgrade worker has completed ok.
	c.Assert(success, jc.IsTrue)
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
