// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual_test

import (
	"fmt"
	"os"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/shell"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/agent"
	coreCloudinit "github.com/juju/juju/cloudinit"
	"github.com/juju/juju/cloudinit/sshinit"
	"github.com/juju/juju/environs/cloudinit"
	"github.com/juju/juju/environs/manual"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/apiserver/client"
	"github.com/juju/juju/version"
)

type provisionerSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&provisionerSuite{})

func (s *provisionerSuite) getArgs(c *gc.C) manual.ProvisionMachineArgs {
	hostname, err := os.Hostname()
	c.Assert(err, gc.IsNil)
	client := s.APIState.Client()
	s.AddCleanup(func(*gc.C) { client.Close() })
	return manual.ProvisionMachineArgs{
		Host:   hostname,
		Client: client,
	}
}

func (s *provisionerSuite) TestProvisionMachine(c *gc.C) {
	const series = "precise"
	const arch = "amd64"

	args := s.getArgs(c)
	hostname := args.Host
	args.Host = "ubuntu@" + args.Host

	envtesting.RemoveTools(c, s.Environ.Storage())
	defer fakeSSH{
		Series:             series,
		Arch:               arch,
		InitUbuntuUser:     true,
		SkipProvisionAgent: true,
	}.install(c).Restore()
	// Attempt to provision a machine with no tools available, expect it to fail.
	machineId, err := manual.ProvisionMachine(args)
	c.Assert(err, jc.Satisfies, params.IsCodeNotFound)
	c.Assert(machineId, gc.Equals, "")

	cfg := s.Environ.Config()
	number, ok := cfg.AgentVersion()
	c.Assert(ok, jc.IsTrue)
	binVersion := version.Binary{number, series, arch}
	envtesting.AssertUploadFakeToolsVersions(c, s.Environ.Storage(), binVersion)

	for i, errorCode := range []int{255, 0} {
		c.Logf("test %d: code %d", i, errorCode)
		defer fakeSSH{
			Series:                 series,
			Arch:                   arch,
			InitUbuntuUser:         true,
			ProvisionAgentExitCode: errorCode,
		}.install(c).Restore()
		machineId, err = manual.ProvisionMachine(args)
		if errorCode != 0 {
			c.Assert(err, gc.ErrorMatches, fmt.Sprintf("subprocess encountered error code %d", errorCode))
			c.Assert(machineId, gc.Equals, "")
		} else {
			c.Assert(err, gc.IsNil)
			c.Assert(machineId, gc.Not(gc.Equals), "")
			// machine ID will be incremented. Even though we failed and the
			// machine is removed, the ID is not reused.
			c.Assert(machineId, gc.Equals, fmt.Sprint(i+1))
			m, err := s.State.Machine(machineId)
			c.Assert(err, gc.IsNil)
			instanceId, err := m.InstanceId()
			c.Assert(err, gc.IsNil)
			c.Assert(instanceId, gc.Equals, instance.Id("manual:"+hostname))
		}
	}

	// Attempting to provision a machine twice should fail. We effect
	// this by checking for existing juju upstart configurations.
	defer fakeSSH{
		Provisioned:        true,
		InitUbuntuUser:     true,
		SkipDetection:      true,
		SkipProvisionAgent: true,
	}.install(c).Restore()
	_, err = manual.ProvisionMachine(args)
	c.Assert(err, gc.Equals, manual.ErrProvisioned)
	defer fakeSSH{
		Provisioned:              true,
		CheckProvisionedExitCode: 255,
		InitUbuntuUser:           true,
		SkipDetection:            true,
		SkipProvisionAgent:       true,
	}.install(c).Restore()
	_, err = manual.ProvisionMachine(args)
	c.Assert(err, gc.ErrorMatches, "error checking if provisioned: subprocess encountered error code 255")
}

func (s *provisionerSuite) TestFinishMachineConfig(c *gc.C) {
	const series = "precise"
	const arch = "amd64"
	defer fakeSSH{
		Series:         series,
		Arch:           arch,
		InitUbuntuUser: true,
	}.install(c).Restore()
	machineId, err := manual.ProvisionMachine(s.getArgs(c))
	c.Assert(err, gc.IsNil)

	// Now check what we would've configured it with.
	mcfg, err := client.MachineConfig(s.State, machineId, agent.BootstrapNonce, "/var/lib/juju")
	c.Assert(err, gc.IsNil)
	c.Check(mcfg, gc.NotNil)
	c.Check(mcfg.APIInfo, gc.NotNil)
	c.Check(mcfg.MongoInfo, gc.NotNil)

	stateInfo, apiInfo, err := s.APIConn.Environ.StateInfo()
	c.Assert(err, gc.IsNil)
	c.Check(mcfg.APIInfo.Addrs, gc.DeepEquals, apiInfo.Addrs)
	c.Check(mcfg.MongoInfo.Addrs, gc.DeepEquals, stateInfo.Addrs)
}

func (s *provisionerSuite) TestProvisioningScript(c *gc.C) {
	const series = "precise"
	const arch = "amd64"
	defer fakeSSH{
		Series:         series,
		Arch:           arch,
		InitUbuntuUser: true,
	}.install(c).Restore()
	machineId, err := manual.ProvisionMachine(s.getArgs(c))
	c.Assert(err, gc.IsNil)

	mcfg, err := client.MachineConfig(s.State, machineId, agent.BootstrapNonce, "/var/lib/juju")
	c.Assert(err, gc.IsNil)
	script, err := manual.ProvisioningScript(mcfg)
	c.Assert(err, gc.IsNil)

	cloudcfg := coreCloudinit.New()
	err = cloudinit.ConfigureJuju(mcfg, cloudcfg)
	c.Assert(err, gc.IsNil)
	cloudcfg.SetAptUpgrade(false)
	sshinitScript, err := sshinit.ConfigureScript(cloudcfg)
	c.Assert(err, gc.IsNil)

	removeLogFile := "rm -f '/var/log/cloud-init-output.log'\n"
	expectedScript := removeLogFile + shell.DumpFileOnErrorScript("/var/log/cloud-init-output.log") + sshinitScript
	c.Assert(script, gc.Equals, expectedScript)
}
