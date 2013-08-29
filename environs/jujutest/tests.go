// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujutest

import (
	"bytes"
	"io"
	"io/ioutil"
	"net/http"
	"sort"

	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/bootstrap"
	"launchpad.net/juju-core/environs/config"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju/testing"
	coretesting "launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/version"
)

// TestConfig contains the configuration for the environment
// This is a is an indirection to make it harder for tests to accidentally
// share the underlying map.
type TestConfig struct {
	Config map[string]interface{}
}

// UpdateConfig modifies the configuration safely by creating a new map
func (testConfig *TestConfig) UpdateConfig(update map[string]interface{}) {
	newConfig := map[string]interface{}{}
	for key, val := range testConfig.Config {
		newConfig[key] = val
	}
	for key, val := range update {
		newConfig[key] = val
	}
	testConfig.Config = newConfig
}

// Tests is a gocheck suite containing tests verifying juju functionality
// against the environment with the given configuration. The
// tests are not designed to be run against a live server - the Environ
// is opened once for each test, and some potentially expensive operations
// may be executed.
type Tests struct {
	coretesting.LoggingSuite
	envtesting.ToolsSuite
	TestConfig TestConfig
	Env        environs.Environ
}

// Open opens an instance of the testing environment.
func (t *Tests) Open(c *C) environs.Environ {
	e, err := environs.NewFromAttrs(t.TestConfig.Config)
	c.Assert(err, IsNil, Commentf("opening environ %#v", t.TestConfig.Config))
	c.Assert(e, NotNil)
	return e
}

func (t *Tests) SetUpTest(c *C) {
	t.LoggingSuite.SetUpTest(c)
	t.ToolsSuite.SetUpTest(c)
	cfg, err := config.New(t.TestConfig.Config)
	c.Assert(err, IsNil)
	t.Env, err = environs.Prepare(cfg)
	c.Assert(err, IsNil)
}

func (t *Tests) TearDownTest(c *C) {
	if t.Env != nil {
		err := t.Env.Destroy(nil)
		c.Check(err, IsNil)
		t.Env = nil
	}
	t.ToolsSuite.TearDownTest(c)
	t.LoggingSuite.TearDownTest(c)
}

func (t *Tests) TestStartStop(c *C) {
	e := t.Open(c)
	envtesting.UploadFakeTools(c, e.Storage())
	cfg, err := e.Config().Apply(map[string]interface{}{
		"agent-version": version.Current.Number.String(),
	})
	c.Assert(err, IsNil)
	err = e.SetConfig(cfg)
	c.Assert(err, IsNil)

	insts, err := e.Instances(nil)
	c.Assert(err, IsNil)
	c.Assert(insts, HasLen, 0)

	inst0, hc := testing.StartInstance(c, e, "0")
	c.Assert(inst0, NotNil)
	id0 := inst0.Id()
	// Sanity check for hardware characteristics.
	c.Assert(hc.Arch, NotNil)
	c.Assert(hc.Mem, NotNil)
	c.Assert(hc.CpuCores, NotNil)

	inst1, _ := testing.StartInstance(c, e, "1")
	c.Assert(inst1, NotNil)
	id1 := inst1.Id()

	insts, err = e.Instances([]instance.Id{id0, id1})
	c.Assert(err, IsNil)
	c.Assert(insts, HasLen, 2)
	c.Assert(insts[0].Id(), Equals, id0)
	c.Assert(insts[1].Id(), Equals, id1)

	// order of results is not specified
	insts, err = e.AllInstances()
	c.Assert(err, IsNil)
	c.Assert(insts, HasLen, 2)
	c.Assert(insts[0].Id(), Not(Equals), insts[1].Id())

	err = e.StopInstances([]instance.Instance{inst0})
	c.Assert(err, IsNil)

	insts, err = e.Instances([]instance.Id{id0, id1})
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
	err := bootstrap.Bootstrap(e, constraints.Value{})
	c.Assert(err, IsNil)

	info, apiInfo, err := e.StateInfo()
	c.Check(info.Addrs, Not(HasLen), 0)
	c.Check(apiInfo.Addrs, Not(HasLen), 0)

	err = bootstrap.Bootstrap(e, constraints.Value{})
	c.Assert(err, ErrorMatches, "environment is already bootstrapped")

	e2 := t.Open(c)
	err = bootstrap.Bootstrap(e2, constraints.Value{})
	c.Assert(err, ErrorMatches, "environment is already bootstrapped")

	info2, apiInfo2, err := e2.StateInfo()
	c.Check(info2, DeepEquals, info)
	c.Check(apiInfo2, DeepEquals, apiInfo)

	err = e2.Destroy(nil)
	c.Assert(err, IsNil)

	// Open again because Destroy invalidates old environments.
	e3 := t.Open(c)

	err = bootstrap.Bootstrap(e3, constraints.Value{})
	c.Assert(err, IsNil)

	err = bootstrap.Bootstrap(e3, constraints.Value{})
	c.Assert(err, NotNil)
}

var noRetry = utils.AttemptStrategy{}

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
	// TODO(dfc) gocheck should grow an SliceEquals checker.
	expected := copyslice(lnames)
	sort.Strings(expected)
	actual := copyslice(names)
	sort.Strings(actual)
	c.Assert(expected, DeepEquals, actual)
}

// copyslice returns a copy of the slice
func copyslice(s []string) []string {
	r := make([]string, len(s))
	copy(r, s)
	return r
}

func checkPutFile(c *C, storage environs.StorageWriter, name string, contents []byte) {
	err := storage.Put(name, bytes.NewBuffer(contents), int64(len(contents)))
	c.Assert(err, IsNil)
}

func checkFileDoesNotExist(c *C, storage environs.StorageReader, name string, attempt utils.AttemptStrategy) {
	var r io.ReadCloser
	var err error
	for a := attempt.Start(); a.Next(); {
		r, err = storage.Get(name)
		if err != nil {
			break
		}
	}
	c.Assert(r, IsNil)
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
}

func checkFileHasContents(c *C, storage environs.StorageReader, name string, contents []byte, attempt utils.AttemptStrategy) {
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
