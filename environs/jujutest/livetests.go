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
	names := make(map[string]environs.Instance)
	insts, err := t.Env.Instances()
	c.Assert(err, IsNil)

	// check there are no duplicate instance ids
	for _, inst := range insts {
		id := inst.Id()
		c.Assert(names[id], IsNil)
		names[id] = inst
	}

	inst, err := t.Env.StartInstance(0)
	c.Assert(err, IsNil)
	c.Assert(inst, NotNil)
	id0 := inst.Id()

	insts, err = t.Env.Instances()
	c.Assert(err, IsNil)

	// check the new instance is found
	found := false
	for _, inst := range insts {
		if inst.Id() == id0 {
			c.Assert(found, Equals, false)
			found = true
		}
	}
	c.Check(found, Equals, true)

	err = t.Env.StopInstances([]environs.Instance{inst})
	c.Assert(err, IsNil)

	insts, err = t.Env.Instances()
	c.Assert(err, IsNil)
	c.Assert(len(insts), Equals, 0, Bug("instances: %v", insts))

	// check the instance is no longer there.
	found = true
	for _, inst := range insts {
		c.Assert(inst.Id(), Not(Equals), id0)
	}
}

func (t *LiveTests) TestBootstrap(c *C) {
	err := t.Env.Bootstrap()
	c.Assert(err, IsNil)

	err = t.Env.Bootstrap()
	c.Assert(err, ErrorMatches, "environment is already bootstrapped")

	err = t.Env.Destroy()
	c.Assert(err, IsNil)

	// check that we can bootstrap after destroy
	err = t.Env.Bootstrap()
	c.Assert(err, IsNil)

	err = t.Env.Destroy()
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
}
