// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujutest

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"sort"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/bootstrap"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/configstore"
	"launchpad.net/juju-core/environs/storage"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/provider/common"
	coretesting "launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/version"
)

// Tests is a gocheck suite containing tests verifying juju functionality
// against the environment with the given configuration. The
// tests are not designed to be run against a live server - the Environ
// is opened once for each test, and some potentially expensive operations
// may be executed.
type Tests struct {
	testbase.LoggingSuite
	TestConfig coretesting.Attrs
	envtesting.ToolsFixture

	// ConfigStore holds the configuration storage
	// used when preparing the environment.
	// This is initialized by SetUpTest.
	ConfigStore configstore.Storage
}

// Open opens an instance of the testing environment.
func (t *Tests) Open(c *gc.C) environs.Environ {
	info, err := t.ConfigStore.ReadInfo(t.TestConfig["name"].(string))
	c.Assert(err, gc.IsNil)
	cfg, err := config.New(config.NoDefaults, info.BootstrapConfig())
	c.Assert(err, gc.IsNil)
	e, err := environs.New(cfg)
	c.Assert(err, gc.IsNil, gc.Commentf("opening environ %#v", cfg.AllAttrs()))
	c.Assert(e, gc.NotNil)
	return e
}

// Prepare prepares an instance of the testing environment.
func (t *Tests) Prepare(c *gc.C) environs.Environ {
	cfg, err := config.New(config.NoDefaults, t.TestConfig)
	c.Assert(err, gc.IsNil)
	e, err := environs.Prepare(cfg, t.ConfigStore)
	c.Assert(err, gc.IsNil, gc.Commentf("preparing environ %#v", t.TestConfig))
	c.Assert(e, gc.NotNil)
	return e
}

func (t *Tests) SetUpTest(c *gc.C) {
	t.LoggingSuite.SetUpTest(c)
	t.ToolsFixture.SetUpTest(c)
	t.ConfigStore = configstore.NewMem()
}

func (t *Tests) TearDownTest(c *gc.C) {
	t.ToolsFixture.TearDownTest(c)
	t.LoggingSuite.TearDownTest(c)
}

func bootstrapContext(c *gc.C) environs.BootstrapContext {
	return envtesting.NewBootstrapContext(coretesting.Context(c))
}

func (t *Tests) TestStartStop(c *gc.C) {
	e := t.Prepare(c)
	envtesting.UploadFakeTools(c, e.Storage())
	cfg, err := e.Config().Apply(map[string]interface{}{
		"agent-version": version.Current.Number.String(),
	})
	c.Assert(err, gc.IsNil)
	err = e.SetConfig(cfg)
	c.Assert(err, gc.IsNil)

	insts, err := e.Instances(nil)
	c.Assert(err, gc.IsNil)
	c.Assert(insts, gc.HasLen, 0)

	inst0, hc := testing.AssertStartInstance(c, e, "0")
	c.Assert(inst0, gc.NotNil)
	id0 := inst0.Id()
	// Sanity check for hardware characteristics.
	c.Assert(hc.Arch, gc.NotNil)
	c.Assert(hc.Mem, gc.NotNil)
	c.Assert(hc.CpuCores, gc.NotNil)

	inst1, _ := testing.AssertStartInstance(c, e, "1")
	c.Assert(inst1, gc.NotNil)
	id1 := inst1.Id()

	insts, err = e.Instances([]instance.Id{id0, id1})
	c.Assert(err, gc.IsNil)
	c.Assert(insts, gc.HasLen, 2)
	c.Assert(insts[0].Id(), gc.Equals, id0)
	c.Assert(insts[1].Id(), gc.Equals, id1)

	// order of results is not specified
	insts, err = e.AllInstances()
	c.Assert(err, gc.IsNil)
	c.Assert(insts, gc.HasLen, 2)
	c.Assert(insts[0].Id(), gc.Not(gc.Equals), insts[1].Id())

	err = e.StopInstances([]instance.Instance{inst0})
	c.Assert(err, gc.IsNil)

	insts, err = e.Instances([]instance.Id{id0, id1})
	c.Assert(err, gc.Equals, environs.ErrPartialInstances)
	c.Assert(insts[0], gc.IsNil)
	c.Assert(insts[1].Id(), gc.Equals, id1)

	insts, err = e.AllInstances()
	c.Assert(err, gc.IsNil)
	c.Assert(insts[0].Id(), gc.Equals, id1)
}

