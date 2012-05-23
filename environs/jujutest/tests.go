package jujutest

import (
	"bytes"
	"io"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/environs"
	"net/http"
	"time"
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
	storage := t.Open(c).Storage()

	names := []string{
		"aa",
		"zzz/aa",
		"zzz/bb",
	}
	for _, name := range names {
		checkFileDoesNotExist(c, storage, name)
		checkPutFile(c, storage, name, []byte(name))
	}
	checkList(c, storage, "", names)
	checkList(c, storage, "a", []string{"aa"})
	checkList(c, storage, "zzz/", []string{"zzz/aa", "zzz/bb"})

	storage2 := t.Open(c).Storage()
	for _, name := range names {
		checkFileHasContents(c, storage2, name, []byte(name))
	}

	// remove the first file and check that the others remain.
	err := storage2.Remove(names[0])
	c.Check(err, IsNil)

	// check that it's ok to remove a file twice.
	err = storage2.Remove(names[0])
	c.Check(err, IsNil)

	// ... and check it's been removed in the other environment
	checkFileDoesNotExist(c, storage, names[0])

	// ... and that the rest of the files are still around
	checkList(c, storage2, "", names[1:])

	for _, name := range names[1:] {
		err := storage2.Remove(name)
		c.Assert(err, IsNil)
	}

	// check they've all gone
	checkList(c, storage2, "", nil)
}

func checkList(c *C, storage environs.StorageReader, prefix string, names []string) {
	lnames, err := storage.List(prefix)
	c.Assert(err, IsNil)
	c.Assert(lnames, DeepEquals, names)
}

func checkPutFile(c *C, storage environs.StorageWriter, name string, contents []byte) {
	err := storage.Put(name, bytes.NewBuffer(contents), int64(len(contents)))
	c.Assert(err, IsNil)
}

func checkFileDoesNotExist(c *C, storage environs.StorageReader, name string) {
	// TODO eventual consistency
	r, err := storage.Get(name)
	c.Check(r, IsNil)
	c.Assert(err, NotNil)
	var notFoundError *environs.NotFoundError
	c.Assert(err, FitsTypeOf, notFoundError)
}

func checkFileHasContents(c *C, storage environs.StorageReader, name string, contents []byte) {
	var r io.ReadCloser
	var err error

	for i := 0; i < 5; i++ {
		r, err = storage.Get(name)
		if err == nil {
			break
		}
		time.Sleep(1e9)
	}
	c.Assert(err, IsNil)
	c.Check(r, NotNil)
	defer r.Close()

	data, err := ioutil.ReadAll(r)
	c.Check(err, IsNil)
	c.Check(data, DeepEquals, contents)

	url, err := storage.URL(name)
	c.Assert(err, IsNil)

	var resp *http.Response
	for i := 0; i < 5; i++ {
		resp, err = http.Get(url)
		c.Assert(err, IsNil)
		if resp.StatusCode != 404 {
			break
		}
		c.Logf("get retrying after earlier get succeeded. *sigh*.")
		time.Sleep(1e9)
	}
	c.Assert(err, IsNil)
	data, err = ioutil.ReadAll(resp.Body)
	c.Assert(err, IsNil)
	defer resp.Body.Close()
	c.Assert(resp.StatusCode, Equals, 200, Commentf("error response: %s", data))
	c.Check(data, DeepEquals, contents)
}
