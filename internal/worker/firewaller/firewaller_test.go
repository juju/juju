// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	"fmt"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/charm/v12"
	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/juju/worker/v3"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/agent/credentialvalidator"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/crossmodelrelations"
	apifirewaller "github.com/juju/juju/api/controller/firewaller"
	"github.com/juju/juju/api/controller/remoterelations"
	apitesting "github.com/juju/juju/api/testing"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/models"
	"github.com/juju/juju/internal/worker/firewaller"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

const allEndpoints = ""

// firewallerBaseSuite implements common functionality for embedding
// into each of the other per-mode suites.
type firewallerBaseSuite struct {
	jujutesting.JujuConnSuite
	charm              *state.Charm
	controllerMachine  *state.Machine
	controllerPassword string

	st                   api.Connection
	firewaller           firewaller.FirewallerAPI
	remoteRelations      *remoterelations.Client
	crossmodelFirewaller *crossmodelrelations.Client
	clock                testclock.AdvanceableClock

	callCtx           context.ProviderCallContext
	credentialsFacade *credentialvalidator.Facade
}

func (s *firewallerBaseSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	s.callCtx = context.NewEmptyCloudCallContext()
}

var _ worker.Worker = (*firewaller.Firewaller)(nil)

func (s *firewallerBaseSuite) setUpTest(c *gc.C, firewallMode string) {
	add := map[string]interface{}{"firewall-mode": firewallMode}
	s.DummyConfig = dummy.SampleConfig().Merge(add).Delete("admin-secret")

	s.JujuConnSuite.SetUpTest(c)
	s.charm = s.AddTestingCharm(c, "wordpress")

	// Create a manager machine and login to the API.
	var err error
	s.controllerMachine, err = s.State.AddMachine(state.UbuntuBase("12.10"), state.JobManageModel)
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

// assertIngressRules retrieves the ingress rules from the provided instance
// and compares them to the expected value.
func (s *firewallerBaseSuite) assertIngressRules(c *gc.C, inst instances.Instance, machineId string,
	expected firewall.IngressRules) {
	fwInst, ok := inst.(instances.InstanceFirewaller)
	c.Assert(ok, gc.Equals, true)

	start := time.Now()
	for {
		// Make it more likely for the dust to have settled (there still may
		// be rare cases where a test passes when it shouldn't if expected
		// is nil, which is the initial value).
		time.Sleep(coretesting.ShortWait)

		got, err := fwInst.IngressRules(s.callCtx, machineId)
		if err != nil {
			c.Fatal(err)
		}
		if got.EqualTo(expected) {
			c.Succeed()
			return
		}
		if time.Since(start) > coretesting.LongWait {
			c.Fatalf("timed out: expected %q; got %q", expected, got)
		}
		time.Sleep(coretesting.ShortWait)
	}
}

// assertEnvironPorts retrieves the open ports of environment and compares them
// to the expected.
func (s *firewallerBaseSuite) assertEnvironPorts(c *gc.C, expected firewall.IngressRules) {
	fwEnv, ok := s.Environ.(environs.Firewaller)
	c.Assert(ok, gc.Equals, true)

	start := time.Now()
	for {
		got, err := fwEnv.IngressRules(s.callCtx)
		if err != nil {
			c.Fatal(err)
		}
		if got.EqualTo(expected) {
			c.Succeed()
			return
		}
		if time.Since(start) > coretesting.LongWait {
			c.Fatalf("timed out: expected %q; got %q", expected, got)
		}
		time.Sleep(coretesting.ShortWait)
	}
}

// assertModelIngressRules retrieves the ingress rules from the model firewall
// and compares them to the expected value
func (s *firewallerBaseSuite) assertModelIngressRules(c *gc.C, expected firewall.IngressRules) {
	fwEnv, ok := s.Environ.(models.ModelFirewaller)
	c.Assert(ok, gc.Equals, true)

	start := time.Now()
	for {
		got, err := fwEnv.ModelIngressRules(s.callCtx)
		if err != nil && !errors.IsNotFound(err) {
			c.Fatal(err)
		}
		if got.EqualTo(expected) {
			c.Succeed()
			return
		}
		if time.Since(start) > coretesting.LongWait {
			c.Fatalf("timed out: expected %q; got %q", expected, got)
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
	watchMachineNotify   func(tag names.MachineTag)
	flushModelNotify     func()
	skipFlushModelNotify func()
}

var _ = gc.Suite(&InstanceModeSuite{})

func (s *InstanceModeSuite) SetUpTest(c *gc.C) {
	s.firewallerBaseSuite.setUpTest(c, config.FwInstance)
	s.watchMachineNotify = nil
	s.flushModelNotify = nil
	s.clock = testclock.NewDilatedWallClock(testing.ShortWait)
}

func (s *InstanceModeSuite) newFirewaller(c *gc.C) worker.Worker {
	return s.newFirewallerWithIPV6CIDRSupport(c, true)
}

func (s *InstanceModeSuite) newFirewallerWithIPV6CIDRSupport(c *gc.C, ipv6CIDRSupport bool) worker.Worker {
	fwEnv, ok := s.Environ.(environs.Firewaller)
	c.Assert(ok, gc.Equals, true)

	modelFwEnv, ok := s.Environ.(models.ModelFirewaller)
	c.Assert(ok, gc.Equals, true)

	cfg := firewaller.Config{
		ModelUUID:              s.State.ModelUUID(),
		Mode:                   config.FwInstance,
		EnvironFirewaller:      fwEnv,
		EnvironModelFirewaller: modelFwEnv,
		EnvironInstances:       s.Environ,
		EnvironIPV6CIDRSupport: ipv6CIDRSupport,
		FirewallerAPI:          s.firewaller,
		RemoteRelationsApi:     s.remoteRelations,
		NewCrossModelFacadeFunc: func(*api.Info) (firewaller.CrossModelFirewallerFacadeCloser, error) {
			return s.crossmodelFirewaller, nil
		},
		Clock:                s.clock,
		Logger:               loggo.GetLogger("test"),
		CredentialAPI:        s.credentialsFacade,
		WatchMachineNotify:   s.watchMachineNotify,
		FlushModelNotify:     s.flushModelNotify,
		SkipFlushModelNotify: s.skipFlushModelNotify,
	}
	fw, err := firewaller.NewFirewaller(cfg)
	c.Assert(err, jc.ErrorIsNil)
	return fw
}

func (s *InstanceModeSuite) newFirewallerWithoutModelFirewaller(c *gc.C) worker.Worker {
	fwEnv, ok := s.Environ.(environs.Firewaller)
	c.Assert(ok, gc.Equals, true)

	cfg := firewaller.Config{
		ModelUUID:              s.State.ModelUUID(),
		Mode:                   config.FwInstance,
		EnvironFirewaller:      fwEnv,
		EnvironInstances:       s.Environ,
		EnvironIPV6CIDRSupport: true,
		FirewallerAPI:          s.firewaller,
		RemoteRelationsApi:     s.remoteRelations,
		NewCrossModelFacadeFunc: func(*api.Info) (firewaller.CrossModelFirewallerFacadeCloser, error) {
			return s.crossmodelFirewaller, nil
		},
		Clock:                s.clock,
		Logger:               loggo.GetLogger("test"),
		CredentialAPI:        s.credentialsFacade,
		WatchMachineNotify:   s.watchMachineNotify,
		FlushModelNotify:     s.flushModelNotify,
		SkipFlushModelNotify: s.skipFlushModelNotify,
	}
	fw, err := firewaller.NewFirewaller(cfg)
	c.Assert(err, jc.ErrorIsNil)
	return fw
}

func (s *InstanceModeSuite) TestStartStop(c *gc.C) {
	fw := s.newFirewaller(c)
	statetesting.AssertKillAndWait(c, fw)
}

func (s *InstanceModeSuite) TestStartStopWithoutModelFirewaller(c *gc.C) {
	fw := s.newFirewallerWithoutModelFirewaller(c)
	statetesting.AssertKillAndWait(c, fw)
}

func (s *InstanceModeSuite) testNotExposedApplication(c *gc.C, fw worker.Worker) {
	app := s.AddTestingApplication(c, "wordpress", s.charm)
	u, m := s.addUnit(c, app)
	inst := s.startInstance(c, m)

	mustOpenPortRanges(c, s.State, u, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
		network.MustParsePortRange("8080/tcp"),
	})

	s.assertIngressRules(c, inst, m.Id(), nil)

	mustClosePortRanges(c, s.State, u, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})

	s.assertIngressRules(c, inst, m.Id(), nil)
}

func (s *InstanceModeSuite) TestNotExposedApplication(c *gc.C) {
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)
	s.testNotExposedApplication(c, fw)
}

