// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujutest

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5/charmrepo"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/environs/sync"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	envtoolstesting "github.com/juju/juju/environs/tools/testing"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

// LiveTests contains tests that are designed to run against a live server
// (e.g. Amazon EC2).  The Environ is opened once only for all the tests
// in the suite, stored in Env, and Destroyed after the suite has completed.
type LiveTests struct {
	gitjujutesting.CleanupSuite

	envtesting.ToolsFixture

	// TestConfig contains the configuration attributes for opening an environment.
	TestConfig coretesting.Attrs

	// Attempt holds a strategy for waiting until the environment
	// becomes logically consistent.
	Attempt utils.AttemptStrategy

	// CanOpenState should be true if the testing environment allows
	// the state to be opened after bootstrapping.
	CanOpenState bool

	// HasProvisioner should be true if the environment has
	// a provisioning agent.
	HasProvisioner bool

	// Env holds the currently opened environment.
	// This is set by PrepareOnce and BootstrapOnce.
	Env environs.Environ

	// ConfigStore holds the configuration storage
	// used when preparing the environment.
	// This is initialized by SetUpSuite.
	ConfigStore configstore.Storage

	prepared     bool
	bootstrapped bool
	toolsStorage storage.Storage
}

func (t *LiveTests) SetUpSuite(c *gc.C) {
	t.CleanupSuite.SetUpSuite(c)
	t.ConfigStore = configstore.NewMem()
}

func (t *LiveTests) SetUpTest(c *gc.C) {
	t.CleanupSuite.SetUpTest(c)
	t.PatchValue(&version.Current.Number, coretesting.FakeVersionNumber)
	storageDir := c.MkDir()
	t.DefaultBaseURL = "file://" + storageDir + "/tools"
	t.ToolsFixture.SetUpTest(c)
	stor, err := filestorage.NewFileStorageWriter(storageDir)
	c.Assert(err, jc.ErrorIsNil)
	t.UploadFakeTools(c, stor, "released", "released")
	t.toolsStorage = stor
	t.CleanupSuite.PatchValue(&envtools.BundleTools, envtoolstesting.GetMockBundleTools(c))
}

func publicAttrs(e environs.Environ) map[string]interface{} {
	cfg := e.Config()
	secrets, err := e.Provider().SecretAttrs(cfg)
	if err != nil {
		panic(err)
	}
	attrs := cfg.AllAttrs()
	for attr := range secrets {
		delete(attrs, attr)
	}
	return attrs
}

func (t *LiveTests) TearDownSuite(c *gc.C) {
	t.Destroy(c)
	t.CleanupSuite.TearDownSuite(c)
}

func (t *LiveTests) TearDownTest(c *gc.C) {
	t.ToolsFixture.TearDownTest(c)
	t.CleanupSuite.TearDownTest(c)
}

// PrepareOnce ensures that the environment is
// available and prepared. It sets t.Env appropriately.
func (t *LiveTests) PrepareOnce(c *gc.C) {
	if t.prepared {
		return
	}
	cfg, err := config.New(config.NoDefaults, t.TestConfig)
	c.Assert(err, jc.ErrorIsNil)
	e, err := environs.Prepare(cfg, envtesting.BootstrapContext(c), t.ConfigStore)
	c.Assert(err, gc.IsNil, gc.Commentf("preparing environ %#v", t.TestConfig))
	c.Assert(e, gc.NotNil)
	t.Env = e
	t.prepared = true
}

func (t *LiveTests) BootstrapOnce(c *gc.C) {
	if t.bootstrapped {
		return
	}
	t.PrepareOnce(c)
	// We only build and upload tools if there will be a state agent that
	// we could connect to (actual live tests, rather than local-only)
	cons := constraints.MustParse("mem=2G")
	if t.CanOpenState {
		_, err := sync.Upload(t.toolsStorage, "released", nil, coretesting.FakeDefaultSeries)
		c.Assert(err, jc.ErrorIsNil)
	}
	err := bootstrap.EnsureNotBootstrapped(t.Env)
	c.Assert(err, jc.ErrorIsNil)
	err = bootstrap.Bootstrap(envtesting.BootstrapContext(c), t.Env, bootstrap.BootstrapParams{Constraints: cons})
	c.Assert(err, jc.ErrorIsNil)
	t.bootstrapped = true
}

