package main

import (
	"bytes"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/version"
	"os"
	"sort"
)

type syncToolsSuite struct {
	testing.LoggingSuite
	home *testing.FakeHome
}

func (s *syncToolsSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)
	s.home = testing.MakeEmptyFakeHome(c)
}

func (s *syncToolsSuite) TearDownTest(c *C) {
	dummy.Reset()
	s.home.Restore()
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

func uploadDummyTools(c *C, vers version.Binary, store environs.Storage) {
	path := environs.ToolsStoragePath(vers)
	content := bytes.NewBufferString("content\n")
	err := store.Put(path, content, int64(content.Len()))
	c.Assert(err, IsNil)
}

func setupDummyEnvironments(c *C) (env environs.Environ, cleanup func()) {
	dummyAttrs := map[string]interface{}{
		"name":         "test-source",
		"type":         "dummy",
		"state-server": false,
		// Note: Without this, you get "no public ssh keys found", which seems
		// a bit odd for the "dummy" environment
		"authorized-keys": "I-am-not-a-real-key",
	}
	env, err := environs.NewFromAttrs(dummyAttrs)
	c.Assert(err, IsNil)
	c.Assert(env, NotNil)
	store := env.PublicStorage().(environs.Storage)
	// Dummy environments always put fake tools, but we don't want it
	// confusing our state, so we delete them
	fakepath := environs.ToolsStoragePath(version.Current)
	err = store.Remove(fakepath)
	c.Assert(err, IsNil)
	// Upload multiple tools
	uploadDummyTools(c, t1000precise.Binary, store)
	uploadDummyTools(c, t1000quantal.Binary, store)
	uploadDummyTools(c, t1000quantal32.Binary, store)
	uploadDummyTools(c, t1900quantal.Binary, store)
	// Overwrite the official source bucket to the new dummy 'test-source',
	// saving the original value for cleanup
	orig := officialBucketAttrs
	officialBucketAttrs = dummyAttrs
	// Create a target dummy environment
	c.Assert(os.Mkdir(testing.HomePath(".juju"), 0775), IsNil)
	jujupath := testing.HomePath(".juju", "environments.yaml")
	err = ioutil.WriteFile(
		jujupath,
		[]byte(`
environments:
    test-target:
        type: dummy
        state-server: false
        authorized-keys: "not-really-one"
`),
		0660)
	c.Assert(err, IsNil)
	return env, func() { officialBucketAttrs = orig }
}

func assertToolsList(c *C, toolsList []*state.Tools, expected ...string) {
	sort.Strings(expected)
	actual := make([]string, len(toolsList))
	for i, tool := range toolsList {
		actual[i] = tool.Binary.String()
	}
	sort.Strings(actual)
	// In gocheck, the empty slice does not equal the nil slice, though it
	// does for our purposes
	if expected == nil {
		expected = []string{}
	}
	c.Assert(actual, DeepEquals, expected)
}

func (s *syncToolsSuite) TestCopyNewestFromDummy(c *C) {
	sourceEnv, cleanup := setupDummyEnvironments(c)
	defer cleanup()
	sourceTools, err := environs.ListTools(sourceEnv, 1)
	c.Assert(err, IsNil)
	assertToolsList(c, sourceTools.Public,
		"1.0.0-precise-amd64", "1.0.0-quantal-amd64",
		"1.0.0-quantal-i386", "1.9.0-quantal-amd64")
	c.Assert(sourceTools.Private, HasLen, 0)
	targetEnv, err := environs.NewFromName("test-target")
	c.Assert(err, IsNil)
	targetTools, err := environs.ListTools(targetEnv, 1)
	// Target env just has the fake tools in the public bucket
	assertToolsList(c, targetTools.Public, version.Current.String())
	// Nothing in private
	assertToolsList(c, targetTools.Private)
	c.Assert(targetTools.Private, HasLen, 0)
	ctx, err := runSyncToolsCommand(c, "-e", "test-target")
	c.Assert(err, IsNil)
	c.Assert(ctx, NotNil)
	targetTools, err = environs.ListTools(targetEnv, 1)
	c.Assert(err, IsNil)
	// No change to the Public bucket
	assertToolsList(c, targetTools.Public, version.Current.String())
	// only the newest added to the private bucket
	assertToolsList(c, targetTools.Private, "1.9.0-quantal-amd64")
}

func (s *syncToolsSuite) TestCopyAllFromDummy(c *C) {
	sourceEnv, cleanup := setupDummyEnvironments(c)
	defer cleanup()
	sourceTools, err := environs.ListTools(sourceEnv, 1)
	c.Assert(err, IsNil)
	assertToolsList(c, sourceTools.Public,
		"1.0.0-precise-amd64", "1.0.0-quantal-amd64",
		"1.0.0-quantal-i386", "1.9.0-quantal-amd64")
	c.Assert(sourceTools.Private, HasLen, 0)
	targetEnv, err := environs.NewFromName("test-target")
	c.Assert(err, IsNil)
	targetTools, err := environs.ListTools(targetEnv, 1)
	// Target env just has the fake tools in the public bucket
	assertToolsList(c, targetTools.Public, version.Current.String())
	// Nothing in private
	assertToolsList(c, targetTools.Private)
	c.Assert(targetTools.Private, HasLen, 0)
	ctx, err := runSyncToolsCommand(c, "-e", "test-target", "--all")
	c.Assert(err, IsNil)
	c.Assert(ctx, NotNil)
	targetTools, err = environs.ListTools(targetEnv, 1)
	c.Assert(err, IsNil)
	// No change to the Public bucket
	assertToolsList(c, targetTools.Public, version.Current.String())
	// all tools added to the private bucket
	assertToolsList(c, targetTools.Private,
		"1.0.0-precise-amd64", "1.0.0-quantal-amd64",
		"1.0.0-quantal-i386", "1.9.0-quantal-amd64")
}

func (s *syncToolsSuite) TestCopyToDummyPublic(c *C) {
	sourceEnv, cleanup := setupDummyEnvironments(c)
	defer cleanup()
	sourceTools, err := environs.ListTools(sourceEnv, 1)
	c.Assert(err, IsNil)
	assertToolsList(c, sourceTools.Public,
		"1.0.0-precise-amd64", "1.0.0-quantal-amd64",
		"1.0.0-quantal-i386", "1.9.0-quantal-amd64")
	c.Assert(sourceTools.Private, HasLen, 0)
	targetEnv, err := environs.NewFromName("test-target")
	c.Assert(err, IsNil)
	targetTools, err := environs.ListTools(targetEnv, 1)
	// Target env just has the fake tools in the public bucket
	assertToolsList(c, targetTools.Public, version.Current.String())
	// Nothing in private
	assertToolsList(c, targetTools.Private)
	c.Assert(targetTools.Private, HasLen, 0)
	ctx, err := runSyncToolsCommand(c, "-e", "test-target", "--public")
	c.Assert(err, IsNil)
	c.Assert(ctx, NotNil)
	targetTools, err = environs.ListTools(targetEnv, 1)
	c.Assert(err, IsNil)
	// newest tools added to the private bucket
	assertToolsList(c, targetTools.Public,
		version.Current.String(), "1.9.0-quantal-amd64")
	assertToolsList(c, targetTools.Private)
}

type toolSuite struct{}

var _ = Suite(&toolSuite{})

var t1000precise = &state.Tools{
	Binary: version.Binary{
		Number: version.Number{1, 0, 0, 0},
		Series: "precise",
		Arch:   "amd64"}}

var t1000quantal32 = &state.Tools{
	Binary: version.Binary{
		Number: version.Number{1, 0, 0, 0},
		Series: "quantal",
		Arch:   "i386"}}

var t1000quantal = &state.Tools{
	Binary: version.Binary{
		Number: version.Number{1, 0, 0, 0},
		Series: "quantal",
		Arch:   "amd64"}}

var t1900quantal = &state.Tools{
	Binary: version.Binary{
		Number: version.Number{1, 9, 0, 0},
		Series: "quantal",
		Arch:   "amd64"}}

var t2000precise = &state.Tools{
	Binary: version.Binary{
		Number: version.Number{2, 0, 0, 0},
		Series: "precise"}}

func (s *toolSuite) TestFindNewestOneTool(c *C) {
	var onlyOneTests = []struct {
		tools *state.Tools
	}{
		{tools: t1000precise},
		{tools: t1000quantal},
		{tools: t1900quantal},
		{tools: t2000precise},
	}
	for _, t := range onlyOneTests {
		toolList := []*state.Tools{t.tools}
		res := findNewest(toolList)
		c.Assert(res, HasLen, 1)
		c.Assert(res[0], Equals, t.tools)
	}
}

func (s *toolSuite) TestFindNewestOnlyOneBest(c *C) {
	var oneBestTests = []struct {
		all  []*state.Tools
		best *state.Tools
	}{
		{
			all:  []*state.Tools{t1000precise, t1900quantal},
			best: t1900quantal,
		},
	}
	for _, t := range oneBestTests {
		res := findNewest(t.all)
		c.Assert(res, HasLen, 1)
		c.Assert(res[0], Equals, t.best)
	}
}

func (s *toolSuite) TestFindNewestMultipleBest(c *C) {
	var oneBestTests = []struct {
		all  []*state.Tools
		best []*state.Tools
	}{{
		all:  []*state.Tools{t1000precise, t1000quantal},
		best: []*state.Tools{t1000precise, t1000quantal},
	},
	}
	for _, t := range oneBestTests {
		res := findNewest(t.all)
		c.Assert(res, DeepEquals, t.best)
	}
}

func (s *toolSuite) TestFindMissingNoTarget(c *C) {
	var allMissingTests = []struct {
		source []*state.Tools
	}{
		{source: []*state.Tools{t1000precise}},
		{source: []*state.Tools{t1000precise, t1000quantal}},
	}
	for _, t := range allMissingTests {
		res := findMissing(t.source, []*state.Tools(nil))
		c.Assert(res, DeepEquals, t.source)
	}
}

func (s *toolSuite) TestFindMissingSameEntries(c *C) {
	var allMissingTests = []struct {
		source []*state.Tools
	}{
		{source: []*state.Tools{t1000precise}},
		{source: []*state.Tools{t1000precise, t1000quantal}},
	}
	for _, t := range allMissingTests {
		res := findMissing(t.source, t.source)
		c.Assert(res, HasLen, 0)
	}
}

func (s *toolSuite) TestFindHasVersionNotSeries(c *C) {
	res := findMissing(
		[]*state.Tools{t1000precise, t1000quantal},
		[]*state.Tools{t1000quantal})
	c.Assert(res, HasLen, 1)
	c.Assert(res[0], Equals, t1000precise)
	res = findMissing(
		[]*state.Tools{t1000precise, t1000quantal},
		[]*state.Tools{t1000precise})
	c.Assert(res, HasLen, 1)
	c.Assert(res[0], Equals, t1000quantal)
}
