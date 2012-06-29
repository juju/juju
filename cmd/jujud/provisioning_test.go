package main

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/testing"
	coretesting "launchpad.net/juju-core/testing"
	"strings"
	"time"
)

type ProvisioningSuite struct {
	coretesting.LoggingSuite
	testing.StateSuite
	op <-chan dummy.Operation
}

var _ = Suite(&ProvisioningSuite{})

var veryShortAttempt = environs.AttemptStrategy{
	Total: 1 * time.Second,
	Delay: 80 * time.Millisecond,
}

func (s *ProvisioningSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)

	// Create the operations channel with more than enough space
	// for those tests that don't listen on it.
	op := make(chan dummy.Operation, 500)
	dummy.Listen(op)
	s.op = op

	env, err := environs.NewEnviron(map[string]interface{}{
		"type":      "dummy",
		"zookeeper": true,
		"name":      "testing",
	})
	c.Assert(err, IsNil)
	err = env.Bootstrap(false)
	c.Assert(err, IsNil)

	// Sanity check
	info, err := env.StateInfo()
	c.Assert(err, IsNil)
	c.Assert(info, DeepEquals, s.StateInfo(c))

	s.StateSuite.SetUpTest(c)
}

func (s *ProvisioningSuite) TearDownTest(c *C) {
	dummy.Reset()
	s.StateSuite.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
}

// invalidateEnvironment alters the environment configuration
// so the ConfigNode returned from the watcher will not pass
// validation.
func (s *ProvisioningSuite) invalidateEnvironment() error {
	env, err := s.St.EnvironConfig()
	if err != nil {
		return err
	}
	env.Set("name", 1)
	_, err = env.Write()
	return err
}

