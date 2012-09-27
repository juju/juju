package jujutest

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/trivial"
	"launchpad.net/juju-core/version"
	"time"
)

// TestStartStop is similar to Tests.TestStartStop except
// that it does not assume a pristine environment.
func (t *LiveTests) TestStartStop(c *C) {
	insts, err := t.Env.Instances(nil)
	c.Assert(err, IsNil)
	c.Check(insts, HasLen, 0)

	inst, err := t.Env.StartInstance(0, InvalidStateInfo, nil)
	c.Assert(err, IsNil)
	c.Assert(inst, NotNil)
	id0 := inst.Id()

	insts, err = t.Env.Instances([]string{id0, id0})
	c.Assert(err, IsNil)
	c.Assert(insts, HasLen, 2)
	c.Assert(insts[0].Id(), Equals, id0)
	c.Assert(insts[1].Id(), Equals, id0)

	insts, err = t.Env.AllInstances()
	c.Assert(err, IsNil)
	// differs from the check above because AllInstances returns
	// a set (without duplicates) containing only one instance.
	c.Assert(insts, HasLen, 1)
	c.Assert(insts[0].Id(), Equals, id0)

	dns, err := inst.WaitDNSName()
	c.Assert(err, IsNil)
	c.Assert(dns, Not(Equals), "")

	insts, err = t.Env.Instances([]string{id0, ""})
	c.Assert(err, Equals, environs.ErrPartialInstances)
	c.Assert(insts, HasLen, 2)
	c.Check(insts[0].Id(), Equals, id0)
	c.Check(insts[1], IsNil)

	err = t.Env.StopInstances([]environs.Instance{inst})
	c.Assert(err, IsNil)

	// Stopping may not be noticed at first due to eventual
	// consistency. Repeat a few times to ensure we get the error.
	for i := 0; i < 20; i++ {
		insts, err = t.Env.Instances([]string{id0})
		if err != nil {
			break
		}
		time.Sleep(0.25e9)
	}
	c.Assert(err, Equals, environs.ErrNoInstances)
	c.Assert(insts, HasLen, 0)
}

