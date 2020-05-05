// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	"fmt"
	"reflect"
	"sync/atomic"
	"time"

	"github.com/juju/charm/v7"
	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/worker/v2"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v2/bakery"

	"github.com/juju/juju/api"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/credentialvalidator"
	"github.com/juju/juju/api/crossmodelrelations"
	apifirewaller "github.com/juju/juju/api/firewaller"
	"github.com/juju/juju/api/remoterelations"
	apitesting "github.com/juju/juju/api/testing"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/firewall"
	corenetwork "github.com/juju/juju/core/network"
	networktesting "github.com/juju/juju/core/network/testing"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/worker/firewaller"
)

// firewallerBaseSuite implements common functionality for embedding
// into each of the other per-mode suites.
type firewallerBaseSuite struct {
	jujutesting.JujuConnSuite
	op                 <-chan dummy.Operation
	charm              *state.Charm
	controllerMachine  *state.Machine
	controllerPassword string

	st                   api.Connection
	firewaller           *apifirewaller.Client
	remoteRelations      *remoterelations.Client
	crossmodelFirewaller *crossmodelrelations.Client
	clock                clock.Clock

	callCtx           context.ProviderCallContext
	credentialsFacade *credentialvalidator.Facade
}

func (s *firewallerBaseSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	s.callCtx = context.NewCloudCallContext()
}

var _ worker.Worker = (*firewaller.Firewaller)(nil)

func (s *firewallerBaseSuite) setUpTest(c *gc.C, firewallMode string) {
	add := map[string]interface{}{"firewall-mode": firewallMode}
	s.DummyConfig = dummy.SampleConfig().Merge(add).Delete("admin-secret")

	s.JujuConnSuite.SetUpTest(c)
	s.charm = s.AddTestingCharm(c, "dummy")

	// Create a manager machine and login to the API.
	var err error
	s.controllerMachine, err = s.State.AddMachine("quantal", state.JobManageModel)
	c.Assert(err, jc.ErrorIsNil)
	s.controllerPassword, err = utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = s.controllerMachine.SetPassword(s.controllerPassword)
	c.Assert(err, jc.ErrorIsNil)
	err = s.controllerMachine.SetProvisioned("i-manager", "", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.st = s.OpenAPIAsMachine(c, s.controllerMachine.Tag(), s.controllerPassword, "fake_nonce")
	c.Assert(s.st, gc.NotNil)

	// Create the API facades.
	firewallerClient, err := apifirewaller.NewClient(s.st)
	c.Assert(err, jc.ErrorIsNil)
	s.firewaller = firewallerClient
	s.remoteRelations = remoterelations.NewClient(s.st)
	c.Assert(s.remoteRelations, gc.NotNil)

	s.credentialsFacade = credentialvalidator.NewFacade(s.st)
}

// assertPorts retrieves the open ports of the instance and compares them
// to the expected.
func (s *firewallerBaseSuite) assertPorts(c *gc.C, inst instances.Instance, machineId string, expected []network.IngressRule) {
	fwInst, ok := inst.(instances.InstanceFirewaller)
	c.Assert(ok, gc.Equals, true)

	start := time.Now()
	for {
		s.BackingState.StartSync()
		got, err := fwInst.IngressRules(s.callCtx, machineId)
		if err != nil {
			c.Fatal(err)
			return
		}
		network.SortIngressRules(got)
		network.SortIngressRules(expected)
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
func (s *firewallerBaseSuite) assertEnvironPorts(c *gc.C, expected []network.IngressRule) {
	fwEnv, ok := s.Environ.(environs.Firewaller)
	c.Assert(ok, gc.Equals, true)

	start := time.Now()
	for {
		s.BackingState.StartSync()
		got, err := fwEnv.IngressRules(s.callCtx)
		if err != nil {
			c.Fatal(err)
			return
		}
		network.SortIngressRules(got)
		network.SortIngressRules(expected)
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
	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)
	id, err := u.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	m, err := s.State.Machine(id)
	c.Assert(err, jc.ErrorIsNil)
	return u, m
}

// startInstance starts a new instance for the given machine.
func (s *firewallerBaseSuite) startInstance(c *gc.C, m *state.Machine) instances.Instance {
	inst, hc := jujutesting.AssertStartInstance(c, s.Environ, s.callCtx, s.ControllerConfig.ControllerUUID(), m.Id())
	err := m.SetProvisioned(inst.Id(), "", "fake_nonce", hc)
	c.Assert(err, jc.ErrorIsNil)
	return inst
}

type InstanceModeSuite struct {
	firewallerBaseSuite
	networktesting.FirewallHelper
}

var _ = gc.Suite(&InstanceModeSuite{})

func (s *InstanceModeSuite) SetUpTest(c *gc.C) {
	s.firewallerBaseSuite.setUpTest(c, config.FwInstance)
}

// mockClock will panic if anything but After is called
type mockClock struct {
	clock.Clock
	wait time.Duration
	c    *gc.C
}

func (m *mockClock) Now() time.Time {
	return time.Now()
}

func (m *mockClock) After(duration time.Duration) <-chan time.Time {
	m.wait = duration
	return time.After(time.Millisecond)
}

func (s *InstanceModeSuite) newFirewaller(c *gc.C) worker.Worker {
	return s.newFirewallerWithClock(c, &mockClock{c: c})
}

func (s *InstanceModeSuite) newFirewallerWithClock(c *gc.C, clock clock.Clock) worker.Worker {
	s.clock = clock
	fwEnv, ok := s.Environ.(environs.Firewaller)
	c.Assert(ok, gc.Equals, true)

	cfg := firewaller.Config{
		ModelUUID:          s.State.ModelUUID(),
		Mode:               config.FwInstance,
		EnvironFirewaller:  fwEnv,
		EnvironInstances:   s.Environ,
		FirewallerAPI:      s.firewaller,
		RemoteRelationsApi: s.remoteRelations,
		NewCrossModelFacadeFunc: func(*api.Info) (firewaller.CrossModelFirewallerFacadeCloser, error) {
			return s.crossmodelFirewaller, nil
		},
		Clock:         s.clock,
		Logger:        loggo.GetLogger("test"),
		CredentialAPI: s.credentialsFacade,
	}
	fw, err := firewaller.NewFirewaller(cfg)
	c.Assert(err, jc.ErrorIsNil)
	return fw
}

func (s *InstanceModeSuite) TestStartStop(c *gc.C) {
	fw := s.newFirewaller(c)
	statetesting.AssertKillAndWait(c, fw)
}

func (s *InstanceModeSuite) TestNotExposedApplication(c *gc.C) {
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	app := s.AddTestingApplication(c, "wordpress", s.charm)
	u, m := s.addUnit(c, app)
	inst := s.startInstance(c, m)

	s.AssertOpenUnitPort(c, u, "", "tcp", 80)
	s.AssertOpenUnitPort(c, u, "", "tcp", 8080)

	s.assertPorts(c, inst, m.Id(), nil)

	s.AssertCloseUnitPort(c, u, "", "tcp", 80)

	s.assertPorts(c, inst, m.Id(), nil)
}

func (s *InstanceModeSuite) TestExposedApplication(c *gc.C) {
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	app := s.AddTestingApplication(c, "wordpress", s.charm)

	err := app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)
	u, m := s.addUnit(c, app)
	inst := s.startInstance(c, m)

	s.AssertOpenUnitPorts(c, u, "", "tcp", 80, 90)
	s.AssertOpenUnitPort(c, u, "", "tcp", 8080)

	s.assertPorts(c, inst, m.Id(), []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 90, "0.0.0.0/0"),
		network.MustNewIngressRule("tcp", 8080, 8080, "0.0.0.0/0"),
	})

	s.AssertCloseUnitPorts(c, u, "", "tcp", 80, 90)

	s.assertPorts(c, inst, m.Id(), []network.IngressRule{
		network.MustNewIngressRule("tcp", 8080, 8080, "0.0.0.0/0"),
	})
}

