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
		Number: version.Number{1, 0, 0, 0},
		Series: "precise",
		Arch:   ""},
	URL: "",
}

var t1000quantal = &state.Tools{
	Binary: version.Binary{
		Number: version.Number{1, 0, 0, 0},
		Series: "quantal",
		Arch:   ""},
	URL: "",
}

var t1900quantal = &state.Tools{
	Binary: version.Binary{
		Number: version.Number{1, 9, 0, 0},
		Series: "quantal",
		Arch:   ""},
	URL: "",
}

var t2000precise = &state.Tools{
	Binary: version.Binary{
		Number: version.Number{2, 0, 0, 0},
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
	}{
		{all: []*state.Tools{t1000precise, t1000quantal},
			best: []*state.Tools{t1000precise, t1000quantal}},
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
