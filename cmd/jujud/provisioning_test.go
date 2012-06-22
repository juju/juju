package main

import (
	"time"

	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/state"
)

type ProvisioningSuite struct {
	logging testing.LoggingSuite
	zkSuite
	st *state.State
	op <-chan dummy.Operation
}

var _ = Suite(&ProvisioningSuite{})

var veryShortAttempt = environs.AttemptStrategy{
	Total: 1 * time.Second,
	Delay: 80 * time.Millisecond,
}

func (s *ProvisioningSuite) SetUpTest(c *C) {
	s.logging.SetUpTest(c)
	dummy.Reset()	

	// Create the operations channel with more than enough space
	// for those tests that don't listen on it.
	op := make(chan dummy.Operation, 20)
	dummy.Listen(op)
	s.op = op

	env, err := environs.NewEnviron(map[string]interface{}{
		"type": "dummy",
		"zookeeper": true,
		"name": "testing",
	})
	c.Assert(err, IsNil)
	err = env.Bootstrap(false)
	c.Assert(err, IsNil)

	s.zkSuite.SetUpTest(c)
	s.st, err = state.Open(s.zkInfo)
	c.Assert(err, IsNil)
}

func (s *ProvisioningSuite) TearDownTest(c *C) {
	s.zkSuite.TearDownTest()
	s.logging.TearDownTest(c)
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
func (s *ProvisioningSuite) checkStartInstance(c *C) {
	// use the non fatal variants to avoid leaking provisioners.    
	for {
		select {
		case o := <-s.op:
			switch o.Kind {
			case dummy.OpStartInstance:
				return
			default:
				c.Logf("ignoring unexpected operation %v", o)
				// ignore
			}
		case <-time.After(2 * time.Second):
			c.Errorf("provisioner did not start an instance")
			return
		}
	}
}

// checkNotStartInstance checks that an instance was not started
func (s *ProvisioningSuite) checkNotStartInstance(c *C) {
	for {
		select {
		case o := <-s.op:
			switch o.Kind {
			case dummy.OpStartInstance:
				c.Errorf("instance started: %v", o)
				return
			default:
				// ignore   
			}
		case <-time.After(200 * time.Millisecond):
			return
		}
	}
}

// checkStopInstance checks that an instance has been stopped.
func (s *ProvisioningSuite) checkStopInstance(c *C) {
	// use the non fatal variants to avoid leaking provisioners.    
	for {
		select {
		case o := <-s.op:
			switch o.Kind {
			case dummy.OpStopInstances:
				return
			default:
				//ignore 
			}
		case <-time.After(2 * time.Second):
			c.Errorf("provisioner did not stop an instance")
			return
		}
	}
}

// checkMachineIdSet checks that the machine now has an instance id.
func (s *ProvisioningSuite) checkMachineIdSet(c *C, m *state.Machine) {
	if s.checkMachineId(c, m, false) {
		c.Errorf("provisioner did not set machine.InstanceId")
	}
}

// checkMachineIdNotSet checks that the machine id is unset.
func (s *ProvisioningSuite) checkMachineIdNotSet(c *C, m *state.Machine) {
	if s.checkMachineId(c, m, true) {
		c.Errorf("provisioner did not clear machine.InstanceId")
	}
}

func (s *ProvisioningSuite) checkMachineId(c *C, m *state.Machine, isEmpty bool) bool {
	// TODO(dfc) add machine.WatchConfig() to avoid having to poll.
	for a := veryShortAttempt.Start(); a.Next(); {
		id, err := m.InstanceId()
		if err != nil {
			c.Check(err, IsNil)
			return false
		}
		if (isEmpty && id == "") && (!isEmpty && id != "") {
			return true
		}
	}
	return false
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
	c.Check(p.Wait(), ErrorMatches, ".* zookeeper is closing")
	c.Assert(p.Stop(), ErrorMatches, ".* zookeeper is closing")
}

// Start and stop one machine, watch the PA.
func (s *ProvisioningSuite) TestSimple(c *C) {
	p, err := NewProvisioner(s.zkInfo)
	c.Assert(err, IsNil)
	defer s.stopProvisioner(c, p)

	// place a new machine into the state
	m, err := s.st.AddMachine()
	c.Assert(err, IsNil)

	s.checkStartInstance(c)
	s.checkMachineIdSet(c, m)

	// now remove it
	c.Assert(s.st.RemoveMachine(m.Id()), IsNil)

	// watch the PA remove it
	s.checkStopInstance(c)
	s.checkMachineIdNotSet(c, m)
}

func (s *ProvisioningSuite) TestProvisioningDoesNotOccurWithAnInvalidEnvironment(c *C) {
	err := s.invalidateEnvironment()
	c.Assert(err, IsNil)

	p, err := NewProvisioner(s.zkInfo)
	c.Assert(err, IsNil)
	defer s.stopProvisioner(c, p)

	// try to create a machine
	_, err = s.st.AddMachine()
	c.Assert(err, IsNil)

	// the PA should not create it
	s.checkNotStartInstance(c)
}

func (s *ProvisioningSuite) TestProvisioningOccursWithFixedEnvironment(c *C) {
	err := s.invalidateEnvironment()
	c.Assert(err, IsNil)

	p, err := NewProvisioner(s.zkInfo)
	c.Assert(err, IsNil)
	defer s.stopProvisioner(c, p)

	// try to create a machine
	m, err := s.st.AddMachine()
	c.Assert(err, IsNil)

	// the PA should not create it
	s.checkNotStartInstance(c)

	err = s.fixEnvironment()
	c.Assert(err, IsNil)

	s.checkStartInstance(c)
	s.checkMachineIdSet(c, m)
}

func (s *ProvisioningSuite) TestProvisioningDoesOccurAfterInvalidEnvironmentPublished(c *C) {
	p, err := NewProvisioner(s.zkInfo)
	c.Assert(err, IsNil)
	defer s.stopProvisioner(c, p)

	// place a new machine into the state
	m, err := s.st.AddMachine()
	c.Assert(err, IsNil)

	s.checkStartInstance(c)
	s.checkMachineIdSet(c, m)

	err = s.invalidateEnvironment()
	c.Assert(err, IsNil)

	// create a second machine
	m, err = s.st.AddMachine()
	c.Assert(err, IsNil)

	// the PA should create it using the old environment
	s.checkStartInstance(c)
	s.checkMachineIdSet(c, m)
}

func (s *ProvisioningSuite) TestProvisioningDoesNotProvisionTheSameMachineAfterRestart(c *C) {
	p, err := NewProvisioner(s.zkInfo)
	c.Check(err, IsNil)
	// we are not using defer s.stopProvisioner(c, p) because we need to control when 
	// the PA is restarted in this test. tf. Methods like Fatalf and Assert should not be used.

	// create a machine
	m, err := s.st.AddMachine()
	c.Check(err, IsNil)

	s.checkStartInstance(c)
	s.checkMachineIdSet(c, m)

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
	s.checkNotStartInstance(c)

	c.Assert(p.Stop(), IsNil)
}

func (s *ProvisioningSuite) TestProvisioningStopsUnknownInstances(c *C) {
	p, err := NewProvisioner(s.zkInfo)
	c.Check(err, IsNil)
	// we are not using defer s.stopProvisioner(c, p) because we need to control when 
	// the PA is restarted in this test. Methods like Fatalf and Assert should not be used.

	// create a machine
	m, err := s.st.AddMachine()
	c.Check(err, IsNil)

	s.checkStartInstance(c)
	s.checkMachineIdSet(c, m)

	// create a second machine
	m, err = s.st.AddMachine()
	c.Check(err, IsNil)

	s.checkStartInstance(c)
	s.checkMachineIdSet(c, m)

	// stop the PA
	c.Check(p.Stop(), IsNil)

	// remove the machine
	err = s.st.RemoveMachine(m.Id())
	c.Check(err, IsNil)

	// start a new provisioner
	p, err = NewProvisioner(s.zkInfo)
	c.Check(err, IsNil)

	s.checkStopInstance(c)

	c.Assert(p.Stop(), IsNil)
}

// This check is different from the one above as it catches the edge case
// where the final machine has been removed from the state while the PA was 
// not running. 
func (s *ProvisioningSuite) TestProvisioningStopsOnlyUnknownInstances(c *C) {
	p, err := NewProvisioner(s.zkInfo)
	c.Check(err, IsNil)
	// we are not using defer s.stopProvisioner(c, p) because we need to control when 
	// the PA is restarted in this test. Methods like Fatalf and Assert should not be used.

	// create a machine
	m, err := s.st.AddMachine()
	c.Check(err, IsNil)

	s.checkStartInstance(c)
	s.checkMachineIdSet(c, m)

	// stop the PA
	c.Check(p.Stop(), IsNil)

	// remove the machine
	err = s.st.RemoveMachine(m.Id())
	c.Check(err, IsNil)

	machines, err := s.st.AllMachines()
	c.Check(err, IsNil)
	c.Check(len(machines), Equals, 0) // it's really gone   

	// start a new provisioner
	p, err = NewProvisioner(s.zkInfo)
	c.Check(err, IsNil)

	s.checkStopInstance(c)

	c.Assert(p.Stop(), IsNil)
}

func (s *ProvisioningSuite) TestProvisioningRecoversAfterInvalidEnvironmentPublished(c *C) {
	p, err := NewProvisioner(s.zkInfo)
	c.Assert(err, IsNil)
	defer s.stopProvisioner(c, p)

	// place a new machine into the state
	m, err := s.st.AddMachine()
	c.Assert(err, IsNil)

	s.checkStartInstance(c)
	s.checkMachineIdSet(c, m)

	err = s.invalidateEnvironment()
	c.Assert(err, IsNil)

	// create a second machine
	m, err = s.st.AddMachine()
	c.Assert(err, IsNil)

	// the PA should create it using the old environment
	s.checkStartInstance(c)
	s.checkMachineIdSet(c, m)

	err = s.fixEnvironment()
	c.Assert(err, IsNil)

	// create a third machine
	m, err = s.st.AddMachine()
	c.Assert(err, IsNil)

	// the PA should create it using the new environment
	s.checkStartInstance(c)
	s.checkMachineIdSet(c, m)
}
