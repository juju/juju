package jujutest

import (
	"bytes"
	"io"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/juju/testing"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/trivial"
	"net/http"
)

// Tests is a gocheck suite containing tests verifying juju functionality
// against the environment with the given configuration. The
// tests are not designed to be run against a live server - the Environ
// is opened once for each test, and some potentially expensive operations
// may be executed.
type Tests struct {
	coretesting.LoggingSuite
	Config map[string]interface{}
	Env    environs.Environ
}

// Open opens an instance of the testing environment.
func (t *Tests) Open(c *C) environs.Environ {
	e, err := environs.NewFromAttrs(t.Config)
	c.Assert(err, IsNil, Commentf("opening environ %#v", t.Config))
	c.Assert(e, NotNil)
	return e
}

func (t *Tests) SetUpTest(c *C) {
	t.LoggingSuite.SetUpTest(c)
	t.Env = t.Open(c)
}

func (t *Tests) TearDownTest(c *C) {
	if t.Env != nil {
		err := t.Env.Destroy(nil)
		c.Check(err, IsNil)
		t.Env = nil
	}
	t.LoggingSuite.TearDownTest(c)
}

func (t *Tests) TestBootstrapWithoutAdminSecret(c *C) {
	m := t.Env.Config().AllAttrs()
	delete(m, "admin-secret")
	env, err := environs.NewFromAttrs(m)
	c.Assert(err, IsNil)
	err = juju.Bootstrap(env, false, []byte(coretesting.CACert + coretesting.CAKey))
	c.Assert(err, ErrorMatches, ".*admin-secret is required for bootstrap")
}

func (t *Tests) TestStartStop(c *C) {
	e := t.Open(c)

	insts, err := e.Instances(nil)
	c.Assert(err, IsNil)
	c.Assert(insts, HasLen, 0)

	inst0, err := e.StartInstance(0, testing.InvalidStateInfo(0), nil)
	c.Assert(err, IsNil)
	c.Assert(inst0, NotNil)
	id0 := inst0.Id()

	inst1, err := e.StartInstance(1, testing.InvalidStateInfo(1), nil)
	c.Assert(err, IsNil)
	c.Assert(inst1, NotNil)
	id1 := inst1.Id()

	insts, err = e.Instances([]string{id0, id1})
	c.Assert(err, IsNil)
	c.Assert(insts, HasLen, 2)
	c.Assert(insts[0].Id(), Equals, id0)
	c.Assert(insts[1].Id(), Equals, id1)

	// order of results is not specified
	insts, err = e.AllInstances()
	c.Assert(err, IsNil)
	c.Assert(insts, HasLen, 2)
	c.Assert(insts[0].Id(), Not(Equals), insts[1].Id())

	err = e.StopInstances([]environs.Instance{inst0})
	c.Assert(err, IsNil)

	insts, err = e.Instances([]string{id0, id1})
	c.Assert(err, Equals, environs.ErrPartialInstances)
	c.Assert(insts[0], IsNil)
	c.Assert(insts[1].Id(), Equals, id1)

	insts, err = e.AllInstances()
	c.Assert(err, IsNil)
	c.Assert(insts[0].Id(), Equals, id1)
}

func (t *Tests) TestBootstrap(c *C) {
	// TODO tests for Bootstrap(true)
	e := t.Open(c)
	err := juju.Bootstrap(e, false, []byte(coretesting.CACert + coretesting.CAKey))
	c.Assert(err, IsNil)

	info, err := e.StateInfo()
	c.Assert(info, NotNil)
	c.Check(info.Addrs, Not(HasLen), 0)

	err = juju.Bootstrap(e, false, []byte(coretesting.CACert + coretesting.CAKey))
	c.Assert(err, ErrorMatches, "environment is already bootstrapped")

	e2 := t.Open(c)
	err = juju.Bootstrap(e2, false, []byte(coretesting.CACert + coretesting.CAKey))
	c.Assert(err, ErrorMatches, "environment is already bootstrapped")

	info2, err := e2.StateInfo()
	c.Check(info2, DeepEquals, info)

	err = e2.Destroy(nil)
	c.Assert(err, IsNil)

	// Open again because Destroy invalidates old environments.
	e3 := t.Open(c)

	err = juju.Bootstrap(e3, false, []byte(coretesting.CACert + coretesting.CAKey))
	c.Assert(err, IsNil)

	err = juju.Bootstrap(e3, false, []byte(coretesting.CACert + coretesting.CAKey))
	c.Assert(err, NotNil)
}

var noRetry = trivial.AttemptStrategy{}

func (t *Tests) TestPersistence(c *C) {
	storage := t.Open(c).Storage()

	names := []string{
		"aa",
		"zzz/aa",
		"zzz/bb",
	}
	for _, name := range names {
		checkFileDoesNotExist(c, storage, name, noRetry)
		checkPutFile(c, storage, name, []byte(name))
	}
	checkList(c, storage, "", names)
	checkList(c, storage, "a", []string{"aa"})
	checkList(c, storage, "zzz/", []string{"zzz/aa", "zzz/bb"})

	storage2 := t.Open(c).Storage()
	for _, name := range names {
		checkFileHasContents(c, storage2, name, []byte(name), noRetry)
	}

	// remove the first file and check that the others remain.
	err := storage2.Remove(names[0])
	c.Check(err, IsNil)

	// check that it's ok to remove a file twice.
	err = storage2.Remove(names[0])
	c.Check(err, IsNil)

	// ... and check it's been removed in the other environment
	checkFileDoesNotExist(c, storage, names[0], noRetry)

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

func checkFileDoesNotExist(c *C, storage environs.StorageReader, name string, attempt trivial.AttemptStrategy) {
	var r io.ReadCloser
	var err error
	for a := attempt.Start(); a.Next(); {
		r, err = storage.Get(name)
		if err != nil {
			break
		}
	}
	c.Assert(r, IsNil)
	var notFoundError *environs.NotFoundError
	c.Assert(err, FitsTypeOf, notFoundError)
}

func checkFileHasContents(c *C, storage environs.StorageReader, name string, contents []byte, attempt trivial.AttemptStrategy) {
	r, err := storage.Get(name)
	c.Assert(err, IsNil)
	c.Check(r, NotNil)
	defer r.Close()

	data, err := ioutil.ReadAll(r)
	c.Check(err, IsNil)
	c.Check(data, DeepEquals, contents)

	url, err := storage.URL(name)
	c.Assert(err, IsNil)

	var resp *http.Response
	for a := attempt.Start(); a.Next(); {
		resp, err = http.Get(url)
		c.Assert(err, IsNil)
		if resp.StatusCode != 404 {
			break
		}
		c.Logf("get retrying after earlier get succeeded. *sigh*.")
	}
	c.Assert(err, IsNil)
	data, err = ioutil.ReadAll(resp.Body)
	c.Assert(err, IsNil)
	defer resp.Body.Close()
	c.Assert(resp.StatusCode, Equals, 200, Commentf("error response: %s", data))
	c.Check(data, DeepEquals, contents)
}
