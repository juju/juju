package jujutest

import (
	"bytes"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/environs"
)

func (t *Tests) TestStartStop(c *C) {
	e := t.open(c)

	insts, err := e.Instances()
	c.Assert(err, IsNil)
	c.Assert(len(insts), Equals, 0)

	inst, err := e.StartInstance(0)
	c.Assert(err, IsNil)
	c.Assert(inst, NotNil)
	id0 := inst.Id()

	insts, err = e.Instances()
	c.Assert(err, IsNil)
	c.Assert(len(insts), Equals, 1)
	c.Assert(insts[0].Id(), Equals, id0)

	err = e.StopInstances([]environs.Instance{inst})
	c.Assert(err, IsNil)

	insts, err = e.Instances()
	c.Assert(err, IsNil)
	c.Assert(len(insts), Equals, 0)
}

func (t *Tests) TestPersistence(c *C) {
	e := t.open(c)
	name := "persistent-file"
	checkFileDoesNotExist(c, e, name)
	checkPutFile(c, e, name, contents)

	e2 := t.open(c)
	checkFileHasContents(c, e2, name, contents)

	// remove the file...
	err := e2.RemoveFile(name)
	c.Check(err, IsNil)

	// ... and check it's been removed in the other environment
	checkFileDoesNotExist(c, e, name)
}

func checkPutFile(c *C, e environs.Environ, name string, contents []byte) {
	err := e.PutFile(name, bytes.NewBuffer(contents), int64(len(contents)))
	c.Assert(err, IsNil)
}

func checkFileDoesNotExist(c *C, e environs.Environ, name string) {
	r, err := e.GetFile(name)
	c.Check(r, IsNil)
	c.Assert(err, NotNil)
}

func checkFileHasContents(c *C, e environs.Environ, name string, contents []byte) {
	r, err := e.GetFile(name)
	c.Check(r, NotNil)
	c.Assert(err, IsNil)

	data, err := ioutil.ReadAll(r)
	c.Check(err, IsNil)
	c.Check(data, Equals, contents)
}
