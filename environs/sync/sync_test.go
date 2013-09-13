// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sync_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/sync"
	envtesting "launchpad.net/juju-core/environs/testing"
	envtools "launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/provider/dummy"
	coretesting "launchpad.net/juju-core/testing"
	coretools "launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/version"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type syncSuite struct {
	coretesting.LoggingSuite
	envtesting.ToolsFixture
	home         *coretesting.FakeHome
	targetEnv    environs.Environ
	origVersion  version.Binary
	origLocation string
	storage      *envtesting.EC2HTTPTestStorage
	localStorage string
}

var _ = gc.Suite(&syncSuite{})

func (s *syncSuite) setUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)
	s.origVersion = version.Current
	// It's important that this be v1.8.x to match the test data.
	version.Current.Number = version.MustParse("1.8.3")

	// We only want to use simplestreams to find any synced tools.
	envtools.UseLegacyFallback = false

	// Create a target environments.yaml.
	s.home = coretesting.MakeFakeHome(c, `
environments:
    test-target:
        type: dummy
        state-server: false
        authorized-keys: "not-really-one"
`)
	var err error
	s.targetEnv, err = environs.PrepareFromName("test-target")
	c.Assert(err, gc.IsNil)
	envtesting.RemoveAllTools(c, s.targetEnv)

	// Create a source storage.
	s.storage, err = envtesting.NewEC2HTTPTestStorage("127.0.0.1")
	c.Assert(err, gc.IsNil)

	// Create a local tools directory.
	s.localStorage = c.MkDir()

	envtools.SetToolPrefix(envtools.NewToolPrefix)
	// Populate both with the public tools.
	for _, vers := range vAll {
		s.storage.PutBinary(vers)
		putBinary(c, s.localStorage, vers)
	}

	// Switch tools location.
	s.origLocation = sync.DefaultToolsLocation
	sync.DefaultToolsLocation = s.storage.Location()
}

func (s *syncSuite) tearDownTest(c *gc.C) {
	c.Assert(s.storage.Stop(), gc.IsNil)
	envtools.UseLegacyFallback = true
	envtools.SetToolPrefix(envtools.DefaultToolPrefix)
	sync.DefaultToolsLocation = s.origLocation
	dummy.Reset()
	s.home.Restore()
	version.Current = s.origVersion
	s.ToolsFixture.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
}

var tests = []struct {
	description string
	ctx         *sync.SyncContext
	source      bool
	tools       []version.Binary
	version     version.Number
	major       int
	minor       int
}{
	{
		description: "copy newest from the filesystem",
		ctx:         &sync.SyncContext{},
		source:      true,
		tools:       v180all,
	},
	{
		description: "copy newest from the dummy environment",
		ctx:         &sync.SyncContext{},
		tools:       v180all,
	},
	{
		description: "copy matching dev from the dummy environment",
		ctx:         &sync.SyncContext{},
		version:     version.MustParse("1.9.3"),
		tools:       v190all,
	},
	{
		description: "copy matching major, minor from the dummy environment",
		ctx:         &sync.SyncContext{},
		major:       3,
		minor:       2,
		tools:       []version.Binary{v320p64},
	},
	{
		description: "copy matching major, minor dev from the dummy environment",
		ctx:         &sync.SyncContext{},
		major:       3,
		minor:       1,
		tools:       []version.Binary{v310p64},
	},
	{
		description: "copy all from the dummy environment",
		ctx: &sync.SyncContext{
			AllVersions: true,
		},
		tools: v1noDev,
	},
	{
		description: "copy all and dev from the dummy environment",
		ctx: &sync.SyncContext{
			AllVersions: true,
			Dev:         true,
		},
		tools: v1all,
	},
}

func (s *syncSuite) TestSyncing(c *gc.C) {
	for i, test := range tests {
		// Perform all tests in a "clean" environment.
		func() {
			s.setUpTest(c)
			defer s.tearDownTest(c)

			c.Logf("test %d: %s", i, test.description)

			if test.source {
				test.ctx.Source = s.localStorage
			}
			if test.version != version.Zero {
				version.Current.Number = test.version
			}
			if test.major > 0 {
				test.ctx.MajorVersion = test.major
				test.ctx.MinorVersion = test.minor
			}
			test.ctx.Target = s.targetEnv.Storage()

			err := sync.SyncTools(test.ctx)
			c.Assert(err, gc.IsNil)

			targetTools, err := envtools.FindTools(s.targetEnv, test.ctx.MajorVersion, test.ctx.MinorVersion, coretools.Filter{})
			c.Assert(err, gc.IsNil)
			assertToolsList(c, targetTools, test.tools)

			assertEmpty(c, s.targetEnv.Storage())
		}()
	}
}

var (
	v100p64 = version.MustParseBinary("1.0.0-precise-amd64")
	v100q64 = version.MustParseBinary("1.0.0-quantal-amd64")
	v100q32 = version.MustParseBinary("1.0.0-quantal-i386")
	v100all = []version.Binary{v100p64, v100q64, v100q32}
	v180q64 = version.MustParseBinary("1.8.0-quantal-amd64")
	v180p32 = version.MustParseBinary("1.8.0-precise-i386")
	v180all = []version.Binary{v180q64, v180p32}
	v190q64 = version.MustParseBinary("1.9.0-quantal-amd64")
	v190p32 = version.MustParseBinary("1.9.0-precise-i386")
	v190all = []version.Binary{v190q64, v190p32}
	v1noDev = append(v100all, v180all...)
	v1all   = append(v1noDev, v190all...)
	v200p64 = version.MustParseBinary("2.0.0-precise-amd64")
	v310p64 = version.MustParseBinary("3.1.0-precise-amd64")
	v320p64 = version.MustParseBinary("3.2.0-precise-amd64")
	vAll    = append(append(v1all, v200p64), v310p64, v320p64)
)

// putBinary stores a faked binary in the test directory.
func putBinary(c *gc.C, storagePath string, v version.Binary) {
	data := v.String()
	name := envtools.StorageName(v)
	filename := filepath.Join(storagePath, name)
	dir := filepath.Dir(filename)
	err := os.MkdirAll(dir, 0755)
	c.Assert(err, gc.IsNil)
	err = ioutil.WriteFile(filename, []byte(data), 0666)
	c.Assert(err, gc.IsNil)
}

func assertEmpty(c *gc.C, storage environs.StorageReader) {
	list, err := envtools.ReadList(storage, 2, 0)
	if len(list) > 0 {
		c.Logf("got unexpected tools: %s", list)
	}
	c.Assert(err, gc.Equals, coretools.ErrNoMatches)
}

func assertToolsList(c *gc.C, list coretools.List, expected []version.Binary) {
	urls := list.URLs()
	c.Check(urls, gc.HasLen, len(expected))
	for _, vers := range expected {
		c.Assert(urls[vers], gc.Not(gc.Equals), "")
	}
}
