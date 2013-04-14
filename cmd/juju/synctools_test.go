package main

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/dummy"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/version"
	"sort"
)

type syncToolsSuite struct {
	testing.LoggingSuite
	home      *testing.FakeHome
	targetEnv environs.Environ
	origAttrs map[string]interface{}
}

func (s *syncToolsSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)

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

	// Create a source environment and populate its public tools.
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
	envtesting.RemoveAllTools(c, env)
	store := env.PublicStorage().(environs.Storage)
	envtesting.UploadFakeToolsVersion(c, store, t1000precise.Binary)
	envtesting.UploadFakeToolsVersion(c, store, t1000quantal.Binary)
	envtesting.UploadFakeToolsVersion(c, store, t1000quantal32.Binary)
	envtesting.UploadFakeToolsVersion(c, store, t1900quantal.Binary)
	envtesting.UploadFakeToolsVersion(c, store, t2000precise.Binary)

	// Overwrite the official source bucket to the new dummy 'test-source',
	// saving the original value for cleanup
	s.origAttrs = officialBucketAttrs
	officialBucketAttrs = dummyAttrs
}

func (s *syncToolsSuite) TearDownTest(c *C) {
	officialBucketAttrs = s.origAttrs
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

func assertEmpty(c *C, storage environs.StorageReader) {
	list, err := tools.ReadList(storage, 1)
	if len(list) > 0 {
		c.Logf("got unexpected tools: %s", list)
	}
	c.Assert(err, Equals, tools.ErrNoTools)
}

func (s *syncToolsSuite) TestCopyNewestFromDummy(c *C) {
	ctx, err := runSyncToolsCommand(c, "-e", "test-target")
	c.Assert(err, IsNil)
	c.Assert(ctx, NotNil)

	// Newest released v1 tools made available to target env.
	targetTools, err := environs.FindAvailableTools(s.targetEnv, 1)
	c.Assert(err, IsNil)
	assertToolsList(c, targetTools,
		"1.0.0-precise-amd64", "1.0.0-quantal-amd64", "1.0.0-quantal-i386")

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
	assertToolsList(c, targetTools, "1.9.0-quantal-amd64")

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
	assertToolsList(c, targetTools,
		"1.0.0-precise-amd64", "1.0.0-quantal-amd64", "1.0.0-quantal-i386")

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
	assertToolsList(c, targetTools,
		"1.0.0-precise-amd64", "1.0.0-quantal-amd64",
		"1.0.0-quantal-i386", "1.9.0-quantal-amd64")

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
	assertToolsList(c, targetTools,
		"1.0.0-precise-amd64", "1.0.0-quantal-amd64", "1.0.0-quantal-i386")

	// Private bucket was not touched.
	assertEmpty(c, s.targetEnv.Storage())
}

func (s *syncToolsSuite) TestCopyToDummyPublicBlockedByPrivate(c *C) {
	envtesting.UploadFakeToolsVersion(c, s.targetEnv.Storage(), t2000precise.Binary)

	_, err := runSyncToolsCommand(c, "-e", "test-target", "--public")
	c.Assert(err, ErrorMatches, "private tools present: public tools would be ignored")
	assertEmpty(c, s.targetEnv.PublicStorage())
}

func mustParseTools(vers string) *state.Tools {
	return &state.Tools{Binary: version.MustParseBinary(vers)}
}

var (
	t1000precise   = mustParseTools("1.0.0-precise-amd64")
	t1000quantal   = mustParseTools("1.0.0-quantal-amd64")
	t1000quantal32 = mustParseTools("1.0.0-quantal-i386")
	t1900quantal   = mustParseTools("1.9.0-quantal-amd64")
	t2000precise   = mustParseTools("2.0.0-precise-amd64")
)
