package main

import (
	"time"

	. "launchpad.net/gocheck"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju-core/juju/cmd"
	"launchpad.net/juju-core/juju/environs/dummy"
	"launchpad.net/juju-core/juju/state"
	"launchpad.net/juju-core/juju/testing"
)

type ProvisioningSuite struct {
	zkConn *zookeeper.Conn
	zkInfo *state.Info
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
	s.zkInfo = &state.Info{
		Addrs: []string{zkAddr},
	}

	s.st, err = state.Initialize(s.zkInfo)
	c.Assert(err, IsNil)

	dummy.Reset()

	// seed /environment to point to dummy
	env, err := s.st.EnvironConfig()
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
	p, err := NewProvisioner(s.zkInfo)
	c.Assert(err, IsNil)
	c.Assert(p.Stop(), IsNil)
}

func (s *ProvisioningSuite) TestProvisionerEnvironmentChange(c *C) {
	p, err := NewProvisioner(s.zkInfo)
	c.Assert(err, IsNil)
	// Twiddle with the environment configuration.
	env, err := s.st.EnvironConfig()
	c.Assert(err, IsNil)
	env.Set("name", "testing2")
	_, err = env.Write()
	c.Assert(err, IsNil)
	env.Set("name", "testing3")
	_, err = env.Write()
	c.Assert(p.Stop(), IsNil)
}

func (s *ProvisioningSuite) TestProvisionerStopOnStateClose(c *C) {
	p, err := NewProvisioner(s.zkInfo)
	c.Assert(err, IsNil)

	p.st.Close()

	c.Assert(p.Wait(), ErrorMatches, "watcher.*")
	c.Assert(p.Stop(), ErrorMatches, "watcher.*")
}

// Start and stop one machine, watch the PA.
func (s *ProvisioningSuite) TestSimple(c *C) {
	p, err := NewProvisioner(s.zkInfo)
	c.Assert(err, IsNil)

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
		c.Errorf("ProvisioningAgent did not action AddMachine after 3 second")
	}

	// now remove it
	c.Assert(s.st.RemoveMachine(m.Id()), IsNil)

	// watch the PA remove it
	select {
	case o := <-op:
		c.Assert(o.Kind, Equals, dummy.OpStopInstances)
	case <-time.After(3 * time.Second):
		c.Errorf("ProvisioningAgent did not action RemoveMachine after 3 second")
	}
	c.Assert(p.Stop(), IsNil)
}

func (s *ProvisioningSuite) TestProvisioningDoesNotOccurWithAnInvalidEnvironment(c *C) {
	// make the environment unpalitable to the config checker
	env, err := s.st.EnvironConfig()
	c.Assert(err, IsNil)

	env.Set("name", 1)
	_, err = env.Write()
	c.Assert(err, IsNil)

	p, err := NewProvisioner(s.zkInfo)
	c.Assert(err, IsNil)

	op := make(chan dummy.Operation, 1)
	dummy.Listen(op)

	// try to create a machine
	_, err = s.st.AddMachine()
	c.Assert(err, IsNil)

	// the PA should not create it
	select {
	case <-op:
		c.Errorf("provisioner started an instance")
	case <-time.After(1 * time.Second):

	}
	c.Assert(p.Stop(), IsNil)
}

func (s *ProvisioningSuite) TestProvisioningOccursWithFixedEnvironment(c *C) {
	// make the environment unpalitable to the config checker
	env, err := s.st.EnvironConfig()
	c.Assert(err, IsNil)
	env.Set("name", 1)
	_, err = env.Write()
	c.Assert(err, IsNil)

	p, err := NewProvisioner(s.zkInfo)
	c.Assert(err, IsNil)
	defer p.Stop()

	op := make(chan dummy.Operation, 1)
	dummy.Listen(op)

	// try to create a machine
	_, err = s.st.AddMachine()
	c.Assert(err, IsNil)

	// the PA should not create it
	select {
	case <-op:
		c.Errorf("provisioner started an instance")
	case <-time.After(1 * time.Second):

	}

	// now fix the environment
	env.Set("name", "testing")
	_, err = env.Write()
	c.Assert(err, IsNil)

	// watch the PA create it
	select {
	case o := <-op:
		c.Assert(o.Kind, Equals, dummy.OpStartInstance)
	case <-time.After(3 * time.Second):
		c.Errorf("ProvisioningAgent did not action AddMachine after 3 second")
	}
}

