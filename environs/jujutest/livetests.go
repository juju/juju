package jujutest

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
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

func (t *LiveTests) TestBootstrap(c *C) {
	t.BootstrapOnce(c)

	// Wait for a while to let eventual consistency catch up, hopefully.
	time.Sleep(t.ConsistencyDelay)
	err := t.Env.Bootstrap(false)
	c.Assert(err, ErrorMatches, "environment is already bootstrapped")

	info, err := t.Env.StateInfo()
	c.Assert(err, IsNil)
	c.Assert(info, NotNil)
	c.Assert(info.Addrs, Not(HasLen), 0)

	if t.CanOpenState {
		st, err := state.Open(info)
		c.Assert(err, IsNil)
		err = st.Close()
		c.Assert(err, IsNil)
	}

	c.Logf("destroy env")
	t.Destroy(c)

	// check that we can bootstrap after destroy
	t.BootstrapOnce(c)
}

func (t *LiveTests) TestBootstrapProvisioner(c *C) {
	if !t.CanOpenState || !t.HasProvisioner {
		c.Skip(fmt.Sprintf("skipping provisioner test, CanOpenState: %v, HasProvisioner: %v", t.CanOpenState, t.HasProvisioner))
	}
	t.BootstrapOnce(c)

	// TODO(dfc) constructing a juju.Conn by hand is a code smell.
	conn, err := juju.NewConnFromAttrs(t.Env.Config().AllAttrs())
	c.Assert(err, IsNil)

	st, err := conn.State()
	c.Assert(err, IsNil)

	// place a new machine into the state
	m, err := st.AddMachine()
	c.Assert(err, IsNil)

	t.assertStartInstance(c, m)

	// now remove it
	c.Assert(st.RemoveMachine(m.Id()), IsNil)

	// watch the PA remove it
	t.assertStopInstance(c, m)
	assertInstanceId(c, m, nil)

	err = st.Close()
	c.Assert(err, IsNil)
}

var waitAgent = environs.AttemptStrategy{
	Total: 30 * time.Second,
	Delay: 1 * time.Second,
}

func (t *LiveTests) assertStartInstance(c *C, m *state.Machine) {
	// Wait for machine to get an instance id.
	for a := waitAgent.Start(); a.Next(); {
		instId, err := m.InstanceId()
		if _, ok := err.(*state.NotFoundError); ok {
			continue
		}
		c.Assert(err, IsNil)
		_, err = t.Env.Instances([]string{instId})
		c.Assert(err, IsNil)
		return
	}
	c.Fatalf("provisioner failed to start machine after %v", waitAgent.Total)
}

func (t *LiveTests) assertStopInstance(c *C, m *state.Machine) {
	// Wait for machine id to be cleared.
	for a := waitAgent.Start(); a.Next(); {
		if instId, err := m.InstanceId(); instId == "" {
			c.Assert(err, FitsTypeOf, &state.NotFoundError{})
			return
		}
	}
	c.Fatalf("provisioner failed to stop machine after %v", waitAgent.Total)
}

// assertInstanceId asserts that the machine has an instance id
// that matches that of the given instance. If the instance is nil,
// It asserts that the instance id is unset.
func assertInstanceId(c *C, m *state.Machine, inst environs.Instance) {
	// TODO(dfc) add machine.WatchConfig() to avoid having to poll.
	var instId, id string
	var err error
	if inst != nil {
		instId = inst.Id()
	}
	for a := waitAgent.Start(); a.Next(); {
		id, err = m.InstanceId()
		_, notset := err.(*state.NotFoundError)
		if notset {
			if inst == nil {
				return
			}
			continue
		}
		c.Assert(err, IsNil)
		break
	}
	c.Assert(err, IsNil)
	c.Assert(id, Equals, instId)
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