// fixEnvironment undoes the work of invalidateEnvironment.
func (s *ProvisioningSuite) fixEnvironment() error {
	env, err := s.St.EnvironConfig()
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

// checkStartInstance checks that an instance has been started
// with a machine id the same as m's, and that the machine's
// instance id has been set appropriately.
func (s *ProvisioningSuite) checkStartInstance(c *C, m *state.Machine) {
	for {
		select {
		case o := <-s.op:
			switch o := o.(type) {
			case dummy.OpStartInstance:
				c.Assert(o.Info, DeepEquals, s.StateInfo(c))
				c.Assert(o.MachineId, Equals, m.Id())
				c.Assert(o.Instance, NotNil)
				s.checkMachineId(c, m, o.Instance)
				return
			default:
				c.Logf("ignoring unexpected operation %#v", o)
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
			switch o.(type) {
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
			switch o.(type) {
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

// checkMachineIdSet checks that the machine has an instance id
// that matches that of the given instance. If the instance is nil,
// It checks that the instance id is unset.
func (s *ProvisioningSuite) checkMachineId(c *C, m *state.Machine, inst environs.Instance) {
	// TODO(dfc) add machine.WatchConfig() to avoid having to poll.
	instId := ""
	if inst != nil {
		instId = inst.Id()
	}
	for a := veryShortAttempt.Start(); a.Next(); {
		_, err := m.InstanceId()
		_, notset := err.(*state.NoInstanceIdError)
		if notset {
			if inst == nil {
				return
			} else {
				continue
			}
		}
		c.Assert(err, IsNil)
		break
	}
	id, err := m.InstanceId()
	c.Assert(err, IsNil)
	c.Assert(id, Equals, instId)
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
	args := []string{"--zookeeper-servers", coretesting.ZkAddr}
	c := &ProvisioningAgent{}
	return c, initCmd(c, args)
}

func (s *ProvisioningSuite) TestProvisionerStartStop(c *C) {
	p, err := NewProvisioner(s.StateInfo(c))
	c.Assert(err, IsNil)
	c.Assert(p.Stop(), IsNil)
}

func (s *ProvisioningSuite) TestProvisionerDifferentStateInfo(c *C) {
	info := *s.StateInfo(c)

	// Use an equivalent but textually different address and check
	// that the info when new instances are started is that returned
	// from the environment, not the one passed in originally.
	info.Addrs = append([]string{}, info.Addrs...)
	if !strings.HasPrefix(info.Addrs[0], "127.0.0.1") {
		c.Fatalf("local address %q not as expected", info.Addrs)
	}
	info.Addrs[0] = "localhost" + info.Addrs[0][len("127.0.0.1"):]

	p, err := NewProvisioner(&info)
	c.Assert(err, IsNil)
	defer s.stopProvisioner(c, p)

	// place a new machine into the state
	m, err := s.St.AddMachine()
	c.Assert(err, IsNil)

	s.checkStartInstance(c, m)
}

func (s *ProvisioningSuite) TestProvisionerEnvironmentChange(c *C) {
	p, err := NewProvisioner(s.StateInfo(c))
	c.Assert(err, IsNil)
	defer s.stopProvisioner(c, p)
	// Twiddle with the environment configuration.
	env, err := s.St.EnvironConfig()
	c.Assert(err, IsNil)
	env.Set("name", "testing2")
	_, err = env.Write()
	c.Assert(err, IsNil)
	env.Set("name", "testing3")
	_, err = env.Write()
}

func (s *ProvisioningSuite) TestProvisionerStopOnStateClose(c *C) {
	p, err := NewProvisioner(s.StateInfo(c))
	c.Assert(err, IsNil)

	p.st.Close()

	// must use Check to avoid leaking PA
	c.Check(p.Wait(), ErrorMatches, ".* zookeeper is closing")
	c.Assert(p.Stop(), ErrorMatches, ".* zookeeper is closing")
}

// Start and stop one machine, watch the PA.
func (s *ProvisioningSuite) TestSimple(c *C) {
	p, err := NewProvisioner(s.StateInfo(c))
	c.Assert(err, IsNil)
	defer s.stopProvisioner(c, p)

	// place a new machine into the state
	m, err := s.St.AddMachine()
	c.Assert(err, IsNil)

	s.checkStartInstance(c, m)

	// now remove it
	c.Assert(s.St.RemoveMachine(m.Id()), IsNil)

	// watch the PA remove it
	s.checkStopInstance(c)
	s.checkMachineId(c, m, nil)
}

func (s *ProvisioningSuite) TestProvisioningDoesNotOccurWithAnInvalidEnvironment(c *C) {
	err := s.invalidateEnvironment()
	c.Assert(err, IsNil)

	p, err := NewProvisioner(s.StateInfo(c))
	c.Assert(err, IsNil)
	defer s.stopProvisioner(c, p)

	// try to create a machine
	_, err = s.St.AddMachine()
	c.Assert(err, IsNil)

	// the PA should not create it
	s.checkNotStartInstance(c)
}

func (s *ProvisioningSuite) TestProvisioningOccursWithFixedEnvironment(c *C) {
	err := s.invalidateEnvironment()
	c.Assert(err, IsNil)

	p, err := NewProvisioner(s.StateInfo(c))
	c.Assert(err, IsNil)
	defer s.stopProvisioner(c, p)

	// try to create a machine
	m, err := s.St.AddMachine()
	c.Assert(err, IsNil)

	// the PA should not create it
	s.checkNotStartInstance(c)

	err = s.fixEnvironment()
	c.Assert(err, IsNil)

	s.checkStartInstance(c, m)
}

func (s *ProvisioningSuite) TestProvisioningDoesOccurAfterInvalidEnvironmentPublished(c *C) {
	p, err := NewProvisioner(s.StateInfo(c))
	c.Assert(err, IsNil)
	defer s.stopProvisioner(c, p)

	// place a new machine into the state
	m, err := s.St.AddMachine()
	c.Assert(err, IsNil)

	s.checkStartInstance(c, m)

	err = s.invalidateEnvironment()
	c.Assert(err, IsNil)

	// create a second machine
	m, err = s.St.AddMachine()
	c.Assert(err, IsNil)

	// the PA should create it using the old environment
	s.checkStartInstance(c, m)
}

func (s *ProvisioningSuite) TestProvisioningDoesNotProvisionTheSameMachineAfterRestart(c *C) {
	p, err := NewProvisioner(s.StateInfo(c))
	c.Check(err, IsNil)
	// we are not using defer s.stopProvisioner(c, p) because we need to control when 
	// the PA is restarted in this test. tf. Methods like Fatalf and Assert should not be used.

	// create a machine
	m, err := s.St.AddMachine()
	c.Check(err, IsNil)

	s.checkStartInstance(c, m)

	// restart the PA
	c.Check(p.Stop(), IsNil)

	p, err = NewProvisioner(s.StateInfo(c))
	c.Check(err, IsNil)

	// check that there is only one machine known
	machines, err := s.St.AllMachines()
	c.Check(err, IsNil)
	c.Check(len(machines), Equals, 1)
	c.Check(machines[0].Id(), Equals, 0)

	// the PA should not create it a second time
	s.checkNotStartInstance(c)

	c.Assert(p.Stop(), IsNil)
}

func (s *ProvisioningSuite) TestProvisioningStopsUnknownInstances(c *C) {
	p, err := NewProvisioner(s.StateInfo(c))
	c.Check(err, IsNil)
	// we are not using defer s.stopProvisioner(c, p) because we need to control when 
	// the PA is restarted in this test. Methods like Fatalf and Assert should not be used.

	// create a machine
	m, err := s.St.AddMachine()
	c.Check(err, IsNil)

	s.checkStartInstance(c, m)

	// create a second machine
	m, err = s.St.AddMachine()
	c.Check(err, IsNil)

	s.checkStartInstance(c, m)

	// stop the PA
	c.Check(p.Stop(), IsNil)

	// remove the machine
	err = s.St.RemoveMachine(m.Id())
	c.Check(err, IsNil)

	// start a new provisioner
	p, err = NewProvisioner(s.StateInfo(c))
	c.Check(err, IsNil)

	s.checkStopInstance(c)

	c.Assert(p.Stop(), IsNil)
}

// This check is different from the one above as it catches the edge case
// where the final machine has been removed from the state while the PA was 
// not running. 
func (s *ProvisioningSuite) TestProvisioningStopsOnlyUnknownInstances(c *C) {
	p, err := NewProvisioner(s.StateInfo(c))
	c.Check(err, IsNil)
	// we are not using defer s.stopProvisioner(c, p) because we need to control when 
	// the PA is restarted in this test. Methods like Fatalf and Assert should not be used.

	// create a machine
	m, err := s.St.AddMachine()
	c.Check(err, IsNil)

	s.checkStartInstance(c, m)

	// stop the PA
	c.Check(p.Stop(), IsNil)

	// remove the machine
	err = s.St.RemoveMachine(m.Id())
	c.Check(err, IsNil)

	machines, err := s.St.AllMachines()
	c.Check(err, IsNil)
	c.Check(len(machines), Equals, 0) // it's really gone   

	// start a new provisioner
	p, err = NewProvisioner(s.StateInfo(c))
	c.Check(err, IsNil)

	s.checkStopInstance(c)

	c.Assert(p.Stop(), IsNil)
}

func (s *ProvisioningSuite) TestProvisioningRecoversAfterInvalidEnvironmentPublished(c *C) {
	p, err := NewProvisioner(s.StateInfo(c))
	c.Assert(err, IsNil)
	defer s.stopProvisioner(c, p)

	// place a new machine into the state
	m, err := s.St.AddMachine()
	c.Assert(err, IsNil)

	s.checkStartInstance(c, m)

	err = s.invalidateEnvironment()
	c.Assert(err, IsNil)

	// create a second machine
	m, err = s.St.AddMachine()
	c.Assert(err, IsNil)

	// the PA should create it using the old environment
	s.checkStartInstance(c, m)

	err = s.fixEnvironment()
	c.Assert(err, IsNil)

	// create a third machine
	m, err = s.St.AddMachine()
	c.Assert(err, IsNil)

	// the PA should create it using the new environment
	s.checkStartInstance(c, m)
}
