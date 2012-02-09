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
// are necessary to allow it to succeed (specified in args).
func CheckAgentCommand(c *C, create acCreator, args []string) main.AgentCommand {
	err := cmd.Parse(create(), args)
	c.Assert(err, ErrorMatches, "--zookeeper-servers option must be set")
	args = append(args, "--zookeeper-servers", "zk")

	err = cmd.Parse(create(), args)
	c.Assert(err, ErrorMatches, "--session-file option must be set")
	args = append(args, "--session-file", "sf")

	ac := create()
	err = cmd.Parse(ac, args)
	c.Assert(err, IsNil)
	conf := ac.Conf()
	c.Assert(conf.Zookeeper, Equals, "zk")
	c.Assert(conf.SessionFile, Equals, "sf")
	c.Assert(conf.JujuDir, Equals, "/var/lib/juju")
	args = append(args, "--juju-directory", "jd")

	ac = create()
	err = cmd.Parse(ac, args)
	c.Assert(err, IsNil)
	conf = ac.Conf()
	c.Assert(conf.Zookeeper, Equals, "zk")
	c.Assert(conf.SessionFile, Equals, "sf")
	c.Assert(conf.JujuDir, Equals, "jd")
	return ac
}

// ParseAgentCommand is a utility function that inserts the always-required args
// before parsing an agent command and returning the result.
func ParseAgentCommand(ac cmd.Command, args []string) error {
	common := []string{
		"--zookeeper-servers", "zk",
		"--session-file", "sf",
		"--juju-directory", "jd",
	}
	return cmd.Parse(ac, append(common, args...))
}