func (t *LiveTests) Destroy(c *gc.C) {
	if t.Env == nil {
		return
	}
	err := environs.Destroy(t.Env, t.ConfigStore)
	c.Assert(err, jc.ErrorIsNil)
	t.bootstrapped = false
	t.prepared = false
	t.Env = nil
}

func (t *LiveTests) TestPrechecker(c *gc.C) {
	// Providers may implement Prechecker. If they do, then they should
	// return nil for empty constraints (excluding the null provider).
	prechecker, ok := t.Env.(state.Prechecker)
	if !ok {
		return
	}
	const placement = ""
	err := prechecker.PrecheckInstance("precise", constraints.Value{}, placement)
	c.Assert(err, jc.ErrorIsNil)
}

// TestStartStop is similar to Tests.TestStartStop except
// that it does not assume a pristine environment.
func (t *LiveTests) TestStartStop(c *gc.C) {
	t.BootstrapOnce(c)

	inst, _ := jujutesting.AssertStartInstance(c, t.Env, "0")
	c.Assert(inst, gc.NotNil)
	id0 := inst.Id()

	insts, err := t.Env.Instances([]instance.Id{id0, id0})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(insts, gc.HasLen, 2)
	c.Assert(insts[0].Id(), gc.Equals, id0)
	c.Assert(insts[1].Id(), gc.Equals, id0)

	// Asserting on the return of AllInstances makes the test fragile,
	// as even comparing the before and after start values can be thrown
	// off if other instances have been created or destroyed in the same
	// time frame. Instead, just check the instance we created exists.
	insts, err = t.Env.AllInstances()
	c.Assert(err, jc.ErrorIsNil)
	found := false
	for _, inst := range insts {
		if inst.Id() == id0 {
			c.Assert(found, gc.Equals, false, gc.Commentf("%v", insts))
			found = true
		}
	}
	c.Assert(found, gc.Equals, true, gc.Commentf("expected %v in %v", inst, insts))

	addresses, err := jujutesting.WaitInstanceAddresses(t.Env, inst.Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addresses, gc.Not(gc.HasLen), 0)

	insts, err = t.Env.Instances([]instance.Id{id0, ""})
	c.Assert(err, gc.Equals, environs.ErrPartialInstances)
	c.Assert(insts, gc.HasLen, 2)
	c.Check(insts[0].Id(), gc.Equals, id0)
	c.Check(insts[1], gc.IsNil)

	err = t.Env.StopInstances(inst.Id())
	c.Assert(err, jc.ErrorIsNil)

	// The machine may not be marked as shutting down
	// immediately. Repeat a few times to ensure we get the error.
	for a := t.Attempt.Start(); a.Next(); {
		insts, err = t.Env.Instances([]instance.Id{id0})
		if err != nil {
			break
		}
	}
	c.Assert(err, gc.Equals, environs.ErrNoInstances)
	c.Assert(insts, gc.HasLen, 0)
}

