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
	// TODO tests for Bootstrap(true)
	e := t.Open(c)
	err := e.Bootstrap(false)
	c.Assert(err, IsNil)

	info, err := e.StateInfo()
	c.Assert(info, NotNil)
	c.Check(info.Addrs, Not(HasLen), 0)

	// TODO eventual consistency.
	err = e.Bootstrap(false)
	c.Assert(err, ErrorMatches, "environment is already bootstrapped")

	e2 := t.Open(c)
	// TODO eventual consistency.
	err = e2.Bootstrap(false)
	c.Assert(err, ErrorMatches, "environment is already bootstrapped")

	info2, err := e2.StateInfo()
	c.Check(info2, DeepEquals, info)

	err = e2.Destroy(nil)
	c.Assert(err, IsNil)

	err = e.Bootstrap(false)
	c.Assert(err, IsNil)

	err = e.Bootstrap(false)
	c.Assert(err, NotNil)
}

func (t *Tests) TestPersistence(c *C) {
	store := t.Open(c).Storage()

	names := []string{
		"aa",
		"zzz/aa",
		"zzz/bb",
	}
	for _, name := range names {
		checkFileDoesNotExist(c, store, name)
		checkPutFile(c, store, name, []byte(name))
	}
	checkList(c, store, "", names)
	checkList(c, store, "a", []string{"aa"})
	checkList(c, store, "zzz/", []string{"zzz/aa", "zzz/bb"})

	store2 := t.Open(c).Storage()
	for _, name := range names {
		checkFileHasContents(c, store2, name, []byte(name))
	}

	// remove the first file and check that the others remain.
	err := store2.Remove(names[0])
	c.Check(err, IsNil)

	// check that it's ok to remove a file twice.
	err = store2.Remove(names[0])
	c.Check(err, IsNil)

	// ... and check it's been removed in the other environment
	checkFileDoesNotExist(c, store, names[0])

	// ... and that the rest of the files are still around
	checkList(c, store2, "", names[1:])

	for _, name := range names[1:] {
		err := store2.Remove(name)
		c.Assert(err, IsNil)
	}

	// check they've all gone
	checkList(c, store2, "", nil)
}

func checkList(c *C, store environs.StorageReader, prefix string, names []string) {
	lnames, err := store.List(prefix)
	c.Assert(err, IsNil)
	c.Assert(lnames, DeepEquals, names)
}

func checkPutFile(c *C, store environs.StorageWriter, name string, contents []byte) {
	err := store.Put(name, bytes.NewBuffer(contents), int64(len(contents)))
	c.Assert(err, IsNil)
}

func checkFileDoesNotExist(c *C, store environs.StorageReader, name string) {
	// TODO eventual consistency
	r, err := store.Get(name)
	c.Check(r, IsNil)
	c.Assert(err, NotNil)
	var notFoundError *environs.NotFoundError
	c.Assert(err, FitsTypeOf, notFoundError)
}

func checkFileHasContents(c *C, store environs.StorageReader, name string, contents []byte) {
	r, err := store.Get(name)
	c.Assert(err, IsNil)
	c.Check(r, NotNil)

	data, err := ioutil.ReadAll(r)
	c.Check(err, IsNil)
	c.Check(data, DeepEquals, contents)
}
