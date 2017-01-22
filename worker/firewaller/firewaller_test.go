// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	"reflect"
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	apifirewaller "github.com/juju/juju/api/firewaller"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/firewaller"
)

// firewallerBaseSuite implements common functionality for embedding
// into each of the other per-mode suites.
type firewallerBaseSuite struct {
	testing.JujuConnSuite
	op    <-chan dummy.Operation
	charm *state.Charm

	st         api.Connection
	firewaller *apifirewaller.State
}

var _ worker.Worker = (*firewaller.Firewaller)(nil)

func (s *firewallerBaseSuite) setUpTest(c *gc.C, firewallMode string) {
	add := map[string]interface{}{"firewall-mode": firewallMode}
	s.DummyConfig = dummy.SampleConfig().Merge(add).Delete("admin-secret")

	s.JujuConnSuite.SetUpTest(c)
	s.charm = s.AddTestingCharm(c, "dummy")

	// Create a manager machine and login to the API.
	machine, err := s.State.AddMachine("quantal", state.JobManageModel)
	c.Assert(err, jc.ErrorIsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned("i-manager", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.st = s.OpenAPIAsMachine(c, machine.Tag(), password, "fake_nonce")
	c.Assert(s.st, gc.NotNil)

	// Create the firewaller API facade.
	s.firewaller = apifirewaller.NewState(s.st)
	c.Assert(s.firewaller, gc.NotNil)
}

// assertPorts retrieves the open ports of the instance and compares them
// to the expected.
func (s *firewallerBaseSuite) assertPorts(c *gc.C, inst instance.Instance, machineId string, expected []network.PortRange) {
	s.BackingState.StartSync()
	start := time.Now()
	for {
		got, err := inst.Ports(machineId)
		if err != nil {
			c.Fatal(err)
			return
		}
		network.SortPortRanges(got)
		network.SortPortRanges(expected)
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
func (s *firewallerBaseSuite) assertEnvironPorts(c *gc.C, expected []network.PortRange) {
	s.BackingState.StartSync()
	start := time.Now()
	for {
		got, err := s.Environ.Ports()
		if err != nil {
			c.Fatal(err)
			return
		}
		network.SortPortRanges(got)
		network.SortPortRanges(expected)
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

func (s *firewallerBaseSuite) addUnit(c *gc.C, app *state.Application) (*state.Unit, *state.Machine) {
	units, err := juju.AddUnits(s.State, app, app.Name(), 1, nil)
	c.Assert(err, jc.ErrorIsNil)
	u := units[0]
	id, err := u.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	m, err := s.State.Machine(id)
	c.Assert(err, jc.ErrorIsNil)
	return u, m
}

// startInstance starts a new instance for the given machine.
func (s *firewallerBaseSuite) startInstance(c *gc.C, m *state.Machine) instance.Instance {
	inst, hc := testing.AssertStartInstance(c, s.Environ, s.ControllerConfig.ControllerUUID(), m.Id())
	err := m.SetProvisioned(inst.Id(), "fake_nonce", hc)
	c.Assert(err, jc.ErrorIsNil)
	return inst
}

type InstanceModeSuite struct {
	firewallerBaseSuite
}

var _ = gc.Suite(&InstanceModeSuite{})

func (s *InstanceModeSuite) SetUpTest(c *gc.C) {
	s.firewallerBaseSuite.setUpTest(c, config.FwInstance)
}

func (s *InstanceModeSuite) TearDownTest(c *gc.C) {
	s.firewallerBaseSuite.JujuConnSuite.TearDownTest(c)
}

func (s *InstanceModeSuite) newFirewaller(c *gc.C) worker.Worker {
	fw, err := firewaller.NewFirewaller(s.Environ, s.firewaller, config.FwInstance)
	c.Assert(err, jc.ErrorIsNil)
	return fw
}

func (s *InstanceModeSuite) TestStartStop(c *gc.C) {
	fw := s.newFirewaller(c)
	statetesting.AssertKillAndWait(c, fw)
}

func (s *InstanceModeSuite) TestNotExposedService(c *gc.C) {
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	app := s.AddTestingService(c, "wordpress", s.charm)
	u, m := s.addUnit(c, app)
	inst := s.startInstance(c, m)

	err := u.OpenPort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)
	err = u.OpenPort("tcp", 8080)
	c.Assert(err, jc.ErrorIsNil)

	s.assertPorts(c, inst, m.Id(), nil)

	err = u.ClosePort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)

	s.assertPorts(c, inst, m.Id(), nil)
}

func (s *InstanceModeSuite) TestExposedService(c *gc.C) {
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	app := s.AddTestingService(c, "wordpress", s.charm)

	err := app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)
	u, m := s.addUnit(c, app)
	inst := s.startInstance(c, m)

	err = u.OpenPorts("tcp", 80, 90)
	c.Assert(err, jc.ErrorIsNil)
	err = u.OpenPort("tcp", 8080)
	c.Assert(err, jc.ErrorIsNil)

	s.assertPorts(c, inst, m.Id(), []network.PortRange{{80, 90, "tcp"}, {8080, 8080, "tcp"}})

	err = u.ClosePorts("tcp", 80, 90)
	c.Assert(err, jc.ErrorIsNil)

	s.assertPorts(c, inst, m.Id(), []network.PortRange{{8080, 8080, "tcp"}})
}

func (s *InstanceModeSuite) TestMultipleExposedServices(c *gc.C) {
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	app1 := s.AddTestingService(c, "wordpress", s.charm)
	err := app1.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	u1, m1 := s.addUnit(c, app1)
	inst1 := s.startInstance(c, m1)
	err = u1.OpenPort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)
	err = u1.OpenPort("tcp", 8080)
	c.Assert(err, jc.ErrorIsNil)

	app2 := s.AddTestingService(c, "mysql", s.charm)
	c.Assert(err, jc.ErrorIsNil)
	err = app2.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	u2, m2 := s.addUnit(c, app2)
	inst2 := s.startInstance(c, m2)
	err = u2.OpenPort("tcp", 3306)
	c.Assert(err, jc.ErrorIsNil)

	s.assertPorts(c, inst1, m1.Id(), []network.PortRange{{80, 80, "tcp"}, {8080, 8080, "tcp"}})
	s.assertPorts(c, inst2, m2.Id(), []network.PortRange{{3306, 3306, "tcp"}})

	err = u1.ClosePort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)
	err = u2.ClosePort("tcp", 3306)
	c.Assert(err, jc.ErrorIsNil)

	s.assertPorts(c, inst1, m1.Id(), []network.PortRange{{8080, 8080, "tcp"}})
	s.assertPorts(c, inst2, m2.Id(), nil)
}

func (s *InstanceModeSuite) TestMachineWithoutInstanceId(c *gc.C) {
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	app := s.AddTestingService(c, "wordpress", s.charm)
	err := app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)
	// add a unit but don't start its instance yet.
	u1, m1 := s.addUnit(c, app)

	// add another unit and start its instance, so that
	// we're sure the firewaller has seen the first instance.
	u2, m2 := s.addUnit(c, app)
	inst2 := s.startInstance(c, m2)
	err = u2.OpenPort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)
	s.assertPorts(c, inst2, m2.Id(), []network.PortRange{{80, 80, "tcp"}})

	inst1 := s.startInstance(c, m1)
	err = u1.OpenPort("tcp", 8080)
	c.Assert(err, jc.ErrorIsNil)
	s.assertPorts(c, inst1, m1.Id(), []network.PortRange{{8080, 8080, "tcp"}})
}

