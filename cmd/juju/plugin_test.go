package main

import (
	"os"

	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/testing"
)

type PluginSuite struct {
	oldPath string
	home    *testing.FakeHome
}

var _ = Suite(&PluginSuite{})

func (suite *PluginSuite) SetUpTest(c *C) {
	suite.oldPath = os.Getenv("PATH")
	suite.home = testing.MakeEmptyFakeHome(c)
	os.Setenv("PATH", testing.HomePath())
}

func (suite *PluginSuite) TearDownTest(c *C) {
	suite.home.Restore()
	os.Setenv("PATH", suite.oldPath)
}

func (*PluginSuite) TestFindPlugins(c *C) {
	plugins := findPlugins()
	c.Assert(plugins, DeepEquals, []string{})
}
