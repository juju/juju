// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"fmt"
	"os"
	"strings"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju/testing"
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
		Env:   s.Conn.Environ,
		State: s.State,
	}
}

func (s *provisionerSuite) TestProvisionMachine(c *gc.C) {
	// Prepare a mock ssh response for the detection phase.
	detectionoutput := strings.Join([]string{
		"edgy",
		"armv4",
		"MemTotal: 4096 kB",
		"processor: 0",
	}, "\n")

	args := s.getArgs(c)
	hostname := args.Host
	args.Host = "ubuntu@" + args.Host

	defer sshresponse(c, detectionScript, detectionoutput, 0)()
	defer sshresponse(c, checkProvisionedScript, "", 0)()
	m, err := ProvisionMachine(args)
	c.Assert(err, gc.ErrorMatches, "no matching tools available")
	c.Assert(m, gc.IsNil)

	toolsList, err := tools.FindBootstrapTools(s.Conn.Environ, constraints.Value{})
	c.Assert(err, gc.IsNil)
	args.Tools = toolsList[0]

	for _, errorCode := range []int{255, 0} {
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
			// machine ID will be 2, not 1. Even though we failed and the
			// machine is removed, the ID is not reused.
			c.Assert(m.Id(), gc.Equals, "2")
			instanceId, err := m.InstanceId()
			c.Assert(err, gc.IsNil)
			c.Assert(instanceId, gc.Equals, instance.Id("manual:"+hostname))
			tools, err := m.AgentTools()
			c.Assert(err, gc.IsNil)
			c.Assert(tools, gc.DeepEquals, toolsList[0])
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
