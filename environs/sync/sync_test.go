// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sync_test

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/environs/sync"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/environs/tools"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/version"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type syncSuite struct {
	coretesting.LoggingSuite
	home         *coretesting.FakeHome
	targetEnv    environs.Environ
	origVersion  version.Binary
	origLocation string
	storage      *envtesting.EC2HTTPTestStorage
	localStorage string
}

var _ = gc.Suite(&syncSuite{})

func (s *syncSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.origVersion = version.Current
	// It's important that this be v1 to match the test data.
	version.Current.Number = version.MustParse("1.2.3")

	// Create a target environments.yaml and make sure its environment is empty.
	s.home = coretesting.MakeFakeHome(c, `
environments:
    test-target:
        type: dummy
        state-server: false
        authorized-keys: "not-really-one"
`)
	var err error
	s.targetEnv, err = environs.NewFromName("test-target")
	c.Assert(err, gc.IsNil)
	envtesting.RemoveAllTools(c, s.targetEnv)

	// Create a source storage.
	s.storage, err = envtesting.NewEC2HTTPTestStorage("127.0.0.1")
	c.Assert(err, gc.IsNil)

	// Create a local tools directory.
	s.localStorage = c.MkDir()

	// Populate both with the public tools.
	for _, vers := range vAll {
		s.storage.PutBinary(vers)
		putBinary(c, s.localStorage, vers)
	}

	s.origLocation = sync.SetDefaultToolsLocation(s.storage.Location())
}

func (s *syncSuite) TearDownTest(c *gc.C) {
	c.Assert(s.storage.Stop(), gc.IsNil)
	sync.SetDefaultToolsLocation(s.origLocation)
	dummy.Reset()
	s.home.Restore()
	version.Current = s.origVersion
	s.LoggingSuite.TearDownTest(c)
}

func (s *syncSuite) TestCopyNewestFromFilesystem(c *gc.C) {
	ctx := &sync.SyncContext{
		EnvName: "test-target",
		Source:  s.localStorage,
		Stdout:  &bytes.Buffer{},
		Stderr:  &bytes.Buffer{},
	}
	err := sync.SyncTools(ctx)
	c.Assert(err, gc.IsNil)

	// Newest released v1 tools made available to target env.
	targetTools, err := environs.FindAvailableTools(s.targetEnv, 1)
	c.Assert(err, gc.IsNil)
	assertToolsList(c, targetTools, v100all)

	// Public bucket was not touched.
	assertEmpty(c, s.targetEnv.PublicStorage())
}

func (s *syncSuite) TestCopyNewestFromDummy(c *gc.C) {
	ctx := &sync.SyncContext{
		EnvName: "test-target",
		Stdout:  &bytes.Buffer{},
		Stderr:  &bytes.Buffer{},
	}
	err := sync.SyncTools(ctx)
	c.Assert(err, gc.IsNil)

	// Newest released v1 tools made available to target env.
	targetTools, err := environs.FindAvailableTools(s.targetEnv, 1)
	c.Assert(err, gc.IsNil)
	assertToolsList(c, targetTools, v100all)

	// Public bucket was not touched.
	assertEmpty(c, s.targetEnv.PublicStorage())
}

func (s *syncSuite) TestCopyNewestDevFromDummy(c *gc.C) {
	ctx := &sync.SyncContext{
		EnvName: "test-target",
		Dev:     true,
		Stdout:  &bytes.Buffer{},
		Stderr:  &bytes.Buffer{},
	}
	err := sync.SyncTools(ctx)
	c.Assert(err, gc.IsNil)

	// Newest v1 dev tools made available to target env.
	targetTools, err := environs.FindAvailableTools(s.targetEnv, 1)
	c.Assert(err, gc.IsNil)
	assertToolsList(c, targetTools, v190all)

	// Public bucket was not touched.
	assertEmpty(c, s.targetEnv.PublicStorage())
}