func (s *InstanceModeSuite) TestNotExposedApplicationWithoutModelFirewaller(c *gc.C) {
	fw := s.newFirewallerWithoutModelFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)
	s.testNotExposedApplication(c, fw)
}

func (s *InstanceModeSuite) TestExposedApplication(c *gc.C) {
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	app := s.AddTestingApplication(c, "wordpress", s.charm)

	err := app.MergeExposeSettings(map[string]state.ExposedEndpoint{
		allEndpoints: {ExposeToCIDRs: []string{firewall.AllNetworksIPV4CIDR}},
	})
	c.Assert(err, jc.ErrorIsNil)
	u, m := s.addUnit(c, app)
	inst := s.startInstance(c, m)

	mustOpenPortRanges(c, s.State, u, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80-90/tcp"),
		network.MustParsePortRange("8080/tcp"),
	})

	s.assertIngressRules(c, inst, m.Id(), firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80-90/tcp"), firewall.AllNetworksIPV4CIDR),
		firewall.NewIngressRule(network.MustParsePortRange("8080/tcp"), firewall.AllNetworksIPV4CIDR),
	})

	mustClosePortRanges(c, s.State, u, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80-90/tcp"),
	})

	s.assertIngressRules(c, inst, m.Id(), firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("8080/tcp"), firewall.AllNetworksIPV4CIDR),
	})
}

func (s *InstanceModeSuite) TestMultipleExposedApplications(c *gc.C) {
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	app1 := s.AddTestingApplication(c, "wordpress", s.charm)
	err := app1.MergeExposeSettings(map[string]state.ExposedEndpoint{
		allEndpoints: {ExposeToCIDRs: []string{firewall.AllNetworksIPV4CIDR}},
	})
	c.Assert(err, jc.ErrorIsNil)

	u1, m1 := s.addUnit(c, app1)
	inst1 := s.startInstance(c, m1)
	mustOpenPortRanges(c, s.State, u1, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
		network.MustParsePortRange("8080/tcp"),
	})

	app2 := s.AddTestingApplication(c, "mysql", s.charm)
	c.Assert(err, jc.ErrorIsNil)
	err = app2.MergeExposeSettings(map[string]state.ExposedEndpoint{
		allEndpoints: {ExposeToCIDRs: []string{firewall.AllNetworksIPV4CIDR}},
	})
	c.Assert(err, jc.ErrorIsNil)

	u2, m2 := s.addUnit(c, app2)
	inst2 := s.startInstance(c, m2)
	mustOpenPortRanges(c, s.State, u2, allEndpoints, []network.PortRange{
		network.MustParsePortRange("3306/tcp"),
	})

	s.assertIngressRules(c, inst1, m1.Id(), firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
		firewall.NewIngressRule(network.MustParsePortRange("8080/tcp"), firewall.AllNetworksIPV4CIDR),
	})
	s.assertIngressRules(c, inst2, m2.Id(), firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("3306/tcp"), firewall.AllNetworksIPV4CIDR),
	})

	mustClosePortRanges(c, s.State, u1, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})
	mustClosePortRanges(c, s.State, u2, allEndpoints, []network.PortRange{
		network.MustParsePortRange("3306/tcp"),
	})

	s.assertIngressRules(c, inst1, m1.Id(), firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("8080/tcp"), firewall.AllNetworksIPV4CIDR),
	})
	s.assertIngressRules(c, inst2, m2.Id(), nil)
}

func (s *InstanceModeSuite) TestMachineWithoutInstanceId(c *gc.C) {
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	app := s.AddTestingApplication(c, "wordpress", s.charm)
	err := app.MergeExposeSettings(map[string]state.ExposedEndpoint{
		allEndpoints: {ExposeToCIDRs: []string{firewall.AllNetworksIPV4CIDR}},
	})
	c.Assert(err, jc.ErrorIsNil)
	// add a unit but don't start its instance yet.
	u1, m1 := s.addUnit(c, app)

	// add another unit and start its instance, so that
	// we're sure the firewaller has seen the first instance.
	u2, m2 := s.addUnit(c, app)
	inst2 := s.startInstance(c, m2)
	mustOpenPortRanges(c, s.State, u2, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})
	s.assertIngressRules(c, inst2, m2.Id(), firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
	})

	inst1 := s.startInstance(c, m1)
	mustOpenPortRanges(c, s.State, u1, allEndpoints, []network.PortRange{
		network.MustParsePortRange("8080/tcp"),
	})
	s.assertIngressRules(c, inst1, m1.Id(), firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("8080/tcp"), firewall.AllNetworksIPV4CIDR),
	})
}

func (s *InstanceModeSuite) TestMultipleUnits(c *gc.C) {
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	app := s.AddTestingApplication(c, "wordpress", s.charm)
	err := app.MergeExposeSettings(map[string]state.ExposedEndpoint{
		allEndpoints: {ExposeToCIDRs: []string{firewall.AllNetworksIPV4CIDR}},
	})
	c.Assert(err, jc.ErrorIsNil)

	u1, m1 := s.addUnit(c, app)
	inst1 := s.startInstance(c, m1)
	mustOpenPortRanges(c, s.State, u1, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})

	u2, m2 := s.addUnit(c, app)
	inst2 := s.startInstance(c, m2)
	mustOpenPortRanges(c, s.State, u2, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})

	s.assertIngressRules(c, inst1, m1.Id(), firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
	})
	s.assertIngressRules(c, inst2, m2.Id(), firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
	})

	mustClosePortRanges(c, s.State, u1, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})
	mustClosePortRanges(c, s.State, u2, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})

	s.assertIngressRules(c, inst1, m1.Id(), nil)
	s.assertIngressRules(c, inst2, m2.Id(), nil)
}

