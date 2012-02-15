package jujutest

import (
	"bytes"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/environs"
)

func (t *Tests) TestStartStop(c *C) {
	e := t.Open(c)

	insts, err := e.Instances()
	c.Assert(err, IsNil)
	c.Assert(len(insts), Equals, 0)

	inst0, err := e.StartInstance(0, InvalidStateInfo)
	c.Assert(err, IsNil)
	c.Assert(inst0, NotNil)
	id0 := inst0.Id()

	insts, err = e.Instances()
	c.Assert(err, IsNil)
	c.Assert(len(insts), Equals, 1)
	c.Assert(insts[0].Id(), Equals, id0)

	err = e.StopInstances([]environs.Instance{inst0})
	c.Assert(err, IsNil)

	insts, err = e.Instances()
	c.Assert(err, IsNil)
	c.Assert(len(insts), Equals, 0)
}

func (t *Tests) TestBootstrap(c *C) {
	e := t.Open(c)
	info, err := e.Bootstrap()
	c.Assert(err, IsNil)
	c.Assert(info, NotNil)
	c.Check(len(info.Addrs), Not(Equals), 0)

	// TODO eventual consistency.
	info2, err := e.Bootstrap()
	c.Assert(info2, IsNil)
	c.Assert(err, ErrorMatches, "environment is already bootstrapped")

	info3, err := e.StateInfo()
	c.Assert(err, IsNil)
	c.Check(info3, Equals, info)

	e2 := t.Open(c)
	// TODO eventual consistency.
	info4, err := e2.Bootstrap()
	c.Assert(info4, IsNil)
	c.Assert(err, ErrorMatches, "environment is already bootstrapped")

	info5, err := e2.StateInfo()
	c.Check(info5, Equals, info)

	err = e2.Destroy()
	c.Assert(err, IsNil)

	info, err = e.Bootstrap()
	c.Assert(err, IsNil)
	c.Assert(info, NotNil)

	info, err = e.Bootstrap()
	c.Assert(err, NotNil)
	c.Assert(info, IsNil)
}

func (t *Tests) TestPersistence(c *C) {
	e := t.Open(c)
	name := "persistent-file"
	checkFileDoesNotExist(c, e, name)
	checkPutFile(c, e, name, contents)

	e2 := t.Open(c)
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
	// TODO eventual consistency
	r, err := e.GetFile(name)
	c.Check(r, IsNil)
	c.Assert(err, NotNil)
}

func checkFileHasContents(c *C, e environs.Environ, name string, contents []byte) {
	r, err := e.GetFile(name)
	c.Assert(err, IsNil)
	c.Check(r, NotNil)

	data, err := ioutil.ReadAll(r)
	c.Check(err, IsNil)
	c.Check(data, Equals, contents)
}
