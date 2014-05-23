// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	"reflect"
	stdtesting "testing"
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/provider/dummy"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	apifirewaller "launchpad.net/juju-core/state/api/firewaller"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/worker"
	"launchpad.net/juju-core/worker/firewaller"
)

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type FirewallerSuite struct {
	testing.JujuConnSuite
	op    <-chan dummy.Operation
	charm *state.Charm

	st         *api.State
	firewaller *apifirewaller.State
}

type FirewallerGlobalModeSuite struct {
	FirewallerSuite
}

var _ worker.Worker = (*firewaller.Firewaller)(nil)

// assertPorts retrieves the open ports of the instance and compares them
// to the expected.
func (s *FirewallerSuite) assertPorts(c *gc.C, inst instance.Instance, machineId string, expected []instance.Port) {
	s.BackingState.StartSync()
	start := time.Now()
	for {
		got, err := inst.Ports(machineId)
		if err != nil {
			c.Fatal(err)
			return
		}
		instance.SortPorts(got)
		instance.SortPorts(expected)
		if reflect.DeepEqual(got, expected) {
			c.Succeed()
			return
		}
		if time.Since(start) > coretesting.LongWait {
			c.Fatalf("timed out: expected %q; got %q", expected, got)
			return
		}
		time.Sleep(coretesting.ShortWait)
	}
}

// assertEnvironPorts retrieves the open ports of environment and compares them
// to the expected.
func (s *FirewallerSuite) assertEnvironPorts(c *gc.C, expected []instance.Port) {
	s.BackingState.StartSync()
	start := time.Now()
	for {
		got, err := s.Conn.Environ.Ports()
		if err != nil {
			c.Fatal(err)
			return
		}
		instance.SortPorts(got)
		instance.SortPorts(expected)
		if reflect.DeepEqual(got, expected) {
			c.Succeed()
			return
		}
		if time.Since(start) > coretesting.LongWait {
			c.Fatalf("timed out: expected %q; got %q", expected, got)
			return
		}
		time.Sleep(coretesting.ShortWait)
	}
}

var _ = gc.Suite(&FirewallerSuite{})

func (s FirewallerGlobalModeSuite) SetUpTest(c *gc.C) {
	add := map[string]interface{}{"firewall-mode": config.FwGlobal}
	s.DummyConfig = dummy.SampleConfig().Merge(add).Delete("admin-secret", "ca-private-key")

	s.FirewallerSuite.SetUpTest(c)
}

