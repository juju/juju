package firewaller_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
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
	coretesting.MgoTestPackage(t)
}

type FirewallerSuite struct {
	testing.JujuConnSuite
	op    <-chan dummy.Operation
	charm *state.Charm
}

// assertPorts retrieves the open ports of the instance and compares them
// to the expected. 
func (s *FirewallerSuite) assertPorts(c *C, inst environs.Instance, machineId int, expected []state.Port) {
	s.State.StartSync()
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
		if time.Since(start) > 5*time.Second {
			c.Fatalf("timed out: expected %q; got %q", expected, got)
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	panic("unreachable")
}

// assertEnvironPorts retrieves the open ports of environment and compares them
// to the expected. 
func (s *FirewallerSuite) assertEnvironPorts(c *C, expected []state.Port) {
	s.State.StartSync()
	start := time.Now()
	for {
		got, err := s.Conn.Environ.Ports()
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
		if time.Since(start) > 5*time.Second {
			c.Fatalf("timed out: expected %q; got %q", expected, got)
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	panic("unreachable")
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
	inst, err := s.Conn.Environ.StartInstance(m.Id(), testing.InvalidStateInfo(m.Id()), nil)
	c.Assert(err, IsNil)
	err = m.SetInstanceId(inst.Id())
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

	s.assertPorts(c, inst, m.Id(), nil)

	err = u.ClosePort("tcp", 80)
	c.Assert(err, IsNil)

	s.assertPorts(c, inst, m.Id(), nil)
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

	s.assertPorts(c, inst, m.Id(), []state.Port{{"tcp", 80}, {"tcp", 8080}})

	err = u.ClosePort("tcp", 80)
	c.Assert(err, IsNil)

	s.assertPorts(c, inst, m.Id(), []state.Port{{"tcp", 8080}})
}

func (s *FirewallerSuite) TestMultipleExposedServices(c *C) {
	fw := firewaller.NewFirewaller(s.State)
	defer func() { c.Assert(fw.Stop(), IsNil) }()

	svc1, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	err = svc1.SetExposed()
	c.Assert(err, IsNil)

	u1, m1 := s.addUnit(c, svc1)
	inst1 := s.startInstance(c, m1)
	err = u1.OpenPort("tcp", 80)
	c.Assert(err, IsNil)
	err = u1.OpenPort("tcp", 8080)
	c.Assert(err, IsNil)

	svc2, err := s.State.AddService("mysql", s.charm)
	c.Assert(err, IsNil)
	err = svc2.SetExposed()
	c.Assert(err, IsNil)

	u2, m2 := s.addUnit(c, svc2)
	inst2 := s.startInstance(c, m2)
	err = u2.OpenPort("tcp", 3306)
	c.Assert(err, IsNil)

	s.assertPorts(c, inst1, m1.Id(), []state.Port{{"tcp", 80}, {"tcp", 8080}})
	s.assertPorts(c, inst2, m2.Id(), []state.Port{{"tcp", 3306}})

	err = u1.ClosePort("tcp", 80)
	c.Assert(err, IsNil)
	err = u2.ClosePort("tcp", 3306)
	c.Assert(err, IsNil)

	s.assertPorts(c, inst1, m1.Id(), []state.Port{{"tcp", 8080}})
	s.assertPorts(c, inst2, m2.Id(), nil)
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
	s.assertPorts(c, inst2, m2.Id(), []state.Port{{"tcp", 80}})

	inst1 := s.startInstance(c, m1)
	err = u1.OpenPort("tcp", 8080)
	c.Assert(err, IsNil)
	s.assertPorts(c, inst1, m1.Id(), []state.Port{{"tcp", 8080}})
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

	s.assertPorts(c, inst1, m1.Id(), []state.Port{{"tcp", 80}})
	s.assertPorts(c, inst2, m2.Id(), []state.Port{{"tcp", 80}})

	err = u1.ClosePort("tcp", 80)
	c.Assert(err, IsNil)
	err = u2.ClosePort("tcp", 80)
	c.Assert(err, IsNil)

	s.assertPorts(c, inst1, m1.Id(), nil)
	s.assertPorts(c, inst2, m2.Id(), nil)
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
	s.assertPorts(c, inst, m.Id(), nil)

	// Starting the firewaller opens the ports.
	fw := firewaller.NewFirewaller(s.State)
	defer func() { c.Assert(fw.Stop(), IsNil) }()

	s.assertPorts(c, inst, m.Id(), []state.Port{{"tcp", 80}, {"tcp", 8080}})

	err = svc.SetExposed()
	c.Assert(err, IsNil)
}

func (s *FirewallerSuite) TestFirewallerStartWithPartialState(c *C) {
	m, err := s.State.AddMachine(state.MachinerWorker)
	c.Assert(err, IsNil)
	inst, err := s.Conn.Environ.StartInstance(m.Id(), testing.InvalidStateInfo(m.Id()), nil)
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

	s.assertPorts(c, inst, m.Id(), nil)

	// Complete steps to open port.
	u, err := svc.AddUnit()
	c.Assert(err, IsNil)
	err = u.AssignToMachine(m)
	c.Assert(err, IsNil)
	err = u.OpenPort("tcp", 80)
	c.Assert(err, IsNil)

	s.assertPorts(c, inst, m.Id(), []state.Port{{"tcp", 80}})
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
	s.assertPorts(c, inst, m.Id(), nil)

	// SeExposed opens the ports.
	err = svc.SetExposed()
	c.Assert(err, IsNil)

	s.assertPorts(c, inst, m.Id(), []state.Port{{"tcp", 80}, {"tcp", 8080}})

	// ClearExposed closes the ports again.
	err = svc.ClearExposed()
	c.Assert(err, IsNil)

	s.assertPorts(c, inst, m.Id(), nil)
}

func (s *FirewallerSuite) TestRemoveUnit(c *C) {
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

	s.assertPorts(c, inst1, m1.Id(), []state.Port{{"tcp", 80}})
	s.assertPorts(c, inst2, m2.Id(), []state.Port{{"tcp", 80}})

	// Remove unit.
	err = u1.EnsureDead()
	c.Assert(err, IsNil)
	err = svc.RemoveUnit(u1)
	c.Assert(err, IsNil)

	s.assertPorts(c, inst1, m1.Id(), nil)
	s.assertPorts(c, inst2, m2.Id(), []state.Port{{"tcp", 80}})
}

func (s *FirewallerSuite) TestRemoveService(c *C) {
	fw := firewaller.NewFirewaller(s.State)
	defer func() { c.Assert(fw.Stop(), IsNil) }()

	svc, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	err = svc.SetExposed()
	c.Assert(err, IsNil)

	u, m := s.addUnit(c, svc)
	inst := s.startInstance(c, m)
	err = u.OpenPort("tcp", 80)
	c.Assert(err, IsNil)

	s.assertPorts(c, inst, m.Id(), []state.Port{{"tcp", 80}})

	// Remove service.
	err = u.EnsureDead()
	c.Assert(err, IsNil)
	err = svc.RemoveUnit(u)
	c.Assert(err, IsNil)
	err = svc.EnsureDead()
	c.Assert(err, IsNil)
	err = s.State.RemoveService(svc)
	c.Assert(err, IsNil)

	s.assertPorts(c, inst, m.Id(), nil)
}

func (s *FirewallerSuite) TestRemoveMultipleServices(c *C) {
	fw := firewaller.NewFirewaller(s.State)
	defer func() { c.Assert(fw.Stop(), IsNil) }()

	svc1, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	err = svc1.SetExposed()
	c.Assert(err, IsNil)

	u1, m1 := s.addUnit(c, svc1)
	inst1 := s.startInstance(c, m1)
	err = u1.OpenPort("tcp", 80)
	c.Assert(err, IsNil)

	svc2, err := s.State.AddService("mysql", s.charm)
	c.Assert(err, IsNil)
	err = svc2.SetExposed()
	c.Assert(err, IsNil)

	u2, m2 := s.addUnit(c, svc2)
	inst2 := s.startInstance(c, m2)
	err = u2.OpenPort("tcp", 3306)
	c.Assert(err, IsNil)

	s.assertPorts(c, inst1, m1.Id(), []state.Port{{"tcp", 80}})
	s.assertPorts(c, inst2, m2.Id(), []state.Port{{"tcp", 3306}})

	// Remove services.
	err = u2.EnsureDead()
	c.Assert(err, IsNil)
	err = svc2.RemoveUnit(u2)
	c.Assert(err, IsNil)
	err = svc2.EnsureDead()
	c.Assert(err, IsNil)
	err = s.State.RemoveService(svc2)
	c.Assert(err, IsNil)

	err = u1.EnsureDead()
	c.Assert(err, IsNil)
	err = svc1.RemoveUnit(u1)
	c.Assert(err, IsNil)
	err = svc1.EnsureDead()
	c.Assert(err, IsNil)
	err = s.State.RemoveService(svc1)
	c.Assert(err, IsNil)

	s.assertPorts(c, inst1, m1.Id(), nil)
	s.assertPorts(c, inst2, m2.Id(), nil)
}

func (s *FirewallerSuite) TestDeadMachine(c *C) {
	fw := firewaller.NewFirewaller(s.State)
	defer func() { c.Assert(fw.Stop(), IsNil) }()

	svc, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	err = svc.SetExposed()
	c.Assert(err, IsNil)

	u, m := s.addUnit(c, svc)
	inst := s.startInstance(c, m)
	err = u.OpenPort("tcp", 80)
	c.Assert(err, IsNil)

	s.assertPorts(c, inst, m.Id(), []state.Port{{"tcp", 80}})

	// Remove unit and service, also tested without. Has no effect.
	err = u.EnsureDead()
	c.Assert(err, IsNil)
	err = svc.RemoveUnit(u)
	c.Assert(err, IsNil)
	err = svc.EnsureDead()
	c.Assert(err, IsNil)
	err = s.State.RemoveService(svc)
	c.Assert(err, IsNil)

	// Kill machine.
	err = m.EnsureDead()
	c.Assert(err, IsNil)

	s.assertPorts(c, inst, m.Id(), nil)
}

func (s *FirewallerSuite) TestRemoveMachine(c *C) {
	fw := firewaller.NewFirewaller(s.State)
	defer func() { c.Assert(fw.Stop(), IsNil) }()

	svc, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	err = svc.SetExposed()
	c.Assert(err, IsNil)

	u, m := s.addUnit(c, svc)
	inst := s.startInstance(c, m)
	err = u.OpenPort("tcp", 80)
	c.Assert(err, IsNil)

	s.assertPorts(c, inst, m.Id(), []state.Port{{"tcp", 80}})

	// Remove unit.
	err = u.EnsureDead()
	c.Assert(err, IsNil)
	err = svc.RemoveUnit(u)
	c.Assert(err, IsNil)

	// Remove machine. Nothing bad should happen, but can't
	// assert port state since the machine must have been
	// destroyed and we lost its reference.
	err = m.EnsureDead()
	c.Assert(err, IsNil)
	err = s.State.RemoveMachine(m.Id())
	c.Assert(err, IsNil)
}

func (s *FirewallerSuite) TestGlobalMode(c *C) {
	// Change configuration.
	oldConfig := s.Conn.Environ.Config()
	defer func() {
		attrs := oldConfig.AllAttrs()
		attrs["admin-secret"] = ""
		oldConfig, err := oldConfig.Apply(attrs)
		c.Assert(err, IsNil)
		err = s.Conn.Environ.SetConfig(oldConfig)
		c.Assert(err, IsNil)
		err = s.State.SetEnvironConfig(oldConfig)
		c.Assert(err, IsNil)
	}()

	attrs := s.Conn.Environ.Config().AllAttrs()
	attrs["firewall-mode"] = config.FwGlobal
	attrs["admin-secret"] = ""
	newConfig, err := s.Conn.Environ.Config().Apply(attrs)
	c.Assert(err, IsNil)
	err = s.Conn.Environ.SetConfig(newConfig)
	c.Assert(err, IsNil)
	err = s.State.SetEnvironConfig(newConfig)
	c.Assert(err, IsNil)

	// Start firewall and open ports.
	fw := firewaller.NewFirewaller(s.State)
	defer func() { c.Assert(fw.Stop(), IsNil) }()

	svc1, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	err = svc1.SetExposed()
	c.Assert(err, IsNil)

	u1, m1 := s.addUnit(c, svc1)
	s.startInstance(c, m1)
	err = u1.OpenPort("tcp", 80)
	c.Assert(err, IsNil)
	err = u1.OpenPort("tcp", 8080)
	c.Assert(err, IsNil)

	svc2, err := s.State.AddService("moinmoin", s.charm)
	c.Assert(err, IsNil)
	err = svc2.SetExposed()
	c.Assert(err, IsNil)

	u2, m2 := s.addUnit(c, svc2)
	s.startInstance(c, m2)
	err = u2.OpenPort("tcp", 80)
	c.Assert(err, IsNil)

	// Check that all expected ports are open in environment.
	s.assertEnvironPorts(c, []state.Port{{"tcp", 80}, {"tcp", 8080}})

	// Check that closing a multiple used port on one machine lets it untouched.
	err = u1.ClosePort("tcp", 80)
	c.Assert(err, IsNil)
	s.assertEnvironPorts(c, []state.Port{{"tcp", 80}, {"tcp", 8080}})

	// Check that closing a single used port is closed.
	err = u1.ClosePort("tcp", 8080)
	c.Assert(err, IsNil)
	s.assertEnvironPorts(c, []state.Port{{"tcp", 80}})

	// Check that closing the last usage of a port closes it globally.
	err = u2.ClosePort("tcp", 80)
	c.Assert(err, IsNil)
	s.assertEnvironPorts(c, nil)
}
