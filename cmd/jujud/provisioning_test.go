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

// invalidateEnvironment alters the environment configuration
// so the ConfigNode returned from the watcher will not pass
// validation.
func (s *ProvisioningSuite) invalidateEnvironment() error {
	env, err := s.st.EnvironConfig()
	if err != nil {
		return err
	}
	env.Set("name", 1)
	_, err = env.Write()
	return err
}

// fixEnvironment undoes the work of invalidateEnvironment.
func (s *ProvisioningSuite) fixEnvironment() error {
	env, err := s.st.EnvironConfig()
	if err != nil {
		return err
	}
	env.Set("name", "testing")
	_, err = env.Write()
	return err
}

func (s *ProvisioningSuite) stopProvisioner(c *C, p *Provisioner) {
	c.Assert(p.Stop(), IsNil)
}

// checkStartInstance checks that an instace has been started.
func (s *ProvisioningSuite) checkStartInstance(c *C, op <-chan dummy.Operation) {
	// use the non fatal variants to avoid leaking provisioners.	
	select {
	case o := <-op:
		switch o.Kind {
		case dummy.OpStartInstance:
			return
		default:
			// ignore
		}
	case <-time.After(3 * time.Second):
		c.Errorf("provisioner did not start an instance")
	}
}

// checkNotStartInstance checks that an instance was not started
func (s *ProvisioningSuite) checkNotStartInstance(c *C, op <-chan dummy.Operation) {
	for {
		select {
		case o := <-op:
			switch o.Kind {
			case dummy.OpStartInstance:
				c.Errorf("instance started: %v", o)
			default:
				// ignore	
			}
		case <-time.After(200 * time.Millisecond):
			return
		}
	}
}

// checkStopInstance checks that an instance has been stopped.
func (s *ProvisioningSuite) checkStopInstance(c *C, op <-chan dummy.Operation) {
	// use the non fatal variants to avoid leaking provisioners.	
	select {
	case o := <-op:
		switch o.Kind {
		case dummy.OpStopInstances:
			return
		default:
			//ignore 
		}
	case <-time.After(3 * time.Second):
		c.Errorf("provisioner did not stop an instance")
	}
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
	defer s.stopProvisioner(c, p)
	// Twiddle with the environment configuration.
	env, err := s.st.EnvironConfig()
	c.Assert(err, IsNil)
	env.Set("name", "testing2")
	_, err = env.Write()
	c.Assert(err, IsNil)
	env.Set("name", "testing3")
	_, err = env.Write()
}

func (s *ProvisioningSuite) TestProvisionerStopOnStateClose(c *C) {
	p, err := NewProvisioner(s.zkInfo)
	c.Assert(err, IsNil)

	p.st.Close()

	// must use Check to avoid leaking PA
	c.Check(p.Wait(), ErrorMatches, "content change channel closed unexpectedly")
	c.Assert(p.Stop(), ErrorMatches, "content change channel closed unexpectedly")
}

// Start and stop one machine, watch the PA.
func (s *ProvisioningSuite) TestSimple(c *C) {
	p, err := NewProvisioner(s.zkInfo)
	c.Assert(err, IsNil)
	defer s.stopProvisioner(c, p)

	op := make(chan dummy.Operation, 1)
	dummy.Listen(op)

	// place a new machine into the state
	m, err := s.st.AddMachine()
	c.Assert(err, IsNil)

	s.checkStartInstance(c, op)

	// now remove it
	c.Assert(s.st.RemoveMachine(m.Id()), IsNil)

	// watch the PA remove it
	s.checkStopInstance(c, op)
}

func (s *ProvisioningSuite) TestProvisioningDoesNotOccurWithAnInvalidEnvironment(c *C) {
	err := s.invalidateEnvironment()
	c.Assert(err, IsNil)

	p, err := NewProvisioner(s.zkInfo)
	c.Assert(err, IsNil)
	defer s.stopProvisioner(c, p)

	op := make(chan dummy.Operation, 1)
	dummy.Listen(op)

	// try to create a machine
	_, err = s.st.AddMachine()
	c.Assert(err, IsNil)

	// the PA should not create it
	s.checkNotStartInstance(c, op)
}