func (s *InstanceModeSuite) TestMultipleExposedApplications(c *gc.C) {
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	app1 := s.AddTestingApplication(c, "wordpress", s.charm)
	err := app1.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	u1, m1 := s.addUnit(c, app1)
	inst1 := s.startInstance(c, m1)
	s.AssertOpenUnitPort(c, u1, "", "tcp", 80)
	s.AssertOpenUnitPort(c, u1, "", "tcp", 8080)

	app2 := s.AddTestingApplication(c, "mysql", s.charm)
	c.Assert(err, jc.ErrorIsNil)
	err = app2.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	u2, m2 := s.addUnit(c, app2)
	inst2 := s.startInstance(c, m2)
	s.AssertOpenUnitPort(c, u2, "", "tcp", 3306)

	s.assertPorts(c, inst1, m1.Id(), []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 80, "0.0.0.0/0"),
		network.MustNewIngressRule("tcp", 8080, 8080, "0.0.0.0/0"),
	})
	s.assertPorts(c, inst2, m2.Id(), []network.IngressRule{
		network.MustNewIngressRule("tcp", 3306, 3306, "0.0.0.0/0"),
	})

	s.AssertCloseUnitPort(c, u1, "", "tcp", 80)
	s.AssertCloseUnitPort(c, u2, "", "tcp", 3306)

	s.assertPorts(c, inst1, m1.Id(), []network.IngressRule{
		network.MustNewIngressRule("tcp", 8080, 8080, "0.0.0.0/0"),
	})
	s.assertPorts(c, inst2, m2.Id(), nil)
}

func (s *InstanceModeSuite) TestMachineWithoutInstanceId(c *gc.C) {
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	app := s.AddTestingApplication(c, "wordpress", s.charm)
	err := app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)
	// add a unit but don't start its instance yet.
	u1, m1 := s.addUnit(c, app)

	// add another unit and start its instance, so that
	// we're sure the firewaller has seen the first instance.
	u2, m2 := s.addUnit(c, app)
	inst2 := s.startInstance(c, m2)
	s.AssertOpenUnitPort(c, u2, "", "tcp", 80)
	s.assertPorts(c, inst2, m2.Id(), []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 80, "0.0.0.0/0"),
	})

	inst1 := s.startInstance(c, m1)
	s.AssertOpenUnitPort(c, u1, "", "tcp", 8080)
	s.assertPorts(c, inst1, m1.Id(), []network.IngressRule{
		network.MustNewIngressRule("tcp", 8080, 8080, "0.0.0.0/0"),
	})
}

func (s *InstanceModeSuite) TestMultipleUnits(c *gc.C) {
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	app := s.AddTestingApplication(c, "wordpress", s.charm)
	err := app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	u1, m1 := s.addUnit(c, app)
	inst1 := s.startInstance(c, m1)
	s.AssertOpenUnitPort(c, u1, "", "tcp", 80)

	u2, m2 := s.addUnit(c, app)
	inst2 := s.startInstance(c, m2)
	s.AssertOpenUnitPort(c, u2, "", "tcp", 80)

	s.assertPorts(c, inst1, m1.Id(), []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 80, "0.0.0.0/0"),
	})
	s.assertPorts(c, inst2, m2.Id(), []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 80, "0.0.0.0/0"),
	})

	s.AssertCloseUnitPort(c, u1, "", "tcp", 80)
	s.AssertCloseUnitPort(c, u2, "", "tcp", 80)

	s.assertPorts(c, inst1, m1.Id(), nil)
	s.assertPorts(c, inst2, m2.Id(), nil)
}

