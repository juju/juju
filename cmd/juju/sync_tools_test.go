package main

import (
	//"crypto/sha256"
	//"encoding/hex"
	//"io/ioutil"
	. "launchpad.net/gocheck"
	//"launchpad.net/juju-core/charm"
	//"launchpad.net/juju-core/juju/testing"
	//"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
	//"net/http"
	//"os"
	//"path/filepath"
	//"sort"
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