func (s *ProvisioningSuite) TestProvisioningDoesOccurAfterInvalidEnvironmentPublished(c *C) {
	p, err := NewProvisioner(s.zkInfo)
	c.Assert(err, IsNil)
	defer p.Stop()

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
		c.Errorf("ProvisioningAgent did not action AddMachine after 3 second")
	}

	// make the environment unpalitable to the config checker
	env, err := s.st.EnvironConfig()
	c.Assert(err, IsNil)
	env.Set("name", 1)
	_, err = env.Write()
	c.Assert(err, IsNil)

	// create a second machine
	_, err = s.st.AddMachine()
	c.Assert(err, IsNil)

	// the PA should create it using the old environment
	// watch the PA create it
	select {
	case o := <-op:
		c.Assert(o.Kind, Equals, dummy.OpStartInstance)
	case <-time.After(3 * time.Second):
		c.Errorf("ProvisioningAgent did not action AddMachine after 3 second")
	}
}

func (s *ProvisioningSuite) TestProvisioningDoesNotProvisionTheSameMachineAfterRestart(c *C) {
	p, err := NewProvisioner(s.zkInfo)
	c.Check(err, IsNil)
	// we are not using defer p.Stop() because we need to control when the PA is    
	// restarted in this test. tf. Methods like Fatalf and Assert should not be used.

	op := make(chan dummy.Operation, 1)
	dummy.Listen(op)

	// create a machine
	_, err = s.st.AddMachine()
	c.Check(err, IsNil)

	// the PA should create it
	select {
	case o := <-op:
		c.Check(o.Kind, Equals, dummy.OpStartInstance)
	case <-time.After(3 * time.Second):
		c.Errorf("ProvisioningAgent did not action AddMachine after 3 second")
	}

	// restart the PA
	c.Check(p.Stop(), IsNil)

	p, err = NewProvisioner(s.zkInfo)
	c.Check(err, IsNil)

	// check that there is only one machine known
	machines, err := p.st.AllMachines()
	c.Check(err, IsNil)
	c.Check(len(machines), Equals, 1)
	c.Check(machines[0].Id(), Equals, 0)
	// the PA should not create it a second time
	select {
	case o := <-op:
		c.Errorf("provisioner started an instance: %v", o)
	case <-time.After(1 * time.Second):
	}
	c.Assert(p.Stop(), IsNil)
}

func (s *ProvisioningSuite) TestProvisioningRecoversAfterInvalidEnvironmentPublished(c *C) {
	p, err := NewProvisioner(s.zkInfo)
	c.Assert(err, IsNil)
	defer p.Stop()

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
		c.Errorf("ProvisioningAgent did not action AddMachine after 3 second")
	}

	// make the environment unpalitable to the config checker
	env, err := s.st.EnvironConfig()
	c.Assert(err, IsNil)
	env.Set("name", 1)
	_, err = env.Write()
	c.Assert(err, IsNil)

	// create a second machine
	_, err = s.st.AddMachine()
	c.Assert(err, IsNil)

	// the PA should create it using the old environment
	// watch the PA create it
	select {
	case o := <-op:
		c.Assert(o.Kind, Equals, dummy.OpStartInstance)
	case <-time.After(3 * time.Second):
		c.Errorf("ProvisioningAgent did not action AddMachine after 3 second")
	}

	// now fix the environment
	env.Set("name", "testing")
	_, err = env.Write()
	c.Assert(err, IsNil)

	// create a third machine
	_, err = s.st.AddMachine()
	c.Assert(err, IsNil)

	// the PA should create it using the new environment
	// watch the PA create it
	select {
	case o := <-op:
		c.Assert(o.Kind, Equals, dummy.OpStartInstance)
	case <-time.After(3 * time.Second):
		c.Errorf("ProvisioningAgent did not action AddMachine after 3 second")
	}
}
