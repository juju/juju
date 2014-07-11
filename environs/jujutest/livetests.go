// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujutest

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"strings"
	"time"

	"github.com/juju/charm"
	charmtesting "github.com/juju/charm/testing"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/environs/sync"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/juju/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/api"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

// LiveTests contains tests that are designed to run against a live server
// (e.g. Amazon EC2).  The Environ is opened once only for all the tests
// in the suite, stored in Env, and Destroyed after the suite has completed.
type LiveTests struct {
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
}

func (t *LiveTests) SetUpSuite(c *gc.C) {
	t.ConfigStore = configstore.NewMem()
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
	if t.Env != nil {
		t.Destroy(c)
	}
}

// PrepareOnce ensures that the environment is
// available and prepared. It sets t.Env appropriately.
func (t *LiveTests) PrepareOnce(c *gc.C) {
	if t.prepared {
		return
	}
	cfg, err := config.New(config.NoDefaults, t.TestConfig)
	c.Assert(err, gc.IsNil)
	e, err := environs.Prepare(cfg, coretesting.Context(c), t.ConfigStore)
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
		_, err := sync.Upload(t.Env.Storage(), nil, coretesting.FakeDefaultSeries)
		c.Assert(err, gc.IsNil)
	}
	t.UploadFakeTools(c, t.Env.Storage())
	err := bootstrap.EnsureNotBootstrapped(t.Env)
	c.Assert(err, gc.IsNil)
	err = bootstrap.Bootstrap(coretesting.Context(c), t.Env, environs.BootstrapParams{Constraints: cons})
	c.Assert(err, gc.IsNil)
	t.bootstrapped = true
}

func (t *LiveTests) Destroy(c *gc.C) {
	if t.Env == nil {
		return
	}
	err := environs.Destroy(t.Env, t.ConfigStore)
	c.Assert(err, gc.IsNil)
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
	c.Assert(err, gc.IsNil)
}

