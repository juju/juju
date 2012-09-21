package main

import (
	"io/ioutil"
	"launchpad.net/gnuflag"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
)

type acCreator func() (cmd.Command, *AgentConf)

func initCmd(c cmd.Command, args []string) error {
	f := gnuflag.NewFlagSet("", gnuflag.ContinueOnError)
	f.SetOutput(ioutil.Discard)
	return c.Init(f, args)
}

// CheckAgentCommand is a utility function for verifying that common agent
// options are handled by a Command; it returns an instance of that
// command pre-parsed with the always-required options and whatever others
// are necessary to allow parsing to succeed (specified in args).
func CheckAgentCommand(c *C, create acCreator, args []string) cmd.Command {
	com, _ := create()
	err := initCmd(com, args)
	c.Assert(err, ErrorMatches, "--state-servers option must be set")
	args = append(args, "--state-servers", "zk1:2181,zk2:2181")

	com, conf := create()
	c.Assert(initCmd(com, args), IsNil)
	c.Assert(conf.StateInfo.Addrs, DeepEquals, []string{"zk1:2181", "zk2:2181"})
	c.Assert(conf.DataDir, Equals, "/var/lib/juju")
	args = append(args, "--data-dir", "jd")

	com, conf = create()
	c.Assert(initCmd(com, args), IsNil)
	c.Assert(conf.StateInfo.Addrs, DeepEquals, []string{"zk1:2181", "zk2:2181"})
	c.Assert(conf.DataDir, Equals, "jd")
	return com
}

// ParseAgentCommand is a utility function that inserts the always-required args
// before parsing an agent command and returning the result.
func ParseAgentCommand(ac cmd.Command, args []string) error {
	common := []string{
		"--state-servers", "zk:2181",
		"--data-dir", "jd",
	}
	return initCmd(ac, append(common, args...))
}
