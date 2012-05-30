package main_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju/go/cmd"
	main "launchpad.net/juju/go/cmd/jujud"
	"launchpad.net/juju/go/state"
	"launchpad.net/juju/go/testing"

	"launchpad.net/juju/go/environs/dummy"
	"launchpad.net/juju/go/log"

	"sync"
	"time"
)

func init() {
	log.Debug = true
}

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

func (s *ProvisioningSuite) TestStartInstance(c *C) {
	a, err := initProvisioningAgent()
	c.Assert(err, IsNil)

	wg := new(sync.WaitGroup)
	wg.Add(1)

	go func() {
		defer wg.Done()
		err = a.Run(nil) // context is unused
		c.Assert(err, ErrorMatches, ".*ZooKeeper connection closed; ")
	}()

	op := make(chan dummy.Operation, 1)
	dummy.Listen(op)

	// place a new machine into the state
	_, err = s.st.AddMachine()
	c.Assert(err, IsNil)

	// watch the PA create it
	select {
	case o := <-op:
		c.Assert(o.Kind, Equals, dummy.OpStartInstance)
	case <-time.After(3 * time.Second):
		c.Fatalf("ProvisioningAgent did not action AddMachine after 3 second")
	}

	// close the PA's underlying state, shutting it down.
	// this races with the PA writing data to the state,
	// so pause a little.
	<-time.After(2 * time.Second)
	a.State.Close()

	wg.Wait()
}

func (s *ProvisioningSuite) TestStopInstance(c *C) {
	a, err := initProvisioningAgent()
	c.Assert(err, IsNil)

	wg := new(sync.WaitGroup)
	wg.Add(1)

	go func() {
		defer wg.Done()
		err = a.Run(nil) // context is unused
		c.Assert(err, ErrorMatches, ".*ZooKeeper connection closed; ")
	}()

	op := make(chan dummy.Operation, 1)
	dummy.Listen(op)

	// place a new machine into the state
	m, err := s.st.AddMachine()
	c.Assert(err, IsNil)

	// watch the PA create it
	select {
	case o := <-op:
		c.Assert(o.Kind, Equals, dummy.OpStartInstance)
	case <-time.After(3 * time.Second):
		c.Fatalf("ProvisioningAgent did not action AddMachine after 3 second")
	}

	// now remove it
	c.Assert(s.st.RemoveMachine(m.Id()), IsNil)

	// watch the PA remove it
	select {
	case o := <-op:
		c.Assert(o.Kind, Equals, dummy.OpStopInstances)
	case <-time.After(3 * time.Second):
		c.Fatalf("ProvisioningAgent did not action RemoveMachine after 3 second")
	}

	// close the PA's underlying state, shutting it down.
	// this races with the PA writing data to the state,
	// so pause a little.
	<-time.After(2 * time.Second)
	a.State.Close()

	wg.Wait()
}