// TestStartStop is similar to Tests.TestStartStop except
// that it does not assume a pristine environment.
func (t *LiveTests) TestStartStop(c *gc.C) {
	t.PrepareOnce(c)
	t.UploadFakeTools(c, t.Env.Storage())

	inst, _ := testing.AssertStartInstance(c, t.Env, "0")
	c.Assert(inst, gc.NotNil)
	id0 := inst.Id()

	insts, err := t.Env.Instances([]instance.Id{id0, id0})
	c.Assert(err, gc.IsNil)
	c.Assert(insts, gc.HasLen, 2)
	c.Assert(insts[0].Id(), gc.Equals, id0)
	c.Assert(insts[1].Id(), gc.Equals, id0)

	// Asserting on the return of AllInstances makes the test fragile,
	// as even comparing the before and after start values can be thrown
	// off if other instances have been created or destroyed in the same
	// time frame. Instead, just check the instance we created exists.
	insts, err = t.Env.AllInstances()
	c.Assert(err, gc.IsNil)
	found := false
	for _, inst := range insts {
		if inst.Id() == id0 {
			c.Assert(found, gc.Equals, false, gc.Commentf("%v", insts))
			found = true
		}
	}
	c.Assert(found, gc.Equals, true, gc.Commentf("expected %v in %v", inst, insts))

	addresses, err := jujutesting.WaitInstanceAddresses(t.Env, inst.Id())
	c.Assert(err, gc.IsNil)
	c.Assert(addresses, gc.Not(gc.HasLen), 0)

	insts, err = t.Env.Instances([]instance.Id{id0, ""})
	c.Assert(err, gc.Equals, environs.ErrPartialInstances)
	c.Assert(insts, gc.HasLen, 2)
	c.Check(insts[0].Id(), gc.Equals, id0)
	c.Check(insts[1], gc.IsNil)

	err = t.Env.StopInstances(inst.Id())
	c.Assert(err, gc.IsNil)

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
	t.PrepareOnce(c)
	t.UploadFakeTools(c, t.Env.Storage())

	inst1, _ := testing.AssertStartInstance(c, t.Env, "1")
	c.Assert(inst1, gc.NotNil)
	defer t.Env.StopInstances(inst1.Id())
	ports, err := inst1.Ports("1")
	c.Assert(err, gc.IsNil)
	c.Assert(ports, gc.HasLen, 0)

	inst2, _ := testing.AssertStartInstance(c, t.Env, "2")
	c.Assert(inst2, gc.NotNil)
	ports, err = inst2.Ports("2")
	c.Assert(err, gc.IsNil)
	c.Assert(ports, gc.HasLen, 0)
	defer t.Env.StopInstances(inst2.Id())

	// Open some ports and check they're there.
	err = inst1.OpenPorts("1", []network.Port{{"udp", 67}, {"tcp", 45}})
	c.Assert(err, gc.IsNil)
	ports, err = inst1.Ports("1")
	c.Assert(err, gc.IsNil)
	c.Assert(ports, gc.DeepEquals, []network.Port{{"tcp", 45}, {"udp", 67}})
	ports, err = inst2.Ports("2")
	c.Assert(err, gc.IsNil)
	c.Assert(ports, gc.HasLen, 0)

	err = inst2.OpenPorts("2", []network.Port{{"tcp", 89}, {"tcp", 45}})
	c.Assert(err, gc.IsNil)

	// Check there's no crosstalk to another machine
	ports, err = inst2.Ports("2")
	c.Assert(err, gc.IsNil)
	c.Assert(ports, gc.DeepEquals, []network.Port{{"tcp", 45}, {"tcp", 89}})
	ports, err = inst1.Ports("1")
	c.Assert(err, gc.IsNil)
	c.Assert(ports, gc.DeepEquals, []network.Port{{"tcp", 45}, {"udp", 67}})

	// Check that opening the same port again is ok.
	oldPorts, err := inst2.Ports("2")
	c.Assert(err, gc.IsNil)
	err = inst2.OpenPorts("2", []network.Port{{"tcp", 45}})
	c.Assert(err, gc.IsNil)
	ports, err = inst2.Ports("2")
	c.Assert(err, gc.IsNil)
	c.Assert(ports, gc.DeepEquals, oldPorts)

	// Check that opening the same port again and another port is ok.
	err = inst2.OpenPorts("2", []network.Port{{"tcp", 45}, {"tcp", 99}})
	c.Assert(err, gc.IsNil)
	ports, err = inst2.Ports("2")
	c.Assert(err, gc.IsNil)
	c.Assert(ports, gc.DeepEquals, []network.Port{{"tcp", 45}, {"tcp", 89}, {"tcp", 99}})

	err = inst2.ClosePorts("2", []network.Port{{"tcp", 45}, {"tcp", 99}})
	c.Assert(err, gc.IsNil)

	// Check that we can close ports and that there's no crosstalk.
	ports, err = inst2.Ports("2")
	c.Assert(err, gc.IsNil)
	c.Assert(ports, gc.DeepEquals, []network.Port{{"tcp", 89}})
	ports, err = inst1.Ports("1")
	c.Assert(err, gc.IsNil)
	c.Assert(ports, gc.DeepEquals, []network.Port{{"tcp", 45}, {"udp", 67}})

	// Check that we can close multiple ports.
	err = inst1.ClosePorts("1", []network.Port{{"tcp", 45}, {"udp", 67}})
	c.Assert(err, gc.IsNil)
	ports, err = inst1.Ports("1")
	c.Assert(ports, gc.HasLen, 0)

	// Check that we can close ports that aren't there.
	err = inst2.ClosePorts("2", []network.Port{{"tcp", 111}, {"udp", 222}})
	c.Assert(err, gc.IsNil)
	ports, err = inst2.Ports("2")
	c.Assert(ports, gc.DeepEquals, []network.Port{{"tcp", 89}})

	// Check errors when acting on environment.
	err = t.Env.OpenPorts([]network.Port{{"tcp", 80}})
	c.Assert(err, gc.ErrorMatches, `invalid firewall mode "instance" for opening ports on environment`)

	err = t.Env.ClosePorts([]network.Port{{"tcp", 80}})
	c.Assert(err, gc.ErrorMatches, `invalid firewall mode "instance" for closing ports on environment`)

	_, err = t.Env.Ports()
	c.Assert(err, gc.ErrorMatches, `invalid firewall mode "instance" for retrieving ports from environment`)
}

