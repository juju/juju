package firewaller_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/testing"
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
		if time.Since(start) > 500*time.Millisecond {
			c.Fatalf("timed out: expected %q; got %q", expected, got)
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	panic("unreachable")
}

type FirewallerSuite struct {
	coretesting.LoggingSuite
	testing.StateSuite
	environ environs.Environ
	op      <-chan dummy.Operation
	charm   *state.Charm
}

var _ = Suite(&FirewallerSuite{})

func (s *FirewallerSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)

	var err error
	config := map[string]interface{}{
		"name":            "testing",
		"type":            "dummy",
		"zookeeper":       true,
		"authorized-keys": "i-am-a-key",
	}
	s.environ, err = environs.NewFromAttrs(config)
	c.Assert(err, IsNil)
	err = s.environ.Bootstrap(false)
	c.Assert(err, IsNil)

	// Sanity check
	info, err := s.environ.StateInfo()
	c.Assert(err, IsNil)
	c.Assert(info, DeepEquals, s.StateInfo(c))

	s.StateSuite.SetUpTest(c)

	s.charm = s.AddTestingCharm(c, "dummy")
}

func (s *FirewallerSuite) TearDownTest(c *C) {
	dummy.Reset()
	s.LoggingSuite.TearDownTest(c)
}

func (s *FirewallerSuite) TestStartStop(c *C) {
	fw, err := firewaller.NewFirewaller(s.State)
	c.Assert(err, IsNil)
	c.Assert(fw.Stop(), IsNil)
}

func (s *FirewallerSuite) TestNotExposedService(c *C) {
	fw, err := firewaller.NewFirewaller(s.State)
	c.Assert(err, IsNil)
	defer func() { c.Assert(fw.Stop(), IsNil) }()

	m, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	err = m.SetInstanceId("testing-0")
	c.Assert(err, IsNil)
	inst, err := s.environ.StartInstance(m.Id(), s.StateInfo(c))
	c.Assert(err, IsNil)

	svc, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	u, err := svc.AddUnit()
	c.Assert(err, IsNil)
	err = u.AssignToMachine(m)
	c.Assert(err, IsNil)
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
	fw, err := firewaller.NewFirewaller(s.State)
	c.Assert(err, IsNil)
	defer func() { c.Assert(fw.Stop(), IsNil) }()

	m, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	err = m.SetInstanceId("testing-0")
	c.Assert(err, IsNil)
	inst, err := s.environ.StartInstance(m.Id(), s.StateInfo(c))
	c.Assert(err, IsNil)

	svc, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	err = svc.SetExposed()
	c.Assert(err, IsNil)
	u, err := svc.AddUnit()
	c.Assert(err, IsNil)
	err = u.AssignToMachine(m)
	c.Assert(err, IsNil)
	err = u.OpenPort("tcp", 80)
	c.Assert(err, IsNil)
	err = u.OpenPort("tcp", 8080)
	c.Assert(err, IsNil)

	assertPorts(c, inst, m.Id(), []state.Port{{"tcp", 80}, {"tcp", 8080}})

	err = u.ClosePort("tcp", 80)
	c.Assert(err, IsNil)

	assertPorts(c, inst, m.Id(), []state.Port{{"tcp", 8080}})
}

func (s *FirewallerSuite) TestMultipleUnits(c *C) {
	fw, err := firewaller.NewFirewaller(s.State)
	c.Assert(err, IsNil)
	defer func() { c.Assert(fw.Stop(), IsNil) }()

	m1, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	err = m1.SetInstanceId("testing-0")
	c.Assert(err, IsNil)
	inst1, err := s.environ.StartInstance(m1.Id(), s.StateInfo(c))
	c.Assert(err, IsNil)
	m2, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	err = m2.SetInstanceId("testing-1")
	c.Assert(err, IsNil)
	inst2, err := s.environ.StartInstance(m2.Id(), s.StateInfo(c))
	c.Assert(err, IsNil)

	svc, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	err = svc.SetExposed()
	c.Assert(err, IsNil)
	u1, err := svc.AddUnit()
	c.Assert(err, IsNil)
	err = u1.AssignToMachine(m1)
	c.Assert(err, IsNil)
	err = u1.OpenPort("tcp", 80)
	c.Assert(err, IsNil)
	u2, err := svc.AddUnit()
	c.Assert(err, IsNil)
	err = u2.AssignToMachine(m2)
	c.Assert(err, IsNil)
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
	m, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	err = m.SetInstanceId("testing-0")
	c.Assert(err, IsNil)
	inst, err := s.environ.StartInstance(m.Id(), s.StateInfo(c))
	c.Assert(err, IsNil)

	svc, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	err = svc.SetExposed()
	c.Assert(err, IsNil)
	u, err := svc.AddUnit()
	c.Assert(err, IsNil)
	err = u.AssignToMachine(m)
	c.Assert(err, IsNil)
	err = u.OpenPort("tcp", 80)
	c.Assert(err, IsNil)
	err = u.OpenPort("tcp", 8080)
	c.Assert(err, IsNil)

	// Nothing open without firewaller.
	assertPorts(c, inst, m.Id(), nil)

	// Starting the firewaller opens the ports.
	fw, err := firewaller.NewFirewaller(s.State)
	c.Assert(err, IsNil)
	defer func() { c.Assert(fw.Stop(), IsNil) }()

	assertPorts(c, inst, m.Id(), []state.Port{{"tcp", 80}, {"tcp", 8080}})
}

func (s *FirewallerSuite) TestFirewallerStartWithPartialState(c *C) {
	m, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	err = m.SetInstanceId("testing-0")
	c.Assert(err, IsNil)
	inst, err := s.environ.StartInstance(m.Id(), s.StateInfo(c))
	c.Assert(err, IsNil)

	svc, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	err = svc.SetExposed()
	c.Assert(err, IsNil)

	// Starting the firewaller, no open ports.
	fw, err := firewaller.NewFirewaller(s.State)
	c.Assert(err, IsNil)
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
	fw, err := firewaller.NewFirewaller(s.State)
	c.Assert(err, IsNil)
	defer func() { c.Assert(fw.Stop(), IsNil) }()

	m, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	err = m.SetInstanceId("testing-0")
	c.Assert(err, IsNil)
	inst, err := s.environ.StartInstance(m.Id(), s.StateInfo(c))
	c.Assert(err, IsNil)
	svc, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	u, err := svc.AddUnit()
	c.Assert(err, IsNil)
	err = u.AssignToMachine(m)
	c.Assert(err, IsNil)
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
	fw, err := firewaller.NewFirewaller(s.State)
	c.Assert(err, IsNil)
	fw.CloseState()
	c.Check(fw.Wait(), ErrorMatches, ".* zookeeper is closing")
	c.Assert(fw.Stop(), ErrorMatches, ".* zookeeper is closing")
}