func (s *InstanceModeSuite) TestStartWithState(c *gc.C) {
	app := s.AddTestingApplication(c, "wordpress", s.charm)
	err := app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)
	u, m := s.addUnit(c, app)
	inst := s.startInstance(c, m)

	s.AssertOpenUnitPort(c, u, "", "tcp", 80)
	s.AssertOpenUnitPort(c, u, "", "tcp", 8080)

	// Nothing open without firewaller.
	s.assertPorts(c, inst, m.Id(), nil)

	// Starting the firewaller opens the ports.
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	s.assertPorts(c, inst, m.Id(), []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 80, "0.0.0.0/0"),
		network.MustNewIngressRule("tcp", 8080, 8080, "0.0.0.0/0"),
	})

	err = app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *InstanceModeSuite) TestStartWithPartialState(c *gc.C) {
	m, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	inst := s.startInstance(c, m)

	app := s.AddTestingApplication(c, "wordpress", s.charm)
	err = app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	// Starting the firewaller, no open ports.
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	s.assertPorts(c, inst, m.Id(), nil)

	// Complete steps to open port.
	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = u.AssignToMachine(m)
	c.Assert(err, jc.ErrorIsNil)
	s.AssertOpenUnitPort(c, u, "", "tcp", 80)

	s.assertPorts(c, inst, m.Id(), []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 80, "0.0.0.0/0"),
	})
}

func (s *InstanceModeSuite) TestStartWithUnexposedApplication(c *gc.C) {
	m, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	inst := s.startInstance(c, m)

	app := s.AddTestingApplication(c, "wordpress", s.charm)
	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = u.AssignToMachine(m)
	c.Assert(err, jc.ErrorIsNil)
	s.AssertOpenUnitPort(c, u, "", "tcp", 80)

	// Starting the firewaller, no open ports.
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	s.assertPorts(c, inst, m.Id(), nil)

	// Expose service.
	err = app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)
	s.assertPorts(c, inst, m.Id(), []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 80, "0.0.0.0/0"),
	})
}

func assertMachineInMachineds(c *gc.C, fw *firewaller.Firewaller, tag names.MachineTag, find bool) {
	machineds := firewaller.GetMachineds(fw)
	_, found := machineds[tag]
	c.Assert(found, gc.Equals, find)
}

func (s *InstanceModeSuite) TestStartMachineWithManualMachine(c *gc.C) {
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	m, err := s.State.AddOneMachine(state.MachineTemplate{
		Series:     "quantal",
		Jobs:       []state.MachineJob{state.JobHostUnits},
		InstanceId: "2",
		Nonce:      "manual:",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = firewaller.StartMachine(fw.(*firewaller.Firewaller), m.MachineTag())
	c.Assert(err, jc.ErrorIsNil)
	assertMachineInMachineds(c, fw.(*firewaller.Firewaller), m.MachineTag(), false)

	m2, err := s.State.AddOneMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	})
	c.Assert(err, jc.ErrorIsNil)
	err = firewaller.StartMachine(fw.(*firewaller.Firewaller), m2.MachineTag())
	c.Assert(err, jc.ErrorIsNil)
	assertMachineInMachineds(c, fw.(*firewaller.Firewaller), m2.MachineTag(), true)
}

func (s *InstanceModeSuite) TestSetClearExposedApplication(c *gc.C) {
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	app := s.AddTestingApplication(c, "wordpress", s.charm)

	u, m := s.addUnit(c, app)
	inst := s.startInstance(c, m)
	s.AssertOpenUnitPort(c, u, "", "tcp", 80)
	s.AssertOpenUnitPort(c, u, "", "tcp", 8080)

	// Not exposed service, so no open port.
	s.assertPorts(c, inst, m.Id(), nil)

	// SeExposed opens the ports.
	err := app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	s.assertPorts(c, inst, m.Id(), []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 80, "0.0.0.0/0"),
		network.MustNewIngressRule("tcp", 8080, 8080, "0.0.0.0/0"),
	})

	// ClearExposed closes the ports again.
	err = app.ClearExposed()
	c.Assert(err, jc.ErrorIsNil)

	s.assertPorts(c, inst, m.Id(), nil)
}

func (s *InstanceModeSuite) TestRemoveUnit(c *gc.C) {
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	app := s.AddTestingApplication(c, "wordpress", s.charm)
	err := app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	u1, m1 := s.addUnit(c, app)
	inst1 := s.startInstance(c, m1)
	s.AssertOpenUnitPort(c, u1, "", "tcp", 80)

	u2, m2 := s.addUnit(c, app)
	inst2 := s.startInstance(c, m2)
	s.AssertOpenUnitPort(c, u2, "", "tcp", 80)

	s.assertPorts(c, inst1, m1.Id(), []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 80, "0.0.0.0/0"),
	})
	s.assertPorts(c, inst2, m2.Id(), []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 80, "0.0.0.0/0"),
	})

	// Remove unit.
	err = u1.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = u1.Remove()
	c.Assert(err, jc.ErrorIsNil)

	s.assertPorts(c, inst1, m1.Id(), nil)
	s.assertPorts(c, inst2, m2.Id(), []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 80, "0.0.0.0/0"),
	})
}

func (s *InstanceModeSuite) TestRemoveApplication(c *gc.C) {
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	app := s.AddTestingApplication(c, "wordpress", s.charm)
	err := app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	u, m := s.addUnit(c, app)
	inst := s.startInstance(c, m)
	s.AssertOpenUnitPort(c, u, "", "tcp", 80)

	s.assertPorts(c, inst, m.Id(), []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 80, "0.0.0.0/0"),
	})

	// Remove application.
	err = u.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = u.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = app.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	s.assertPorts(c, inst, m.Id(), nil)
}

