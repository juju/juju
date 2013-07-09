// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/worker"
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

var _ worker.Worker = (*firewaller.Firewaller)(nil)

// assertPorts retrieves the open ports of the instance and compares them
// to the expected.
func (s *FirewallerSuite) assertPorts(c *C, inst instance.Instance, machineId string, expected []instance.Port) {
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
func (s *FirewallerSuite) assertEnvironPorts(c *C, expected []instance.Port) {
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
	units, err := s.Conn.AddUnits(svc, 1, "", "")
	c.Assert(err, IsNil)
	u := units[0]
	id, err := u.AssignedMachineId()
	c.Assert(err, IsNil)
	m, err := s.State.Machine(id)
	c.Assert(err, IsNil)
	return u, m
}

func (s *FirewallerSuite) setGlobalMode(c *C) func(*C) {
	oldConfig := s.Conn.Environ.Config()
	restore := func(rc *C) {
		attrs := oldConfig.AllAttrs()
		attrs["admin-secret"] = ""
		oldConfig, err := oldConfig.Apply(attrs)
		rc.Assert(err, IsNil)
		err = s.Conn.Environ.SetConfig(oldConfig)
		rc.Assert(err, IsNil)
		err = s.State.SetEnvironConfig(oldConfig)
		rc.Assert(err, IsNil)
	}

	attrs := s.Conn.Environ.Config().AllAttrs()
	attrs["firewall-mode"] = config.FwGlobal
	attrs["admin-secret"] = ""
	newConfig, err := s.Conn.Environ.Config().Apply(attrs)
	c.Assert(err, IsNil)
	err = s.Conn.Environ.SetConfig(newConfig)
	c.Assert(err, IsNil)
	err = s.State.SetEnvironConfig(newConfig)
	c.Assert(err, IsNil)

	return restore
}

// startInstance starts a new instance for the given machine.
func (s *FirewallerSuite) startInstance(c *C, m *state.Machine) instance.Instance {
	inst, hc := testing.StartInstance(c, s.Conn.Environ, m.Id())
	err := m.SetProvisioned(inst.Id(), "fake_nonce", hc)
	c.Assert(err, IsNil)
	return inst
}

func (s *FirewallerSuite) TestNotExposedService(c *C) {
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

	s.assertPorts(c, inst, m.Id(), nil)

	err = u.ClosePort("tcp", 80)
	c.Assert(err, IsNil)

	s.assertPorts(c, inst, m.Id(), nil)
}

func (s *FirewallerSuite) TestExposedService(c *C) {
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
	err = u.OpenPort("tcp", 8080)
	c.Assert(err, IsNil)

	s.assertPorts(c, inst, m.Id(), []instance.Port{{"tcp", 80}, {"tcp", 8080}})

	err = u.ClosePort("tcp", 80)
	c.Assert(err, IsNil)

	s.assertPorts(c, inst, m.Id(), []instance.Port{{"tcp", 8080}})
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

	s.assertPorts(c, inst1, m1.Id(), []instance.Port{{"tcp", 80}, {"tcp", 8080}})
	s.assertPorts(c, inst2, m2.Id(), []instance.Port{{"tcp", 3306}})

	err = u1.ClosePort("tcp", 80)
	c.Assert(err, IsNil)
	err = u2.ClosePort("tcp", 3306)
	c.Assert(err, IsNil)

	s.assertPorts(c, inst1, m1.Id(), []instance.Port{{"tcp", 8080}})
	s.assertPorts(c, inst2, m2.Id(), nil)
}

func (s *FirewallerSuite) TestMachineWithoutInstanceId(c *C) {
	fw := firewaller.NewFirewaller(s.State)
	defer func() { c.Assert(fw.Stop(), IsNil) }()

	svc, err := s.State.AddService("wordpress", s.charm)
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
	s.assertPorts(c, inst2, m2.Id(), []instance.Port{{"tcp", 80}})

	inst1 := s.startInstance(c, m1)
	err = u1.OpenPort("tcp", 8080)
	c.Assert(err, IsNil)
	s.assertPorts(c, inst1, m1.Id(), []instance.Port{{"tcp", 8080}})
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

	s.assertPorts(c, inst1, m1.Id(), []instance.Port{{"tcp", 80}})
	s.assertPorts(c, inst2, m2.Id(), []instance.Port{{"tcp", 80}})

	err = u1.ClosePort("tcp", 80)
	c.Assert(err, IsNil)
	err = u2.ClosePort("tcp", 80)
	c.Assert(err, IsNil)

	s.assertPorts(c, inst1, m1.Id(), nil)
	s.assertPorts(c, inst2, m2.Id(), nil)
}

func (s *FirewallerSuite) TestStartWithState(c *C) {
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

	s.assertPorts(c, inst, m.Id(), []instance.Port{{"tcp", 80}, {"tcp", 8080}})

	err = svc.SetExposed()
	c.Assert(err, IsNil)
}

func (s *FirewallerSuite) TestStartWithPartialState(c *C) {
	m, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	inst := s.startInstance(c, m)

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

	s.assertPorts(c, inst, m.Id(), []instance.Port{{"tcp", 80}})
}

func (s *FirewallerSuite) TestStartWithUnexposedService(c *C) {
	m, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	inst := s.startInstance(c, m)

	svc, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	u, err := svc.AddUnit()
	c.Assert(err, IsNil)
	err = u.AssignToMachine(m)
	c.Assert(err, IsNil)
	err = u.OpenPort("tcp", 80)
	c.Assert(err, IsNil)

	// Starting the firewaller, no open ports.
	fw := firewaller.NewFirewaller(s.State)
	defer func() { c.Assert(fw.Stop(), IsNil) }()

	s.assertPorts(c, inst, m.Id(), nil)

	// Expose service.
	err = svc.SetExposed()
	c.Assert(err, IsNil)
	s.assertPorts(c, inst, m.Id(), []instance.Port{{"tcp", 80}})
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

	s.assertPorts(c, inst, m.Id(), []instance.Port{{"tcp", 80}, {"tcp", 8080}})

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

	s.assertPorts(c, inst1, m1.Id(), []instance.Port{{"tcp", 80}})
	s.assertPorts(c, inst2, m2.Id(), []instance.Port{{"tcp", 80}})

	// Remove unit.
	err = u1.EnsureDead()
	c.Assert(err, IsNil)
	err = u1.Remove()
	c.Assert(err, IsNil)

	s.assertPorts(c, inst1, m1.Id(), nil)
	s.assertPorts(c, inst2, m2.Id(), []instance.Port{{"tcp", 80}})
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

	s.assertPorts(c, inst, m.Id(), []instance.Port{{"tcp", 80}})

	// Remove service.
	err = u.EnsureDead()
	c.Assert(err, IsNil)
	err = u.Remove()
	c.Assert(err, IsNil)
	err = svc.Destroy()
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

	s.assertPorts(c, inst1, m1.Id(), []instance.Port{{"tcp", 80}})
	s.assertPorts(c, inst2, m2.Id(), []instance.Port{{"tcp", 3306}})

	// Remove services.
	err = u2.EnsureDead()
	c.Assert(err, IsNil)
	err = u2.Remove()
	c.Assert(err, IsNil)
	err = svc2.Destroy()
	c.Assert(err, IsNil)

	err = u1.EnsureDead()
	c.Assert(err, IsNil)
	err = u1.Remove()
	c.Assert(err, IsNil)
	err = svc1.Destroy()
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

	s.assertPorts(c, inst, m.Id(), []instance.Port{{"tcp", 80}})

	// Remove unit and service, also tested without. Has no effect.
	err = u.EnsureDead()
	c.Assert(err, IsNil)
	err = u.Remove()
	c.Assert(err, IsNil)
	err = svc.Destroy()
	c.Assert(err, IsNil)

	// Kill machine.
	err = m.Refresh()
	c.Assert(err, IsNil)
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

	s.assertPorts(c, inst, m.Id(), []instance.Port{{"tcp", 80}})

	// Remove unit.
	err = u.EnsureDead()
	c.Assert(err, IsNil)
	err = u.Remove()
	c.Assert(err, IsNil)

	// Remove machine. Nothing bad should happen, but can't
	// assert port state since the machine must have been
	// destroyed and we lost its reference.
	err = m.Refresh()
	c.Assert(err, IsNil)
	err = m.EnsureDead()
	c.Assert(err, IsNil)
	err = m.Remove()
	c.Assert(err, IsNil)
}

func (s *FirewallerSuite) TestGlobalMode(c *C) {
	// Change configuration.
	restore := s.setGlobalMode(c)
	defer restore(c)

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

	s.assertEnvironPorts(c, []instance.Port{{"tcp", 80}, {"tcp", 8080}})

	// Closing a port opened by a different unit won't touch the environment.
	err = u1.ClosePort("tcp", 80)
	c.Assert(err, IsNil)
	s.assertEnvironPorts(c, []instance.Port{{"tcp", 80}, {"tcp", 8080}})

	// Closing a port used just once changes the environment.
	err = u1.ClosePort("tcp", 8080)
	c.Assert(err, IsNil)
	s.assertEnvironPorts(c, []instance.Port{{"tcp", 80}})

	// Closing the last port also modifies the environment.
	err = u2.ClosePort("tcp", 80)
	c.Assert(err, IsNil)
	s.assertEnvironPorts(c, nil)
}

func (s *FirewallerSuite) TestGlobalModeStartWithUnexposedService(c *C) {
	// Change configuration.
	restore := s.setGlobalMode(c)
	defer restore(c)

	m, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	s.startInstance(c, m)

	svc, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	u, err := svc.AddUnit()
	c.Assert(err, IsNil)
	err = u.AssignToMachine(m)
	c.Assert(err, IsNil)
	err = u.OpenPort("tcp", 80)
	c.Assert(err, IsNil)

	// Starting the firewaller, no open ports.
	fw := firewaller.NewFirewaller(s.State)
	defer func() { c.Assert(fw.Stop(), IsNil) }()

	s.assertEnvironPorts(c, nil)

	// Expose service.
	err = svc.SetExposed()
	c.Assert(err, IsNil)
	s.assertEnvironPorts(c, []instance.Port{{"tcp", 80}})
}

func (s *FirewallerSuite) TestGlobalModeRestart(c *C) {
	// Change configuration.
	restore := s.setGlobalMode(c)
	defer restore(c)

	// Start firewall and open ports.
	fw := firewaller.NewFirewaller(s.State)

	svc, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	err = svc.SetExposed()
	c.Assert(err, IsNil)

	u, m := s.addUnit(c, svc)
	s.startInstance(c, m)
	err = u.OpenPort("tcp", 80)
	c.Assert(err, IsNil)
	err = u.OpenPort("tcp", 8080)
	c.Assert(err, IsNil)

	s.assertEnvironPorts(c, []instance.Port{{"tcp", 80}, {"tcp", 8080}})

	// Stop firewall and close one and open a different port.
	err = fw.Stop()
	c.Assert(err, IsNil)

	err = u.ClosePort("tcp", 8080)
	c.Assert(err, IsNil)
	err = u.OpenPort("tcp", 8888)
	c.Assert(err, IsNil)

	// Start firewall and check port.
	fw = firewaller.NewFirewaller(s.State)
	defer func() { c.Assert(fw.Stop(), IsNil) }()

	s.assertEnvironPorts(c, []instance.Port{{"tcp", 80}, {"tcp", 8888}})
}

func (s *FirewallerSuite) TestGlobalModeRestartUnexposedService(c *C) {
	// Change configuration.
	restore := s.setGlobalMode(c)
	defer restore(c)

	// Start firewall and open ports.
	fw := firewaller.NewFirewaller(s.State)

	svc, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	err = svc.SetExposed()
	c.Assert(err, IsNil)

	u, m := s.addUnit(c, svc)
	s.startInstance(c, m)
	err = u.OpenPort("tcp", 80)
	c.Assert(err, IsNil)
	err = u.OpenPort("tcp", 8080)
	c.Assert(err, IsNil)

	s.assertEnvironPorts(c, []instance.Port{{"tcp", 80}, {"tcp", 8080}})

	// Stop firewall and clear exposed flag on service.
	err = fw.Stop()
	c.Assert(err, IsNil)

	err = svc.ClearExposed()
	c.Assert(err, IsNil)

	// Start firewall and check port.
	fw = firewaller.NewFirewaller(s.State)
	defer func() { c.Assert(fw.Stop(), IsNil) }()

	s.assertEnvironPorts(c, nil)
}

func (s *FirewallerSuite) TestGlobalModeRestartPortCount(c *C) {
	// Change configuration.
	restore := s.setGlobalMode(c)
	defer restore(c)

	// Start firewall and open ports.
	fw := firewaller.NewFirewaller(s.State)

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

	s.assertEnvironPorts(c, []instance.Port{{"tcp", 80}, {"tcp", 8080}})

	// Stop firewall and add another service using the port.
	err = fw.Stop()
	c.Assert(err, IsNil)

	svc2, err := s.State.AddService("moinmoin", s.charm)
	c.Assert(err, IsNil)
	err = svc2.SetExposed()
	c.Assert(err, IsNil)

	u2, m2 := s.addUnit(c, svc2)
	s.startInstance(c, m2)
	err = u2.OpenPort("tcp", 80)
	c.Assert(err, IsNil)

	// Start firewall and check port.
	fw = firewaller.NewFirewaller(s.State)
	defer func() { c.Assert(fw.Stop(), IsNil) }()

	s.assertEnvironPorts(c, []instance.Port{{"tcp", 80}, {"tcp", 8080}})

	// Closing a port opened by a different unit won't touch the environment.
	err = u1.ClosePort("tcp", 80)
	c.Assert(err, IsNil)
	s.assertEnvironPorts(c, []instance.Port{{"tcp", 80}, {"tcp", 8080}})

	// Closing a port used just once changes the environment.
	err = u1.ClosePort("tcp", 8080)
	c.Assert(err, IsNil)
	s.assertEnvironPorts(c, []instance.Port{{"tcp", 80}})

	// Closing the last port also modifies the environment.
	err = u2.ClosePort("tcp", 80)
	c.Assert(err, IsNil)
	s.assertEnvironPorts(c, nil)
}