func (t *LiveTests) TestPorts(c *gc.C) {
	t.BootstrapOnce(c)

	inst1, _ := jujutesting.AssertStartInstance(c, t.Env, "1")
	c.Assert(inst1, gc.NotNil)
	defer t.Env.StopInstances(inst1.Id())
	ports, err := inst1.Ports("1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ports, gc.HasLen, 0)

	inst2, _ := jujutesting.AssertStartInstance(c, t.Env, "2")
	c.Assert(inst2, gc.NotNil)
	ports, err = inst2.Ports("2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ports, gc.HasLen, 0)
	defer t.Env.StopInstances(inst2.Id())

	// Open some ports and check they're there.
	err = inst1.OpenPorts("1", []network.PortRange{{67, 67, "udp"}, {45, 45, "tcp"}, {80, 100, "tcp"}})
	c.Assert(err, jc.ErrorIsNil)
	ports, err = inst1.Ports("1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ports, gc.DeepEquals, []network.PortRange{{45, 45, "tcp"}, {80, 100, "tcp"}, {67, 67, "udp"}})
	ports, err = inst2.Ports("2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ports, gc.HasLen, 0)

	err = inst2.OpenPorts("2", []network.PortRange{{89, 89, "tcp"}, {45, 45, "tcp"}, {20, 30, "tcp"}})
	c.Assert(err, jc.ErrorIsNil)

	// Check there's no crosstalk to another machine
	ports, err = inst2.Ports("2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ports, gc.DeepEquals, []network.PortRange{{20, 30, "tcp"}, {45, 45, "tcp"}, {89, 89, "tcp"}})
	ports, err = inst1.Ports("1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ports, gc.DeepEquals, []network.PortRange{{45, 45, "tcp"}, {80, 100, "tcp"}, {67, 67, "udp"}})

	// Check that opening the same port again is ok.
	oldPorts, err := inst2.Ports("2")
	c.Assert(err, jc.ErrorIsNil)
	err = inst2.OpenPorts("2", []network.PortRange{{45, 45, "tcp"}})
	c.Assert(err, jc.ErrorIsNil)
	err = inst2.OpenPorts("2", []network.PortRange{{20, 30, "tcp"}})
	c.Assert(err, jc.ErrorIsNil)
	ports, err = inst2.Ports("2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ports, gc.DeepEquals, oldPorts)

	// Check that opening the same port again and another port is ok.
	err = inst2.OpenPorts("2", []network.PortRange{{45, 45, "tcp"}, {99, 99, "tcp"}})
	c.Assert(err, jc.ErrorIsNil)
	ports, err = inst2.Ports("2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ports, gc.DeepEquals, []network.PortRange{{20, 30, "tcp"}, {45, 45, "tcp"}, {89, 89, "tcp"}, {99, 99, "tcp"}})

	err = inst2.ClosePorts("2", []network.PortRange{{45, 45, "tcp"}, {99, 99, "tcp"}, {20, 30, "tcp"}})
	c.Assert(err, jc.ErrorIsNil)

	// Check that we can close ports and that there's no crosstalk.
	ports, err = inst2.Ports("2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ports, gc.DeepEquals, []network.PortRange{{89, 89, "tcp"}})
	ports, err = inst1.Ports("1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ports, gc.DeepEquals, []network.PortRange{{45, 45, "tcp"}, {80, 100, "tcp"}, {67, 67, "udp"}})

	// Check that we can close multiple ports.
	err = inst1.ClosePorts("1", []network.PortRange{{45, 45, "tcp"}, {67, 67, "udp"}, {80, 100, "tcp"}})
	c.Assert(err, jc.ErrorIsNil)
	ports, err = inst1.Ports("1")
	c.Assert(ports, gc.HasLen, 0)

	// Check that we can close ports that aren't there.
	err = inst2.ClosePorts("2", []network.PortRange{{111, 111, "tcp"}, {222, 222, "udp"}, {600, 700, "tcp"}})
	c.Assert(err, jc.ErrorIsNil)
	ports, err = inst2.Ports("2")
	c.Assert(ports, gc.DeepEquals, []network.PortRange{{89, 89, "tcp"}})

	// Check errors when acting on environment.
	err = t.Env.OpenPorts([]network.PortRange{{80, 80, "tcp"}})
	c.Assert(err, gc.ErrorMatches, `invalid firewall mode "instance" for opening ports on environment`)

	err = t.Env.ClosePorts([]network.PortRange{{80, 80, "tcp"}})
	c.Assert(err, gc.ErrorMatches, `invalid firewall mode "instance" for closing ports on environment`)

	_, err = t.Env.Ports()
	c.Assert(err, gc.ErrorMatches, `invalid firewall mode "instance" for retrieving ports from environment`)
}

func (t *LiveTests) TestGlobalPorts(c *gc.C) {
	t.BootstrapOnce(c)

	// Change configuration.
	oldConfig := t.Env.Config()
	defer func() {
		err := t.Env.SetConfig(oldConfig)
		c.Assert(err, jc.ErrorIsNil)
	}()

	attrs := t.Env.Config().AllAttrs()
	attrs["firewall-mode"] = config.FwGlobal
	newConfig, err := t.Env.Config().Apply(attrs)
	c.Assert(err, jc.ErrorIsNil)
	err = t.Env.SetConfig(newConfig)
	c.Assert(err, jc.ErrorIsNil)

	// Create instances and check open ports on both instances.
	inst1, _ := jujutesting.AssertStartInstance(c, t.Env, "1")
	defer t.Env.StopInstances(inst1.Id())
	ports, err := t.Env.Ports()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ports, gc.HasLen, 0)

	inst2, _ := jujutesting.AssertStartInstance(c, t.Env, "2")
	ports, err = t.Env.Ports()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ports, gc.HasLen, 0)
	defer t.Env.StopInstances(inst2.Id())

	err = t.Env.OpenPorts([]network.PortRange{{67, 67, "udp"}, {45, 45, "tcp"}, {89, 89, "tcp"}, {99, 99, "tcp"}, {100, 110, "tcp"}})
	c.Assert(err, jc.ErrorIsNil)

	ports, err = t.Env.Ports()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ports, gc.DeepEquals, []network.PortRange{{45, 45, "tcp"}, {89, 89, "tcp"}, {99, 99, "tcp"}, {100, 110, "tcp"}, {67, 67, "udp"}})

	// Check closing some ports.
	err = t.Env.ClosePorts([]network.PortRange{{99, 99, "tcp"}, {67, 67, "udp"}})
	c.Assert(err, jc.ErrorIsNil)

	ports, err = t.Env.Ports()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ports, gc.DeepEquals, []network.PortRange{{45, 45, "tcp"}, {89, 89, "tcp"}, {100, 110, "tcp"}})

	// Check that we can close ports that aren't there.
	err = t.Env.ClosePorts([]network.PortRange{{111, 111, "tcp"}, {222, 222, "udp"}, {2000, 2500, "tcp"}})
	c.Assert(err, jc.ErrorIsNil)

	ports, err = t.Env.Ports()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ports, gc.DeepEquals, []network.PortRange{{45, 45, "tcp"}, {89, 89, "tcp"}, {100, 110, "tcp"}})

	// Check errors when acting on instances.
	err = inst1.OpenPorts("1", []network.PortRange{{80, 80, "tcp"}})
	c.Assert(err, gc.ErrorMatches, `invalid firewall mode "global" for opening ports on instance`)

	err = inst1.ClosePorts("1", []network.PortRange{{80, 80, "tcp"}})
	c.Assert(err, gc.ErrorMatches, `invalid firewall mode "global" for closing ports on instance`)

	_, err = inst1.Ports("1")
	c.Assert(err, gc.ErrorMatches, `invalid firewall mode "global" for retrieving ports from instance`)
}

func (t *LiveTests) TestBootstrapMultiple(c *gc.C) {
	// bootstrap.Bootstrap no longer raises errors if the environment is
	// already up, this has been moved into the bootstrap command.
	t.BootstrapOnce(c)

	err := bootstrap.EnsureNotBootstrapped(t.Env)
	c.Assert(err, gc.ErrorMatches, "environment is already bootstrapped")

	c.Logf("destroy env")
	env := t.Env
	t.Destroy(c)
	err = env.Destroy() // Again, should work fine and do nothing.
	c.Assert(err, jc.ErrorIsNil)

	// check that we can bootstrap after destroy
	t.BootstrapOnce(c)
}

func (t *LiveTests) TestBootstrapAndDeploy(c *gc.C) {
	if !t.CanOpenState || !t.HasProvisioner {
		c.Skip(fmt.Sprintf("skipping provisioner test, CanOpenState: %v, HasProvisioner: %v", t.CanOpenState, t.HasProvisioner))
	}
	t.BootstrapOnce(c)

	// TODO(niemeyer): Stop growing this kitchen sink test and split it into proper parts.

	c.Logf("opening state")
	st := t.Env.(jujutesting.GetStater).GetStateInAPIServer()

	env, err := st.Environment()
	c.Assert(err, jc.ErrorIsNil)
	owner := env.Owner()

	c.Logf("opening API connection")
	apiState, err := juju.NewAPIState(owner, t.Env, api.DefaultDialOpts())
	c.Assert(err, jc.ErrorIsNil)
	defer apiState.Close()

	// Check that the agent version has made it through the
	// bootstrap process (it's optional in the config.Config)
	cfg, err := st.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	agentVersion, ok := cfg.AgentVersion()
	c.Check(ok, jc.IsTrue)
	c.Check(agentVersion, gc.Equals, version.Current.Number)

	// Check that the constraints have been set in the environment.
	cons, err := st.EnvironConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons.String(), gc.Equals, "mem=2048M")

	// Wait for machine agent to come up on the bootstrap
	// machine and find the deployed series from that.
	m0, err := st.Machine("0")
	c.Assert(err, jc.ErrorIsNil)

	instId0, err := m0.InstanceId()
	c.Assert(err, jc.ErrorIsNil)

	// Check that the API connection is working.
	status, err := apiState.Client().Status(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Machines["0"].InstanceId, gc.Equals, string(instId0))

	mw0 := newMachineToolWaiter(m0)
	defer mw0.Stop()

	// If the series has not been specified, we expect the most recent Ubuntu LTS release to be used.
	expectedVersion := version.Current
	expectedVersion.Series = config.LatestLtsSeries()

	mtools0 := waitAgentTools(c, mw0, expectedVersion)

	// Create a new service and deploy a unit of it.
	c.Logf("deploying service")
	repoDir := c.MkDir()
	url := testcharms.Repo.ClonedURL(repoDir, mtools0.Version.Series, "dummy")
	sch, err := jujutesting.PutCharm(st, url, &charmrepo.LocalRepository{Path: repoDir}, false)
	c.Assert(err, jc.ErrorIsNil)
	svc, err := st.AddService("dummy", owner.String(), sch, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	units, err := juju.AddUnits(st, svc, 1, "")
	c.Assert(err, jc.ErrorIsNil)
	unit := units[0]

	// Wait for the unit's machine and associated agent to come up
	// and announce itself.
	mid1, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	m1, err := st.Machine(mid1)
	c.Assert(err, jc.ErrorIsNil)
	mw1 := newMachineToolWaiter(m1)
	defer mw1.Stop()
	waitAgentTools(c, mw1, mtools0.Version)

	err = m1.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	instId1, err := m1.InstanceId()
	c.Assert(err, jc.ErrorIsNil)
	uw := newUnitToolWaiter(unit)
	defer uw.Stop()
	utools := waitAgentTools(c, uw, expectedVersion)

	// Check that we can upgrade the environment.
	newVersion := utools.Version
	newVersion.Patch++
	t.checkUpgrade(c, st, newVersion, mw0, mw1, uw)

	// BUG(niemeyer): Logic below is very much wrong. Must be:
	//
	// 1. EnsureDying on the unit and EnsureDying on the machine
	// 2. Unit dies by itself
	// 3. Machine removes dead unit
	// 4. Machine dies by itself
	// 5. Provisioner removes dead machine
	//

	// Now remove the unit and its assigned machine and
	// check that the PA removes it.
	c.Logf("removing unit")
	err = unit.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	// Wait until unit is dead
	uwatch := unit.Watch()
	defer uwatch.Stop()
	for unit.Life() != state.Dead {
		c.Logf("waiting for unit change")
		<-uwatch.Changes()
		err := unit.Refresh()
		c.Logf("refreshed; err %v", err)
		if errors.IsNotFound(err) {
			c.Logf("unit has been removed")
			break
		}
		c.Assert(err, jc.ErrorIsNil)
	}
	for {
		c.Logf("destroying machine")
		err := m1.Destroy()
		if err == nil {
			break
		}
		c.Assert(err, gc.FitsTypeOf, &state.HasAssignedUnitsError{})
		time.Sleep(5 * time.Second)
		err = m1.Refresh()
		if errors.IsNotFound(err) {
			break
		}
		c.Assert(err, jc.ErrorIsNil)
	}
	c.Logf("waiting for instance to be removed")
	t.assertStopInstance(c, t.Env, instId1)
}

type tooler interface {
	Life() state.Life
	AgentTools() (*coretools.Tools, error)
	Refresh() error
	String() string
}

type watcher interface {
	Stop() error
	Err() error
}

type toolsWaiter struct {
	lastTools *coretools.Tools
	// changes is a chan of struct{} so that it can
	// be used with different kinds of entity watcher.
	changes chan struct{}
	watcher watcher
	tooler  tooler
}

func newMachineToolWaiter(m *state.Machine) *toolsWaiter {
	w := m.Watch()
	waiter := &toolsWaiter{
		changes: make(chan struct{}, 1),
		watcher: w,
		tooler:  m,
	}
	go func() {
		for _ = range w.Changes() {
			waiter.changes <- struct{}{}
		}
		close(waiter.changes)
	}()
	return waiter
}

func newUnitToolWaiter(u *state.Unit) *toolsWaiter {
	w := u.Watch()
	waiter := &toolsWaiter{
		changes: make(chan struct{}, 1),
		watcher: w,
		tooler:  u,
	}
	go func() {
		for _ = range w.Changes() {
			waiter.changes <- struct{}{}
		}
		close(waiter.changes)
	}()
	return waiter
}

func (w *toolsWaiter) Stop() error {
	return w.watcher.Stop()
}

// NextTools returns the next changed tools, waiting
// until the tools are actually set.
func (w *toolsWaiter) NextTools(c *gc.C) (*coretools.Tools, error) {
	for _ = range w.changes {
		err := w.tooler.Refresh()
		if err != nil {
			return nil, fmt.Errorf("cannot refresh: %v", err)
		}
		if w.tooler.Life() == state.Dead {
			return nil, fmt.Errorf("object is dead")
		}
		tools, err := w.tooler.AgentTools()
		if errors.IsNotFound(err) {
			c.Logf("tools not yet set")
			continue
		}
		if err != nil {
			return nil, err
		}
		changed := w.lastTools == nil || *tools != *w.lastTools
		w.lastTools = tools
		if changed {
			return tools, nil
		}
		c.Logf("found same tools")
	}
	return nil, fmt.Errorf("watcher closed prematurely: %v", w.watcher.Err())
}

// waitAgentTools waits for the given agent
// to start and returns the tools that it is running.
func waitAgentTools(c *gc.C, w *toolsWaiter, expect version.Binary) *coretools.Tools {
	c.Logf("waiting for %v to signal agent version", w.tooler.String())
	tools, err := w.NextTools(c)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(tools.Version, gc.Equals, expect)
	return tools
}

// checkUpgrade sets the environment agent version and checks that
// all the provided watchers upgrade to the requested version.
func (t *LiveTests) checkUpgrade(c *gc.C, st *state.State, newVersion version.Binary, waiters ...*toolsWaiter) {
	c.Logf("putting testing version of juju tools")
	upgradeTools, err := sync.Upload(t.toolsStorage, "released", &newVersion.Number, newVersion.Series)
	c.Assert(err, jc.ErrorIsNil)
	// sync.Upload always returns tools for the series on which the tests are running.
	// We are only interested in checking the version.Number below so need to fake the
	// upgraded tools series to match that of newVersion.
	upgradeTools.Version.Series = newVersion.Series

	// Check that the put version really is the version we expect.
	c.Assert(upgradeTools.Version, gc.Equals, newVersion)
	err = statetesting.SetAgentVersion(st, newVersion.Number)
	c.Assert(err, jc.ErrorIsNil)

	for i, w := range waiters {
		c.Logf("waiting for upgrade of %d: %v", i, w.tooler.String())

		waitAgentTools(c, w, newVersion)
		c.Logf("upgrade %d successful", i)
	}
}

var waitAgent = utils.AttemptStrategy{
	Total: 30 * time.Second,
	Delay: 1 * time.Second,
}

func (t *LiveTests) assertStartInstance(c *gc.C, m *state.Machine) {
	// Wait for machine to get an instance id.
	for a := waitAgent.Start(); a.Next(); {
		err := m.Refresh()
		c.Assert(err, jc.ErrorIsNil)
		instId, err := m.InstanceId()
		if err != nil {
			c.Assert(err, jc.Satisfies, errors.IsNotProvisioned)
			continue
		}
		_, err = t.Env.Instances([]instance.Id{instId})
		c.Assert(err, jc.ErrorIsNil)
		return
	}
	c.Fatalf("provisioner failed to start machine after %v", waitAgent.Total)
}

func (t *LiveTests) assertStopInstance(c *gc.C, env environs.Environ, instId instance.Id) {
	var err error
	for a := waitAgent.Start(); a.Next(); {
		_, err = t.Env.Instances([]instance.Id{instId})
		if err == nil {
			continue
		}
		if err == environs.ErrNoInstances {
			return
		}
		c.Logf("error from Instances: %v", err)
	}
	c.Fatalf("provisioner failed to stop machine after %v", waitAgent.Total)
}

// assertInstanceId asserts that the machine has an instance id
// that matches that of the given instance. If the instance is nil,
// It asserts that the instance id is unset.
func assertInstanceId(c *gc.C, m *state.Machine, inst instance.Instance) {
	var wantId, gotId instance.Id
	var err error
	if inst != nil {
		wantId = inst.Id()
	}
	for a := waitAgent.Start(); a.Next(); {
		err := m.Refresh()
		c.Assert(err, jc.ErrorIsNil)
		gotId, err = m.InstanceId()
		if err != nil {
			c.Assert(err, jc.Satisfies, errors.IsNotProvisioned)
			if inst == nil {
				return
			}
			continue
		}
		break
	}
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotId, gc.Equals, wantId)
}

// Check that we get a consistent error when asking for an instance without
// a valid machine config.
func (t *LiveTests) TestStartInstanceWithEmptyNonceFails(c *gc.C) {
	machineId := "4"
	stateInfo := jujutesting.FakeStateInfo(machineId)
	apiInfo := jujutesting.FakeAPIInfo(machineId)
	instanceConfig, err := instancecfg.NewInstanceConfig(machineId, "", "released", "quantal", true, nil, stateInfo, apiInfo)
	c.Assert(err, jc.ErrorIsNil)

	t.PrepareOnce(c)
	possibleTools := envtesting.AssertUploadFakeToolsVersions(c, t.toolsStorage, "released", "released", version.MustParseBinary("5.4.5-trusty-amd64"))
	result, err := t.Env.StartInstance(environs.StartInstanceParams{
		Tools:          possibleTools,
		InstanceConfig: instanceConfig,
	})
	if result != nil && result.Instance != nil {
		err := t.Env.StopInstances(result.Instance.Id())
		c.Check(err, jc.ErrorIsNil)
	}
	c.Assert(result, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, ".*missing machine nonce")
}

func (t *LiveTests) TestBootstrapWithDefaultSeries(c *gc.C) {
	if !t.HasProvisioner {
		c.Skip("HasProvisioner is false; cannot test deployment")
	}

	current := version.Current
	other := current
	other.Series = "quantal"
	if current == other {
		other.Series = "precise"
	}

	dummyCfg, err := config.New(config.NoDefaults, dummy.SampleConfig().Merge(coretesting.Attrs{
		"state-server": false,
		"name":         "dummy storage",
	}))
	dummyenv, err := environs.Prepare(dummyCfg, envtesting.BootstrapContext(c), configstore.NewMem())
	c.Assert(err, jc.ErrorIsNil)
	defer dummyenv.Destroy()

	t.Destroy(c)

	attrs := t.TestConfig.Merge(coretesting.Attrs{"default-series": other.Series})
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	env, err := environs.Prepare(cfg, envtesting.BootstrapContext(c), t.ConfigStore)
	c.Assert(err, jc.ErrorIsNil)
	defer environs.Destroy(env, t.ConfigStore)

	err = bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{})
	c.Assert(err, jc.ErrorIsNil)

	st := t.Env.(jujutesting.GetStater).GetStateInAPIServer()
	// Wait for machine agent to come up on the bootstrap
	// machine and ensure it deployed the proper series.
	m0, err := st.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	mw0 := newMachineToolWaiter(m0)
	defer mw0.Stop()

	waitAgentTools(c, mw0, other)
}
