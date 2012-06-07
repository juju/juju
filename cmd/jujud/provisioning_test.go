package main

import (
	. "launchpad.net/gocheck"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju-core/juju/cmd"
	"launchpad.net/juju-core/juju/environs/dummy"
	"launchpad.net/juju-core/juju/state"
	"launchpad.net/juju-core/juju/testing"
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
	testing.ZkRemoveTree(s.zkConn, "/")
	s.st, err = state.Initialize(info)
	c.Assert(err, IsNil)

	dummy.Reset()
}

func (s *ProvisioningSuite) TearDownTest(c *C) {
	s.zkConn.Close()
}

func (s *ProvisioningSuite) TestParseSuccess(c *C) {
	create := func() (cmd.Command, *AgentConf) {
		a := &ProvisioningAgent{}
		return a, &a.Conf
	}
	CheckAgentCommand(c, create, []string{})
}

func (s *ProvisioningSuite) TestParseUnknown(c *C) {
	a := &ProvisioningAgent{}
	err := ParseAgentCommand(a, []string{"nincompoops"})
	c.Assert(err, ErrorMatches, `unrecognized args: \["nincompoops"\]`)
}

func initProvisioningAgent() (*ProvisioningAgent, error) {
	args := []string{"--zookeeper-servers", zkAddr}
	c := &ProvisioningAgent{}
	return c, initCmd(c, args)
}

func (s *ProvisioningSuite) TestProvisionerStartStop(c *C) {
	p := NewProvisioner(s.st)
	c.Assert(p.Stop(), IsNil)
}

func (s *ProvisioningSuite) TestProvisionerEnvironmentChange(c *C) {
	p := NewProvisioner(s.st)

	// Change environment configuration to point to dummy.
	env, err := s.st.EnvironConfig()
	c.Assert(err, IsNil)
	env.Set("type", "dummy")
	env.Set("zookeeper", false)
	env.Set("name", "testing")
	_, err = env.Write()
	c.Assert(err, IsNil)

	// Twiddle with the environment configuration.
	env, err = s.st.EnvironConfig()
	c.Assert(err, IsNil)
	env.Set("name", "testing2")
	_, err = env.Write()
	c.Assert(err, IsNil)
	env.Set("name", "testing3")
	_, err = env.Write()
	c.Assert(p.Stop(), IsNil)
}

func (s *ProvisioningSuite) TestProvisionerStopOnStateClose(c *C) {
	p := NewProvisioner(s.st)

	// Change environment configuration to point to dummy.
	env, err := s.st.EnvironConfig()
	c.Assert(err, IsNil)
	env.Set("type", "dummy")
	env.Set("zookeeper", false)
	env.Set("name", "testing")
	_, err = env.Write()
	c.Assert(err, IsNil)

	s.st.Close()

	c.Assert(p.Wait(), ErrorMatches, "watcher.*")
	c.Assert(p.Stop(), ErrorMatches, "watcher.*")

}
