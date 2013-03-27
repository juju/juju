package main

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/version"
)

type syncToolsSuite struct {
	coretesting.LoggingSuite
	home coretesting.FakeHome
}

func (s *syncToolsSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)
	s.home = coretesting.MakeEmptyFakeHome(c)
}

func (s *syncToolsSuite) TearDownTest(c *C) {
	s.home.Restore()
	s.LoggingSuite.TearDownTest(c)
}

var _ = Suite(&syncToolsSuite{})

func (s *syncToolsSuite) TestHelp(c *C) {
	opc, errc := runCommand(new(SyncToolsCommand), "-h")
	c.Check(<-errc, ErrorMatches, "flag: help requested")
	c.Check(<-opc, IsNil)
}

type toolSuite struct{}

var _ = Suite(&toolSuite{})

var t1000precise = &state.Tools{
	Binary: version.Binary{
		Number: version.Number{Major: 1, Minor: 0, Patch: 0, Build: 0},
		Series: "precise",
		Arch:   ""},
	URL: "",
}

var t1000quantal = &state.Tools{
	Binary: version.Binary{
		Number: version.Number{Major: 1, Minor: 0, Patch: 0, Build: 0},
		Series: "quantal",
		Arch:   ""},
	URL: "",
}

var t1900quantal = &state.Tools{
	Binary: version.Binary{
		Number: version.Number{Major: 1, Minor: 9, Patch: 0, Build: 0},
		Series: "quantal",
		Arch:   ""},
	URL: "",
}

var t2000precise = &state.Tools{
	Binary: version.Binary{
		Number: version.Number{Major: 2, Minor: 0, Patch: 0, Build: 0},
		Series: "precise",
		Arch:   ""},
	URL: "",
}

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
		{all: []*state.Tools{t1000precise, t1900quantal},
			best: t1900quantal},
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
	}{
		{all: []*state.Tools{t1000precise, t1000quantal},
			best: []*state.Tools{t1000precise, t1000quantal}},
	}
	for _, t := range oneBestTests {
		res := findNewest(t.all)
		c.Assert(res, DeepEquals, t.best)
	}
}