func (t *LiveTests) TestPorts(c *C) {
	inst1, err := t.Env.StartInstance(1, InvalidStateInfo, nil)
	c.Assert(err, IsNil)
	c.Assert(inst1, NotNil)
	defer t.Env.StopInstances([]environs.Instance{inst1})

	ports, err := inst1.Ports(1)
	c.Assert(err, IsNil)
	c.Assert(ports, HasLen, 0)

	inst2, err := t.Env.StartInstance(2, InvalidStateInfo, nil)
	c.Assert(err, IsNil)
	c.Assert(inst2, NotNil)
	ports, err = inst2.Ports(2)
	c.Assert(err, IsNil)
	c.Assert(ports, HasLen, 0)
	defer t.Env.StopInstances([]environs.Instance{inst2})

	// Open some ports and check they're there.
	err = inst1.OpenPorts(1, []state.Port{{"udp", 67}, {"tcp", 45}})
	c.Assert(err, IsNil)
	ports, err = inst1.Ports(1)
	c.Assert(err, IsNil)
	c.Assert(ports, DeepEquals, []state.Port{{"tcp", 45}, {"udp", 67}})
	ports, err = inst2.Ports(2)
	c.Assert(err, IsNil)
	c.Assert(ports, HasLen, 0)

	// Check there's no crosstalk to another machine
	err = inst2.OpenPorts(2, []state.Port{{"tcp", 89}, {"tcp", 45}})
	c.Assert(err, IsNil)
	ports, err = inst2.Ports(2)
	c.Assert(err, IsNil)
	c.Assert(ports, DeepEquals, []state.Port{{"tcp", 45}, {"tcp", 89}})
	ports, err = inst1.Ports(1)
	c.Assert(err, IsNil)
	c.Assert(ports, DeepEquals, []state.Port{{"tcp", 45}, {"udp", 67}})

	// Check that opening the same port again is ok.
	err = inst2.OpenPorts(2, []state.Port{{"tcp", 45}})
	c.Assert(err, IsNil)
	ports, err = inst2.Ports(2)
	c.Assert(err, IsNil)
	c.Assert(ports, DeepEquals, []state.Port{{"tcp", 45}, {"tcp", 89}})

	// Check that opening the same port again and another port is ok.
	err = inst2.OpenPorts(2, []state.Port{{"tcp", 45}, {"tcp", 99}})
	c.Assert(err, IsNil)
	ports, err = inst2.Ports(2)
	c.Assert(err, IsNil)
	c.Assert(ports, DeepEquals, []state.Port{{"tcp", 45}, {"tcp", 89}, {"tcp", 99}})

	// Check that we can close ports and that there's no crosstalk.
	err = inst2.ClosePorts(2, []state.Port{{"tcp", 45}, {"tcp", 99}})
	c.Assert(err, IsNil)
	ports, err = inst2.Ports(2)
	c.Assert(err, IsNil)
	c.Assert(ports, DeepEquals, []state.Port{{"tcp", 89}})
	ports, err = inst1.Ports(1)
	c.Assert(err, IsNil)
	c.Assert(ports, DeepEquals, []state.Port{{"tcp", 45}, {"udp", 67}})

	// Check that we can close multiple ports.
	err = inst1.ClosePorts(1, []state.Port{{"tcp", 45}, {"udp", 67}})
	c.Assert(err, IsNil)
	ports, err = inst1.Ports(1)
	c.Assert(ports, HasLen, 0)

	// Check that we can close ports that aren't there.
	err = inst2.ClosePorts(2, []state.Port{{"tcp", 111}, {"udp", 222}})
	c.Assert(err, IsNil)
	ports, err = inst2.Ports(2)
	c.Assert(ports, DeepEquals, []state.Port{{"tcp", 89}})
}

func (t *LiveTests) TestBootstrapMultiple(c *C) {
	t.BootstrapOnce(c)

	// Wait for a while to let eventual consistency catch up, hopefully.
	time.Sleep(t.ConsistencyDelay)
	err := t.Env.Bootstrap(false)
	c.Assert(err, ErrorMatches, "environment is already bootstrapped")

	c.Logf("destroy env")
	t.Destroy(c)

	// check that we can bootstrap after destroy
	t.BootstrapOnce(c)
}

func (t *LiveTests) TestBootstrapAndDeploy(c *C) {
	if !t.CanOpenState || !t.HasProvisioner {
		c.Skip(fmt.Sprintf("skipping provisioner test, CanOpenState: %v, HasProvisioner: %v", t.CanOpenState, t.HasProvisioner))
	}
	t.BootstrapOnce(c)

	// TODO(niemeyer): Stop growing this kitchen sink test and split it into proper parts.

	c.Logf("opening connection")
	conn, err := juju.NewConn(t.Env)
	c.Assert(err, IsNil)
	defer conn.Close()

	// Check that the agent version has made it through the
	// bootstrap process (it's optional in the config.Config)
	cfg, err := conn.State.EnvironConfig()
	c.Assert(err, IsNil)
	c.Check(cfg.AgentVersion(), Equals, version.Current.Number)

	// Wait for machine agent to come up on the bootstrap
	// machine and find the deployed series from that.
	// Wait for machine agent to come up on the bootstrap
	// machine and find the deployed series from that.
	m0, err := conn.State.Machine(0)
	c.Assert(err, IsNil)
	w0 := newMachineToolWaiter(m0)
	defer w0.Stop()

	tools0 := waitAgentTools(c, w0, version.Current)

	// Create a new service and deploy a unit of it.
	c.Logf("deploying service")
	repoDir := c.MkDir()
	url := testing.Charms.ClonedURL(repoDir, tools0.Series, "dummy")
	sch, err := conn.PutCharm(url, &charm.LocalRepository{repoDir}, false)

	c.Assert(err, IsNil)
	svc, err := conn.AddService("", sch)
	c.Assert(err, IsNil)
	units, err := conn.AddUnits(svc, 1)
	c.Assert(err, IsNil)
	unit := units[0]

	// Wait for the unit's machine and associated agent to come up
	// and announce itself.
	mid1, err := unit.AssignedMachineId()
	c.Assert(err, IsNil)
	m1, err := conn.State.Machine(mid1)
	c.Assert(err, IsNil)
	w1 := newMachineToolWaiter(m1)
	defer w1.Stop()
	tools1 := waitAgentTools(c, w1, tools0.Binary)
	instId1, err := m1.InstanceId()
	c.Assert(err, IsNil)

	// Check that we can upgrade the environment.
	newVersion := tools1.Binary
	newVersion.Patch++
	t.checkUpgrade(c, conn, newVersion, w0, w1)

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
	err = unit.EnsureDead()
	c.Assert(err, IsNil)
	err = svc.RemoveUnit(unit)
	c.Assert(err, IsNil)
	err = m1.EnsureDead()
	c.Assert(err, IsNil)
	err = conn.State.RemoveMachine(mid1)
	c.Assert(err, IsNil)

	c.Logf("waiting for instance to be removed")
	t.assertStopInstance(c, conn.Environ, instId1)
}

