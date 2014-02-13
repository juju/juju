// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	"io/ioutil"
	"path/filepath"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	jujutesting "launchpad.net/juju-core/juju/testing"
	syslogtesting "launchpad.net/juju-core/log/syslog/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/upgrades"
)

type rsyslogSuite struct {
	jujutesting.JujuConnSuite

	syslogPath string
	ctx        upgrades.Context
}

var _ = gc.Suite(&rsyslogSuite{})

func (s *rsyslogSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	dir := c.MkDir()
	s.syslogPath = filepath.Join(dir, "fakesyslog.conf")
	s.PatchValue(&environs.RsyslogConfPath, s.syslogPath)

	apiState, _ := s.OpenAPIAsNewMachine(c, state.JobManageState, state.JobManageEnviron)

	s.ctx = &mockContext{
		agentConfig: &mockAgentConfig{
			tag: "machine-tag", namespace: "namespace", apiAddresses: []string{"server:1234"}},
		apiState: apiState,
	}
}

func (s *rsyslogSuite) TestStateServerUpgrade(c *gc.C) {
	err := upgrades.UpgradeStateServerRsyslogConfig(s.ctx)
	c.Assert(err, gc.IsNil)

	data, err := ioutil.ReadFile(s.syslogPath)
	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Equals, syslogtesting.ExpectedAccumulateSyslogConf(c, "machine-tag", "namespace", 2345))
}

func (s *rsyslogSuite) TestStateServerUpgradeIdempotent(c *gc.C) {
	s.TestStateServerUpgrade(c)
	s.TestStateServerUpgrade(c)
}

func (s *rsyslogSuite) TestHostMachineUpgrade(c *gc.C) {
	err := upgrades.UpgradeHostMachineRsyslogConfig(s.ctx)
	c.Assert(err, gc.IsNil)

	data, err := ioutil.ReadFile(s.syslogPath)
	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Equals, syslogtesting.ExpectedForwardSyslogConf(c, "machine-tag", "namespace", 2345))
}

func (s *rsyslogSuite) TestHostServerUpgradeIdempotent(c *gc.C) {
	s.TestHostMachineUpgrade(c)
	s.TestHostMachineUpgrade(c)
}
