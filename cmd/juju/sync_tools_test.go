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

func deletePublicTools(c *C, store environs.Storage) {
	// Dummy environments always put fake tools, but we don't want it
	// confusing our state, so we delete them
	dummyTools, err := store.List("tools/juju")
	c.Assert(err, IsNil)
	for _, path := range dummyTools {
		err = store.Remove(path)
		c.Assert(err, IsNil)
	}
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
	deletePublicTools(c, store)
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

func setupTargetEnv(c *C) environs.Environ {
	targetEnv, err := environs.NewFromName("test-target")
	c.Assert(err, IsNil)
	store := targetEnv.PublicStorage().(environs.Storage)
	deletePublicTools(c, store)
	targetTools, err := environs.ListTools(targetEnv, 1)
	// Target has no tools.
	c.Assert(targetTools.Public, HasLen, 0)
	c.Assert(targetTools.Private, HasLen, 0)
	return targetEnv
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

	targetEnv := setupTargetEnv(c)

	ctx, err := runSyncToolsCommand(c, "-e", "test-target")
	c.Assert(err, IsNil)
	c.Assert(ctx, NotNil)
	targetTools, err := environs.ListTools(targetEnv, 1)
	c.Assert(err, IsNil)
	// No change to the Public bucket
	c.Assert(targetTools.Public, HasLen, 0)
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

	targetEnv := setupTargetEnv(c)

	ctx, err := runSyncToolsCommand(c, "-e", "test-target", "--all")
	c.Assert(err, IsNil)
	c.Assert(ctx, NotNil)
	targetTools, err := environs.ListTools(targetEnv, 1)
	c.Assert(err, IsNil)
	// No change to the Public bucket
	c.Assert(targetTools.Public, HasLen, 0)
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

	targetEnv := setupTargetEnv(c)

	ctx, err := runSyncToolsCommand(c, "-e", "test-target", "--public")
	c.Assert(err, IsNil)
	c.Assert(ctx, NotNil)
	targetTools, err := environs.ListTools(targetEnv, 1)
	c.Assert(err, IsNil)
	// newest tools added to the private bucket
	assertToolsList(c, targetTools.Public, "1.9.0-quantal-amd64")
	c.Assert(targetTools.Private, HasLen, 0)
}

type toolSuite struct{}

var _ = Suite(&toolSuite{})

func mustParseTools(major, minor, patch, build int, series string, arch string) *state.Tools {
	return &state.Tools{
		Binary: version.Binary{
			Number: version.Number{major, minor, patch, build},
			Series: series,
			Arch:   arch}}
}

var (
	t1000precise   = mustParseTools(1, 0, 0, 0, "precise", "amd64")
	t1000quantal   = mustParseTools(1, 0, 0, 0, "quantal", "amd64")
	t1000quantal32 = mustParseTools(1, 0, 0, 0, "quantal", "i386")
	t1900quantal   = mustParseTools(1, 9, 0, 0, "quantal", "amd64")
	t2000precise   = mustParseTools(2, 0, 0, 0, "precise", "amd64")
)

func (s *toolSuite) TestFindNewestOneTool(c *C) {
	for i, t := range []*state.Tools{
		t1000precise,
		t1000quantal,
		t1900quantal,
		t2000precise,
	} {
		c.Log("test: %d %s", i, t.Binary.String())
		toolList := []*state.Tools{t}
		res := findNewest(toolList)
		c.Assert(res, HasLen, 1)
		c.Assert(res[0], Equals, t)
	}
}

func (s *toolSuite) TestFindNewestOnlyOneBest(c *C) {
	res := findNewest([]*state.Tools{t1000precise, t1900quantal})
	c.Assert(res, HasLen, 1)
	c.Assert(res[0], Equals, t1900quantal)
}

func (s *toolSuite) TestFindNewestMultipleBest(c *C) {
	source := []*state.Tools{t1000precise, t1000quantal}
	res := findNewest(source)
	c.Assert(res, HasLen, 2)
	// Order isn't strictly specified, but findNewest currently returns the
	// order in source, so it makes the test easier to write
	c.Assert(res, DeepEquals, source)
}

func (s *toolSuite) TestFindMissingNoTarget(c *C) {
	for i, t := range [][]*state.Tools{
		[]*state.Tools{t1000precise},
		[]*state.Tools{t1000precise, t1000quantal},
	} {
		c.Log("test: %d", i)
		res := findMissing(t, []*state.Tools(nil))
		c.Assert(res, DeepEquals, t)
	}
}

func (s *toolSuite) TestFindMissingSameEntries(c *C) {
	for i, t := range [][]*state.Tools{
		[]*state.Tools{t1000precise},
		[]*state.Tools{t1000precise, t1000quantal},
	} {
		c.Log("test: %d", i)
		res := findMissing(t, t)
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

func (s *toolSuite) TestFindHasDifferentArch(c *C) {
	res := findMissing(
		[]*state.Tools{t1000quantal, t1000quantal32},
		[]*state.Tools{t1000quantal})
	c.Assert(res, HasLen, 1)
	c.Assert(res[0], Equals, t1000quantal32)
	res = findMissing(
		[]*state.Tools{t1000quantal, t1000quantal32},
		[]*state.Tools{t1000quantal32})
	c.Assert(res, HasLen, 1)
	c.Assert(res[0], Equals, t1000quantal)
}
