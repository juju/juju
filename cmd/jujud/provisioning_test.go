package main_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju/go/cmd"
	main "launchpad.net/juju/go/cmd/jujud"
	"launchpad.net/juju/go/state"

)

type ProvisioningSuite struct {
	zkConn *zookeeper.Conn
	st     *state.State
}

var _ = Suite(&ProvisioningSuite{})

func (s *ProvisioningSuite) TestParseSuccess(c *C) {
	create := func() (cmd.Command, *main.AgentConf) {
		a := &main.ProvisioningAgent{}
		return a, &a.Conf
	}
	CheckAgentCommand(c, create, []string{})
}

func (s *ProvisioningSuite) TestParseUnknown(c *C) {
	a := &main.ProvisioningAgent{}
	err := ParseAgentCommand(a, []string{"nincompoops"})
	c.Assert(err, ErrorMatches, `unrecognized args: \["nincompoops"\]`)
}

func initProvisioningAgent() (*main.ProvisioningAgent, error) {
	args := []string{"--zookeeper-servers", zkAddr}
	c := main.NewProvisioningAgent()
	return c, initCmd(c, args)
}