func (s *InstanceModeSuite) TestMultipleUnits(c *gc.C) {
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	app := s.AddTestingService(c, "wordpress", s.charm)
	err := app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	u1, m1 := s.addUnit(c, app)
	inst1 := s.startInstance(c, m1)
	err = u1.OpenPort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)

	u2, m2 := s.addUnit(c, app)
	inst2 := s.startInstance(c, m2)
	err = u2.OpenPort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)

	s.assertPorts(c, inst1, m1.Id(), []network.PortRange{{80, 80, "tcp"}})
	s.assertPorts(c, inst2, m2.Id(), []network.PortRange{{80, 80, "tcp"}})

	err = u1.ClosePort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)
	err = u2.ClosePort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)

	s.assertPorts(c, inst1, m1.Id(), nil)
	s.assertPorts(c, inst2, m2.Id(), nil)
}

func (s *InstanceModeSuite) TestStartWithState(c *gc.C) {
	app := s.AddTestingService(c, "wordpress", s.charm)
	err := app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)
	u, m := s.addUnit(c, app)
	inst := s.startInstance(c, m)

	err = u.OpenPort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)
	err = u.OpenPort("tcp", 8080)
	c.Assert(err, jc.ErrorIsNil)

	// Nothing open without firewaller.
	s.assertPorts(c, inst, m.Id(), nil)

	// Starting the firewaller opens the ports.
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	s.assertPorts(c, inst, m.Id(), []network.PortRange{{80, 80, "tcp"}, {8080, 8080, "tcp"}})

	err = app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *InstanceModeSuite) TestStartWithPartialState(c *gc.C) {
	m, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	inst := s.startInstance(c, m)

	app := s.AddTestingService(c, "wordpress", s.charm)
	err = app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	// Starting the firewaller, no open ports.
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	s.assertPorts(c, inst, m.Id(), nil)

	// Complete steps to open port.
	u, err := app.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = u.AssignToMachine(m)
	c.Assert(err, jc.ErrorIsNil)
	err = u.OpenPort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)

	s.assertPorts(c, inst, m.Id(), []network.PortRange{{80, 80, "tcp"}})
}

