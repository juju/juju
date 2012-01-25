package jujutest

import (
	"bytes"
	"fmt"
	"io/ioutil"
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
	name := fmt.Sprintf("testfile", time.Now().UnixNano())

	// check that file does not currently exist.
	r, err := t.env.GetFile(name)
	c.Check(r, IsNil)
	c.Assert(err, NotNil)

	// create a new file
	err = t.env.PutFile(name, bytes.NewBuffer(contents), int64(len(contents)))
	c.Assert(err, IsNil)

	// check the file now exists...
	r, err = t.env.GetFile(name)
	c.Check(r, NotNil)
	c.Assert(err, IsNil)

	// ... and has the correct contents
	data, err := ioutil.ReadAll(r)
	c.Check(err, IsNil)
	c.Check(data, Equals, contents)

	// check that we can overwrite the file
	err = t.env.PutFile(name, bytes.NewBuffer(contents2), int64(len(contents2)))
	c.Assert(err, IsNil)

	// check the file now has the new contents
	r, err = t.env.GetFile(name)
	c.Check(r, NotNil)
	c.Assert(err, IsNil)
	data, err = ioutil.ReadAll(r)
	c.Check(err, IsNil)
	c.Check(data, Equals, contents2)

	// remove the file...
	err = t.env.RemoveFile(name)
	c.Check(err, IsNil)

	// ...and check that the file has been removed correctly.
	r, err = t.env.GetFile(name)
	c.Check(r, IsNil)
	c.Assert(err, NotNil)
}