func (s *InstanceModeSuite) TestRemoveMultipleApplications(c *gc.C) {
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	app1 := s.AddTestingApplication(c, "wordpress", s.charm)
	err := app1.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	u1, m1 := s.addUnit(c, app1)
	inst1 := s.startInstance(c, m1)
	s.AssertOpenUnitPort(c, u1, "", "tcp", 80)

	app2 := s.AddTestingApplication(c, "mysql", s.charm)
	err = app2.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	u2, m2 := s.addUnit(c, app2)
	inst2 := s.startInstance(c, m2)
	s.AssertOpenUnitPort(c, u2, "", "tcp", 3306)

	s.assertPorts(c, inst1, m1.Id(), []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 80, "0.0.0.0/0"),
	})
	s.assertPorts(c, inst2, m2.Id(), []network.IngressRule{
		network.MustNewIngressRule("tcp", 3306, 3306, "0.0.0.0/0"),
	})

	// Remove applications.
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

	app := s.AddTestingApplication(c, "wordpress", s.charm)
	err := app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	u, m := s.addUnit(c, app)
	inst := s.startInstance(c, m)
	s.AssertOpenUnitPort(c, u, "", "tcp", 80)

	s.assertPorts(c, inst, m.Id(), []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 80, "0.0.0.0/0"),
	})

	// Remove unit and application, also tested without. Has no effect.
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

	app := s.AddTestingApplication(c, "wordpress", s.charm)
	err := app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	u, m := s.addUnit(c, app)
	inst := s.startInstance(c, m)
	s.AssertOpenUnitPort(c, u, "", "tcp", 80)

	s.assertPorts(c, inst, m.Id(), []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 80, "0.0.0.0/0"),
	})

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

	// TODO (manadart 2019-02-01): This fails intermittently with a "not found"
	// error for the machine. This is not a huge problem in production, as the
	// worker will restart and proceed happily thereafter.
	// That error is detected here for expediency, but the ideal mitigation is
	// a refactoring of the worker logic as per LP:1814277.
	fw.Kill()
	err = fw.Wait()
	c.Assert(err == nil || params.IsCodeNotFound(err), jc.IsTrue)
}

func (s *InstanceModeSuite) TestStartWithStateOpenPortsBroken(c *gc.C) {
	app := s.AddTestingApplication(c, "wordpress", s.charm)
	err := app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)
	u, m := s.addUnit(c, app)
	inst := s.startInstance(c, m)

	s.AssertOpenUnitPort(c, u, "", "tcp", 80)

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

func (s *InstanceModeSuite) setupRemoteRelationRequirerRoleConsumingSide(
	c *gc.C, published chan bool, apiErr *bool, ingressRequired *bool, clock clock.Clock,
) (worker.Worker, *state.RelationUnit) {
	// Set up the consuming model - create the local app.
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	// Set up the consuming model - create the remote app.
	offeringModelTag := names.NewModelTag(utils.MustNewUUID().String())
	appToken := utils.MustNewUUID().String()
	app, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "mysql", SourceModel: offeringModelTag,
		Endpoints: []charm.Relation{{Name: "database", Interface: "mysql", Role: "provider", Scope: "global"}},
	})
	c.Assert(err, jc.ErrorIsNil)
	// Create the external controller info.
	ec := state.NewExternalControllers(s.State)
	_, err = ec.Save(crossmodel.ControllerInfo{
		ControllerTag: coretesting.ControllerTag,
		Addrs:         []string{"1.2.3.4:1234"},
		CACert:        coretesting.CACert}, offeringModelTag.Id())
	c.Assert(err, jc.ErrorIsNil)

	mac, err := apitesting.NewMacaroon("apimac")
	c.Assert(err, jc.ErrorIsNil)
	var relToken string
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CrossModelRelations")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "PublishIngressNetworkChanges")
		expected := params.IngressNetworksChanges{
			Changes: []params.IngressNetworksChangeEvent{{
				RelationToken:    relToken,
				ApplicationToken: appToken,
			}},
		}

		// Extract macaroons so we can compare them separately
		// (as they can't be compared using DeepEquals due to 'UnmarshaledAs')
		expectedMacs := arg.(params.IngressNetworksChanges).Changes[0].Macaroons
		arg.(params.IngressNetworksChanges).Changes[0].Macaroons = nil
		c.Assert(len(expectedMacs), gc.Equals, 1)
		apitesting.MacaroonEquals(c, expectedMacs[0], mac)

		// Networks may be empty or not, depending on coalescing of watcher events.
		// We may get an initial empty event followed by an event with a network.
		// Or we may get just the event with a network.
		// Set the arg networks to empty and compare below.
		changes := arg.(params.IngressNetworksChanges)
		argNetworks := changes.Changes[0].Networks
		argIngressRequired := changes.Changes[0].IngressRequired

		changes.Changes[0].Networks = nil
		expected.Changes[0].IngressRequired = argIngressRequired
		expected.Changes[0].BakeryVersion = bakery.LatestVersion
		c.Check(arg, gc.DeepEquals, expected)

		if !*ingressRequired {
			c.Assert(changes.Changes[0].Networks, gc.HasLen, 0)
		}
		if *ingressRequired && len(argNetworks) > 0 {
			c.Assert(argIngressRequired, jc.IsTrue)
			c.Assert(argNetworks, jc.DeepEquals, []string{"10.0.0.4/32"})
		}
		changes.Changes[0].Macaroons = expectedMacs
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}
		if *apiErr {
			return errors.New("fail")
		}
		if !*ingressRequired || len(argNetworks) > 0 {
			published <- true
		}
		return nil
	})

	s.crossmodelFirewaller = crossmodelrelations.NewClient(apiCaller)
	c.Assert(s.crossmodelFirewaller, gc.NotNil)

	// Create the firewaller facade on the consuming model.
	fw := s.newFirewallerWithClock(c, clock)

	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	// Export the relation details so the firewaller knows it's ready to be processed.
	re := s.State.RemoteEntities()
	relToken, err = re.ExportLocalEntity(rel.Tag())
	c.Assert(err, jc.ErrorIsNil)
	err = re.SaveMacaroon(rel.Tag(), mac)
	c.Assert(err, jc.ErrorIsNil)
	err = re.ImportRemoteEntity(app.Tag(), appToken)
	c.Assert(err, jc.ErrorIsNil)

	// We should not have published any ingress events yet - no unit has entered scope.
	select {
	case <-time.After(coretesting.ShortWait):
	case <-published:
		c.Fatal("unexpected ingress change to be published")
	}

	// Add a public address to the consuming unit so the firewaller can use it.
	wpm := s.Factory.MakeMachine(c, &factory.MachineParams{
		Addresses: corenetwork.SpaceAddresses{corenetwork.NewSpaceAddress("10.0.0.4")},
	})
	u, err := wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = u.AssignToMachine(wpm)
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel.Unit(u)
	c.Assert(err, jc.ErrorIsNil)
	return fw, ru
}