func (t *Tests) TestBootstrap(c *gc.C) {
	e := t.Prepare(c)
	envtesting.UploadFakeTools(c, e.Storage())
	err := common.EnsureNotBootstrapped(e)
	c.Assert(err, gc.IsNil)
	err = bootstrap.Bootstrap(bootstrapContext(c), e, constraints.Value{})
	c.Assert(err, gc.IsNil)

	info, apiInfo, err := e.StateInfo()
	c.Check(info.Addrs, gc.Not(gc.HasLen), 0)
	c.Check(apiInfo.Addrs, gc.Not(gc.HasLen), 0)

	err = common.EnsureNotBootstrapped(e)
	c.Assert(err, gc.ErrorMatches, "environment is already bootstrapped")

	e2 := t.Open(c)
	envtesting.UploadFakeTools(c, e2.Storage())
	err = common.EnsureNotBootstrapped(e2)
	c.Assert(err, gc.ErrorMatches, "environment is already bootstrapped")

	info2, apiInfo2, err := e2.StateInfo()
	c.Check(info2, gc.DeepEquals, info)
	c.Check(apiInfo2, gc.DeepEquals, apiInfo)

	err = environs.Destroy(e2, t.ConfigStore)
	c.Assert(err, gc.IsNil)

	// Prepare again because Destroy invalidates old environments.
	e3 := t.Prepare(c)
	envtesting.UploadFakeTools(c, e3.Storage())

	err = common.EnsureNotBootstrapped(e3)
	c.Assert(err, gc.IsNil)
	err = bootstrap.Bootstrap(bootstrapContext(c), e3, constraints.Value{})
	c.Assert(err, gc.IsNil)

	err = common.EnsureNotBootstrapped(e3)
	c.Assert(err, gc.ErrorMatches, "environment is already bootstrapped")
}

var noRetry = utils.AttemptStrategy{}

func (t *Tests) TestPersistence(c *gc.C) {
	stor := t.Prepare(c).Storage()

	names := []string{
		"aa",
		"zzz/aa",
		"zzz/bb",
	}
	for _, name := range names {
		checkFileDoesNotExist(c, stor, name, noRetry)
		checkPutFile(c, stor, name, []byte(name))
	}
	checkList(c, stor, "", names)
	checkList(c, stor, "a", []string{"aa"})
	checkList(c, stor, "zzz/", []string{"zzz/aa", "zzz/bb"})

	storage2 := t.Open(c).Storage()
	for _, name := range names {
		checkFileHasContents(c, storage2, name, []byte(name), noRetry)
	}

	// remove the first file and check that the others remain.
	err := storage2.Remove(names[0])
	c.Check(err, gc.IsNil)

	// check that it's ok to remove a file twice.
	err = storage2.Remove(names[0])
	c.Check(err, gc.IsNil)

	// ... and check it's been removed in the other environment
	checkFileDoesNotExist(c, stor, names[0], noRetry)

	// ... and that the rest of the files are still around
	checkList(c, storage2, "", names[1:])

	for _, name := range names[1:] {
		err := storage2.Remove(name)
		c.Assert(err, gc.IsNil)
	}

	// check they've all gone
	checkList(c, storage2, "", nil)
}

func checkList(c *gc.C, stor storage.StorageReader, prefix string, names []string) {
	lnames, err := storage.List(stor, prefix)
	c.Assert(err, gc.IsNil)
	// TODO(dfc) gocheck should grow an SliceEquals checker.
	expected := copyslice(lnames)
	sort.Strings(expected)
	actual := copyslice(names)
	sort.Strings(actual)
	c.Assert(expected, gc.DeepEquals, actual)
}

// copyslice returns a copy of the slice
func copyslice(s []string) []string {
	r := make([]string, len(s))
	copy(r, s)
	return r
}

func checkPutFile(c *gc.C, stor storage.StorageWriter, name string, contents []byte) {
	err := stor.Put(name, bytes.NewBuffer(contents), int64(len(contents)))
	c.Assert(err, gc.IsNil)
}

func checkFileDoesNotExist(c *gc.C, stor storage.StorageReader, name string, attempt utils.AttemptStrategy) {
	r, err := storage.GetWithRetry(stor, name, attempt)
	c.Assert(r, gc.IsNil)
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
}

func checkFileHasContents(c *gc.C, stor storage.StorageReader, name string, contents []byte, attempt utils.AttemptStrategy) {
	r, err := storage.GetWithRetry(stor, name, attempt)
	c.Assert(err, gc.IsNil)
	c.Check(r, gc.NotNil)
	defer r.Close()

	data, err := ioutil.ReadAll(r)
	c.Check(err, gc.IsNil)
	c.Check(data, gc.DeepEquals, contents)

	url, err := stor.URL(name)
	c.Assert(err, gc.IsNil)

	var resp *http.Response
	for a := attempt.Start(); a.Next(); {
		resp, err = http.Get(url)
		c.Assert(err, gc.IsNil)
		if resp.StatusCode != 404 {
			break
		}
		c.Logf("get retrying after earlier get succeeded. *sigh*.")
	}
	c.Assert(err, gc.IsNil)
	data, err = ioutil.ReadAll(resp.Body)
	c.Assert(err, gc.IsNil)
	defer resp.Body.Close()
	c.Assert(resp.StatusCode, gc.Equals, 200, gc.Commentf("error response: %s", data))
	c.Check(data, gc.DeepEquals, contents)
}