func (s *ProvisioningSuite) TestProvisioningOccursWithFixedEnvironment(c *C) {
	err := s.invalidateEnvironment()
	c.Assert(err, IsNil)

	p, err := NewProvisioner(s.zkInfo)
	c.Assert(err, IsNil)
	defer s.stopProvisioner(c, p)

	op := make(chan dummy.Operation, 1)
	dummy.Listen(op)

	// try to create a machine
	_, err = s.st.AddMachine()
	c.Assert(err, IsNil)

	// the PA should not create it
	s.checkNotStartInstance(c, op)

	err = s.fixEnvironment()
	c.Assert(err, IsNil)

	s.checkStartInstance(c, op)
}

func (s *ProvisioningSuite) TestProvisioningDoesOccurAfterInvalidEnvironmentPublished(c *C) {
	p, err := NewProvisioner(s.zkInfo)
	c.Assert(err, IsNil)
	defer s.stopProvisioner(c, p)

	op := make(chan dummy.Operation, 1)
	dummy.Listen(op)

	// place a new machine into the state
	_, err = s.st.AddMachine()
	c.Assert(err, IsNil)

	s.checkStartInstance(c, op)

	err = s.invalidateEnvironment()
	c.Assert(err, IsNil)

	// create a second machine
	_, err = s.st.AddMachine()
	c.Assert(err, IsNil)

	// the PA should create it using the old environment
	s.checkStartInstance(c, op)
}

func (s *ProvisioningSuite) TestProvisioningDoesNotProvisionTheSameMachineAfterRestart(c *C) {
	p, err := NewProvisioner(s.zkInfo)
	c.Check(err, IsNil)
	// we are not using defer s.stopProvisioner(c, p) because we need to control when 
	// the PA is restarted in this test. tf. Methods like Fatalf and Assert should not be used.
	op := make(chan dummy.Operation, 1)
	dummy.Listen(op)

	// create a machine
	_, err = s.st.AddMachine()
	c.Check(err, IsNil)

	s.checkStartInstance(c, op)

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
	s.checkNotStartInstance(c, op)

	c.Assert(p.Stop(), IsNil)
}

func (s *ProvisioningSuite) TestProvisioningStopsUnknownInstances(c *C) {
	p, err := NewProvisioner(s.zkInfo)
	c.Check(err, IsNil)
	// we are not using defer s.stopProvisioner(c, p) because we need to control when 
	// the PA is restarted in this test. Methods like Fatalf and Assert should not be used.
	op := make(chan dummy.Operation, 1)
	dummy.Listen(op)

	// create a machine
	m, err := s.st.AddMachine()
	c.Check(err, IsNil)

	s.checkStartInstance(c, op)

	// stop the PA
	c.Check(p.Stop(), IsNil)

	// remove the machine
	err = s.st.RemoveMachine(m.Id())
	c.Check(err, IsNil)

	// start a new provisioner
	p, err = NewProvisioner(s.zkInfo)
	c.Check(err, IsNil)

	s.checkStopInstance(c, op)

	c.Assert(p.Stop(), IsNil)
}

func (s *ProvisioningSuite) TestProvisioningRecoversAfterInvalidEnvironmentPublished(c *C) {
	p, err := NewProvisioner(s.zkInfo)
	c.Assert(err, IsNil)
	defer s.stopProvisioner(c, p)

	op := make(chan dummy.Operation, 1)
	dummy.Listen(op)

	// place a new machine into the state
	_, err = s.st.AddMachine()
	c.Assert(err, IsNil)

	s.checkStartInstance(c, op)

	err = s.invalidateEnvironment()
	c.Assert(err, IsNil)

	// create a second machine
	_, err = s.st.AddMachine()
	c.Assert(err, IsNil)

	// the PA should create it using the old environment
	s.checkStartInstance(c, op)

	err = s.fixEnvironment()
	c.Assert(err, IsNil)

	// create a third machine
	_, err = s.st.AddMachine()
	c.Assert(err, IsNil)

	// the PA should create it using the new environment
	s.checkStartInstance(c, op)
}