func (s *syncSuite) TestCopyAllFromDummy(c *gc.C) {
	ctx := &sync.SyncContext{
		EnvName:     "test-target",
		AllVersions: true,
		Stdout:      &bytes.Buffer{},
		Stderr:      &bytes.Buffer{},
	}
	err := sync.SyncTools(ctx)
	c.Assert(err, gc.IsNil)

	// All released v1 tools made available to target env.
	targetTools, err := environs.FindAvailableTools(s.targetEnv, 1)
	c.Assert(err, gc.IsNil)
	assertToolsList(c, targetTools, v100all)

	// Public bucket was not touched.
	assertEmpty(c, s.targetEnv.PublicStorage())
}

func (s *syncSuite) TestCopyAllDevFromDummy(c *gc.C) {
	ctx := &sync.SyncContext{
		EnvName:     "test-target",
		AllVersions: true,
		Dev:         true,
		Stdout:      &bytes.Buffer{},
		Stderr:      &bytes.Buffer{},
	}
	err := sync.SyncTools(ctx)
	c.Assert(err, gc.IsNil)

	// All v1 tools, dev and release, made available to target env.
	targetTools, err := environs.FindAvailableTools(s.targetEnv, 1)
	c.Assert(err, gc.IsNil)
	assertToolsList(c, targetTools, v1all)

	// Public bucket was not touched.
	assertEmpty(c, s.targetEnv.PublicStorage())
}

func (s *syncSuite) TestCopyToDummyPublic(c *gc.C) {
	ctx := &sync.SyncContext{
		EnvName:      "test-target",
		PublicBucket: true,
		Stdout:       &bytes.Buffer{},
		Stderr:       &bytes.Buffer{},
	}
	err := sync.SyncTools(ctx)
	c.Assert(err, gc.IsNil)

	// Newest released tools made available to target env.
	targetTools, err := environs.FindAvailableTools(s.targetEnv, 1)
	c.Assert(err, gc.IsNil)
	assertToolsList(c, targetTools, v100all)

	// Private bucket was not touched.
	assertEmpty(c, s.targetEnv.Storage())
}

func (s *syncSuite) TestCopyToDummyPublicBlockedByPrivate(c *gc.C) {
	envtesting.UploadFakeToolsVersion(c, s.targetEnv.Storage(), v200p64)
	ctx := &sync.SyncContext{
		EnvName:      "test-target",
		PublicBucket: true,
		Stdout:       &bytes.Buffer{},
		Stderr:       &bytes.Buffer{},
	}
	err := sync.SyncTools(ctx)
	c.Assert(err, gc.ErrorMatches, "private tools present: public tools would be ignored")
	assertEmpty(c, s.targetEnv.PublicStorage())
}

var (
	v100p64 = version.MustParseBinary("1.0.0-precise-amd64")
	v100q64 = version.MustParseBinary("1.0.0-quantal-amd64")
	v100q32 = version.MustParseBinary("1.0.0-quantal-i386")
	v100all = []version.Binary{v100p64, v100q64, v100q32}
	v190q64 = version.MustParseBinary("1.9.0-quantal-amd64")
	v190p32 = version.MustParseBinary("1.9.0-precise-i386")
	v190all = []version.Binary{v190q64, v190p32}
	v1all   = append(v100all, v190all...)
	v200p64 = version.MustParseBinary("2.0.0-precise-amd64")
	vAll    = append(v1all, v200p64)
)

// putBinary stores a faked binary in the test directory.
func putBinary(c *gc.C, storagePath string, v version.Binary) {
	data := v.String()
	name := tools.StorageName(v)
	filename := filepath.Join(storagePath, name)
	dir := filepath.Dir(filename)
	err := os.MkdirAll(dir, 0755)
	c.Assert(err, gc.IsNil)
	err = ioutil.WriteFile(filename, []byte(data), 0666)
	c.Assert(err, gc.IsNil)
}

func assertEmpty(c *gc.C, storage environs.StorageReader) {
	list, err := tools.ReadList(storage, 1)
	if len(list) > 0 {
		c.Logf("got unexpected tools: %s", list)
	}
	c.Assert(err, gc.Equals, tools.ErrNoTools)
}

func assertToolsList(c *gc.C, list tools.List, expected []version.Binary) {
	urls := list.URLs()
	c.Check(urls, gc.HasLen, len(expected))
	for _, vers := range expected {
		c.Assert(urls[vers], gc.Not(gc.Equals), "")
	}
}
