// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"fmt"
	"os"
	"strings"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
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
	// Prepare a mock ssh response for the detection phase.
	const series = "precise"
	const arch = "amd64"
	detectionoutput := strings.Join([]string{
		series,
		arch,
		"MemTotal: 4096 kB",
		"processor: 0",
	}, "\n")

	args := s.getArgs(c)
	hostname := args.Host
	args.Host = "ubuntu@" + args.Host

	envtesting.RemoveTools(c, s.Conn.Environ.Storage())
	envtesting.RemoveTools(c, s.Conn.Environ.PublicStorage().(environs.Storage))
	defer sshresponse(c, detectionScript, detectionoutput, 0)()
	defer sshresponse(c, checkProvisionedScript, "", 0)()
	m, err := ProvisionMachine(args)
	c.Assert(err, gc.ErrorMatches, "no tools available")
	c.Assert(m, gc.IsNil)

	cfg := s.Conn.Environ.Config()
	number, ok := cfg.AgentVersion()
	c.Assert(ok, jc.IsTrue)
	binVersion := version.Binary{number, series, arch}
	envtesting.UploadFakeToolsVersion(c, s.Conn.Environ.Storage(), binVersion)

	for i, errorCode := range []int{255, 0} {
		defer sshresponse(c, "", "", errorCode)() // executing script
		defer sshresponse(c, detectionScript, detectionoutput, 0)()
		defer sshresponse(c, checkProvisionedScript, "", 0)()
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
	defer sshresponse(c, checkProvisionedScript, "/etc/init/jujud-machine-0.conf", 0)()
	_, err = ProvisionMachine(args)
	c.Assert(err, gc.Equals, ErrProvisioned)
	defer sshresponse(c, checkProvisionedScript, "/etc/init/jujud-machine-0.conf", 255)()
	_, err = ProvisionMachine(args)
	c.Assert(err, gc.ErrorMatches, "error checking if provisioned: exit status 255")
}