func (s *InstanceModeSuite) TestRemoteRelationRequirerRoleConsumingSide(c *gc.C) {
	published := make(chan bool)
	ingressRequired := true
	apiErr := false
	fw, ru := s.setupRemoteRelationRequirerRoleConsumingSide(c, published, &apiErr, &ingressRequired, &mockClock{c: c})
	defer statetesting.AssertKillAndWait(c, fw)

	// Add a unit on the consuming app and have it enter the relation scope.
	// This will trigger the firewaller to publish the changes.
	err := ru.EnterScope(map[string]interface{}{})
	c.Assert(err, jc.ErrorIsNil)
	s.BackingState.StartSync()
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatal("time out waiting for ingress change to be published on enter scope")
	case <-published:
	}

	// Check the relation ready poll time is as expected.
	c.Assert(s.clock.(*mockClock).wait, gc.Equals, 3*time.Second)

	// Change should be sent when unit leaves scope.
	ingressRequired = false
	err = ru.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	s.BackingState.StartSync()
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatal("time out waiting for ingress change to be published on leave scope")
	case <-published:
	}
}

func (s *InstanceModeSuite) TestRemoteRelationWorkerError(c *gc.C) {
	published := make(chan bool)
	ingressRequired := true
	apiErr := true
	fw, ru := s.setupRemoteRelationRequirerRoleConsumingSide(c, published, &apiErr, &ingressRequired, testclock.NewClock(time.Time{}))
	defer statetesting.AssertKillAndWait(c, fw)

	// Add a unit on the consuming app and have it enter the relation scope.
	// This will trigger the firewaller to try and publish the changes.
	err := ru.EnterScope(map[string]interface{}{})
	c.Assert(err, jc.ErrorIsNil)
	s.BackingState.StartSync()

	// We should not have published any ingress events yet - no changed published.
	select {
	case <-time.After(coretesting.ShortWait):
	case <-published:
		c.Fatal("unexpected ingress change to be published")
	}

	// Give the worker time to restart and try again.
	apiErr = false
	s.clock.(*testclock.Clock).WaitAdvance(60*time.Second, coretesting.LongWait, 1)
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatal("time out waiting for ingress change to be published on enter scope")
	case <-published:
	}
}

func (s *InstanceModeSuite) TestRemoteRelationProviderRoleConsumingSide(c *gc.C) {
	// Set up the consuming model - create the local app.
	s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	// Set up the consuming model - create the remote app.
	offeringModelTag := names.NewModelTag(utils.MustNewUUID().String())
	appToken := utils.MustNewUUID().String()
	app, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "wordpress", SourceModel: offeringModelTag,
		Endpoints: []charm.Relation{{Name: "database", Interface: "mysql", Role: "requirer", Scope: "global"}},
	})
	c.Assert(err, jc.ErrorIsNil)
	// Create the external controller info.
	ec := state.NewExternalControllers(s.State)
	_, err = ec.Save(crossmodel.ControllerInfo{
		ControllerTag: coretesting.ControllerTag,
		Addrs:         []string{"1.2.3.4:1234"},
		CACert:        coretesting.CACert}, offeringModelTag.Id())
	c.Assert(err, jc.ErrorIsNil)

	mac, err := apitesting.NewMacaroon("apimac")
	c.Assert(err, jc.ErrorIsNil)
	watched := make(chan bool)
	var relToken string
	callCount := int32(0)
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		switch atomic.LoadInt32(&callCount) {
		case 0:
			c.Check(objType, gc.Equals, "CrossModelRelations")
			c.Check(version, gc.Equals, 0)
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "WatchEgressAddressesForRelations")
			expected := params.RemoteEntityArgs{
				Args: []params.RemoteEntityArg{{
					Token: relToken,
				}},
			}
			// Extract macaroons so we can compare them separately
			// (as they can't be compared using DeepEquals due to 'UnmarshaledAs')
			rArgs := arg.(params.RemoteEntityArgs)
			newMacs := rArgs.Args[0].Macaroons
			rArgs.Args[0].Macaroons = nil
			apitesting.MacaroonEquals(c, newMacs[0], mac)
			c.Check(arg, gc.DeepEquals, expected)
			c.Assert(result, gc.FitsTypeOf, &params.StringsWatchResults{})
			*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{
				Results: []params.StringsWatchResult{{StringsWatcherId: "1"}},
			}
			watched <- true
		default:
			c.Check(objType, gc.Equals, "StringsWatcher")
		}
		atomic.AddInt32(&callCount, 1)
		return nil
	})

	s.crossmodelFirewaller = crossmodelrelations.NewClient(apiCaller)
	c.Assert(s.crossmodelFirewaller, gc.NotNil)

	// Create the firewaller facade on the consuming model.
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	// Export the relation details so the firewaller knows it's ready to be processed.
	re := s.State.RemoteEntities()
	relToken, err = re.ExportLocalEntity(rel.Tag())
	c.Assert(err, jc.ErrorIsNil)
	err = re.SaveMacaroon(rel.Tag(), mac)
	c.Assert(err, jc.ErrorIsNil)
	err = re.ImportRemoteEntity(app.Tag(), appToken)
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-time.After(coretesting.LongWait):
		c.Fatal("time out waiting for watcher call")
	case <-watched:
	}

	// Check the relation ready poll time is as expected.
	c.Assert(s.clock.(*mockClock).wait, gc.Equals, 3*time.Second)
}