func (s *InstanceModeSuite) TestStartWithState(c *gc.C) {
	app := s.AddTestingApplication(c, "wordpress", s.charm)
	err := app.MergeExposeSettings(map[string]state.ExposedEndpoint{
		allEndpoints: {ExposeToCIDRs: []string{firewall.AllNetworksIPV4CIDR}},
	})
	c.Assert(err, jc.ErrorIsNil)
	u, m := s.addUnit(c, app)
	inst := s.startInstance(c, m)

	mustOpenPortRanges(c, s.State, u, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
		network.MustParsePortRange("8080/tcp"),
	})

	// Nothing open without firewaller.
	s.assertIngressRules(c, inst, m.Id(), nil)

	// Starting the firewaller opens the ports.
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	s.assertIngressRules(c, inst, m.Id(), firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
		firewall.NewIngressRule(network.MustParsePortRange("8080/tcp"), firewall.AllNetworksIPV4CIDR),
	})

	err = app.MergeExposeSettings(nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *InstanceModeSuite) TestStartWithPartialState(c *gc.C) {
	m, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	inst := s.startInstance(c, m)

	app := s.AddTestingApplication(c, "wordpress", s.charm)
	err = app.MergeExposeSettings(map[string]state.ExposedEndpoint{
		allEndpoints: {ExposeToCIDRs: []string{firewall.AllNetworksIPV4CIDR}},
	})
	c.Assert(err, jc.ErrorIsNil)

	// Starting the firewaller, no open ports.
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	s.assertIngressRules(c, inst, m.Id(), nil)

	// Complete steps to open port.
	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = u.AssignToMachine(m)
	c.Assert(err, jc.ErrorIsNil)
	mustOpenPortRanges(c, s.State, u, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})

	s.assertIngressRules(c, inst, m.Id(), firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
	})
}

func (s *InstanceModeSuite) TestStartWithUnexposedApplication(c *gc.C) {
	m, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	inst := s.startInstance(c, m)

	app := s.AddTestingApplication(c, "wordpress", s.charm)
	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = u.AssignToMachine(m)
	c.Assert(err, jc.ErrorIsNil)
	mustOpenPortRanges(c, s.State, u, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})

	// Starting the firewaller, no open ports.
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	s.assertIngressRules(c, inst, m.Id(), nil)

	// Expose service.
	err = app.MergeExposeSettings(map[string]state.ExposedEndpoint{
		allEndpoints: {ExposeToCIDRs: []string{firewall.AllNetworksIPV4CIDR}},
	})
	c.Assert(err, jc.ErrorIsNil)
	s.assertIngressRules(c, inst, m.Id(), firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
	})
}

func (s *InstanceModeSuite) TestStartMachineWithManualMachine(c *gc.C) {
	watching := make(chan names.MachineTag, 10) // buffer to ensure test never blocks
	s.watchMachineNotify = func(tag names.MachineTag) {
		watching <- tag
	}
	assertWatching := func(expected names.MachineTag) {
		select {
		case got := <-watching:
			c.Assert(got, gc.Equals, expected)
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting to watch machine %v", expected.Id())
		}
	}

	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	// Wait for manager machine (started by setUpTest)
	assertWatching(names.NewMachineTag("0"))

	_, err := s.State.AddOneMachine(state.MachineTemplate{
		Base:       state.UbuntuBase("12.10"),
		Jobs:       []state.MachineJob{state.JobHostUnits},
		InstanceId: "2",
		Nonce:      "manual:",
	})
	c.Assert(err, jc.ErrorIsNil)
	select {
	case tag := <-watching:
		c.Fatalf("shouldn't be watching manual machine %v", tag)
	case <-time.After(coretesting.ShortWait):
	}

	m, err := s.State.AddOneMachine(state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	})
	c.Assert(err, jc.ErrorIsNil)
	assertWatching(m.MachineTag())
}

// TODO: remove once JujuConnSuite is gone.
type firewallerAPIShim struct {
	firewaller.FirewallerAPI
	newModelMachineWatcher func() (watcher.StringsWatcher, error)
}

func (a *firewallerAPIShim) WatchModelMachines() (watcher.StringsWatcher, error) {
	return a.newModelMachineWatcher()
}

func (s *InstanceModeSuite) TestFlushModelAfterFirstMachineOnly(c *gc.C) {
	machinesChan := make(chan []string, 1)
	machinesChan <- []string{}
	s.firewaller = &firewallerAPIShim{
		FirewallerAPI: s.firewaller,
		newModelMachineWatcher: func() (watcher.StringsWatcher, error) {
			return watchertest.NewMockStringsWatcher(machinesChan), nil
		},
	}

	flush := make(chan struct{}, 10) // buffer to ensure test never blocks
	s.flushModelNotify = func() {
		flush <- struct{}{}
	}
	skipFlush := make(chan struct{}, 10)
	s.skipFlushModelNotify = func() {
		skipFlush <- struct{}{}
	}

	fw := s.newFirewaller(c)
	fwObj := fw.(*firewaller.Firewaller)

	defer statetesting.AssertKillAndWait(c, fwObj)

	// Initial event from model config watcher
	select {
	case <-skipFlush:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for initial event")
	}

	m1, err := s.State.AddOneMachine(state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	})
	c.Assert(err, jc.ErrorIsNil)
	machinesChan <- []string{m1.Id()}

	select {
	case <-flush:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for first machine event")
	}

	m2, err := s.State.AddOneMachine(state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	})
	c.Assert(err, jc.ErrorIsNil)
	machinesChan <- []string{m2.Id()}

	// Since the initial event successfully configured the model firewall
	// the next machine shouldn't trigger a model flush
	select {
	case <-flush:
		c.Fatalf("unexpected model flush creating machine")
	case <-time.After(coretesting.ShortWait):
	}
}

func (s *InstanceModeSuite) TestFlushModelShouldNotBeCalledWhenNoMachinesAvailable(c *gc.C) {
	machinesChan := make(chan []string, 1)
	machinesChan <- []string{}
	s.firewaller = &firewallerAPIShim{
		FirewallerAPI: s.firewaller,
		newModelMachineWatcher: func() (watcher.StringsWatcher, error) {
			return watchertest.NewMockStringsWatcher(machinesChan), nil
		},
	}

	flush := make(chan struct{}, 10) // buffer to ensure test never blocks
	s.flushModelNotify = func() {
		flush <- struct{}{}
	}
	skipFlush := make(chan struct{}, 10)
	s.skipFlushModelNotify = func() {
		skipFlush <- struct{}{}
	}

	fw := s.newFirewaller(c)
	fwObj := fw.(*firewaller.Firewaller)

	defer statetesting.AssertKillAndWait(c, fwObj)

	// Initial event from model config watcher
	select {
	case <-skipFlush:
	case <-flush:
		c.Fatalf("unexpected model flush when no machines are available")
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for initial event")
	}

	c.Assert(fwObj.NeedsToFlushModel(), jc.IsTrue)
}

func (s *InstanceModeSuite) TestFlushMachineInvokesFlushModel(c *gc.C) {
	machinesChan := make(chan []string, 1)
	machinesChan <- []string{}
	s.firewaller = &firewallerAPIShim{
		FirewallerAPI: s.firewaller,
		newModelMachineWatcher: func() (watcher.StringsWatcher, error) {
			return watchertest.NewMockStringsWatcher(machinesChan), nil
		},
	}
	flush := make(chan struct{}, 10) // buffer to ensure test never blocks
	s.flushModelNotify = func() {
		flush <- struct{}{}
	}
	skipFlush := make(chan struct{}, 10)
	s.skipFlushModelNotify = func() {
		skipFlush <- struct{}{}
	}

	fw := s.newFirewaller(c)
	fwObj := fw.(*firewaller.Firewaller)
	defer statetesting.AssertKillAndWait(c, fwObj)

	// Initial event from model config watcher
	select {
	case <-skipFlush:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for initial event")
	}

	m1, err := s.State.AddOneMachine(state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	})
	c.Assert(err, jc.ErrorIsNil)
	machinesChan <- []string{m1.Id()}

	select {
	case <-flush:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for first machine event")
	}

	firewaller.SetNeedsToFlushModel(fwObj, true)
	err = firewaller.FlushMachine(fwObj)

	select {
	case <-flush:
	case <-skipFlush:
		c.Fatalf("unexpected skip model flush when needsToFlushModel is true and machines are available")
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for first machine event")
	}
}

