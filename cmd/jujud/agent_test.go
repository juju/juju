package main_test

import (
	"fmt"
	"launchpad.net/gnuflag"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/agent"
	"launchpad.net/juju/go/cmd"
	main "launchpad.net/juju/go/cmd/jujud"
)

type AgentSuite struct{}

var _ = Suite(&AgentSuite{})

type TestAgentFlags struct {
	Value string
}

func (af *TestAgentFlags) Name() string {
	return "secret"
}

func (af *TestAgentFlags) Agent() agent.Agent {
	return nil
}

func (af *TestAgentFlags) InitFlagSet(f *gnuflag.FlagSet) {
	f.StringVar(&af.Value, "option", af.Value, "a value you can set")
}

func (af *TestAgentFlags) ParsePositional(args []string) error {
	if len(args) == 1 {
		if args[0] == "please" {
			return nil
		}
	}
	return fmt.Errorf("insufficiently polite")
}

func parseTestAgentFlags(c *C, args []string) (*TestAgentFlags, *main.AgentCommand, error) {
	f := &TestAgentFlags{}
	ac := main.NewAgentCommand(f)
	err := cmd.Parse(ac, true, args)
	return f, ac, err
}

func (s *AgentSuite) TestRequiredArgs(c *C) {
	args := []string{}
	_, _, err := parseTestAgentFlags(c, args)
	c.Assert(err, ErrorMatches, "--zookeeper-servers option must be set")

	args = append(args, "--zookeeper-servers", "zk")
	_, _, err = parseTestAgentFlags(c, args)
	c.Assert(err, ErrorMatches, "--session-file option must be set")

	args = append(args, "--session-file", "sf")
	_, _, err = parseTestAgentFlags(c, args)
	c.Assert(err, ErrorMatches, "insufficiently polite")

	args = append(args, "please")
	j, ac, err := parseTestAgentFlags(c, args)
	c.Assert(err, IsNil)
	c.Assert(ac.Zookeeper, Equals, "zk")
	c.Assert(ac.SessionFile, Equals, "sf")
	c.Assert(ac.JujuDir, Equals, "/var/lib/juju")
	c.Assert(j.Value, Equals, "")

	args = append(args, "--juju-directory", "jd")
	_, ac, err = parseTestAgentFlags(c, args)
	c.Assert(err, IsNil)
	c.Assert(ac.JujuDir, Equals, "jd")

	args = append(args, "--option", "value")
	j, _, err = parseTestAgentFlags(c, args)
	c.Assert(err, IsNil)
	c.Assert(j.Value, Equals, "value")
}

func (s *AgentSuite) TestInfo(c *C) {
	info := main.NewAgentCommand(&TestAgentFlags{}).Info()
	c.Assert(info.Name, Equals, "secret")
	c.Assert(info.Usage, Equals, "secret [options]")
	c.Assert(info.Purpose, Equals, "run a juju secret agent")
}