func (s *InstanceModeSuite) TestStartWithUnexposedService(c *gc.C) {
	m, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	inst := s.startInstance(c, m)

	app := s.AddTestingService(c, "wordpress", s.charm)
	u, err := app.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = u.AssignToMachine(m)
	c.Assert(err, jc.ErrorIsNil)
	err = u.OpenPort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)

	// Starting the firewaller, no open ports.
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	s.assertPorts(c, inst, m.Id(), nil)

	// Expose service.
	err = app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)
	s.assertPorts(c, inst, m.Id(), []network.PortRange{{80, 80, "tcp"}})
}

func (s *InstanceModeSuite) TestSetClearExposedService(c *gc.C) {
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	app := s.AddTestingService(c, "wordpress", s.charm)

	u, m := s.addUnit(c, app)
	inst := s.startInstance(c, m)
	err := u.OpenPort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)
	err = u.OpenPort("tcp", 8080)
	c.Assert(err, jc.ErrorIsNil)

	// Not exposed service, so no open port.
	s.assertPorts(c, inst, m.Id(), nil)

	// SeExposed opens the ports.
	err = app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	s.assertPorts(c, inst, m.Id(), []network.PortRange{{80, 80, "tcp"}, {8080, 8080, "tcp"}})

	// ClearExposed closes the ports again.
	err = app.ClearExposed()
	c.Assert(err, jc.ErrorIsNil)

	s.assertPorts(c, inst, m.Id(), nil)
}

func (s *InstanceModeSuite) TestRemoveUnit(c *gc.C) {
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	app := s.AddTestingService(c, "wordpress", s.charm)
	err := app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	u1, m1 := s.addUnit(c, app)
	inst1 := s.startInstance(c, m1)
	err = u1.OpenPort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)

	u2, m2 := s.addUnit(c, app)
	inst2 := s.startInstance(c, m2)
	err = u2.OpenPort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)

	s.assertPorts(c, inst1, m1.Id(), []network.PortRange{{80, 80, "tcp"}})
	s.assertPorts(c, inst2, m2.Id(), []network.PortRange{{80, 80, "tcp"}})

	// Remove unit.
	err = u1.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = u1.Remove()
	c.Assert(err, jc.ErrorIsNil)

	s.assertPorts(c, inst1, m1.Id(), nil)
	s.assertPorts(c, inst2, m2.Id(), []network.PortRange{{80, 80, "tcp"}})
}

