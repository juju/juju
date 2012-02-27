package jujutest

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/environs"
	"time"
)

// TestStartStop is similar to Tests.TestStartStop except
// that it does not assume a pristine environment.
func (t *LiveTests) TestStartStop(c *C) {
	insts, err := t.Env.Instances(nil)
	c.Assert(err, IsNil)
	c.Check(len(insts), Equals, 0)

	inst, err := t.Env.StartInstance(0, InvalidStateInfo)
	c.Assert(err, IsNil)
	c.Assert(inst, NotNil)
	id0 := inst.Id()

	insts, err = t.Env.Instances([]string{id0, id0})
	c.Assert(err, IsNil)
	c.Assert(len(insts), Equals, 2)
	c.Assert(insts[0], Equals, inst)
	c.Assert(insts[1], Equals, inst)

	dns, err := inst.DNSName()
	c.Assert(err, IsNil)
	c.Assert(dns, Not(Equals), "")

	insts, err = t.Env.Instances([]string{id0, ""})
	c.Assert(err, Equals, environs.ErrMissingInstance)
	c.Assert(len(insts), Equals, 2, Bug("instances: %v", insts))
	c.Check(insts[0].Id(), Equals, id0)
	c.Check(insts[1], IsNil)

	err = t.Env.StopInstances([]environs.Instance{inst})
	c.Assert(err, IsNil)

	insts, err = t.Env.Instances([]string{id0})
	c.Assert(err, Equals, environs.ErrMissingInstance)
	c.Assert(len(insts), Equals, 0, Bug("instances: %v", insts))

	// check the instance is no longer there.
	for _, inst := range insts {
		c.Assert(inst.Id(), Not(Equals), id0)
	}
}

func (t *LiveTests) TestBootstrap(c *C) {
	c.Logf("initial bootstrap")
	err := t.Env.Bootstrap()
	c.Assert(err, IsNil)

	c.Logf("duplicate bootstrap")
	// Wait for a while to let eventual consistency catch up, hopefully.
	time.Sleep(t.ConsistencyDelay)
	err = t.Env.Bootstrap()
	c.Assert(err, ErrorMatches, "environment is already bootstrapped")

	info, err := t.Env.StateInfo()
	c.Assert(err, IsNil)
	c.Assert(info, NotNil)
	c.Check(len(info.Addrs), Not(Equals), 0, Bug("addrs: %q", info.Addrs))

	// TODO uncomment when State has a close method
	// st.Close()

	c.Logf("destroy env")
	err = t.Env.Destroy(nil)
	c.Assert(err, IsNil)

	// check that we can bootstrap after destroy
	err = t.Env.Bootstrap()
	c.Assert(err, IsNil)

	err = t.Env.Destroy(nil)
	c.Assert(err, IsNil)
}

// TODO check that binary data works ok?
var contents = []byte("hello\n")
var contents2 = []byte("goodbye\n\n")

func (t *LiveTests) TestFile(c *C) {
	name := fmt.Sprint("testfile", time.Now().UnixNano())
	checkFileDoesNotExist(c, t.Env, name)
	checkPutFile(c, t.Env, name, contents)
	checkFileHasContents(c, t.Env, name, contents)
	checkPutFile(c, t.Env, name, contents2) // check that we can overwrite the file
	checkFileHasContents(c, t.Env, name, contents2)
	err := t.Env.RemoveFile(name)
	c.Check(err, IsNil)
	checkFileDoesNotExist(c, t.Env, name)
	// removing a file that does not exist should not be an error.
	err = t.Env.RemoveFile(name)
	c.Check(err, IsNil)
}
