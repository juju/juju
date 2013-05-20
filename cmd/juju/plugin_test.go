package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/checkers"
)

type PluginSuite struct {
	oldPath string
	home    *testing.FakeHome
}

var _ = Suite(&PluginSuite{})

func (suite *PluginSuite) SetUpTest(c *C) {
	suite.oldPath = os.Getenv("PATH")
	suite.home = testing.MakeSampleHome(c)
	os.Setenv("PATH", "/bin:"+testing.HomePath())
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

func (suite *PluginSuite) TestGatherDescriptionsInParallel(c *C) {
	suite.makeFullPlugin("foo", 0, 0.1)
	suite.makeFullPlugin("bar", 0, 0.15)
	suite.makeFullPlugin("baz", 0, 0.3)
	suite.makeFullPlugin("error", 1, 0.1)
	suite.makeFullPlugin("slow", 0, 0.2)

	start := time.Now()
	results := GetPluginDescriptions()
	elapsed := time.Since(start)

	// 300 for baz above + 50ms wiggle room
	expectedDuration := 350 * time.Millisecond

	c.Assert(results, HasLen, 5)
	c.Check(elapsed, checkers.DurationLessThan, expectedDuration)
	c.Assert(results[0].name, Equals, "bar")
	c.Assert(results[0].description, Equals, "bar description")
	c.Assert(results[1].name, Equals, "baz")
	c.Assert(results[1].description, Equals, "baz description")
	c.Assert(results[2].name, Equals, "error")
	c.Assert(results[2].description, Equals, "error occurred running 'juju-error --description'")
	c.Assert(results[3].name, Equals, "foo")
	c.Assert(results[3].description, Equals, "foo description")
	c.Assert(results[4].name, Equals, "slow")
	c.Assert(results[4].description, Equals, "slow description")
}

func (suite *PluginSuite) TestHelpPluginsWithNoPlugins(c *C) {
	output := badrun(c, 0, "help", "plugins")
	c.Assert(output, checkers.HasPrefix, PluginTopicText)
	c.Assert(output, checkers.HasSuffix, "\n\nNo plugins found.\n")
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

const pluginTemplate = `#!/bin/bash

if [ "$1" = "--description" ]; then
  sleep sleep-time
  echo "plugin-name description"
  exit exit-status
fi

if [ "$1" = "--help" ]; then
  echo "plugin-name longer help"
  echo ""
  echo "something useful"
  exit exit-status
fi

echo plugin-name $*
exit exit-status
`

func (suite *PluginSuite) makeFullPlugin(name string, exitStatus int, sleepTime float64) {
	content := strings.Replace(pluginTemplate, "plugin-name", name, -1)
	content = strings.Replace(content, "sleep-time", fmt.Sprintf("%f", sleepTime), -1)
	content = strings.Replace(content, "exit-status", fmt.Sprintf("%d", exitStatus), -1)
	filename := testing.HomePath("juju-" + name)
	ioutil.WriteFile(filename, []byte(content), 0755)
}