func (s *InstanceModeSuite) TestRemoveService(c *gc.C) {
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	app := s.AddTestingService(c, "wordpress", s.charm)
	err := app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	u, m := s.addUnit(c, app)
	inst := s.startInstance(c, m)
	err = u.OpenPort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)

	s.assertPorts(c, inst, m.Id(), []network.PortRange{{80, 80, "tcp"}})

	// Remove service.
	err = u.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = u.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = app.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	s.assertPorts(c, inst, m.Id(), nil)
}

func (s *InstanceModeSuite) TestRemoveMultipleServices(c *gc.C) {
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	app1 := s.AddTestingService(c, "wordpress", s.charm)
	err := app1.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	u1, m1 := s.addUnit(c, app1)
	inst1 := s.startInstance(c, m1)
	err = u1.OpenPort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)

	app2 := s.AddTestingService(c, "mysql", s.charm)
	err = app2.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	u2, m2 := s.addUnit(c, app2)
	inst2 := s.startInstance(c, m2)
	err = u2.OpenPort("tcp", 3306)
	c.Assert(err, jc.ErrorIsNil)

	s.assertPorts(c, inst1, m1.Id(), []network.PortRange{{80, 80, "tcp"}})
	s.assertPorts(c, inst2, m2.Id(), []network.PortRange{{3306, 3306, "tcp"}})

	// Remove services.
	err = u2.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = u2.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = app2.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	err = u1.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = u1.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = app1.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	s.assertPorts(c, inst1, m1.Id(), nil)
	s.assertPorts(c, inst2, m2.Id(), nil)
}

func (s *InstanceModeSuite) TestDeadMachine(c *gc.C) {
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	app := s.AddTestingService(c, "wordpress", s.charm)
	err := app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	u, m := s.addUnit(c, app)
	inst := s.startInstance(c, m)
	err = u.OpenPort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)

	s.assertPorts(c, inst, m.Id(), []network.PortRange{{80, 80, "tcp"}})

	// Remove unit and service, also tested without. Has no effect.
	err = u.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = u.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = app.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	// Kill machine.
	err = m.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	err = m.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	s.assertPorts(c, inst, m.Id(), nil)
}

func (s *InstanceModeSuite) TestRemoveMachine(c *gc.C) {
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	app := s.AddTestingService(c, "wordpress", s.charm)
	err := app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	u, m := s.addUnit(c, app)
	inst := s.startInstance(c, m)
	err = u.OpenPort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)

	s.assertPorts(c, inst, m.Id(), []network.PortRange{{80, 80, "tcp"}})

	// Remove unit.
	err = u.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = u.Remove()
	c.Assert(err, jc.ErrorIsNil)

	// Remove machine. Nothing bad should happen, but can't
	// assert port state since the machine must have been
	// destroyed and we lost its reference.
	err = m.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	err = m.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = m.Remove()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *InstanceModeSuite) TestStartWithStateOpenPortsBroken(c *gc.C) {
	app := s.AddTestingService(c, "wordpress", s.charm)
	err := app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)
	u, m := s.addUnit(c, app)
	inst := s.startInstance(c, m)

	err = u.OpenPort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)

	// Nothing open without firewaller.
	s.assertPorts(c, inst, m.Id(), nil)
	dummy.SetInstanceBroken(inst, "OpenPorts")

	// Starting the firewaller should attempt to open the ports,
	// and fail due to the method being broken.
	fw := s.newFirewaller(c)

	errc := make(chan error, 1)
	go func() { errc <- fw.Wait() }()
	s.BackingState.StartSync()
	select {
	case err := <-errc:
		c.Assert(err, gc.ErrorMatches,
			`cannot respond to units changes for "machine-1": dummyInstance.OpenPorts is broken`)
	case <-time.After(coretesting.LongWait):
		fw.Kill()
		fw.Wait()
		c.Fatal("timed out waiting for firewaller to stop")
	}
}

