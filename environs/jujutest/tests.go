package jujutest

import (
	"bytes"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/environs"
)

func (t *Tests) TestStartStop(c *C) {
	e := t.Open(c)

	insts, err := e.Instances(nil)
	c.Assert(err, IsNil)
	c.Assert(insts, HasLen, 0)

	inst0, err := e.StartInstance(0, InvalidStateInfo)
	c.Assert(err, IsNil)
	c.Assert(inst0, NotNil)
	id0 := inst0.Id()

	inst1, err := e.StartInstance(1, InvalidStateInfo)
	c.Assert(err, IsNil)
	c.Assert(inst1, NotNil)
	id1 := inst1.Id()

	insts, err = e.Instances([]string{id0, id1})
	c.Assert(err, IsNil)
	c.Assert(insts, HasLen, 2)
	c.Assert(insts[0].Id(), Equals, id0)
	c.Assert(insts[1].Id(), Equals, id1)

	err = e.StopInstances([]environs.Instance{inst0})
	c.Assert(err, IsNil)

	insts, err = e.Instances([]string{id0, id1})
	c.Assert(err, Equals, environs.ErrPartialInstances)
	c.Assert(insts[0], IsNil)
	c.Assert(insts[1].Id(), Equals, id1)
}

func (t *Tests) TestBootstrap(c *C) {
	e := t.Open(c)
	err := e.Bootstrap()
	c.Assert(err, IsNil)

	info, err := e.StateInfo()
	c.Assert(info, NotNil)
	c.Check(info.Addrs, Not(HasLen), 0)

	// TODO eventual consistency.
	err = e.Bootstrap()
	c.Assert(err, ErrorMatches, "environment is already bootstrapped")

	e2 := t.Open(c)
	// TODO eventual consistency.
	err = e2.Bootstrap()
	c.Assert(err, ErrorMatches, "environment is already bootstrapped")

	info2, err := e2.StateInfo()
	c.Check(info2, DeepEquals, info)

	err = e2.Destroy(nil)
	c.Assert(err, IsNil)

	err = e.Bootstrap()
	c.Assert(err, IsNil)

	err = e.Bootstrap()
	c.Assert(err, NotNil)
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
	c.Check(data, DeepEquals, contents)
}
