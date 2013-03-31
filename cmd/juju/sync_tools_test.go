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
)

type syncToolsSuite struct {
	testing.LoggingSuite
	home testing.FakeHome
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

func (s *syncToolsSuite) TestReadFromDummy(c *C) {
	dummyAttrs := map[string]interface{}{
		"name":         "test-dummy",
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
	path := environs.ToolsStoragePath(t1000precise.Binary)
	content := bytes.NewBufferString("content\n")
	err = store.Put(path, content, int64(content.Len()))
	c.Assert(err, IsNil)
	orig := officialBucketAttrs
	defer func() { officialBucketAttrs = orig }()
	officialBucketAttrs = dummyAttrs
	c.Assert(os.Mkdir(testing.HomePath(".juju"), 0775), IsNil)
	jujupath := testing.HomePath(".juju", "environments.yaml")
	err = ioutil.WriteFile(
		jujupath,
		[]byte(`
environments:
    target:
        type: dummy
        state-server: false
        authorized-keys: "not-really-one"
`),
		0660)
	c.Assert(err, IsNil)
	tools, err := environs.ListTools(env, t1000precise.Binary.Major)
	c.Assert(err, IsNil)
	// The one we just uploaded
	c.Assert(tools.Public, HasLen, 2)
	if tools.Public[0].Binary == t1000precise.Binary {
		c.Assert(tools.Public[0].Binary, Equals, t1000precise.Binary)
		c.Assert(tools.Public[1].Binary, Equals, version.Current)
	} else {
		c.Assert(tools.Public[0].Binary, Equals, version.Current)
		c.Assert(tools.Public[1].Binary, Equals, t1000precise.Binary)
	}
	c.Assert(tools.Private, HasLen, 0)
	ctx, err := runSyncToolsCommand(c, "-e", "target")
	c.Assert(err, IsNil)
	c.Assert(ctx, NotNil)
	tools, err = environs.ListTools(env, t1000precise.Binary.Major)
	c.Assert(err, IsNil)
	c.Assert(tools.Private, HasLen, 2)
	c.Assert(tools.Private[0].Binary, Equals, t1000precise.Binary)
	if tools.Private[0].Binary == t1000precise.Binary {
		c.Assert(tools.Private[0].Binary, Equals, t1000precise.Binary)
		c.Assert(tools.Private[1].Binary, Equals, version.Current)
	} else {
		c.Assert(tools.Private[0].Binary, Equals, version.Current)
		c.Assert(tools.Private[1].Binary, Equals, t1000precise.Binary)
	}
}

type toolSuite struct{}

var _ = Suite(&toolSuite{})

var t1000precise = &state.Tools{
	Binary: version.Binary{
		Number: version.Number{1, 0, 0, 0},
		Series: "precise",
		Arch:   "amd64"}}

var t1000quantal = &state.Tools{
	Binary: version.Binary{
		Number: version.Number{1, 0, 0, 0},
		Series: "quantal"}}

var t1900quantal = &state.Tools{
	Binary: version.Binary{
		Number: version.Number{1, 9, 0, 0},
		Series: "quantal"}}

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
