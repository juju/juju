package firewaller_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/worker/firewaller"
	"reflect"
	stdtesting "testing"
	"time"
)

func TestPackage(t *stdtesting.T) {
	coretesting.ZkTestPackage(t)
}

// assertPorts retrieves the open ports of the instance and compares them
// to the expected. 
func assertPorts(c *C, inst environs.Instance, machineId int, expected []state.Port) {
	start := time.Now()
	for {
		got, err := inst.Ports(machineId)
		if err != nil {
			c.Fatal(err)
			return
		}
		state.SortPorts(got)
		state.SortPorts(expected)
		if reflect.DeepEqual(got, expected) {
			c.Succeed()
			return
		}
		if time.Since(start) > 5000*time.Millisecond {
			c.Fatalf("timed out: expected %q; got %q", expected, got)
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	panic("unreachable")
}

type FirewallerSuite struct {
	testing.JujuConnSuite
	op    <-chan dummy.Operation
	charm *state.Charm
}

var _ = Suite(&FirewallerSuite{})

func (s *FirewallerSuite) SetUpTest(c *C) {
	s.JujuConnSuite.SetUpTest(c)
	s.charm = s.AddTestingCharm(c, "dummy")
}

func (s *FirewallerSuite) TestStartStop(c *C) {
	fw := firewaller.NewFirewaller(s.State)
	c.Assert(fw.Stop(), IsNil)
}

func (s *FirewallerSuite) addUnit(c *C, svc *state.Service) (*state.Unit, *state.Machine) {
	units, err := s.Conn.AddUnits(svc, 1)
	c.Assert(err, IsNil)
	u := units[0]
	id, err := u.AssignedMachineId()
	c.Assert(err, IsNil)
	m, err := s.State.Machine(id)
	c.Assert(err, IsNil)
	return u, m
}

// startInstance starts a new instance for the given machine.
func (s *FirewallerSuite) startInstance(c *C, m *state.Machine) environs.Instance {
	inst, err := s.Conn.Environ.StartInstance(m.Id(), s.StateInfo(c), nil)
	c.Assert(err, IsNil)
	err = m.SetInstanceId(inst.Id())
	c.Assert(err, IsNil)

	// Check that instance has actually been started
	config, err := s.State.EnvironConfig()
	c.Assert(err, IsNil)
	env, err := environs.NewFromAttrs(config.Map())
	_, err = env.Instances([]string{inst.Id()})
	c.Assert(err, IsNil)
	return inst
}

func (s *FirewallerSuite) TestNotExposedService(c *C) {
	fw := firewaller.NewFirewaller(s.State)
	defer func() { c.Assert(fw.Stop(), IsNil) }()

	svc, err := s.Conn.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	u, m := s.addUnit(c, svc)
	inst := s.startInstance(c, m)

	err = u.OpenPort("tcp", 80)
	c.Assert(err, IsNil)
	err = u.OpenPort("tcp", 8080)
	c.Assert(err, IsNil)

	assertPorts(c, inst, m.Id(), nil)

	err = u.ClosePort("tcp", 80)
	c.Assert(err, IsNil)

	assertPorts(c, inst, m.Id(), nil)
}

func (s *FirewallerSuite) TestExposedService(c *C) {
	fw := firewaller.NewFirewaller(s.State)
	defer func() { c.Assert(fw.Stop(), IsNil) }()

	svc, err := s.Conn.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)

	err = svc.SetExposed()
	c.Assert(err, IsNil)
	u, m := s.addUnit(c, svc)
	inst := s.startInstance(c, m)

	err = u.OpenPort("tcp", 80)
	c.Assert(err, IsNil)
	err = u.OpenPort("tcp", 8080)
	c.Assert(err, IsNil)

	assertPorts(c, inst, m.Id(), []state.Port{{"tcp", 80}, {"tcp", 8080}})

	err = u.ClosePort("tcp", 80)
	c.Assert(err, IsNil)

	assertPorts(c, inst, m.Id(), []state.Port{{"tcp", 8080}})
}

func (s *FirewallerSuite) TestMachineWithoutInstanceId(c *C) {
	fw := firewaller.NewFirewaller(s.State)
	defer func() { c.Assert(fw.Stop(), IsNil) }()

	svc, err := s.Conn.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	err = svc.SetExposed()
	c.Assert(err, IsNil)
	// add a unit but don't start its instance yet.
	u1, m1 := s.addUnit(c, svc)

	// add another unit and start its instance, so that
	// we're sure the firewaller has seen the first instance.
	u2, m2 := s.addUnit(c, svc)
	inst2 := s.startInstance(c, m2)
	err = u2.OpenPort("tcp", 80)
	c.Assert(err, IsNil)
	assertPorts(c, inst2, m2.Id(), []state.Port{{"tcp", 80}})

	inst1 := s.startInstance(c, m1)
	err = u1.OpenPort("tcp", 8080)
	c.Assert(err, IsNil)
	assertPorts(c, inst1, m1.Id(), []state.Port{{"tcp", 8080}})
}

func (s *FirewallerSuite) TestMultipleUnits(c *C) {
	fw := firewaller.NewFirewaller(s.State)
	defer func() { c.Assert(fw.Stop(), IsNil) }()

	svc, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	err = svc.SetExposed()
	c.Assert(err, IsNil)

	u1, m1 := s.addUnit(c, svc)
	inst1 := s.startInstance(c, m1)
	err = u1.OpenPort("tcp", 80)
	c.Assert(err, IsNil)

	u2, m2 := s.addUnit(c, svc)
	inst2 := s.startInstance(c, m2)
	err = u2.OpenPort("tcp", 80)
	c.Assert(err, IsNil)

	assertPorts(c, inst1, m1.Id(), []state.Port{{"tcp", 80}})
	assertPorts(c, inst2, m2.Id(), []state.Port{{"tcp", 80}})

	err = u1.ClosePort("tcp", 80)
	c.Assert(err, IsNil)
	err = u2.ClosePort("tcp", 80)
	c.Assert(err, IsNil)

	assertPorts(c, inst1, m1.Id(), nil)
	assertPorts(c, inst2, m2.Id(), nil)
}

func (s *FirewallerSuite) TestFirewallerStartWithState(c *C) {
	svc, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	err = svc.SetExposed()
	c.Assert(err, IsNil)
	u, m := s.addUnit(c, svc)
	inst := s.startInstance(c, m)

	err = u.OpenPort("tcp", 80)
	c.Assert(err, IsNil)
	err = u.OpenPort("tcp", 8080)
	c.Assert(err, IsNil)

	// Nothing open without firewaller.
	assertPorts(c, inst, m.Id(), nil)

	// Starting the firewaller opens the ports.
	fw := firewaller.NewFirewaller(s.State)
	defer func() { c.Assert(fw.Stop(), IsNil) }()

	assertPorts(c, inst, m.Id(), []state.Port{{"tcp", 80}, {"tcp", 8080}})

	err = svc.SetExposed()
	c.Assert(err, IsNil)
}

func (s *FirewallerSuite) TestFirewallerStartWithPartialState(c *C) {
	m, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	inst, err := s.Conn.Environ.StartInstance(m.Id(), s.StateInfo(c), nil)
	c.Assert(err, IsNil)
	err = m.SetInstanceId(inst.Id())
	c.Assert(err, IsNil)

	svc, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	err = svc.SetExposed()
	c.Assert(err, IsNil)

	// Starting the firewaller, no open ports.
	fw := firewaller.NewFirewaller(s.State)
	defer func() { c.Assert(fw.Stop(), IsNil) }()

	assertPorts(c, inst, m.Id(), nil)

	// Complete steps to open port.
	u, err := svc.AddUnit()
	c.Assert(err, IsNil)
	err = u.AssignToMachine(m)
	c.Assert(err, IsNil)
	err = u.OpenPort("tcp", 80)
	c.Assert(err, IsNil)

	assertPorts(c, inst, m.Id(), []state.Port{{"tcp", 80}})
}

func (s *FirewallerSuite) TestSetClearExposedService(c *C) {
	fw := firewaller.NewFirewaller(s.State)
	defer func() { c.Assert(fw.Stop(), IsNil) }()

	svc, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)

	u, m := s.addUnit(c, svc)
	inst := s.startInstance(c, m)
	err = u.OpenPort("tcp", 80)
	c.Assert(err, IsNil)
	err = u.OpenPort("tcp", 8080)
	c.Assert(err, IsNil)

	// Not exposed service, so no open port.
	assertPorts(c, inst, m.Id(), nil)

	// SeExposed opens the ports.
	err = svc.SetExposed()
	c.Assert(err, IsNil)

	assertPorts(c, inst, m.Id(), []state.Port{{"tcp", 80}, {"tcp", 8080}})

	// ClearExposed closes the ports again.
	err = svc.ClearExposed()
	c.Assert(err, IsNil)

	assertPorts(c, inst, m.Id(), nil)
}

func (s *FirewallerSuite) TestFirewallerStopOnStateClose(c *C) {
	st, err := state.Open(s.StateInfo(c))
	c.Assert(err, IsNil)
	fw := firewaller.NewFirewaller(st)
	st.Close()
	c.Check(fw.Wait(), ErrorMatches, ".* zookeeper is closing")
	c.Assert(fw.Stop(), ErrorMatches, ".* zookeeper is closing")
}