func (s *InstanceModeSuite) TestFlushMachineShouldNotInvokeFlushModel(c *gc.C) {
	machinesChan := make(chan []string, 1)
	machinesChan <- []string{}
	s.firewaller = &firewallerAPIShim{
		FirewallerAPI: s.firewaller,
		newModelMachineWatcher: func() (watcher.StringsWatcher, error) {
			return watchertest.NewMockStringsWatcher(machinesChan), nil
		},
	}

	flush := false
	s.flushModelNotify = func() {
		flush = true
	}
	skipFlush := make(chan struct{}, 10)
	s.skipFlushModelNotify = func() {
		skipFlush <- struct{}{}
	}

	fw := s.newFirewaller(c)
	fwObj := fw.(*firewaller.Firewaller)
	defer statetesting.AssertKillAndWait(c, fwObj)

	// Initial event from model config watcher
	select {
	case <-skipFlush:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for initial event")
	}

	firewaller.SetNeedsToFlushModel(fwObj, false)
	err := firewaller.FlushMachine(fwObj)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(flush, jc.IsFalse)
}

func (s *InstanceModeSuite) TestSetClearExposedApplication(c *gc.C) {
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	app := s.AddTestingApplication(c, "wordpress", s.charm)

	u, m := s.addUnit(c, app)
	inst := s.startInstance(c, m)
	mustOpenPortRanges(c, s.State, u, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
		network.MustParsePortRange("8080/tcp"),
	})

	// Not exposed service, so no open port.
	s.assertIngressRules(c, inst, m.Id(), nil)

	// SeExposed opens the ports.
	err := app.MergeExposeSettings(map[string]state.ExposedEndpoint{
		allEndpoints: {ExposeToCIDRs: []string{firewall.AllNetworksIPV4CIDR}},
	})
	c.Assert(err, jc.ErrorIsNil)

	s.assertIngressRules(c, inst, m.Id(), firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
		firewall.NewIngressRule(network.MustParsePortRange("8080/tcp"), firewall.AllNetworksIPV4CIDR),
	})

	// ClearExposed closes the ports again.
	err = app.ClearExposed()
	c.Assert(err, jc.ErrorIsNil)

	s.assertIngressRules(c, inst, m.Id(), nil)
}

func (s *InstanceModeSuite) TestRemoveUnit(c *gc.C) {
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	app := s.AddTestingApplication(c, "wordpress", s.charm)
	err := app.MergeExposeSettings(map[string]state.ExposedEndpoint{
		allEndpoints: {ExposeToCIDRs: []string{firewall.AllNetworksIPV4CIDR}},
	})
	c.Assert(err, jc.ErrorIsNil)

	u1, m1 := s.addUnit(c, app)
	inst1 := s.startInstance(c, m1)
	mustOpenPortRanges(c, s.State, u1, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})

	u2, m2 := s.addUnit(c, app)
	inst2 := s.startInstance(c, m2)
	mustOpenPortRanges(c, s.State, u2, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})

	s.assertIngressRules(c, inst1, m1.Id(), firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
	})
	s.assertIngressRules(c, inst2, m2.Id(), firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
	})

	// Remove unit.
	err = u1.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = u1.Remove()
	c.Assert(err, jc.ErrorIsNil)

	s.assertIngressRules(c, inst1, m1.Id(), nil)
	s.assertIngressRules(c, inst2, m2.Id(), firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
	})
}

func (s *InstanceModeSuite) TestRemoveApplication(c *gc.C) {
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	app := s.AddTestingApplication(c, "wordpress", s.charm)
	err := app.MergeExposeSettings(map[string]state.ExposedEndpoint{
		allEndpoints: {ExposeToCIDRs: []string{firewall.AllNetworksIPV4CIDR}},
	})
	c.Assert(err, jc.ErrorIsNil)

	u, m := s.addUnit(c, app)
	inst := s.startInstance(c, m)
	mustOpenPortRanges(c, s.State, u, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})

	s.assertIngressRules(c, inst, m.Id(), firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
	})

	// Remove application.
	err = u.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = u.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = app.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	s.assertIngressRules(c, inst, m.Id(), nil)
}

func (s *InstanceModeSuite) TestRemoveMultipleApplications(c *gc.C) {
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	app1 := s.AddTestingApplication(c, "wordpress", s.charm)
	err := app1.MergeExposeSettings(map[string]state.ExposedEndpoint{
		allEndpoints: {ExposeToCIDRs: []string{firewall.AllNetworksIPV4CIDR}},
	})
	c.Assert(err, jc.ErrorIsNil)

	u1, m1 := s.addUnit(c, app1)
	inst1 := s.startInstance(c, m1)
	mustOpenPortRanges(c, s.State, u1, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})

	app2 := s.AddTestingApplication(c, "mysql", s.charm)
	err = app2.MergeExposeSettings(map[string]state.ExposedEndpoint{
		allEndpoints: {ExposeToCIDRs: []string{firewall.AllNetworksIPV4CIDR}},
	})
	c.Assert(err, jc.ErrorIsNil)

	u2, m2 := s.addUnit(c, app2)
	inst2 := s.startInstance(c, m2)
	mustOpenPortRanges(c, s.State, u2, allEndpoints, []network.PortRange{
		network.MustParsePortRange("3306/tcp"),
	})

	s.assertIngressRules(c, inst1, m1.Id(), firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
	})
	s.assertIngressRules(c, inst2, m2.Id(), firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("3306/tcp"), firewall.AllNetworksIPV4CIDR),
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

	s.assertIngressRules(c, inst1, m1.Id(), nil)
	s.assertIngressRules(c, inst2, m2.Id(), nil)
}

func (s *InstanceModeSuite) TestDeadMachine(c *gc.C) {
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	app := s.AddTestingApplication(c, "wordpress", s.charm)
	err := app.MergeExposeSettings(map[string]state.ExposedEndpoint{
		allEndpoints: {ExposeToCIDRs: []string{firewall.AllNetworksIPV4CIDR}},
	})
	c.Assert(err, jc.ErrorIsNil)

	u, m := s.addUnit(c, app)
	inst := s.startInstance(c, m)
	mustOpenPortRanges(c, s.State, u, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})

	s.assertIngressRules(c, inst, m.Id(), firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
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

	s.assertIngressRules(c, inst, m.Id(), nil)
}

func (s *InstanceModeSuite) TestRemoveMachine(c *gc.C) {
	fw := s.newFirewaller(c)

	app := s.AddTestingApplication(c, "wordpress", s.charm)
	err := app.MergeExposeSettings(map[string]state.ExposedEndpoint{
		allEndpoints: {ExposeToCIDRs: []string{firewall.AllNetworksIPV4CIDR}},
	})
	c.Assert(err, jc.ErrorIsNil)

	u, m := s.addUnit(c, app)
	inst := s.startInstance(c, m)
	mustOpenPortRanges(c, s.State, u, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})

	s.assertIngressRules(c, inst, m.Id(), firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
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
	err := app.MergeExposeSettings(map[string]state.ExposedEndpoint{
		allEndpoints: {ExposeToCIDRs: []string{firewall.AllNetworksIPV4CIDR}},
	})
	c.Assert(err, jc.ErrorIsNil)
	u, m := s.addUnit(c, app)
	inst := s.startInstance(c, m)

	mustOpenPortRanges(c, s.State, u, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})

	// Nothing open without firewaller.
	s.assertIngressRules(c, inst, m.Id(), nil)
	dummy.SetInstanceBroken(inst, "OpenPorts")

	// Starting the firewaller should attempt to open the ports,
	// and fail due to the method being broken.
	fw := s.newFirewaller(c)

	errc := make(chan error, 1)
	go func() { errc <- fw.Wait() }()
	select {
	case err := <-errc:
		c.Assert(err, gc.ErrorMatches,
			`cannot respond to units changes for "machine-1", \"deadbeef-0bad-400d-8000-4b1d0d06f00d\": dummyInstance.OpenPorts is broken`)
	case <-time.After(coretesting.LongWait):
		fw.Kill()
		fw.Wait()
		c.Fatal("timed out waiting for firewaller to stop")
	}
}