func (t *LiveTests) TestGlobalPorts(c *gc.C) {
	t.PrepareOnce(c)
	t.UploadFakeTools(c, t.Env.Storage())

	// Change configuration.
	oldConfig := t.Env.Config()
	defer func() {
		err := t.Env.SetConfig(oldConfig)
		c.Assert(err, gc.IsNil)
	}()

	attrs := t.Env.Config().AllAttrs()
	attrs["firewall-mode"] = "global"
	newConfig, err := t.Env.Config().Apply(attrs)
	c.Assert(err, gc.IsNil)
	err = t.Env.SetConfig(newConfig)
	c.Assert(err, gc.IsNil)

	// Create instances and check open ports on both instances.
	inst1, _ := testing.AssertStartInstance(c, t.Env, "1")
	defer t.Env.StopInstances(inst1.Id())
	ports, err := t.Env.Ports()
	c.Assert(err, gc.IsNil)
	c.Assert(ports, gc.HasLen, 0)

	inst2, _ := testing.AssertStartInstance(c, t.Env, "2")
	ports, err = t.Env.Ports()
	c.Assert(err, gc.IsNil)
	c.Assert(ports, gc.HasLen, 0)
	defer t.Env.StopInstances(inst2.Id())

	err = t.Env.OpenPorts([]network.Port{{"udp", 67}, {"tcp", 45}, {"tcp", 89}, {"tcp", 99}})
	c.Assert(err, gc.IsNil)

	ports, err = t.Env.Ports()
	c.Assert(err, gc.IsNil)
	c.Assert(ports, gc.DeepEquals, []network.Port{{"tcp", 45}, {"tcp", 89}, {"tcp", 99}, {"udp", 67}})

	// Check closing some ports.
	err = t.Env.ClosePorts([]network.Port{{"tcp", 99}, {"udp", 67}})
	c.Assert(err, gc.IsNil)

	ports, err = t.Env.Ports()
	c.Assert(err, gc.IsNil)
	c.Assert(ports, gc.DeepEquals, []network.Port{{"tcp", 45}, {"tcp", 89}})

	// Check that we can close ports that aren't there.
	err = t.Env.ClosePorts([]network.Port{{"tcp", 111}, {"udp", 222}})
	c.Assert(err, gc.IsNil)

	ports, err = t.Env.Ports()
	c.Assert(err, gc.IsNil)
	c.Assert(ports, gc.DeepEquals, []network.Port{{"tcp", 45}, {"tcp", 89}})

	// Check errors when acting on instances.
	err = inst1.OpenPorts("1", []network.Port{{"tcp", 80}})
	c.Assert(err, gc.ErrorMatches, `invalid firewall mode "global" for opening ports on instance`)

	err = inst1.ClosePorts("1", []network.Port{{"tcp", 80}})
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
	env.Destroy() // Again, should work fine and do nothing.

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
	st := t.Env.(testing.GetStater).GetStateInAPIServer()

	c.Logf("opening API connection")
	apiConn, err := juju.NewAPIConn(t.Env, api.DefaultDialOpts())
	c.Assert(err, gc.IsNil)
	defer apiConn.Close()

	// Check that the agent version has made it through the
	// bootstrap process (it's optional in the config.Config)
	cfg, err := st.EnvironConfig()
	c.Assert(err, gc.IsNil)
	agentVersion, ok := cfg.AgentVersion()
	c.Check(ok, gc.Equals, true)
	c.Check(agentVersion, gc.Equals, version.Current.Number)

	// Check that the constraints have been set in the environment.
	cons, err := st.EnvironConstraints()
	c.Assert(err, gc.IsNil)
	c.Assert(cons.String(), gc.Equals, "mem=2048M")

	// Wait for machine agent to come up on the bootstrap
	// machine and find the deployed series from that.
	m0, err := st.Machine("0")
	c.Assert(err, gc.IsNil)

	instId0, err := m0.InstanceId()
	c.Assert(err, gc.IsNil)

	// Check that the API connection is working.
	status, err := apiConn.State.Client().Status(nil)
	c.Assert(err, gc.IsNil)
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
	url := charmtesting.Charms.ClonedURL(repoDir, mtools0.Version.Series, "dummy")
	sch, err := testing.PutCharm(st, url, &charm.LocalRepository{Path: repoDir}, false)
	c.Assert(err, gc.IsNil)
	svc, err := st.AddService("dummy", "user-admin", sch, nil)
	c.Assert(err, gc.IsNil)
	units, err := juju.AddUnits(st, svc, 1, "")
	c.Assert(err, gc.IsNil)
	unit := units[0]

	// Wait for the unit's machine and associated agent to come up
	// and announce itself.
	mid1, err := unit.AssignedMachineId()
	c.Assert(err, gc.IsNil)
	m1, err := st.Machine(mid1)
	c.Assert(err, gc.IsNil)
	mw1 := newMachineToolWaiter(m1)
	defer mw1.Stop()
	waitAgentTools(c, mw1, mtools0.Version)

	err = m1.Refresh()
	c.Assert(err, gc.IsNil)
	instId1, err := m1.InstanceId()
	c.Assert(err, gc.IsNil)
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
	c.Assert(err, gc.IsNil)

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
		c.Assert(err, gc.IsNil)
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
		c.Assert(err, gc.IsNil)
	}
	c.Logf("waiting for instance to be removed")
	t.assertStopInstance(c, t.Env, instId1)
}

