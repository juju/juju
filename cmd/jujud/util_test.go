package main_test

import (
	"launchpad.net/gnuflag"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/cmd"
	main "launchpad.net/juju/go/cmd/jujud"
)

type acCreator func() (cmd.Command, *main.AgentConf)

func initCmd(c cmd.Command, args []string) error {
	return c.Init(gnuflag.NewFlagSet("", gnuflag.ContinueOnError), args)
}

// CheckAgentCommand is a utility function for verifying that common agent
// options are handled by a Command; it returns an instance of that
// command pre-parsed with the always-required options and whatever others
// are necessary to allow parsing to succeed (specified in args).
func CheckAgentCommand(c *C, create acCreator, args []string) cmd.Command {
	com, _ := create()
	err := initCmd(com, args)
	c.Assert(err, ErrorMatches, "--zookeeper-servers option must be set")
	args = append(args, "--zookeeper-servers", "zk1:2181,zk2:2181")

	com, conf := create()
	c.Assert(initCmd(com, args), IsNil)
	c.Assert(conf.StateInfo.Addrs, DeepEquals, []string{"zk1:2181", "zk2:2181"})
	c.Assert(conf.JujuDir, Equals, "/var/lib/juju")
	args = append(args, "--juju-directory", "jd")

	com, conf = create()
	c.Assert(initCmd(com, args), IsNil)
	c.Assert(conf.StateInfo.Addrs, DeepEquals, []string{"zk1:2181", "zk2:2181"})
	c.Assert(conf.JujuDir, Equals, "jd")
	return com
}

// ParseAgentCommand is a utility function that inserts the always-required args
// before parsing an agent command and returning the result.
func ParseAgentCommand(ac cmd.Command, args []string) error {
	common := []string{
		"--zookeeper-servers", "zk:2181",
		"--juju-directory", "jd",
	}
	return initCmd(ac, append(common, args...))
}