func (s *InstanceModeSuite) TestDefaultModelFirewall(c *gc.C) {
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	ctrlCfg, err := s.State.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	apiPort := ctrlCfg.APIPort()

	s.assertModelIngressRules(c, firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("22"), "0.0.0.0/0", "::/0"),
		firewall.NewIngressRule(network.MustParsePortRange(strconv.Itoa(apiPort)), "0.0.0.0/0", "::/0"),
	})
}

func (s *InstanceModeSuite) TestConfigureModelFirewall(c *gc.C) {
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)

	ctrlCfg, err := s.State.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	apiPort := ctrlCfg.APIPort()

	s.assertModelIngressRules(c, firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("22"), "0.0.0.0/0", "::/0"),
		firewall.NewIngressRule(network.MustParsePortRange(strconv.Itoa(apiPort)), "0.0.0.0/0", "::/0"),
	})

	err = model.UpdateModelConfig(map[string]interface{}{
		config.SSHAllowKey: "192.168.0.0/24",
	}, nil)
	c.Assert(err, jc.ErrorIsNil)

	s.assertModelIngressRules(c, firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("22"), "192.168.0.0/24"),
		firewall.NewIngressRule(network.MustParsePortRange(strconv.Itoa(apiPort)), "0.0.0.0/0", "::/0"),
	})
}

func (s *InstanceModeSuite) setupRemoteRelationRequirerRoleConsumingSide(
	c *gc.C, published chan bool, shouldErr func() bool, ingressRequired *bool, clock clock.Clock,
) (worker.Worker, *state.RelationUnit) {
	// Set up the consuming model - create the local app.
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	// Set up the consuming model - create the remote app.
	offeringModelTag := names.NewModelTag(utils.MustNewUUID().String())
	// Create the external controller info.
	ec := state.NewExternalControllers(s.State)
	_, err := ec.Save(crossmodel.ControllerInfo{
		ControllerTag: coretesting.ControllerTag,
		Addrs:         []string{"1.2.3.4:1234"},
		CACert:        coretesting.CACert}, offeringModelTag.Id())
	c.Assert(err, jc.ErrorIsNil)

	mac, err := apitesting.NewMacaroon("apimac")
	c.Assert(err, jc.ErrorIsNil)
	var relToken string
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string,
		arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CrossModelRelations")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "PublishIngressNetworkChanges")
		expected := params.IngressNetworksChanges{
			Changes: []params.IngressNetworksChangeEvent{{
				RelationToken: relToken,
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
		if shouldErr() {
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
	fw := s.newFirewaller(c)

	_, err = s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "mysql", SourceModel: offeringModelTag,
		Endpoints: []charm.Relation{{Name: "database", Interface: "mysql", Role: "provider", Scope: "global"}},
	})
	c.Assert(err, jc.ErrorIsNil)
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

	// We should not have published any ingress events yet - no unit has entered scope.
	select {
	case <-time.After(coretesting.ShortWait):
	case <-published:
		c.Fatal("unexpected ingress change to be published")
	}

	// Add a public address to the consuming unit so the firewaller can use it.
	wpm := s.Factory.MakeMachine(c, &factory.MachineParams{
		Addresses: network.SpaceAddresses{network.NewSpaceAddress("10.0.0.4")},
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
	apiErr := func() bool {
		return false
	}
	fw, ru := s.setupRemoteRelationRequirerRoleConsumingSide(c, published, apiErr, &ingressRequired, s.clock)
	defer statetesting.AssertKillAndWait(c, fw)

	// Add a unit on the consuming app and have it enter the relation scope.
	// This will trigger the firewaller to publish the changes.
	err := ru.EnterScope(map[string]interface{}{})
	c.Assert(err, jc.ErrorIsNil)
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatal("time out waiting for ingress change to be published on enter scope")
	case <-published:
	}

	// Change should be sent when unit leaves scope.
	ingressRequired = false
	err = ru.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatal("time out waiting for ingress change to be published on leave scope")
	case <-published:
	}
}

func (s *InstanceModeSuite) TestRemoteRelationWorkerError(c *gc.C) {
	published := make(chan bool, 1)
	ingressRequired := true

	apiCalled := make(chan struct{}, 1)
	callCount := 0
	apiErr := func() bool {
		select {
		case apiCalled <- struct{}{}:
		case <-time.After(coretesting.ShortWait):
		}
		callCount += 1
		return callCount == 1
	}
	fw, ru := s.setupRemoteRelationRequirerRoleConsumingSide(c, published, apiErr, &ingressRequired, s.clock)
	defer statetesting.AssertKillAndWait(c, fw)

	// Add a unit on the consuming app and have it enter the relation scope.
	// This will trigger the firewaller to try and publish the changes.
	err := ru.EnterScope(map[string]interface{}{})
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-apiCalled:
	case <-time.After(coretesting.LongWait):
		c.Fatal("time out waiting for api to be called")
	}

	// We should not have published any ingress events yet - no changed published.
	select {
	case <-time.After(coretesting.ShortWait):
	case <-published:
		c.Fatal("unexpected ingress change to be published")
	}

	s.clock.Advance(time.Minute)

	// Give the worker time to restart and try again.
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
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string,
		arg, result interface{}) error {
		switch atomic.LoadInt32(&callCount) {
		case 0:
			c.Check(objType, gc.Equals, "CrossModelRelations")
			c.Check(version, gc.Equals, 0)
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "WatchEgressAddressesForRelations")

			rArgs := arg.(params.RemoteEntityArgs).Args
			c.Assert(rArgs, gc.HasLen, 1)
			c.Check(rArgs[0].Token, gc.Equals, relToken)

			macs := rArgs[0].Macaroons
			c.Assert(macs, gc.HasLen, 1)
			c.Assert(macs[0], gc.NotNil)
			apitesting.MacaroonEquals(c, macs[0], mac)

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
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string,
		arg, result interface{}) error {
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
		Addresses: network.SpaceAddresses{network.NewSpaceAddress("10.0.0.4")},
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
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatal("time out waiting for ingress change to be published on enter scope")
	case <-published:
	}

	// Check that the relation status is set to error
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

	mustOpenPortRanges(c, s.State, u, allEndpoints, []network.PortRange{
		network.MustParsePortRange("3306/tcp"),
	})

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
	s.assertIngressRules(c, inst, m.Id(), nil)

	// Save a new ingress network against the relation.
	rin := state.NewRelationIngressNetworks(s.State)
	_, err = rin.Save(rel.Tag().Id(), false, ingress)
	c.Assert(err, jc.ErrorIsNil)

	// Ports opened.
	s.assertIngressRules(c, inst, m.Id(), firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("3306/tcp"), expected...),
	})

	// Change should be sent when ingress networks disappear.
	_, err = rin.Save(rel.Tag().Id(), false, nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertIngressRules(c, inst, m.Id(), nil)

	_, err = rin.Save(rel.Tag().Id(), false, ingress)
	c.Assert(err, jc.ErrorIsNil)
	s.assertIngressRules(c, inst, m.Id(), firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("3306/tcp"), expected...),
	})

	// And again when relation is suspended.
	err = rel.SetSuspended(true, "")
	c.Assert(err, jc.ErrorIsNil)
	s.assertIngressRules(c, inst, m.Id(), nil)

	// And again when relation is resumed.
	err = rel.SetSuspended(false, "")
	c.Assert(err, jc.ErrorIsNil)
	s.assertIngressRules(c, inst, m.Id(), firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("3306/tcp"), expected...),
	})

	// And again when relation is destroyed.
	err = rel.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	s.assertIngressRules(c, inst, m.Id(), nil)
}