type tooler interface {
	Life() state.Life
	AgentTools() (*state.Tools, error)
	Refresh() error
	String() string
}

type watcher interface {
	Stop() error
	Err() error
}

type toolsWaiter struct {
	lastTools *state.Tools
	// changes is a chan of struct{} so that it can
	// be used with different kinds of entity watcher.
	changes   chan struct{}
	watcher
	tooler tooler
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

// NextTools returns the next changed tools, waiting
// until the tools are actually set.
func (w *toolsWaiter) NextTools(c *C) (*state.Tools, error) {
	for _ = range w.changes {
		err := w.tooler.Refresh()
		if err != nil {
			return nil, fmt.Errorf("cannot refresh: %v", err)
		}
		if w.tooler.Life() == state.Dead {
			return nil, fmt.Errorf("object is dead")
		}
		tools, err := w.tooler.AgentTools()
		if state.IsNotFound(err) {
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
	return nil, fmt.Errorf("watcher closed prematurely: %v", w.Err())
}

// waitAgentTools waits for the given agent
// to start and returns the tools that it is running.
func waitAgentTools(c *C, w *toolsWaiter, expect version.Binary) *state.Tools {
	c.Logf("waiting for %v to signal agent version", w.tooler.String())
	tools, err := w.NextTools(c)
	c.Assert(err, IsNil)
	c.Check(tools.Binary, Equals, expect)
	return tools
}

// checkUpgrade sets the environment agent version and checks that
// all the provided watchers upgrade to the requested version.
func (t *LiveTests) checkUpgrade(c *C, conn *juju.Conn, newVersion version.Binary, waiters ...*toolsWaiter) {
	c.Logf("putting testing version of juju tools")
	upgradeTools, err := environs.PutTools(t.Env.Storage(), &newVersion)
	c.Assert(err, IsNil)

	// Check that the put version really is the version we expect.
	c.Assert(upgradeTools.Binary, Equals, newVersion)
	err = setAgentVersion(conn.State, newVersion.Number)
	c.Assert(err, IsNil)

	for i, w := range waiters {
		c.Logf("waiting for upgrade of %d: %v", i, w.tooler.String())

		waitAgentTools(c, w, newVersion)
		c.Logf("upgrade %d successful", i)
	}
}

// setAgentVersion sets the current agent version in the state's
// environment configuration.
func setAgentVersion(st *state.State, vers version.Number) error {
	cfg, err := st.EnvironConfig()
	if err != nil {
		return err
	}
	attrs := cfg.AllAttrs()
	attrs["agent-version"] = vers.String()
	cfg, err = config.New(attrs)
	if err != nil {
		panic(fmt.Errorf("config refused agent-version: %v", err))
	}
	return st.SetEnvironConfig(cfg)
}

var waitAgent = trivial.AttemptStrategy{
	Total: 30 * time.Second,
	Delay: 1 * time.Second,
}

func (t *LiveTests) assertStartInstance(c *C, m *state.Machine) {
	// Wait for machine to get an instance id.
	for a := waitAgent.Start(); a.Next(); {
		err := m.Refresh()
		c.Assert(err, IsNil)
		instId, err := m.InstanceId()
		if state.IsNotFound(err) {
			continue
		}
		c.Assert(err, IsNil)
		_, err = t.Env.Instances([]string{instId})
		c.Assert(err, IsNil)
		return
	}
	c.Fatalf("provisioner failed to start machine after %v", waitAgent.Total)
}

func (t *LiveTests) assertStopInstance(c *C, env environs.Environ, instId string) {
	var err error
	for a := waitAgent.Start(); a.Next(); {
		_, err = t.Env.Instances([]string{instId})
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
func assertInstanceId(c *C, m *state.Machine, inst environs.Instance) {
	var wantId, gotId string
	var err error
	if inst != nil {
		wantId = inst.Id()
	}
	for a := waitAgent.Start(); a.Next(); {
		err := m.Refresh()
		c.Assert(err, IsNil)
		gotId, err = m.InstanceId()
		if state.IsNotFound(err) {
			if inst == nil {
				return
			}
			continue
		}
		c.Assert(err, IsNil)
		break
	}
	c.Assert(err, IsNil)
	c.Assert(gotId, Equals, wantId)
}

// TODO check that binary data works ok?
var contents = []byte("hello\n")
var contents2 = []byte("goodbye\n\n")

func (t *LiveTests) TestFile(c *C) {
	name := fmt.Sprint("testfile", time.Now().UnixNano())
	storage := t.Env.Storage()

	checkFileDoesNotExist(c, storage, name)
	checkPutFile(c, storage, name, contents)
	checkFileHasContents(c, storage, name, contents)
	checkPutFile(c, storage, name, contents2) // check that we can overwrite the file
	checkFileHasContents(c, storage, name, contents2)

	// check that the listed contents include the
	// expected name.
	names, err := storage.List("")
	c.Assert(err, IsNil)
	found := false
	for _, lname := range names {
		if lname == name {
			found = true
			break
		}
	}
	if !found {
		c.Errorf("file name %q not found in file list %q", name, names)
	}

	err = storage.Remove(name)
	c.Check(err, IsNil)
	checkFileDoesNotExist(c, storage, name)
	// removing a file that does not exist should not be an error.
	err = storage.Remove(name)
	c.Check(err, IsNil)
}

// Check that we can't start an instance running tools
// that correspond with no available platform.
func (t *LiveTests) TestStartInstanceOnUnknownPlatform(c *C) {
	vers := version.Current
	// Note that we want this test to function correctly in the
	// dummy environment, so to avoid enumerating all possible
	// platforms in the dummy provider, it treats only series and/or
	// architectures with the "unknown" prefix as invalid.
	vers.Series = "unknownseries"
	vers.Arch = "unknownarch"
	name := environs.ToolsStoragePath(vers)
	storage := t.Env.Storage()
	checkPutFile(c, storage, name, []byte("fake tools on invalid series"))
	defer storage.Remove(name)

	url, err := storage.URL(name)
	c.Assert(err, IsNil)
	tools := &state.Tools{
		Binary: vers,
		URL:    url,
	}

	inst, err := t.Env.StartInstance(4, InvalidStateInfo, tools)
	if inst != nil {
		err := t.Env.StopInstances([]environs.Instance{inst})
		c.Check(err, IsNil)
	}
	c.Assert(inst, IsNil)
	c.Assert(err, ErrorMatches, "cannot find image.*")
}