type GlobalModeSuite struct {
	firewallerBaseSuite
}

var _ = gc.Suite(&GlobalModeSuite{})

func (s *GlobalModeSuite) SetUpTest(c *gc.C) {
	s.firewallerBaseSuite.setUpTest(c, config.FwGlobal)
}

func (s *GlobalModeSuite) TearDownTest(c *gc.C) {
	s.firewallerBaseSuite.JujuConnSuite.TearDownTest(c)
}

func (s *GlobalModeSuite) newFirewaller(c *gc.C) worker.Worker {
	fw, err := firewaller.NewFirewaller(s.Environ, s.firewaller, config.FwGlobal)
	c.Assert(err, jc.ErrorIsNil)
	return fw
}

func (s *GlobalModeSuite) TestStartStop(c *gc.C) {
	fw := s.newFirewaller(c)
	statetesting.AssertKillAndWait(c, fw)
}

func (s *GlobalModeSuite) TestGlobalMode(c *gc.C) {
	// Start firewaller and open ports.
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	app1 := s.AddTestingService(c, "wordpress", s.charm)
	err := app1.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	u1, m1 := s.addUnit(c, app1)
	s.startInstance(c, m1)
	err = u1.OpenPorts("tcp", 80, 90)
	c.Assert(err, jc.ErrorIsNil)
	err = u1.OpenPort("tcp", 8080)
	c.Assert(err, jc.ErrorIsNil)

	app2 := s.AddTestingService(c, "moinmoin", s.charm)
	c.Assert(err, jc.ErrorIsNil)
	err = app2.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	u2, m2 := s.addUnit(c, app2)
	s.startInstance(c, m2)
	err = u2.OpenPorts("tcp", 80, 90)
	c.Assert(err, jc.ErrorIsNil)

	s.assertEnvironPorts(c, []network.PortRange{{80, 90, "tcp"}, {8080, 8080, "tcp"}})

	// Closing a port opened by a different unit won't touch the environment.
	err = u1.ClosePorts("tcp", 80, 90)
	c.Assert(err, jc.ErrorIsNil)
	s.assertEnvironPorts(c, []network.PortRange{{80, 90, "tcp"}, {8080, 8080, "tcp"}})

	// Closing a port used just once changes the environment.
	err = u1.ClosePort("tcp", 8080)
	c.Assert(err, jc.ErrorIsNil)
	s.assertEnvironPorts(c, []network.PortRange{{80, 90, "tcp"}})

	// Closing the last port also modifies the environment.
	err = u2.ClosePorts("tcp", 80, 90)
	c.Assert(err, jc.ErrorIsNil)
	s.assertEnvironPorts(c, nil)
}

func (s *GlobalModeSuite) TestStartWithUnexposedService(c *gc.C) {
	m, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	s.startInstance(c, m)

	app := s.AddTestingService(c, "wordpress", s.charm)
	u, err := app.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = u.AssignToMachine(m)
	c.Assert(err, jc.ErrorIsNil)
	err = u.OpenPort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)

	// Starting the firewaller, no open ports.
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	s.assertEnvironPorts(c, nil)

	// Expose service.
	err = app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)
	s.assertEnvironPorts(c, []network.PortRange{{80, 80, "tcp"}})
}