func (s *InstanceModeSuite) TestRemoteRelationIngressRejected(c *gc.C) {
	// Set up the consuming model - create the local app.
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	// Set up the consuming model - create the remote app.
	offeringModelTag := names.NewModelTag(utils.MustNewUUID().String())
	appToken := utils.MustNewUUID().String()

	app, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "mysql", SourceModel: offeringModelTag,
		Endpoints: []charm.Relation{{Name: "database", Interface: "mysql", Role: "provider", Scope: "global"}},
	})
	c.Assert(err, jc.ErrorIsNil)
	// Create the external controller info.
	ec := state.NewExternalControllers(s.State)
	_, err = ec.Save(crossmodel.ControllerInfo{
		ControllerTag: coretesting.ControllerTag,
		Addrs:         []string{"1.2.3.4:1234"},
		CACert:        coretesting.CACert}, offeringModelTag.Id())
	c.Assert(err, jc.ErrorIsNil)

	mac, err := apitesting.NewMacaroon("apimac")
	c.Assert(err, jc.ErrorIsNil)

	published := make(chan bool)
	done := false
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: &params.Error{Code: params.CodeForbidden, Message: "error"}}},
		}
		// We can get more than one api call depending on the
		// granularity of watcher events.
		if !done {
			done = true
			published <- true
		}
		return nil
	})

	s.crossmodelFirewaller = crossmodelrelations.NewClient(apiCaller)
	c.Assert(s.crossmodelFirewaller, gc.NotNil)

	// Create the firewaller facade on the consuming model.
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	// Export the relation details so the firewaller knows it's ready to be processed.
	re := s.State.RemoteEntities()
	_, err = re.ExportLocalEntity(rel.Tag())
	c.Assert(err, jc.ErrorIsNil)
	err = re.SaveMacaroon(rel.Tag(), mac)
	c.Assert(err, jc.ErrorIsNil)
	err = re.ImportRemoteEntity(app.Tag(), appToken)
	c.Assert(err, jc.ErrorIsNil)

	// Add a public address to the consuming unit so the firewaller can use it.
	wpm := s.Factory.MakeMachine(c, &factory.MachineParams{
		Addresses: corenetwork.SpaceAddresses{corenetwork.NewSpaceAddress("10.0.0.4")},
	})
	u, err := wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = u.AssignToMachine(wpm)
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel.Unit(u)
	c.Assert(err, jc.ErrorIsNil)

	// Add a unit on the consuming app and have it enter the relation scope.
	// This will trigger the firewaller to publish the changes.
	err = ru.EnterScope(map[string]interface{}{})
	c.Assert(err, jc.ErrorIsNil)
	s.BackingState.StartSync()
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatal("time out waiting for ingress change to be published on enter scope")
	case <-published:
	}

	// Check that the relation status is set to error.
	s.BackingState.StartSync()
	for attempt := coretesting.LongAttempt.Start(); attempt.Next(); {
		relStatus, err := rel.Status()
		c.Check(err, jc.ErrorIsNil)
		if relStatus.Status != status.Error {
			continue
		}
		c.Check(relStatus.Message, gc.Equals, "error")
		return
	}
	c.Fatal("time out waiting for relation status to be updated")
}

func (s *InstanceModeSuite) assertIngressCidrs(c *gc.C, ingress []string, expected []string) {
	// Set up the offering model - create the local app.
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	u, m := s.addUnit(c, mysql)
	inst := s.startInstance(c, m)

	s.AssertOpenUnitPort(c, u, "", "tcp", 3306)

	// Set up the offering model - create the remote app.
	consumingModelTag := names.NewModelTag(utils.MustNewUUID().String())
	relToken := utils.MustNewUUID().String()
	appToken := utils.MustNewUUID().String()
	app, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "wordpress", SourceModel: consumingModelTag, IsConsumerProxy: true,
		Endpoints: []charm.Relation{{Name: "db", Interface: "mysql", Role: "requirer", Scope: "global"}},
	})
	c.Assert(err, jc.ErrorIsNil)
	s.WaitForModelWatchersIdle(c, s.State.ModelUUID())

	// Create the firewaller facade on the offering model.
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	// Export the relation details so the firewaller knows it's ready to be processed.
	re := s.State.RemoteEntities()
	err = re.ImportRemoteEntity(rel.Tag(), relToken)
	c.Assert(err, jc.ErrorIsNil)
	err = re.ImportRemoteEntity(app.Tag(), appToken)
	c.Assert(err, jc.ErrorIsNil)

	// No port changes yet.
	s.assertPorts(c, inst, m.Id(), nil)

	// Save a new ingress network against the relation.
	rin := state.NewRelationIngressNetworks(s.State)
	_, err = rin.Save(rel.Tag().Id(), false, ingress)
	c.Assert(err, jc.ErrorIsNil)

	//Ports opened.
	s.assertPorts(c, inst, m.Id(), []network.IngressRule{
		network.MustNewIngressRule("tcp", 3306, 3306, expected...),
	})

	// Check the relation ready poll time is as expected.
	c.Assert(s.clock.(*mockClock).wait, gc.Equals, 3*time.Second)

	// Change should be sent when ingress networks disappear.
	_, err = rin.Save(rel.Tag().Id(), false, nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertPorts(c, inst, m.Id(), nil)

	_, err = rin.Save(rel.Tag().Id(), false, ingress)
	c.Assert(err, jc.ErrorIsNil)
	s.assertPorts(c, inst, m.Id(), []network.IngressRule{
		network.MustNewIngressRule("tcp", 3306, 3306, expected...),
	})

	// And again when relation is suspended.
	err = rel.SetSuspended(true, "")
	c.Assert(err, jc.ErrorIsNil)
	s.assertPorts(c, inst, m.Id(), nil)

	// And again when relation is resumed.
	err = rel.SetSuspended(false, "")
	c.Assert(err, jc.ErrorIsNil)
	s.assertPorts(c, inst, m.Id(), []network.IngressRule{
		network.MustNewIngressRule("tcp", 3306, 3306, expected...),
	})

	// And again when relation is destroyed.
	err = rel.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	s.assertPorts(c, inst, m.Id(), nil)
}

