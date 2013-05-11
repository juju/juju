package main

import (
	"fmt"
	"io/ioutil"
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
	suite.home = testing.MakeSampleHome(c)
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

func (suite *PluginSuite) TestFindPluginsOrder(c *C) {
	suite.makePlugin("foo", 0744)
	suite.makePlugin("bar", 0654)
	suite.makePlugin("baz", 0645)
	plugins := findPlugins()
	c.Assert(plugins, DeepEquals, []string{"juju-bar", "juju-baz", "juju-foo"})
}

func (suite *PluginSuite) TestFindPluginsIgnoreNotExec(c *C) {
	suite.makePlugin("foo", 0644)
	suite.makePlugin("bar", 0666)
	plugins := findPlugins()
	c.Assert(plugins, DeepEquals, []string{})
}

func (suite *PluginSuite) TestRunPluginExising(c *C) {
	suite.makePlugin("foo", 0755)
	ctx := testing.Context(c)
	err := RunPlugin(ctx, "foo", []string{"some params"})
	c.Assert(err, IsNil)
	c.Assert(testing.Stdout(ctx), Equals, "foo erewhemos some params\n")
	c.Assert(testing.Stderr(ctx), Equals, "")
}

func (suite *PluginSuite) TestRunPluginExisingJujuEnv(c *C) {
	suite.makePlugin("foo", 0755)
	os.Setenv("JUJU_ENV", "omg")
	ctx := testing.Context(c)
	err := RunPlugin(ctx, "foo", []string{"some params"})
	c.Assert(err, IsNil)
	c.Assert(testing.Stdout(ctx), Equals, "foo omg some params\n")
	c.Assert(testing.Stderr(ctx), Equals, "")
}

func (suite *PluginSuite) TestRunPluginExisingDashE(c *C) {
	suite.makePlugin("foo", 0755)
	ctx := testing.Context(c)
	err := RunPlugin(ctx, "foo", []string{"-e plugins-rock some params"})
	c.Assert(err, IsNil)
	c.Assert(testing.Stdout(ctx), Equals, "foo plugins-rock some params\n")
	c.Assert(testing.Stderr(ctx), Equals, "")
}

func (suite *PluginSuite) TestRunPluginWithFailing(c *C) {
	suite.makeFailingPlugin("foo", 2)
	ctx := testing.Context(c)
	err := RunPlugin(ctx, "foo", []string{"some params"})
	c.Assert(err, ErrorMatches, "exit status 2")
	c.Assert(testing.Stdout(ctx), Equals, "failing\n")
	c.Assert(testing.Stderr(ctx), Equals, "")
}

func (suite *PluginSuite) makePlugin(name string, perm os.FileMode) {
	content := fmt.Sprintf("#!/bin/bash\necho %s $JUJU_ENV $*", name)
	filename := testing.HomePath("juju-" + name)
	ioutil.WriteFile(filename, []byte(content), perm)
}

func (suite *PluginSuite) makeFailingPlugin(name string, exitStatus int) {
	content := fmt.Sprintf("#!/bin/bash\necho failing\nexit %d", exitStatus)
	filename := testing.HomePath("juju-" + name)
	ioutil.WriteFile(filename, []byte(content), 0755)
}