func (s *GlobalModeSuite) TestRestart(c *gc.C) {
	// Start firewaller and open ports.
	fw := s.newFirewaller(c)

	app := s.AddTestingService(c, "wordpress", s.charm)
	err := app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	u, m := s.addUnit(c, app)
	s.startInstance(c, m)
	err = u.OpenPorts("tcp", 80, 90)
	c.Assert(err, jc.ErrorIsNil)
	err = u.OpenPort("tcp", 8080)
	c.Assert(err, jc.ErrorIsNil)

	s.assertEnvironPorts(c, []network.PortRange{{80, 90, "tcp"}, {8080, 8080, "tcp"}})

	// Stop firewaller and close one and open a different port.
	err = worker.Stop(fw)
	c.Assert(err, jc.ErrorIsNil)

	err = u.ClosePort("tcp", 8080)
	c.Assert(err, jc.ErrorIsNil)
	err = u.OpenPort("tcp", 8888)
	c.Assert(err, jc.ErrorIsNil)

	// Start firewaller and check port.
	fw = s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	s.assertEnvironPorts(c, []network.PortRange{{80, 90, "tcp"}, {8888, 8888, "tcp"}})
}

func (s *GlobalModeSuite) TestRestartUnexposedService(c *gc.C) {
	// Start firewaller and open ports.
	fw := s.newFirewaller(c)

	app := s.AddTestingService(c, "wordpress", s.charm)
	err := app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	u, m := s.addUnit(c, app)
	s.startInstance(c, m)
	err = u.OpenPort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)
	err = u.OpenPort("tcp", 8080)
	c.Assert(err, jc.ErrorIsNil)

	s.assertEnvironPorts(c, []network.PortRange{{80, 80, "tcp"}, {8080, 8080, "tcp"}})

	// Stop firewaller and clear exposed flag on service.
	err = worker.Stop(fw)
	c.Assert(err, jc.ErrorIsNil)

	err = app.ClearExposed()
	c.Assert(err, jc.ErrorIsNil)

	// Start firewaller and check port.
	fw = s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	s.assertEnvironPorts(c, nil)
}

func (s *GlobalModeSuite) TestRestartPortCount(c *gc.C) {
	// Start firewaller and open ports.
	fw := s.newFirewaller(c)

	app1 := s.AddTestingService(c, "wordpress", s.charm)
	err := app1.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	u1, m1 := s.addUnit(c, app1)
	s.startInstance(c, m1)
	err = u1.OpenPort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)
	err = u1.OpenPort("tcp", 8080)
	c.Assert(err, jc.ErrorIsNil)

	s.assertEnvironPorts(c, []network.PortRange{{80, 80, "tcp"}, {8080, 8080, "tcp"}})

	// Stop firewaller and add another service using the port.
	err = worker.Stop(fw)
	c.Assert(err, jc.ErrorIsNil)

	app2 := s.AddTestingService(c, "moinmoin", s.charm)
	err = app2.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	u2, m2 := s.addUnit(c, app2)
	s.startInstance(c, m2)
	err = u2.OpenPort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)

	// Start firewaller and check port.
	fw = s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	s.assertEnvironPorts(c, []network.PortRange{{80, 80, "tcp"}, {8080, 8080, "tcp"}})

	// Closing a port opened by a different unit won't touch the environment.
	err = u1.ClosePort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)
	s.assertEnvironPorts(c, []network.PortRange{{80, 80, "tcp"}, {8080, 8080, "tcp"}})

	// Closing a port used just once changes the environment.
	err = u1.ClosePort("tcp", 8080)
	c.Assert(err, jc.ErrorIsNil)
	s.assertEnvironPorts(c, []network.PortRange{{80, 80, "tcp"}})

	// Closing the last port also modifies the environment.
	err = u2.ClosePort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)
	s.assertEnvironPorts(c, nil)
}

type NoneModeSuite struct {
	firewallerBaseSuite
}

var _ = gc.Suite(&NoneModeSuite{})

func (s *NoneModeSuite) SetUpTest(c *gc.C) {
	s.firewallerBaseSuite.setUpTest(c, config.FwNone)
}

func (s *NoneModeSuite) TestStopImmediately(c *gc.C) {
	_, err := firewaller.NewFirewaller(s.Environ, s.firewaller, config.FwNone)
	c.Assert(err, gc.ErrorMatches, `invalid firewall-mode "none"`)
}