func (s *FirewallerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.charm = s.AddTestingCharm(c, "dummy")

	// Create a manager machine and login to the API.
	machine, err := s.State.AddMachine("quantal", state.JobManageEnviron)
	c.Assert(err, gc.IsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	err = machine.SetPassword(password)
	c.Assert(err, gc.IsNil)
	err = machine.SetProvisioned("i-manager", "fake_nonce", nil)
	c.Assert(err, gc.IsNil)
	s.st = s.OpenAPIAsMachine(c, machine.Tag(), password, "fake_nonce")
	c.Assert(s.st, gc.NotNil)

	// Create the firewaller API facade.
	s.firewaller = s.st.Firewaller()
	c.Assert(s.firewaller, gc.NotNil)
}

func (s *FirewallerSuite) TestStartStop(c *gc.C) {
	fw, err := firewaller.NewFirewaller(s.firewaller)
	c.Assert(err, gc.IsNil)
	c.Assert(fw.Stop(), gc.IsNil)
}

func (s *FirewallerSuite) addUnit(c *gc.C, svc *state.Service) (*state.Unit, *state.Machine) {
	units, err := juju.AddUnits(s.State, svc, 1, "")
	c.Assert(err, gc.IsNil)
	u := units[0]
	id, err := u.AssignedMachineId()
	c.Assert(err, gc.IsNil)
	m, err := s.State.Machine(id)
	c.Assert(err, gc.IsNil)
	return u, m
}

// startInstance starts a new instance for the given machine.
func (s *FirewallerSuite) startInstance(c *gc.C, m *state.Machine) instance.Instance {
	inst, hc := testing.AssertStartInstance(c, s.Conn.Environ, m.Id())
	err := m.SetProvisioned(inst.Id(), "fake_nonce", hc)
	c.Assert(err, gc.IsNil)
	return inst
}

func (s *FirewallerSuite) TestNotExposedService(c *gc.C) {
	fw, err := firewaller.NewFirewaller(s.firewaller)
	c.Assert(err, gc.IsNil)
	defer func() { c.Assert(fw.Stop(), gc.IsNil) }()

	svc := s.AddTestingService(c, "wordpress", s.charm)
	u, m := s.addUnit(c, svc)
	inst := s.startInstance(c, m)

	err = u.OpenPort("tcp", 80)
	c.Assert(err, gc.IsNil)
	err = u.OpenPort("tcp", 8080)
	c.Assert(err, gc.IsNil)

	s.assertPorts(c, inst, m.Id(), nil)

	err = u.ClosePort("tcp", 80)
	c.Assert(err, gc.IsNil)

	s.assertPorts(c, inst, m.Id(), nil)
}

func (s *FirewallerSuite) TestExposedService(c *gc.C) {
	fw, err := firewaller.NewFirewaller(s.firewaller)
	c.Assert(err, gc.IsNil)
	defer func() { c.Assert(fw.Stop(), gc.IsNil) }()

	svc := s.AddTestingService(c, "wordpress", s.charm)

	err = svc.SetExposed()
	c.Assert(err, gc.IsNil)
	u, m := s.addUnit(c, svc)
	inst := s.startInstance(c, m)

	err = u.OpenPort("tcp", 80)
	c.Assert(err, gc.IsNil)
	err = u.OpenPort("tcp", 8080)
	c.Assert(err, gc.IsNil)

	s.assertPorts(c, inst, m.Id(), []instance.Port{{"tcp", 80}, {"tcp", 8080}})

	err = u.ClosePort("tcp", 80)
	c.Assert(err, gc.IsNil)

	s.assertPorts(c, inst, m.Id(), []instance.Port{{"tcp", 8080}})
}

func (s *FirewallerSuite) TestMultipleExposedServices(c *gc.C) {
	fw, err := firewaller.NewFirewaller(s.firewaller)
	c.Assert(err, gc.IsNil)
	defer func() { c.Assert(fw.Stop(), gc.IsNil) }()

	svc1 := s.AddTestingService(c, "wordpress", s.charm)
	err = svc1.SetExposed()
	c.Assert(err, gc.IsNil)

	u1, m1 := s.addUnit(c, svc1)
	inst1 := s.startInstance(c, m1)
	err = u1.OpenPort("tcp", 80)
	c.Assert(err, gc.IsNil)
	err = u1.OpenPort("tcp", 8080)
	c.Assert(err, gc.IsNil)

	svc2 := s.AddTestingService(c, "mysql", s.charm)
	c.Assert(err, gc.IsNil)
	err = svc2.SetExposed()
	c.Assert(err, gc.IsNil)

	u2, m2 := s.addUnit(c, svc2)
	inst2 := s.startInstance(c, m2)
	err = u2.OpenPort("tcp", 3306)
	c.Assert(err, gc.IsNil)

	s.assertPorts(c, inst1, m1.Id(), []instance.Port{{"tcp", 80}, {"tcp", 8080}})
	s.assertPorts(c, inst2, m2.Id(), []instance.Port{{"tcp", 3306}})

	err = u1.ClosePort("tcp", 80)
	c.Assert(err, gc.IsNil)
	err = u2.ClosePort("tcp", 3306)
	c.Assert(err, gc.IsNil)

	s.assertPorts(c, inst1, m1.Id(), []instance.Port{{"tcp", 8080}})
	s.assertPorts(c, inst2, m2.Id(), nil)
}

func (s *FirewallerSuite) TestMachineWithoutInstanceId(c *gc.C) {
	fw, err := firewaller.NewFirewaller(s.firewaller)
	c.Assert(err, gc.IsNil)
	defer func() { c.Assert(fw.Stop(), gc.IsNil) }()

	svc := s.AddTestingService(c, "wordpress", s.charm)
	err = svc.SetExposed()
	c.Assert(err, gc.IsNil)
	// add a unit but don't start its instance yet.
	u1, m1 := s.addUnit(c, svc)

	// add another unit and start its instance, so that
	// we're sure the firewaller has seen the first instance.
	u2, m2 := s.addUnit(c, svc)
	inst2 := s.startInstance(c, m2)
	err = u2.OpenPort("tcp", 80)
	c.Assert(err, gc.IsNil)
	s.assertPorts(c, inst2, m2.Id(), []instance.Port{{"tcp", 80}})

	inst1 := s.startInstance(c, m1)
	err = u1.OpenPort("tcp", 8080)
	c.Assert(err, gc.IsNil)
	s.assertPorts(c, inst1, m1.Id(), []instance.Port{{"tcp", 8080}})
}

func (s *FirewallerSuite) TestMultipleUnits(c *gc.C) {
	fw, err := firewaller.NewFirewaller(s.firewaller)
	c.Assert(err, gc.IsNil)
	defer func() { c.Assert(fw.Stop(), gc.IsNil) }()

	svc := s.AddTestingService(c, "wordpress", s.charm)
	err = svc.SetExposed()
	c.Assert(err, gc.IsNil)

	u1, m1 := s.addUnit(c, svc)
	inst1 := s.startInstance(c, m1)
	err = u1.OpenPort("tcp", 80)
	c.Assert(err, gc.IsNil)

	u2, m2 := s.addUnit(c, svc)
	inst2 := s.startInstance(c, m2)
	err = u2.OpenPort("tcp", 80)
	c.Assert(err, gc.IsNil)

	s.assertPorts(c, inst1, m1.Id(), []instance.Port{{"tcp", 80}})
	s.assertPorts(c, inst2, m2.Id(), []instance.Port{{"tcp", 80}})

	err = u1.ClosePort("tcp", 80)
	c.Assert(err, gc.IsNil)
	err = u2.ClosePort("tcp", 80)
	c.Assert(err, gc.IsNil)

	s.assertPorts(c, inst1, m1.Id(), nil)
	s.assertPorts(c, inst2, m2.Id(), nil)
}

func (s *FirewallerSuite) TestStartWithState(c *gc.C) {
	svc := s.AddTestingService(c, "wordpress", s.charm)
	err := svc.SetExposed()
	c.Assert(err, gc.IsNil)
	u, m := s.addUnit(c, svc)
	inst := s.startInstance(c, m)

	err = u.OpenPort("tcp", 80)
	c.Assert(err, gc.IsNil)
	err = u.OpenPort("tcp", 8080)
	c.Assert(err, gc.IsNil)

	// Nothing open without firewaller.
	s.assertPorts(c, inst, m.Id(), nil)

	// Starting the firewaller opens the ports.
	fw, err := firewaller.NewFirewaller(s.firewaller)
	c.Assert(err, gc.IsNil)
	defer func() { c.Assert(fw.Stop(), gc.IsNil) }()

	s.assertPorts(c, inst, m.Id(), []instance.Port{{"tcp", 80}, {"tcp", 8080}})

	err = svc.SetExposed()
	c.Assert(err, gc.IsNil)
}

func (s *FirewallerSuite) TestStartWithPartialState(c *gc.C) {
	m, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	inst := s.startInstance(c, m)

	svc := s.AddTestingService(c, "wordpress", s.charm)
	err = svc.SetExposed()
	c.Assert(err, gc.IsNil)

	// Starting the firewaller, no open ports.
	fw, err := firewaller.NewFirewaller(s.firewaller)
	c.Assert(err, gc.IsNil)
	defer func() { c.Assert(fw.Stop(), gc.IsNil) }()

	s.assertPorts(c, inst, m.Id(), nil)

	// Complete steps to open port.
	u, err := svc.AddUnit()
	c.Assert(err, gc.IsNil)
	err = u.AssignToMachine(m)
	c.Assert(err, gc.IsNil)
	err = u.OpenPort("tcp", 80)
	c.Assert(err, gc.IsNil)

	s.assertPorts(c, inst, m.Id(), []instance.Port{{"tcp", 80}})
}

func (s *FirewallerSuite) TestStartWithUnexposedService(c *gc.C) {
	m, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	inst := s.startInstance(c, m)

	svc := s.AddTestingService(c, "wordpress", s.charm)
	u, err := svc.AddUnit()
	c.Assert(err, gc.IsNil)
	err = u.AssignToMachine(m)
	c.Assert(err, gc.IsNil)
	err = u.OpenPort("tcp", 80)
	c.Assert(err, gc.IsNil)

	// Starting the firewaller, no open ports.
	fw, err := firewaller.NewFirewaller(s.firewaller)
	c.Assert(err, gc.IsNil)
	defer func() { c.Assert(fw.Stop(), gc.IsNil) }()

	s.assertPorts(c, inst, m.Id(), nil)

	// Expose service.
	err = svc.SetExposed()
	c.Assert(err, gc.IsNil)
	s.assertPorts(c, inst, m.Id(), []instance.Port{{"tcp", 80}})
}

func (s *FirewallerSuite) TestSetClearExposedService(c *gc.C) {
	fw, err := firewaller.NewFirewaller(s.firewaller)
	c.Assert(err, gc.IsNil)
	defer func() { c.Assert(fw.Stop(), gc.IsNil) }()

	svc := s.AddTestingService(c, "wordpress", s.charm)

	u, m := s.addUnit(c, svc)
	inst := s.startInstance(c, m)
	err = u.OpenPort("tcp", 80)
	c.Assert(err, gc.IsNil)
	err = u.OpenPort("tcp", 8080)
	c.Assert(err, gc.IsNil)

	// Not exposed service, so no open port.
	s.assertPorts(c, inst, m.Id(), nil)

	// SeExposed opens the ports.
	err = svc.SetExposed()
	c.Assert(err, gc.IsNil)

	s.assertPorts(c, inst, m.Id(), []instance.Port{{"tcp", 80}, {"tcp", 8080}})

	// ClearExposed closes the ports again.
	err = svc.ClearExposed()
	c.Assert(err, gc.IsNil)

	s.assertPorts(c, inst, m.Id(), nil)
}

func (s *FirewallerSuite) TestRemoveUnit(c *gc.C) {
	fw, err := firewaller.NewFirewaller(s.firewaller)
	c.Assert(err, gc.IsNil)
	defer func() { c.Assert(fw.Stop(), gc.IsNil) }()

	svc := s.AddTestingService(c, "wordpress", s.charm)
	err = svc.SetExposed()
	c.Assert(err, gc.IsNil)

	u1, m1 := s.addUnit(c, svc)
	inst1 := s.startInstance(c, m1)
	err = u1.OpenPort("tcp", 80)
	c.Assert(err, gc.IsNil)

	u2, m2 := s.addUnit(c, svc)
	inst2 := s.startInstance(c, m2)
	err = u2.OpenPort("tcp", 80)
	c.Assert(err, gc.IsNil)

	s.assertPorts(c, inst1, m1.Id(), []instance.Port{{"tcp", 80}})
	s.assertPorts(c, inst2, m2.Id(), []instance.Port{{"tcp", 80}})

	// Remove unit.
	err = u1.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = u1.Remove()
	c.Assert(err, gc.IsNil)

	s.assertPorts(c, inst1, m1.Id(), nil)
	s.assertPorts(c, inst2, m2.Id(), []instance.Port{{"tcp", 80}})
}

func (s *FirewallerSuite) TestRemoveService(c *gc.C) {
	fw, err := firewaller.NewFirewaller(s.firewaller)
	c.Assert(err, gc.IsNil)
	defer func() { c.Assert(fw.Stop(), gc.IsNil) }()

	svc := s.AddTestingService(c, "wordpress", s.charm)
	err = svc.SetExposed()
	c.Assert(err, gc.IsNil)

	u, m := s.addUnit(c, svc)
	inst := s.startInstance(c, m)
	err = u.OpenPort("tcp", 80)
	c.Assert(err, gc.IsNil)

	s.assertPorts(c, inst, m.Id(), []instance.Port{{"tcp", 80}})

	// Remove service.
	err = u.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = u.Remove()
	c.Assert(err, gc.IsNil)
	err = svc.Destroy()
	c.Assert(err, gc.IsNil)
	s.assertPorts(c, inst, m.Id(), nil)
}

func (s *FirewallerSuite) TestRemoveMultipleServices(c *gc.C) {
	fw, err := firewaller.NewFirewaller(s.firewaller)
	c.Assert(err, gc.IsNil)
	defer func() { c.Assert(fw.Stop(), gc.IsNil) }()

	svc1 := s.AddTestingService(c, "wordpress", s.charm)
	err = svc1.SetExposed()
	c.Assert(err, gc.IsNil)

	u1, m1 := s.addUnit(c, svc1)
	inst1 := s.startInstance(c, m1)
	err = u1.OpenPort("tcp", 80)
	c.Assert(err, gc.IsNil)

	svc2 := s.AddTestingService(c, "mysql", s.charm)
	err = svc2.SetExposed()
	c.Assert(err, gc.IsNil)

	u2, m2 := s.addUnit(c, svc2)
	inst2 := s.startInstance(c, m2)
	err = u2.OpenPort("tcp", 3306)
	c.Assert(err, gc.IsNil)

	s.assertPorts(c, inst1, m1.Id(), []instance.Port{{"tcp", 80}})
	s.assertPorts(c, inst2, m2.Id(), []instance.Port{{"tcp", 3306}})

	// Remove services.
	err = u2.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = u2.Remove()
	c.Assert(err, gc.IsNil)
	err = svc2.Destroy()
	c.Assert(err, gc.IsNil)

	err = u1.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = u1.Remove()
	c.Assert(err, gc.IsNil)
	err = svc1.Destroy()
	c.Assert(err, gc.IsNil)

	s.assertPorts(c, inst1, m1.Id(), nil)
	s.assertPorts(c, inst2, m2.Id(), nil)
}

func (s *FirewallerSuite) TestDeadMachine(c *gc.C) {
	fw, err := firewaller.NewFirewaller(s.firewaller)
	c.Assert(err, gc.IsNil)
	defer func() { c.Assert(fw.Stop(), gc.IsNil) }()

	svc := s.AddTestingService(c, "wordpress", s.charm)
	err = svc.SetExposed()
	c.Assert(err, gc.IsNil)

	u, m := s.addUnit(c, svc)
	inst := s.startInstance(c, m)
	err = u.OpenPort("tcp", 80)
	c.Assert(err, gc.IsNil)

	s.assertPorts(c, inst, m.Id(), []instance.Port{{"tcp", 80}})

	// Remove unit and service, also tested without. Has no effect.
	err = u.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = u.Remove()
	c.Assert(err, gc.IsNil)
	err = svc.Destroy()
	c.Assert(err, gc.IsNil)

	// Kill machine.
	err = m.Refresh()
	c.Assert(err, gc.IsNil)
	err = m.EnsureDead()
	c.Assert(err, gc.IsNil)

	s.assertPorts(c, inst, m.Id(), nil)
}

func (s *FirewallerSuite) TestRemoveMachine(c *gc.C) {
	fw, err := firewaller.NewFirewaller(s.firewaller)
	c.Assert(err, gc.IsNil)
	defer func() { c.Assert(fw.Stop(), gc.IsNil) }()

	svc := s.AddTestingService(c, "wordpress", s.charm)
	err = svc.SetExposed()
	c.Assert(err, gc.IsNil)

	u, m := s.addUnit(c, svc)
	inst := s.startInstance(c, m)
	err = u.OpenPort("tcp", 80)
	c.Assert(err, gc.IsNil)

	s.assertPorts(c, inst, m.Id(), []instance.Port{{"tcp", 80}})

	// Remove unit.
	err = u.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = u.Remove()
	c.Assert(err, gc.IsNil)

	// Remove machine. Nothing bad should happen, but can't
	// assert port state since the machine must have been
	// destroyed and we lost its reference.
	err = m.Refresh()
	c.Assert(err, gc.IsNil)
	err = m.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = m.Remove()
	c.Assert(err, gc.IsNil)
}

func (s *FirewallerGlobalModeSuite) TestGlobalMode(c *gc.C) {
	// Start firewaller and open ports.
	fw, err := firewaller.NewFirewaller(s.firewaller)
	c.Assert(err, gc.IsNil)
	defer func() { c.Assert(fw.Stop(), gc.IsNil) }()

	svc1 := s.AddTestingService(c, "wordpress", s.charm)
	err = svc1.SetExposed()
	c.Assert(err, gc.IsNil)

	u1, m1 := s.addUnit(c, svc1)
	s.startInstance(c, m1)
	err = u1.OpenPort("tcp", 80)
	c.Assert(err, gc.IsNil)
	err = u1.OpenPort("tcp", 8080)
	c.Assert(err, gc.IsNil)

	svc2 := s.AddTestingService(c, "moinmoin", s.charm)
	c.Assert(err, gc.IsNil)
	err = svc2.SetExposed()
	c.Assert(err, gc.IsNil)

	u2, m2 := s.addUnit(c, svc2)
	s.startInstance(c, m2)
	err = u2.OpenPort("tcp", 80)
	c.Assert(err, gc.IsNil)

	s.assertEnvironPorts(c, []instance.Port{{"tcp", 80}, {"tcp", 8080}})

	// Closing a port opened by a different unit won't touch the environment.
	err = u1.ClosePort("tcp", 80)
	c.Assert(err, gc.IsNil)
	s.assertEnvironPorts(c, []instance.Port{{"tcp", 80}, {"tcp", 8080}})

	// Closing a port used just once changes the environment.
	err = u1.ClosePort("tcp", 8080)
	c.Assert(err, gc.IsNil)
	s.assertEnvironPorts(c, []instance.Port{{"tcp", 80}})

	// Closing the last port also modifies the environment.
	err = u2.ClosePort("tcp", 80)
	c.Assert(err, gc.IsNil)
	s.assertEnvironPorts(c, nil)
}

func (s *FirewallerGlobalModeSuite) TestGlobalModeStartWithUnexposedService(c *gc.C) {
	m, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	s.startInstance(c, m)

	svc := s.AddTestingService(c, "wordpress", s.charm)
	u, err := svc.AddUnit()
	c.Assert(err, gc.IsNil)
	err = u.AssignToMachine(m)
	c.Assert(err, gc.IsNil)
	err = u.OpenPort("tcp", 80)
	c.Assert(err, gc.IsNil)

	// Starting the firewaller, no open ports.
	fw, err := firewaller.NewFirewaller(s.firewaller)
	c.Assert(err, gc.IsNil)
	defer func() { c.Assert(fw.Stop(), gc.IsNil) }()

	s.assertEnvironPorts(c, nil)

	// Expose service.
	err = svc.SetExposed()
	c.Assert(err, gc.IsNil)
	s.assertEnvironPorts(c, []instance.Port{{"tcp", 80}})
}

func (s *FirewallerGlobalModeSuite) TestGlobalModeRestart(c *gc.C) {
	// Start firewaller and open ports.
	fw, err := firewaller.NewFirewaller(s.firewaller)
	c.Assert(err, gc.IsNil)

	svc := s.AddTestingService(c, "wordpress", s.charm)
	err = svc.SetExposed()
	c.Assert(err, gc.IsNil)

	u, m := s.addUnit(c, svc)
	s.startInstance(c, m)
	err = u.OpenPort("tcp", 80)
	c.Assert(err, gc.IsNil)
	err = u.OpenPort("tcp", 8080)
	c.Assert(err, gc.IsNil)

	s.assertEnvironPorts(c, []instance.Port{{"tcp", 80}, {"tcp", 8080}})

	// Stop firewaller and close one and open a different port.
	err = fw.Stop()
	c.Assert(err, gc.IsNil)

	err = u.ClosePort("tcp", 8080)
	c.Assert(err, gc.IsNil)
	err = u.OpenPort("tcp", 8888)
	c.Assert(err, gc.IsNil)

	// Start firewaller and check port.
	fw, err = firewaller.NewFirewaller(s.firewaller)
	c.Assert(err, gc.IsNil)
	defer func() { c.Assert(fw.Stop(), gc.IsNil) }()

	s.assertEnvironPorts(c, []instance.Port{{"tcp", 80}, {"tcp", 8888}})
}

func (s *FirewallerGlobalModeSuite) TestGlobalModeRestartUnexposedService(c *gc.C) {
	// Start firewaller and open ports.
	fw, err := firewaller.NewFirewaller(s.firewaller)
	c.Assert(err, gc.IsNil)

	svc := s.AddTestingService(c, "wordpress", s.charm)
	err = svc.SetExposed()
	c.Assert(err, gc.IsNil)

	u, m := s.addUnit(c, svc)
	s.startInstance(c, m)
	err = u.OpenPort("tcp", 80)
	c.Assert(err, gc.IsNil)
	err = u.OpenPort("tcp", 8080)
	c.Assert(err, gc.IsNil)

	s.assertEnvironPorts(c, []instance.Port{{"tcp", 80}, {"tcp", 8080}})

	// Stop firewaller and clear exposed flag on service.
	err = fw.Stop()
	c.Assert(err, gc.IsNil)

	err = svc.ClearExposed()
	c.Assert(err, gc.IsNil)

	// Start firewaller and check port.
	fw, err = firewaller.NewFirewaller(s.firewaller)
	c.Assert(err, gc.IsNil)
	defer func() { c.Assert(fw.Stop(), gc.IsNil) }()

	s.assertEnvironPorts(c, nil)
}

func (s *FirewallerGlobalModeSuite) TestGlobalModeRestartPortCount(c *gc.C) {
	// Start firewaller and open ports.
	fw, err := firewaller.NewFirewaller(s.firewaller)
	c.Assert(err, gc.IsNil)

	svc1 := s.AddTestingService(c, "wordpress", s.charm)
	err = svc1.SetExposed()
	c.Assert(err, gc.IsNil)

	u1, m1 := s.addUnit(c, svc1)
	s.startInstance(c, m1)
	err = u1.OpenPort("tcp", 80)
	c.Assert(err, gc.IsNil)
	err = u1.OpenPort("tcp", 8080)
	c.Assert(err, gc.IsNil)

	s.assertEnvironPorts(c, []instance.Port{{"tcp", 80}, {"tcp", 8080}})

	// Stop firewaller and add another service using the port.
	err = fw.Stop()
	c.Assert(err, gc.IsNil)

	svc2 := s.AddTestingService(c, "moinmoin", s.charm)
	err = svc2.SetExposed()
	c.Assert(err, gc.IsNil)

	u2, m2 := s.addUnit(c, svc2)
	s.startInstance(c, m2)
	err = u2.OpenPort("tcp", 80)
	c.Assert(err, gc.IsNil)

	// Start firewaller and check port.
	fw, err = firewaller.NewFirewaller(s.firewaller)
	c.Assert(err, gc.IsNil)
	defer func() { c.Assert(fw.Stop(), gc.IsNil) }()

	s.assertEnvironPorts(c, []instance.Port{{"tcp", 80}, {"tcp", 8080}})

	// Closing a port opened by a different unit won't touch the environment.
	err = u1.ClosePort("tcp", 80)
	c.Assert(err, gc.IsNil)
	s.assertEnvironPorts(c, []instance.Port{{"tcp", 80}, {"tcp", 8080}})

	// Closing a port used just once changes the environment.
	err = u1.ClosePort("tcp", 8080)
	c.Assert(err, gc.IsNil)
	s.assertEnvironPorts(c, []instance.Port{{"tcp", 80}})

	// Closing the last port also modifies the environment.
	err = u2.ClosePort("tcp", 80)
	c.Assert(err, gc.IsNil)
	s.assertEnvironPorts(c, nil)
}