func (s *InstanceModeSuite) TestRemoteRelationProviderRoleOffering(c *gc.C) {
	s.assertIngressCidrs(c, []string{"10.0.0.4/16"}, []string{"10.0.0.4/16"})
}

func (s *InstanceModeSuite) TestRemoteRelationIngressFallbackToWhitelist(c *gc.C) {
	m, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	m.UpdateModelConfig(map[string]interface{}{
		config.SAASIngressAllowKey: "192.168.1.0/16",
	}, nil)
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

func (s *InstanceModeSuite) TestExposedApplicationWithExposedEndpoints(c *gc.C) {
	// Create a spaces and add a subnet to it
	sp1, err := s.State.AddSpace("space1", network.Id("sp-1"), nil, false)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSubnet(network.SubnetInfo{
		ID:        "subnet-1",
		CIDR:      "42.42.0.0/16",
		SpaceID:   sp1.Id(),
		SpaceName: sp1.Name(),
		IsPublic:  false,
	})
	c.Assert(err, jc.ErrorIsNil)

	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	app := s.AddTestingApplication(c, "wordpress", s.charm)

	u, m := s.addUnit(c, app)
	inst := s.startInstance(c, m)

	mustOpenPortRanges(c, s.State, u, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})
	mustOpenPortRanges(c, s.State, u, "url", []network.PortRange{
		network.MustParsePortRange("1337/tcp"),
		network.MustParsePortRange("1337/udp"),
	})

	err = app.MergeExposeSettings(map[string]state.ExposedEndpoint{
		allEndpoints: {
			ExposeToCIDRs: []string{"10.0.0.0/24"},
		},
		"url": {
			ExposeToCIDRs:    []string{"192.168.0.0/24", "192.168.1.0/24"},
			ExposeToSpaceIDs: []string{sp1.Id()},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	s.assertIngressRules(c, inst, m.Id(), firewall.IngressRules{
		// We have opened port 80 for ALL endpoints (including "url"),
		// then exposed ALL endpoints to 10.0.0.0/24 and the "url"
		// endpoint to 192.168.{0,1}.0/24 and 42.42.0.0/16 (subnet
		// of space-1).
		//
		// We expect to see port 80 use all three CIDRs as valid input sources
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), "10.0.0.0/24", "192.168.0.0/24", "192.168.1.0/24",
			"42.42.0.0/16"),
		//
		// The 1337/{tcp,udp} ports have only been opened for the "url"
		// endpoint and the "url" endpoint has been exposed to 192.168.{0,1}.0/24
		// and 42.42.0.0/16 (the subnet for space-1).
		//
		// The ports should only be reachable from these CIDRs. Note
		// that the expose for the wildcard ("") endpoint is ignored
		// here as the expose settings for the "url" endpoint must
		// supersede it.
		firewall.NewIngressRule(network.MustParsePortRange("1337/tcp"), "192.168.0.0/24", "192.168.1.0/24",
			"42.42.0.0/16"),
		firewall.NewIngressRule(network.MustParsePortRange("1337/udp"), "192.168.0.0/24", "192.168.1.0/24",
			"42.42.0.0/16"),
	})

	// Change the expose settings and remove the entry for the wildcard endpoint
	err = app.UnsetExposeSettings([]string{allEndpoints})
	c.Assert(err, jc.ErrorIsNil)

	s.assertIngressRules(c, inst, m.Id(), firewall.IngressRules{
		// We unexposed the wildcard endpoint so only the "url" endpoint
		// remains exposed. This endpoint has ports 1337/{tcp,udp}
		// explicitly open as well as port 80 which is opened for ALL
		// endpoints. These three ports should be exposed to the
		// CIDRs used when the "url" endpoint was exposed
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), "192.168.0.0/24", "192.168.1.0/24",
			"42.42.0.0/16"),
		firewall.NewIngressRule(network.MustParsePortRange("1337/tcp"), "192.168.0.0/24", "192.168.1.0/24",
			"42.42.0.0/16"),
		firewall.NewIngressRule(network.MustParsePortRange("1337/udp"), "192.168.0.0/24", "192.168.1.0/24",
			"42.42.0.0/16"),
	})
}

func (s *InstanceModeSuite) TestExposedApplicationWithExposedEndpointsWhenSpaceTopologyChanges(c *gc.C) {
	// Create two spaces and add a subnet to each one
	sp1, err := s.State.AddSpace("space1", network.Id("sp-1"), nil, false)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSubnet(network.SubnetInfo{
		ID:        "subnet-1",
		CIDR:      "192.168.0.0/24",
		SpaceID:   sp1.Id(),
		SpaceName: sp1.Name(),
		IsPublic:  false,
	})
	c.Assert(err, jc.ErrorIsNil)

	sp2, err := s.State.AddSpace("space2", network.Id("sp-2"), nil, true)
	c.Assert(err, jc.ErrorIsNil)
	sub2, err := s.State.AddSubnet(network.SubnetInfo{
		ID:        "subnet-2",
		CIDR:      "192.168.1.0/24",
		SpaceID:   sp2.Id(),
		SpaceName: sp2.Name(),
		IsPublic:  false,
	})
	c.Assert(err, jc.ErrorIsNil)

	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	app := s.AddTestingApplication(c, "wordpress", s.charm)

	u, m := s.addUnit(c, app)
	inst := s.startInstance(c, m)
	// Open port 80 for all endpoints
	mustOpenPortRanges(c, s.State, u, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})

	// Expose app to space-1
	err = app.MergeExposeSettings(map[string]state.ExposedEndpoint{
		allEndpoints: {
			ExposeToSpaceIDs: []string{sp1.Id()},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	s.assertIngressRules(c, inst, m.Id(), firewall.IngressRules{
		// We expect to see port 80 use the subnet-1 CIDR
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), "192.168.0.0/24"),
	})

	// Trigger a space topology change by moving subnet-2 into space 1
	moveOps := s.State.UpdateSubnetSpaceOps(sub2.ID(), sp1.Id())
	c.Assert(s.State.ApplyOperation(modelOp{moveOps}), jc.ErrorIsNil)

	// Check that worker picked up the change and updated the rules
	s.assertIngressRules(c, inst, m.Id(), firewall.IngressRules{
		// We expect to see port 80 use subnet-{1,2} CIDRs
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), "192.168.0.0/24", "192.168.1.0/24"),
	})
}