func (s *InstanceModeSuite) TestRemoteRelationProviderRoleOffering(c *gc.C) {
	s.assertIngressCidrs(c, []string{"10.0.0.4/16"}, []string{"10.0.0.4/16"})
}

func (s *InstanceModeSuite) TestRemoteRelationIngressFallbackToPublic(c *gc.C) {
	var ingress []string
	for i := 1; i < 30; i++ {
		ingress = append(ingress, fmt.Sprintf("10.%d.0.1/32", i))
	}
	s.assertIngressCidrs(c, ingress, []string{"0.0.0.0/0"})
}

func (s *InstanceModeSuite) TestRemoteRelationIngressFallbackToWhitelist(c *gc.C) {
	fwRules := state.NewFirewallRules(s.State)
	err := fwRules.Save(state.NewFirewallRule(firewall.JujuApplicationOfferRule, []string{"192.168.1.0/16"}))
	c.Assert(err, jc.ErrorIsNil)
	var ingress []string
	for i := 1; i < 30; i++ {
		ingress = append(ingress, fmt.Sprintf("10.%d.0.1/32", i))
	}
	s.assertIngressCidrs(c, ingress, []string{"192.168.1.0/16"})
}

func (s *InstanceModeSuite) TestRemoteRelationIngressMergesCIDRS(c *gc.C) {
	ingress := []string{
		"192.0.1.254/31",
		"192.0.2.0/28",
		"192.0.2.16/28",
		"192.0.2.32/28",
		"192.0.2.48/28",
		"192.0.2.64/28",
		"192.0.2.80/28",
		"192.0.2.96/28",
		"192.0.2.112/28",
		"192.0.2.128/28",
		"192.0.2.144/28",
		"192.0.2.160/28",
		"192.0.2.176/28",
		"192.0.2.192/28",
		"192.0.2.208/28",
		"192.0.2.224/28",
		"192.0.2.240/28",
		"192.0.3.0/28",
		"192.0.4.0/28",
		"192.0.5.0/28",
		"192.0.6.0/28",
	}
	expected := []string{
		"192.0.1.254/31",
		"192.0.2.0/24",
		"192.0.3.0/28",
		"192.0.4.0/28",
		"192.0.5.0/28",
		"192.0.6.0/28",
	}
	s.assertIngressCidrs(c, ingress, expected)
}

type GlobalModeSuite struct {
	firewallerBaseSuite
	networktesting.FirewallHelper
}

var _ = gc.Suite(&GlobalModeSuite{})

func (s *GlobalModeSuite) SetUpTest(c *gc.C) {
	s.firewallerBaseSuite.setUpTest(c, config.FwGlobal)
}

func (s *GlobalModeSuite) TearDownTest(c *gc.C) {
	s.firewallerBaseSuite.JujuConnSuite.TearDownTest(c)
}

func (s *GlobalModeSuite) newFirewaller(c *gc.C) worker.Worker {
	fwEnv, ok := s.Environ.(environs.Firewaller)
	c.Assert(ok, gc.Equals, true)

	cfg := firewaller.Config{
		ModelUUID:          s.State.ModelUUID(),
		Mode:               config.FwGlobal,
		EnvironFirewaller:  fwEnv,
		EnvironInstances:   s.Environ,
		FirewallerAPI:      s.firewaller,
		RemoteRelationsApi: s.remoteRelations,
		NewCrossModelFacadeFunc: func(*api.Info) (firewaller.CrossModelFirewallerFacadeCloser, error) {
			return s.crossmodelFirewaller, nil
		},
		Logger:        loggo.GetLogger("test"),
		CredentialAPI: s.credentialsFacade,
	}
	fw, err := firewaller.NewFirewaller(cfg)
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

	app1 := s.AddTestingApplication(c, "wordpress", s.charm)
	err := app1.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	u1, m1 := s.addUnit(c, app1)
	s.startInstance(c, m1)
	s.AssertOpenUnitPorts(c, u1, "", "tcp", 80, 90)
	s.AssertOpenUnitPort(c, u1, "", "tcp", 8080)

	app2 := s.AddTestingApplication(c, "moinmoin", s.charm)
	c.Assert(err, jc.ErrorIsNil)
	err = app2.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	u2, m2 := s.addUnit(c, app2)
	s.startInstance(c, m2)
	s.AssertOpenUnitPorts(c, u2, "", "tcp", 80, 90)

	s.assertEnvironPorts(c, []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 90, "0.0.0.0/0"),
		network.MustNewIngressRule("tcp", 8080, 8080, "0.0.0.0/0"),
	})

	// Closing a port opened by a different unit won't touch the environment.
	s.AssertCloseUnitPorts(c, u1, "", "tcp", 80, 90)
	s.assertEnvironPorts(c, []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 90, "0.0.0.0/0"),
		network.MustNewIngressRule("tcp", 8080, 8080, "0.0.0.0/0"),
	})

	// Closing a port used just once changes the environment.
	s.AssertCloseUnitPort(c, u1, "", "tcp", 8080)
	s.assertEnvironPorts(c, []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 90, "0.0.0.0/0"),
	})

	// Closing the last port also modifies the environment.
	s.AssertCloseUnitPorts(c, u2, "", "tcp", 80, 90)
	s.assertEnvironPorts(c, nil)
}

