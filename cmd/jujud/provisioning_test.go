package main_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju/go/cmd"
	main "launchpad.net/juju/go/cmd/jujud"
	"launchpad.net/juju/go/environs/dummy"
	"launchpad.net/juju/go/state"
	"launchpad.net/juju/go/testing"
)

type ProvisioningSuite struct {
	zkConn *zookeeper.Conn
	st     *state.State
}

var _ = Suite(&ProvisioningSuite{})

func (s *ProvisioningSuite) SetUpTest(c *C) {
	zk, session, err := zookeeper.Dial(zkAddr, 15e9)
	c.Assert(err, IsNil)
	event := <-session
	c.Assert(event.Ok(), Equals, true)
	c.Assert(event.Type, Equals, zookeeper.EVENT_SESSION)
	c.Assert(event.State, Equals, zookeeper.STATE_CONNECTED)

	s.zkConn = zk
	info := &state.Info{
		Addrs: []string{zkAddr},
	}
	s.st, err = state.Initialize(info)
	c.Assert(err, IsNil)

	dummy.Reset()

	// seed /environment to point to dummy
	env, err := s.st.Environment()
	c.Assert(err, IsNil)
	env.Set("type", "dummy")
	env.Set("zookeeper", false)
	env.Set("name", "testing")
	_, err = env.Write()
	c.Assert(err, IsNil)
}

func (s *ProvisioningSuite) TearDownTest(c *C) {
	testing.ZkRemoveTree(s.zkConn, "/")
	s.zkConn.Close()
}

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

func (s *ProvisioningSuite) TestProvisionerStartStop(c *C) {
	p := main.NewProvisioner(s.st)
	c.Assert(p.Stop(), IsNil)
}