func (t *LiveTests) TestBootstrapVerifyStorage(c *gc.C) {
	// Bootstrap automatically verifies that storage is writable.
	t.BootstrapOnce(c)
	environ := t.Env
	stor := environ.Storage()
	reader, err := storage.Get(stor, "bootstrap-verify")
	c.Assert(err, gc.IsNil)
	defer reader.Close()
	contents, err := ioutil.ReadAll(reader)
	c.Assert(err, gc.IsNil)
	c.Check(string(contents), gc.Equals,
		"juju-core storage writing verified: ok\n")
}

func restoreBootstrapVerificationFile(c *gc.C, stor storage.Storage) {
	content := "juju-core storage writing verified: ok\n"
	contentReader := strings.NewReader(content)
	err := stor.Put("bootstrap-verify", contentReader,
		int64(len(content)))
	c.Assert(err, gc.IsNil)
}

func (t *LiveTests) TestCheckEnvironmentOnConnect(c *gc.C) {
	// When new connection is established to a bootstraped environment,
	// it is checked that we are running against a juju-core environment.
	if !t.CanOpenState {
		c.Skip("CanOpenState is false; cannot open state connection")
	}
	t.BootstrapOnce(c)

	apiConn, err := juju.NewAPIConn(t.Env, api.DefaultDialOpts())
	c.Assert(err, gc.IsNil)
	apiConn.Close()
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
	c.Assert(err, gc.IsNil)
	c.Check(tools.Version, gc.Equals, expect)
	return tools
}

// checkUpgrade sets the environment agent version and checks that
// all the provided watchers upgrade to the requested version.
func (t *LiveTests) checkUpgrade(c *gc.C, st *state.State, newVersion version.Binary, waiters ...*toolsWaiter) {
	c.Logf("putting testing version of juju tools")
	upgradeTools, err := sync.Upload(t.Env.Storage(), &newVersion.Number, newVersion.Series)
	c.Assert(err, gc.IsNil)
	// sync.Upload always returns tools for the series on which the tests are running.
	// We are only interested in checking the version.Number below so need to fake the
	// upgraded tools series to match that of newVersion.
	upgradeTools.Version.Series = newVersion.Series

	// Check that the put version really is the version we expect.
	c.Assert(upgradeTools.Version, gc.Equals, newVersion)
	err = statetesting.SetAgentVersion(st, newVersion.Number)
	c.Assert(err, gc.IsNil)

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
		c.Assert(err, gc.IsNil)
		instId, err := m.InstanceId()
		if err != nil {
			c.Assert(err, jc.Satisfies, state.IsNotProvisionedError)
			continue
		}
		_, err = t.Env.Instances([]instance.Id{instId})
		c.Assert(err, gc.IsNil)
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
		c.Assert(err, gc.IsNil)
		gotId, err = m.InstanceId()
		if err != nil {
			c.Assert(err, jc.Satisfies, state.IsNotProvisionedError)
			if inst == nil {
				return
			}
			continue
		}
		break
	}
	c.Assert(err, gc.IsNil)
	c.Assert(gotId, gc.Equals, wantId)
}

// TODO check that binary data works ok?
var contents = []byte("hello\n")
var contents2 = []byte("goodbye\n\n")