func (s *InstanceModeSuite) TestExposedApplicationWithExposedEndpointsWhenSpaceDeleted(c *gc.C) {
	// Create two spaces and add a subnet to each one
	sp1, err := s.State.AddSpace("space1", "sp-1", nil, false)
	c.Assert(err, jc.ErrorIsNil)
	sub1, err := s.State.AddSubnet(network.SubnetInfo{
		ID:        "subnet-1",
		CIDR:      "192.168.0.0/24",
		SpaceID:   sp1.Id(),
		SpaceName: sp1.Name(),
		IsPublic:  false,
	})
	c.Assert(err, jc.ErrorIsNil)

	sp2, err := s.State.AddSpace("space2", "sp-2", nil, true)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSubnet(network.SubnetInfo{
		ID:        "subnet-2",
		CIDR:      "192.168.1.0/24",
		SpaceID:   sp2.Id(),
		SpaceName: sp2.Name(),
		IsPublic:  false,
	})
	c.Assert(err, jc.ErrorIsNil)

	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	app := s.AddTestingApplication(c, "wordpress", s.charm)

	u, m := s.addUnit(c, app)
	inst := s.startInstance(c, m)
	// Open port 80 for all endpoints
	mustOpenPortRanges(c, s.State, u, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})

	// Expose app to space-1
	err = app.MergeExposeSettings(map[string]state.ExposedEndpoint{
		allEndpoints: {
			ExposeToSpaceIDs: []string{sp1.Id()},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	s.assertIngressRules(c, inst, m.Id(), firewall.IngressRules{
		// We expect to see port 80 use the subnet-1 CIDR
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), "192.168.0.0/24"),
	})

	// Simulate the deletion of a space, with subnets moving back to alpha.
	moveOps := s.State.UpdateSubnetSpaceOps(sub1.ID(), network.AlphaSpaceId)
	deleteOps, err := sp1.RemoveSpaceOps()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.State.ApplyOperation(modelOp{append(moveOps, deleteOps...)}), jc.ErrorIsNil)

	// We expect to see NO ingress rules as the referenced space does not exist.
	s.assertIngressRules(c, inst, m.Id(), nil)
}

func (s *InstanceModeSuite) TestExposedApplicationWithExposedEndpointsWhenSpaceHasNoSubnets(c *gc.C) {
	// Create a space with a single subnet
	sp1, err := s.State.AddSpace("space1", "sp-1", nil, false)
	c.Assert(err, jc.ErrorIsNil)
	sub1, err := s.State.AddSubnet(network.SubnetInfo{
		ID:        "subnet-1",
		CIDR:      "192.168.0.0/24",
		SpaceID:   sp1.Id(),
		SpaceName: sp1.Name(),
		IsPublic:  false,
	})
	c.Assert(err, jc.ErrorIsNil)

	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	app := s.AddTestingApplication(c, "wordpress", s.charm)

	u, m := s.addUnit(c, app)
	inst := s.startInstance(c, m)
	// Open port 80 for all endpoints and 1337 for the "url" endpoint
	mustOpenPortRanges(c, s.State, u, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})
	mustOpenPortRanges(c, s.State, u, "url", []network.PortRange{
		network.MustParsePortRange("1337/tcp"),
	})

	// Expose app to space-1
	err = app.MergeExposeSettings(map[string]state.ExposedEndpoint{
		allEndpoints: {
			ExposeToSpaceIDs: []string{sp1.Id()},
		},
		"url": {
			ExposeToSpaceIDs: []string{sp1.Id()},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	s.assertIngressRules(c, inst, m.Id(), firewall.IngressRules{
		// We expect to see port 80 and 1337 use the subnet-1 CIDR
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), "192.168.0.0/24"),
		firewall.NewIngressRule(network.MustParsePortRange("1337/tcp"), "192.168.0.0/24"),
	})

	// Move endpoint back to alpha space. This will leave space-1 with no
	// endpoints.
	moveOps := s.State.UpdateSubnetSpaceOps(sub1.ID(), network.AlphaSpaceId)
	c.Assert(s.State.ApplyOperation(modelOp{moveOps}), jc.ErrorIsNil)

	// We expect to see NO ingress rules (and warnings in the logs) as
	// there are no CIDRs to access the exposed application.
	s.assertIngressRules(c, inst, m.Id(), nil)
}

func (s *InstanceModeSuite) TestExposeToIPV6CIDRsOnIPV4OnlyProvider(c *gc.C) {
	supportsIPV6CIDRs := false
	fw := s.newFirewallerWithIPV6CIDRSupport(c, supportsIPV6CIDRs)
	defer statetesting.AssertKillAndWait(c, fw)

	app := s.AddTestingApplication(c, "wordpress", s.charm)

	u, m := s.addUnit(c, app)
	inst := s.startInstance(c, m)

	mustOpenPortRanges(c, s.State, u, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})

	err := app.MergeExposeSettings(map[string]state.ExposedEndpoint{
		allEndpoints: {
			ExposeToCIDRs: []string{"10.0.0.0/24", "2002::1234:abcd:ffff:c0a8:101/64"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	// Since the provider only supports IPV4 CIDRs, the firewall worker
	// will filter the IPV6 CIDRs when opening ports.
	s.assertIngressRules(c, inst, m.Id(), firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), "10.0.0.0/24"),
	})
}

type modelOp struct {
	ops []txn.Op
}

func (m modelOp) Build(_ int) ([]txn.Op, error) { return m.ops, nil }

func (m modelOp) Done(err error) error { return err }

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
	return s.newFirewallerWithIPV6CIDRSupport(c, true)
}

func (s *GlobalModeSuite) newFirewallerWithIPV6CIDRSupport(c *gc.C, supportIPV6CIDRs bool) worker.Worker {
	fwEnv, ok := s.Environ.(environs.Firewaller)
	c.Assert(ok, gc.Equals, true)

	modelFwEnv, ok := s.Environ.(models.ModelFirewaller)
	c.Assert(ok, gc.Equals, true)

	cfg := firewaller.Config{
		ModelUUID:              s.State.ModelUUID(),
		Mode:                   config.FwGlobal,
		EnvironFirewaller:      fwEnv,
		EnvironModelFirewaller: modelFwEnv,
		EnvironInstances:       s.Environ,
		EnvironIPV6CIDRSupport: supportIPV6CIDRs,
		FirewallerAPI:          s.firewaller,
		RemoteRelationsApi:     s.remoteRelations,
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
	err := app1.MergeExposeSettings(map[string]state.ExposedEndpoint{
		allEndpoints: {ExposeToCIDRs: []string{firewall.AllNetworksIPV4CIDR}},
	})
	c.Assert(err, jc.ErrorIsNil)

	u1, m1 := s.addUnit(c, app1)
	s.startInstance(c, m1)
	mustOpenPortRanges(c, s.State, u1, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80-90/tcp"),
		network.MustParsePortRange("8080/tcp"),
	})

	app2 := s.AddTestingApplication(c, "moinmoin", s.charm)
	c.Assert(err, jc.ErrorIsNil)
	err = app2.MergeExposeSettings(map[string]state.ExposedEndpoint{
		allEndpoints: {ExposeToCIDRs: []string{firewall.AllNetworksIPV4CIDR}},
	})
	c.Assert(err, jc.ErrorIsNil)

	u2, m2 := s.addUnit(c, app2)
	s.startInstance(c, m2)
	mustOpenPortRanges(c, s.State, u2, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80-90/tcp"),
	})

	s.assertEnvironPorts(c, firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80-90/tcp"), firewall.AllNetworksIPV4CIDR),
		firewall.NewIngressRule(network.MustParsePortRange("8080/tcp"), firewall.AllNetworksIPV4CIDR),
	})

	// Closing a port opened by a different unit won't touch the environment.
	mustClosePortRanges(c, s.State, u1, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80-90/tcp"),
	})
	s.assertEnvironPorts(c, firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80-90/tcp"), firewall.AllNetworksIPV4CIDR),
		firewall.NewIngressRule(network.MustParsePortRange("8080/tcp"), firewall.AllNetworksIPV4CIDR),
	})

	// Closing a port used just once changes the environment.
	mustClosePortRanges(c, s.State, u1, allEndpoints, []network.PortRange{
		network.MustParsePortRange("8080/tcp"),
	})
	s.assertEnvironPorts(c, firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80-90/tcp"), firewall.AllNetworksIPV4CIDR),
	})

	// Closing the last port also modifies the environment.
	mustClosePortRanges(c, s.State, u2, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80-90/tcp"),
	})
	s.assertEnvironPorts(c, nil)
}

