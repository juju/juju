package main_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/cmd"
	main "launchpad.net/juju/go/cmd/jujud"
)

type acCreator func() main.AgentCommand

// CheckAgentCommand is a utility function for verifying that common agent
// options are handled by an AgentCommand; it returns an instance of that
// command pre-parsed with the always-required options and whatever others
// are necessary to allow parsing to succeed (specified in args).
func CheckAgentCommand(c *C, create acCreator, args []string) main.AgentCommand {
	err := cmd.Parse(create(), args)
	c.Assert(err, ErrorMatches, "--zookeeper-servers option must be set")
	args = append(args, "--zookeeper-servers", "zk1:2181,zk2:2181")

	err = cmd.Parse(create(), args)
	c.Assert(err, ErrorMatches, "--session-file option must be set")
	args = append(args, "--session-file", "sf")

	ac := create()
	c.Assert(cmd.Parse(ac, args), IsNil)
	c.Assert(ac.StateInfo().Addrs, Equals, []string{"zk1:2181", "zk2:2181"})
	c.Assert(ac.SessionFile(), Equals, "sf")
	c.Assert(ac.JujuDir(), Equals, "/var/lib/juju")
	args = append(args, "--juju-directory", "jd")

	ac = create()
	c.Assert(cmd.Parse(ac, args), IsNil)
	c.Assert(ac.StateInfo().Addrs, Equals, []string{"zk1:2181", "zk2:2181"})
	c.Assert(ac.SessionFile(), Equals, "sf")
	c.Assert(ac.JujuDir(), Equals, "jd")
	return ac
}

// ParseAgentCommand is a utility function that inserts the always-required args
// before parsing an agent command and returning the result.
func ParseAgentCommand(ac cmd.Command, args []string) error {
	common := []string{
		"--zookeeper-servers", "zk:2181",
		"--session-file", "sf",
		"--juju-directory", "jd",
	}
	return cmd.Parse(ac, append(common, args...))
}