func (t *LiveTests) TestFile(c *gc.C) {
	t.PrepareOnce(c)
	name := fmt.Sprint("testfile", time.Now().UnixNano())
	stor := t.Env.Storage()

	checkFileDoesNotExist(c, stor, name, t.Attempt)
	checkPutFile(c, stor, name, contents)
	checkFileHasContents(c, stor, name, contents, t.Attempt)
	checkPutFile(c, stor, name, contents2) // check that we can overwrite the file
	checkFileHasContents(c, stor, name, contents2, t.Attempt)

	// check that the listed contents include the
	// expected name.
	found := false
	var names []string
attempt:
	for a := t.Attempt.Start(); a.Next(); {
		var err error
		names, err = stor.List("")
		c.Assert(err, gc.IsNil)
		for _, lname := range names {
			if lname == name {
				found = true
				break attempt
			}
		}
	}
	if !found {
		c.Errorf("file name %q not found in file list %q", name, names)
	}
	err := stor.Remove(name)
	c.Check(err, gc.IsNil)
	checkFileDoesNotExist(c, stor, name, t.Attempt)
	// removing a file that does not exist should not be an error.
	err = stor.Remove(name)
	c.Check(err, gc.IsNil)

	// RemoveAll deletes all files from storage.
	checkPutFile(c, stor, "file-1.txt", contents)
	checkPutFile(c, stor, "file-2.txt", contents)
	err = stor.RemoveAll()
	c.Check(err, gc.IsNil)
	checkFileDoesNotExist(c, stor, "file-1.txt", t.Attempt)
	checkFileDoesNotExist(c, stor, "file-2.txt", t.Attempt)
}

// Check that we get a consistent error when asking for an instance without
// a valid machine config.
func (t *LiveTests) TestStartInstanceWithEmptyNonceFails(c *gc.C) {
	machineId := "4"
	stateInfo := testing.FakeStateInfo(machineId)
	apiInfo := testing.FakeAPIInfo(machineId)
	machineConfig := environs.NewMachineConfig(machineId, "", nil, stateInfo, apiInfo)

	t.PrepareOnce(c)
	possibleTools := envtesting.AssertUploadFakeToolsVersions(c, t.Env.Storage(), version.MustParseBinary("5.4.5-precise-amd64"))
	inst, _, _, err := t.Env.StartInstance(environs.StartInstanceParams{
		Tools:         possibleTools,
		MachineConfig: machineConfig,
	})
	if inst != nil {
		err := t.Env.StopInstances(inst.Id())
		c.Check(err, gc.IsNil)
	}
	c.Assert(inst, gc.IsNil)
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
	dummyenv, err := environs.Prepare(dummyCfg, coretesting.Context(c), configstore.NewMem())
	c.Assert(err, gc.IsNil)
	defer dummyenv.Destroy()

	t.Destroy(c)

	attrs := t.TestConfig.Merge(coretesting.Attrs{"default-series": other.Series})
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)
	env, err := environs.Prepare(cfg, coretesting.Context(c), t.ConfigStore)
	c.Assert(err, gc.IsNil)
	defer environs.Destroy(env, t.ConfigStore)

	currentName := envtools.StorageName(current)
	otherName := envtools.StorageName(other)
	envStorage := env.Storage()
	dummyStorage := dummyenv.Storage()

	defer envStorage.Remove(otherName)

	_, err = sync.Upload(dummyStorage, &current.Number)
	c.Assert(err, gc.IsNil)

	// This will only work while cross-compiling across releases is safe,
	// which depends on external elements. Tends to be safe for the last
	// few releases, but we may have to refactor some day.
	err = storageCopy(dummyStorage, currentName, envStorage, otherName)
	c.Assert(err, gc.IsNil)

	err = bootstrap.Bootstrap(coretesting.Context(c), env, environs.BootstrapParams{})
	c.Assert(err, gc.IsNil)

	st := t.Env.(testing.GetStater).GetStateInAPIServer()
	// Wait for machine agent to come up on the bootstrap
	// machine and ensure it deployed the proper series.
	m0, err := st.Machine("0")
	c.Assert(err, gc.IsNil)
	mw0 := newMachineToolWaiter(m0)
	defer mw0.Stop()

	waitAgentTools(c, mw0, other)
}

func storageCopy(source storage.Storage, sourcePath string, target storage.Storage, targetPath string) error {
	rc, err := storage.Get(source, sourcePath)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	_, err = io.Copy(&buf, rc)
	rc.Close()
	if err != nil {
		return err
	}
	return target.Put(targetPath, &buf, int64(buf.Len()))
}
