// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"os"
	"path/filepath"

	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/dummy"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/version"
)

type syncToolsSuite struct {
	testing.LoggingSuite
	home         *testing.FakeHome
	targetEnv    environs.Environ
	origVersion  version.Binary
	origLocation string
	storage      *envtesting.EC2HTTPTestStorage
	localStorage string
}

func (s *syncToolsSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)
	s.origVersion = version.Current
	// It's important that this be v1 to match the test data.
	version.Current.Number = version.MustParse("1.2.3")

	// Create a target environments.yaml and make sure its environment is empty.
	s.home = testing.MakeFakeHome(c, `
environments:
    test-target:
        type: dummy
        state-server: false
        authorized-keys: "not-really-one"
`)
	var err error
	s.targetEnv, err = environs.NewFromName("test-target")
	c.Assert(err, IsNil)
	envtesting.RemoveAllTools(c, s.targetEnv)

	// Create a source storage.
	s.storage, err = envtesting.NewEC2HTTPTestStorage("127.0.0.1")
	c.Assert(err, IsNil)

	// Create a local tool directory.
	s.localStorage = c.MkDir()

	// Populate both with the public tools.
	for _, vers := range vAll {
		s.storage.PutBinary(vers)
		putBinary(c, s.localStorage, vers)
	}

	s.origLocation = defaultToolsLocation
	defaultToolsLocation = s.storage.Location()
}

func (s *syncToolsSuite) TearDownTest(c *C) {
	c.Assert(s.storage.Stop(), IsNil)
	defaultToolsLocation = s.origLocation
	dummy.Reset()
	s.home.Restore()
	version.Current = s.origVersion
	s.LoggingSuite.TearDownTest(c)
}

var _ = Suite(&syncToolsSuite{})

func runSyncToolsCommand(c *C, args ...string) (*cmd.Context, error) {
	return testing.RunCommand(c, &SyncToolsCommand{}, args)
}

func (s *syncToolsSuite) TestHelp(c *C) {
	ctx, err := runSyncToolsCommand(c, "-h")
	c.Assert(err, ErrorMatches, "flag: help requested")
	c.Assert(ctx, IsNil)
}

func assertToolsList(c *C, list tools.List, expected []version.Binary) {
	urls := list.URLs()
	c.Check(urls, HasLen, len(expected))
	for _, vers := range expected {
		c.Assert(urls[vers], Not(Equals), "")
	}
}

func assertEmpty(c *C, storage environs.StorageReader) {
	list, err := tools.ReadList(storage, 1)
	if len(list) > 0 {
		c.Logf("got unexpected tools: %s", list)
	}
	c.Assert(err, Equals, tools.ErrNoTools)
}

func (s *syncToolsSuite) TestCopyNewestFromFilesystem(c *C) {
	ctx, err := runSyncToolsCommand(c, "-e", "test-target", "--source", s.localStorage)
	c.Assert(err, IsNil)
	c.Assert(ctx, NotNil)

	// Newest released v1 tools made available to target env.
	targetTools, err := environs.FindAvailableTools(s.targetEnv, 1)
	c.Assert(err, IsNil)
	assertToolsList(c, targetTools, v100all)

	// Public bucket was not touched.
	assertEmpty(c, s.targetEnv.PublicStorage())
}

func (s *syncToolsSuite) TestCopyNewestFromDummy(c *C) {
	ctx, err := runSyncToolsCommand(c, "-e", "test-target")
	c.Assert(err, IsNil)
	c.Assert(ctx, NotNil)

	// Newest released v1 tools made available to target env.
	targetTools, err := environs.FindAvailableTools(s.targetEnv, 1)
	c.Assert(err, IsNil)
	assertToolsList(c, targetTools, v100all)

	// Public bucket was not touched.
	assertEmpty(c, s.targetEnv.PublicStorage())
}

func (s *syncToolsSuite) TestCopyNewestDevFromDummy(c *C) {
	ctx, err := runSyncToolsCommand(c, "-e", "test-target", "--dev")
	c.Assert(err, IsNil)
	c.Assert(ctx, NotNil)

	// Newest v1 dev tools made available to target env.
	targetTools, err := environs.FindAvailableTools(s.targetEnv, 1)
	c.Assert(err, IsNil)
	assertToolsList(c, targetTools, v190all)

	// Public bucket was not touched.
	assertEmpty(c, s.targetEnv.PublicStorage())
}

func (s *syncToolsSuite) TestCopyAllFromDummy(c *C) {
	ctx, err := runSyncToolsCommand(c, "-e", "test-target", "--all")
	c.Assert(err, IsNil)
	c.Assert(ctx, NotNil)

	// All released v1 tools made available to target env.
	targetTools, err := environs.FindAvailableTools(s.targetEnv, 1)
	c.Assert(err, IsNil)
	assertToolsList(c, targetTools, v100all)

	// Public bucket was not touched.
	assertEmpty(c, s.targetEnv.PublicStorage())
}

func (s *syncToolsSuite) TestCopyAllDevFromDummy(c *C) {
	ctx, err := runSyncToolsCommand(c, "-e", "test-target", "--all", "--dev")
	c.Assert(err, IsNil)
	c.Assert(ctx, NotNil)

	// All v1 tools, dev and release, made available to target env.
	targetTools, err := environs.FindAvailableTools(s.targetEnv, 1)
	c.Assert(err, IsNil)
	assertToolsList(c, targetTools, v1all)

	// Public bucket was not touched.
	assertEmpty(c, s.targetEnv.PublicStorage())
}

func (s *syncToolsSuite) TestCopyToDummyPublic(c *C) {
	ctx, err := runSyncToolsCommand(c, "-e", "test-target", "--public")
	c.Assert(err, IsNil)
	c.Assert(ctx, NotNil)

	// Newest released tools made available to target env.
	targetTools, err := environs.FindAvailableTools(s.targetEnv, 1)
	c.Assert(err, IsNil)
	assertToolsList(c, targetTools, v100all)

	// Private bucket was not touched.
	assertEmpty(c, s.targetEnv.Storage())
}

func (s *syncToolsSuite) TestCopyToDummyPublicBlockedByPrivate(c *C) {
	envtesting.UploadFakeToolsVersion(c, s.targetEnv.Storage(), v200p64)

	_, err := runSyncToolsCommand(c, "-e", "test-target", "--public")
	c.Assert(err, ErrorMatches, "private tools present: public tools would be ignored")
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
func putBinary(c *C, storagePath string, v version.Binary) {
	data := v.String()
	name := tools.StorageName(v)
	path := filepath.Join(storagePath, name)
	dir, _ := filepath.Split(path)
	err := os.MkdirAll(dir, 0755)
	c.Assert(err, IsNil)
	file, err := os.Create(path)
	c.Assert(err, IsNil)
	defer file.Close()
	_, err = file.Write([]byte(data))
	c.Assert(err, IsNil)
}
