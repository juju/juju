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
	insts, err := t.env.Instances()
	c.Assert(err, IsNil)

	// check there are no duplicate instance ids
	for _, inst := range insts {
		id := inst.Id()
		c.Assert(names[id], IsNil)
		names[id] = inst
	}

	inst, err := t.env.StartInstance(1)
	c.Assert(err, IsNil)
	c.Assert(inst, NotNil)
	id0 := inst.Id()

	insts, err = t.env.Instances()
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

	err = t.env.StopInstances([]environs.Instance{inst})
	c.Assert(err, IsNil)

	insts, err = t.env.Instances()
	c.Assert(err, IsNil)
	c.Assert(len(insts), Equals, 0)

	// check the instance is no longer there.
	found = true
	for _, inst := range insts {
		c.Assert(inst.Id(), Not(Equals), id0)
	}
}

// TODO check that binary data works ok?
var contents = []byte("hello\n")
var contents2 = []byte("goodbye\n\n")

func (t *LiveTests) TestFile(c *C) {
	name := fmt.Sprint("testfile", time.Now().UnixNano())

	// check that file does not currently exist.
	checkFileDoesNotExist(c, t.env, name)

	checkPutFile(c, t.env, name, contents)

	checkFileHasContents(c, t.env, name, contents)

	// check that we can overwrite the file
	checkPutFile(c, t.env, name, contents2)

	checkFileHasContents(c, t.env, name, contents2)

	// remove the file...
	err := t.env.RemoveFile(name)
	c.Check(err, IsNil)

	// ...and check that the file has been removed correctly.
	checkFileDoesNotExist(c, t.env, name)
}