func (s *GlobalModeSuite) TestStartWithUnexposedApplication(c *gc.C) {
	m, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	s.startInstance(c, m)

	app := s.AddTestingApplication(c, "wordpress", s.charm)
	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = u.AssignToMachine(m)
	c.Assert(err, jc.ErrorIsNil)
	mustOpenPortRanges(c, s.State, u, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})

	// Starting the firewaller, no open ports.
	fw := s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	s.assertEnvironPorts(c, nil)

	// Expose application.
	err = app.MergeExposeSettings(map[string]state.ExposedEndpoint{
		allEndpoints: {ExposeToCIDRs: []string{firewall.AllNetworksIPV4CIDR}},
	})
	c.Assert(err, jc.ErrorIsNil)
	s.assertEnvironPorts(c, firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
	})
}

func (s *GlobalModeSuite) TestRestart(c *gc.C) {
	// Start firewaller and open ports.
	fw := s.newFirewaller(c)

	app := s.AddTestingApplication(c, "wordpress", s.charm)
	err := app.MergeExposeSettings(map[string]state.ExposedEndpoint{
		allEndpoints: {ExposeToCIDRs: []string{firewall.AllNetworksIPV4CIDR}},
	})
	c.Assert(err, jc.ErrorIsNil)

	u, m := s.addUnit(c, app)
	s.startInstance(c, m)
	mustOpenPortRanges(c, s.State, u, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80-90/tcp"),
		network.MustParsePortRange("8080/tcp"),
	})
	c.Assert(err, jc.ErrorIsNil)

	s.assertEnvironPorts(c, firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80-90/tcp"), firewall.AllNetworksIPV4CIDR),
		firewall.NewIngressRule(network.MustParsePortRange("8080/tcp"), firewall.AllNetworksIPV4CIDR),
	})

	// Stop firewaller and close one and open a different port.
	err = worker.Stop(fw)
	c.Assert(err, jc.ErrorIsNil)

	mustClosePortRanges(c, s.State, u, allEndpoints, []network.PortRange{
		network.MustParsePortRange("8080/tcp"),
	})
	mustOpenPortRanges(c, s.State, u, allEndpoints, []network.PortRange{
		network.MustParsePortRange("8888/tcp"),
	})

	// Start firewaller and check port.
	fw = s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	s.assertEnvironPorts(c, firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80-90/tcp"), firewall.AllNetworksIPV4CIDR),
		firewall.NewIngressRule(network.MustParsePortRange("8888/tcp"), firewall.AllNetworksIPV4CIDR),
	})
}

func (s *GlobalModeSuite) TestRestartUnexposedApplication(c *gc.C) {
	// Start firewaller and open ports.
	fw := s.newFirewaller(c)

	app := s.AddTestingApplication(c, "wordpress", s.charm)
	err := app.MergeExposeSettings(map[string]state.ExposedEndpoint{
		allEndpoints: {ExposeToCIDRs: []string{firewall.AllNetworksIPV4CIDR}},
	})
	c.Assert(err, jc.ErrorIsNil)

	u, m := s.addUnit(c, app)
	s.startInstance(c, m)
	mustOpenPortRanges(c, s.State, u, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
		network.MustParsePortRange("8080/tcp"),
	})

	s.assertEnvironPorts(c, firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
		firewall.NewIngressRule(network.MustParsePortRange("8080/tcp"), firewall.AllNetworksIPV4CIDR),
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
	err := app1.MergeExposeSettings(map[string]state.ExposedEndpoint{
		allEndpoints: {ExposeToCIDRs: []string{firewall.AllNetworksIPV4CIDR}},
	})
	c.Assert(err, jc.ErrorIsNil)

	u1, m1 := s.addUnit(c, app1)
	s.startInstance(c, m1)
	mustOpenPortRanges(c, s.State, u1, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
		network.MustParsePortRange("8080/tcp"),
	})

	s.assertEnvironPorts(c, firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
		firewall.NewIngressRule(network.MustParsePortRange("8080/tcp"), firewall.AllNetworksIPV4CIDR),
	})

	// Stop firewaller and add another application using the port.
	err = worker.Stop(fw)
	c.Assert(err, jc.ErrorIsNil)

	app2 := s.AddTestingApplication(c, "moinmoin", s.charm)
	err = app2.MergeExposeSettings(map[string]state.ExposedEndpoint{
		allEndpoints: {ExposeToCIDRs: []string{firewall.AllNetworksIPV4CIDR}},
	})
	c.Assert(err, jc.ErrorIsNil)

	u2, m2 := s.addUnit(c, app2)
	s.startInstance(c, m2)
	mustOpenPortRanges(c, s.State, u2, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})

	// Start firewaller and check port.
	fw = s.newFirewaller(c)
	defer statetesting.AssertKillAndWait(c, fw)

	s.assertEnvironPorts(c, firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
		firewall.NewIngressRule(network.MustParsePortRange("8080/tcp"), firewall.AllNetworksIPV4CIDR),
	})

	// Closing a port opened by a different unit won't touch the environment.
	mustClosePortRanges(c, s.State, u1, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})
	s.assertEnvironPorts(c, firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
		firewall.NewIngressRule(network.MustParsePortRange("8080/tcp"), firewall.AllNetworksIPV4CIDR),
	})

	// Closing a port used just once changes the environment.
	mustClosePortRanges(c, s.State, u1, allEndpoints, []network.PortRange{
		network.MustParsePortRange("8080/tcp"),
	})
	s.assertEnvironPorts(c, firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
	})

	// Closing the last port also modifies the environment.
	mustClosePortRanges(c, s.State, u2, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})
	s.assertEnvironPorts(c, nil)
}

func (s *GlobalModeSuite) TestExposeToIPV6CIDRsOnIPV4OnlyProvider(c *gc.C) {
	supportsIPV6CIDRs := false
	fw := s.newFirewallerWithIPV6CIDRSupport(c, supportsIPV6CIDRs)
	defer statetesting.AssertKillAndWait(c, fw)

	app := s.AddTestingApplication(c, "wordpress", s.charm)
	u, _ := s.addUnit(c, app)

	mustOpenPortRanges(c, s.State, u, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})

	err := app.MergeExposeSettings(map[string]state.ExposedEndpoint{
		allEndpoints: {
			ExposeToCIDRs: []string{"10.0.0.0/24", "2002::1234:abcd:ffff:c0a8:101/64"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	// Since the provider only supports IPV4 CIDRs, the firewall worker
	// will filter the IPV6 CIDRs when opening ports.
	s.assertEnvironPorts(c, firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), "10.0.0.0/24"),
	})
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

func mustOpenPortRanges(c *gc.C, st *state.State, u *state.Unit, endpointName string, portRanges []network.PortRange) {
	unitPortRanges, err := u.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)

	for _, pr := range portRanges {
		unitPortRanges.Open(endpointName, pr)
	}

	c.Assert(st.ApplyOperation(unitPortRanges.Changes()), jc.ErrorIsNil)
}

func mustClosePortRanges(c *gc.C, st *state.State, u *state.Unit, endpointName string, portRanges []network.PortRange) {
	unitPortRanges, err := u.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)

	for _, pr := range portRanges {
		unitPortRanges.Close(endpointName, pr)
	}

	c.Assert(st.ApplyOperation(unitPortRanges.Changes()), jc.ErrorIsNil)
}