func (s *GlobalModeSuite) TestStartWithUnexposedApplication(c *gc.C) {
	m, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	s.startInstance(c, m)

	app := s.AddTestingApplication(c, "wordpress", s.charm)
	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = u.AssignToMachine(m)
	c.Assert(err, jc.ErrorIsNil)
	s.AssertOpenUnitPort(c, u, "", "tcp", 80)

	// Starting the firewaller, no open ports.
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	s.assertEnvironPorts(c, nil)

	// Expose application.
	err = app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)
	s.assertEnvironPorts(c, []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 80, "0.0.0.0/0"),
	})
}

func (s *GlobalModeSuite) TestRestart(c *gc.C) {
	// Start firewaller and open ports.
	fw := s.newFirewaller(c)

	app := s.AddTestingApplication(c, "wordpress", s.charm)
	err := app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	u, m := s.addUnit(c, app)
	s.startInstance(c, m)
	s.AssertOpenUnitPorts(c, u, "", "tcp", 80, 90)
	s.AssertOpenUnitPort(c, u, "", "tcp", 8080)
	c.Assert(err, jc.ErrorIsNil)

	s.assertEnvironPorts(c, []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 90, "0.0.0.0/0"),
		network.MustNewIngressRule("tcp", 8080, 8080, "0.0.0.0/0"),
	})

	// Stop firewaller and close one and open a different port.
	err = worker.Stop(fw)
	c.Assert(err, jc.ErrorIsNil)

	s.AssertCloseUnitPort(c, u, "", "tcp", 8080)
	s.AssertOpenUnitPort(c, u, "", "tcp", 8888)

	// Start firewaller and check port.
	fw = s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	s.assertEnvironPorts(c, []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 90, "0.0.0.0/0"),
		network.MustNewIngressRule("tcp", 8888, 8888, "0.0.0.0/0"),
	})
}

func (s *GlobalModeSuite) TestRestartUnexposedApplication(c *gc.C) {
	// Start firewaller and open ports.
	fw := s.newFirewaller(c)

	app := s.AddTestingApplication(c, "wordpress", s.charm)
	err := app.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	u, m := s.addUnit(c, app)
	s.startInstance(c, m)
	s.AssertOpenUnitPort(c, u, "", "tcp", 80)
	s.AssertOpenUnitPort(c, u, "", "tcp", 8080)

	s.assertEnvironPorts(c, []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 80, "0.0.0.0/0"),
		network.MustNewIngressRule("tcp", 8080, 8080, "0.0.0.0/0"),
	})

	// Stop firewaller and clear exposed flag on application.
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

	app1 := s.AddTestingApplication(c, "wordpress", s.charm)
	err := app1.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	u1, m1 := s.addUnit(c, app1)
	s.startInstance(c, m1)
	s.AssertOpenUnitPort(c, u1, "", "tcp", 80)
	s.AssertOpenUnitPort(c, u1, "", "tcp", 8080)

	s.assertEnvironPorts(c, []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 80, "0.0.0.0/0"),
		network.MustNewIngressRule("tcp", 8080, 8080, "0.0.0.0/0"),
	})

	// Stop firewaller and add another application using the port.
	err = worker.Stop(fw)
	c.Assert(err, jc.ErrorIsNil)

	app2 := s.AddTestingApplication(c, "moinmoin", s.charm)
	err = app2.SetExposed()
	c.Assert(err, jc.ErrorIsNil)

	u2, m2 := s.addUnit(c, app2)
	s.startInstance(c, m2)
	s.AssertOpenUnitPort(c, u2, "", "tcp", 80)

	// Start firewaller and check port.
	fw = s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	s.assertEnvironPorts(c, []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 80, "0.0.0.0/0"),
		network.MustNewIngressRule("tcp", 8080, 8080, "0.0.0.0/0"),
	})

	// Closing a port opened by a different unit won't touch the environment.
	s.AssertCloseUnitPort(c, u1, "", "tcp", 80)
	s.assertEnvironPorts(c, []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 80, "0.0.0.0/0"),
		network.MustNewIngressRule("tcp", 8080, 8080, "0.0.0.0/0"),
	})

	// Closing a port used just once changes the environment.
	s.AssertCloseUnitPort(c, u1, "", "tcp", 8080)
	s.assertEnvironPorts(c, []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 80, "0.0.0.0/0"),
	})

	// Closing the last port also modifies the environment.
	s.AssertCloseUnitPort(c, u2, "", "tcp", 80)
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
	fwEnv, ok := s.Environ.(environs.Firewaller)
	c.Assert(ok, gc.Equals, true)

	cfg := firewaller.Config{
		ModelUUID:          s.State.ModelUUID(),
		Mode:               config.FwNone,
		EnvironFirewaller:  fwEnv,
		EnvironInstances:   s.Environ,
		FirewallerAPI:      s.firewaller,
		RemoteRelationsApi: s.remoteRelations,
		NewCrossModelFacadeFunc: func(*api.Info) (firewaller.CrossModelFirewallerFacadeCloser, error) {
			return s.crossmodelFirewaller, nil
		},
		Logger:        loggo.GetLogger("test"),
		CredentialAPI: s.credentialsFacade,
	}
	_, err := firewaller.NewFirewaller(cfg)
	c.Assert(err, gc.ErrorMatches, `invalid firewall-mode "none"`)
}
