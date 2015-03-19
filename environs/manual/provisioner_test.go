// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual_test

import (
	"fmt"
	"os"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/shell"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver/client"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloudconfig"
	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/environs/manual"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type provisionerSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&provisionerSuite{})

func (s *provisionerSuite) getArgs(c *gc.C) manual.ProvisionMachineArgs {
	hostname, err := os.Hostname()
	c.Assert(err, jc.ErrorIsNil)
	client := s.APIState.Client()
	s.AddCleanup(func(*gc.C) { client.Close() })
	return manual.ProvisionMachineArgs{
		Host:           hostname,
		Client:         client,
		UpdateBehavior: &params.UpdateBehavior{true, true},
	}
}

func (s *provisionerSuite) TestProvisionMachine(c *gc.C) {
	const series = coretesting.FakeDefaultSeries
	const arch = "amd64"
	const operatingSystem = version.Ubuntu

	args := s.getArgs(c)
	hostname := args.Host
	args.Host = "ubuntu@" + args.Host

	defaultToolsURL := envtools.DefaultBaseURL
	envtools.DefaultBaseURL = ""

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
	binVersion := version.Binary{number, series, arch, operatingSystem}
	envtesting.AssertUploadFakeToolsVersions(c, s.DefaultToolsStorage, "released", "released", binVersion)
	envtools.DefaultBaseURL = defaultToolsURL

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
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(machineId, gc.Not(gc.Equals), "")
			// machine ID will be incremented. Even though we failed and the
			// machine is removed, the ID is not reused.
			c.Assert(machineId, gc.Equals, fmt.Sprint(i+1))
			m, err := s.State.Machine(machineId)
			c.Assert(err, jc.ErrorIsNil)
			instanceId, err := m.InstanceId()
			c.Assert(err, jc.ErrorIsNil)
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

func (s *provisionerSuite) TestFinishInstancConfig(c *gc.C) {
	const series = coretesting.FakeDefaultSeries
	const arch = "amd64"
	defer fakeSSH{
		Series:         series,
		Arch:           arch,
		InitUbuntuUser: true,
	}.install(c).Restore()
	machineId, err := manual.ProvisionMachine(s.getArgs(c))
	c.Assert(err, jc.ErrorIsNil)

	// Now check what we would've configured it with.
	icfg, err := client.InstanceConfig(s.State, machineId, agent.BootstrapNonce, "/var/lib/juju")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(icfg, gc.NotNil)
	c.Check(icfg.APIInfo, gc.NotNil)
	c.Check(icfg.MongoInfo, gc.NotNil)

	stateInfo := s.MongoInfo(c)
	apiInfo := s.APIInfo(c)
	c.Check(icfg.APIInfo.Addrs, gc.DeepEquals, apiInfo.Addrs)
	c.Check(icfg.MongoInfo.Addrs, gc.DeepEquals, stateInfo.Addrs)
}

func (s *provisionerSuite) TestProvisioningScript(c *gc.C) {
	const series = coretesting.FakeDefaultSeries
	const arch = "amd64"
	defer fakeSSH{
		Series:         series,
		Arch:           arch,
		InitUbuntuUser: true,
	}.install(c).Restore()

	machineId, err := manual.ProvisionMachine(s.getArgs(c))
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.UpdateEnvironConfig(
		map[string]interface{}{
			"enable-os-upgrade": false,
		}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	icfg, err := client.InstanceConfig(s.State, machineId, agent.BootstrapNonce, "/var/lib/juju")

	c.Assert(err, jc.ErrorIsNil)
	script, err := manual.ProvisioningScript(icfg)
	c.Assert(err, jc.ErrorIsNil)

	cloudcfg, err := cloudinit.New(series)
	c.Assert(err, jc.ErrorIsNil)
	udata, err := cloudconfig.NewUserdataConfig(icfg, cloudcfg)
	c.Assert(err, jc.ErrorIsNil)
	err = udata.ConfigureJuju()
	c.Assert(err, jc.ErrorIsNil)
	cloudcfg.SetSystemUpgrade(false)
	provisioningScript, err := cloudcfg.RenderScript()
	c.Assert(err, jc.ErrorIsNil)

	removeLogFile := "rm -f '/var/log/cloud-init-output.log'\n"
	expectedScript := removeLogFile + shell.DumpFileOnErrorScript("/var/log/cloud-init-output.log") + provisioningScript
	c.Assert(script, gc.Equals, expectedScript)
}
