// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"fmt"
	"os"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/storage"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/version"
)

type provisionerSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&provisionerSuite{})

func (s *provisionerSuite) getArgs(c *gc.C) ProvisionMachineArgs {
	hostname, err := os.Hostname()
	c.Assert(err, gc.IsNil)
	return ProvisionMachineArgs{
		Host:  hostname,
		State: s.State,
	}
}

func (s *provisionerSuite) TestProvisionMachine(c *gc.C) {
	const series = "precise"
	const arch = "amd64"

	args := s.getArgs(c)
	hostname := args.Host
	args.Host = "ubuntu@" + args.Host

	envtesting.RemoveTools(c, s.Conn.Environ.Storage())
	envtesting.RemoveTools(c, s.Conn.Environ.PublicStorage().(storage.Storage))
	defer fakeSSH{
		series: series, arch: arch, skipProvisionAgent: true,
	}.install(c).Restore()
	m, err := ProvisionMachine(args)
	c.Assert(err, gc.ErrorMatches, "no tools available")
	c.Assert(m, gc.IsNil)

	cfg := s.Conn.Environ.Config()
	number, ok := cfg.AgentVersion()
	c.Assert(ok, jc.IsTrue)
	binVersion := version.Binary{number, series, arch}
	envtesting.UploadFakeToolsVersion(c, s.Conn.Environ.Storage(), binVersion)

	for i, errorCode := range []int{255, 0} {
		c.Logf("test %d: code %d", i, errorCode)
		defer fakeSSH{
			series: series,
			arch:   arch,
			provisionAgentExitCode: errorCode,
		}.install(c).Restore()
		m, err = ProvisionMachine(args)
		if errorCode != 0 {
			c.Assert(err, gc.ErrorMatches, fmt.Sprintf("exit status %d", errorCode))
			c.Assert(m, gc.IsNil)
		} else {
			c.Assert(err, gc.IsNil)
			c.Assert(m, gc.NotNil)
			// machine ID will be incremented. Even though we failed and the
			// machine is removed, the ID is not reused.
			c.Assert(m.Id(), gc.Equals, fmt.Sprint(i))
			instanceId, err := m.InstanceId()
			c.Assert(err, gc.IsNil)
			c.Assert(instanceId, gc.Equals, instance.Id("manual:"+hostname))
		}
	}

	// Attempting to provision a machine twice should fail. We effect
	// this by checking for existing juju upstart configurations.
	defer installFakeSSH(c, "", "/etc/init/jujud-machine-0.conf", 0)()
	_, err = ProvisionMachine(args)
	c.Assert(err, gc.Equals, ErrProvisioned)
	defer installFakeSSH(c, "", "/etc/init/jujud-machine-0.conf", 255)()
	_, err = ProvisionMachine(args)
	c.Assert(err, gc.ErrorMatches, "error checking if provisioned: exit status 255")
}
