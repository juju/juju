package jujutest

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/state"
	"time"
)

// TestStartStop is similar to Tests.TestStartStop except
// that it does not assume a pristine environment.
func (t *LiveTests) TestStartStop(c *C) {
	insts, err := t.Env.Instances(nil)
	c.Assert(err, IsNil)
	c.Check(insts, HasLen, 0)

	inst, err := t.Env.StartInstance(0, InvalidStateInfo)
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

func (t *LiveTests) TestBootstrap(c *C) {
	t.BootstrapOnce(c)

	// Wait for a while to let eventual consistency catch up, hopefully.
	time.Sleep(t.ConsistencyDelay)
	err := t.Env.Bootstrap(false)
	c.Assert(err, ErrorMatches, "environment is already bootstrapped")

	info, err := t.Env.StateInfo()
	c.Assert(err, IsNil)
	c.Assert(info, NotNil)
	c.Check(info.Addrs, Not(HasLen), 0)

	if t.CanOpenState {
		st, err := state.Open(info)
		c.Assert(err, IsNil)
		env, err := st.EnvironConfig()
		c.Assert(err, IsNil)
		err = t.seedSecrets(env)	
		c.Assert(err, IsNil)
		if t.HasProvisioner {
			t.testProvisioning(c, st)
		}
		st.Close()
	}

	c.Logf("destroy env")
	t.Destroy(c)

	// check that we can bootstrap after destroy
	t.BootstrapOnce(c)
}

// seed secrets pushes secrets into the state
func (t *LiveTests) seedSecrets(env *state.ConfigNode) error {
	// this disgusting hack only works for ec2 without real credentials
	env.Set("type", "ec2")
	env.Set("name", "sample")
	env.Set("region", "test")
	env.Set("control-bucket", "test-bucket")
	env.Set("public-bucket", "public-tools")
	env.Set("access-key", "x")
	env.Set("secret-key", "x")
	_, err := env.Write()
	return err
}

func (t *LiveTests) testProvisioning(c *C, st *state.State) {
	// place a new machine into the state
	m, err := st.AddMachine()
	c.Assert(err, IsNil)

	t.checkStartInstance(c, m)

	// now remove it
	c.Assert(st.RemoveMachine(m.Id()), IsNil)

	// watch the PA remove it
	t.checkStopInstance(c, m)
	checkMachineId(c, m, nil)
}

var agentReaction = environs.AttemptStrategy{
	Total: 30 * time.Second,
	Delay: 1 * time.Second,
}

func (t *LiveTests) checkStartInstance(c *C, m *state.Machine) (instId string) {
	// Wait for machine to get instance id.
	for a := agentReaction.Start(); a.Next(); {
		var err error
		instId, err = m.InstanceId()
		if _, ok := err.(*state.NoInstanceIdError); ok {
			continue
		}
		c.Assert(err, IsNil)
		if instId != "" {
			break
		}
	}
	if instId == "" {
		c.Fatalf("provisioner failed to start machine after %v", agentReaction.Total)
	}
	_, err := t.Env.Instances([]string{instId})
	c.Assert(err, IsNil)
	return
}

func (t *LiveTests) checkStopInstance(c *C, m *state.Machine) {
	// Wait for machine to get instance id.
	for a := agentReaction.Start(); a.Next(); {
		if instId, err := m.InstanceId() ; instId == "" {
			c.Assert(err, FitsTypeOf, &state.NoInstanceIdError{})
			return
		}
	}
	c.Fatalf("provisioner failed to stop machine after %v", agentReaction.Total)
}

// checkMachineIdSet checks that the machine has an instance id
// that matches that of the given instance. If the instance is nil,
// It checks that the instance id is unset.
func checkMachineId(c *C, m *state.Machine, inst environs.Instance) {
        // TODO(dfc) add machine.WatchConfig() to avoid having to poll.
        instId := ""
        if inst != nil {
                instId = inst.Id()
        }
        for a := agentReaction.Start(); a.Next(); {
                _, err := m.InstanceId()
                _, notset := err.(*state.NoInstanceIdError)
                if notset {
                        if inst == nil {
                                return
                        } else {
                                continue
                        }
                }
                c.Assert(err, IsNil)
                break
        }
        id, err := m.InstanceId()
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
