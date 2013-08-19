// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"text/template"
	"time"

	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/testing"
	. "launchpad.net/juju-core/testing/checkers"
)

type PluginSuite struct {
	testing.LoggingSuite
	oldPath string
	home    *testing.FakeHome
}

var _ = Suite(&PluginSuite{})

func (suite *PluginSuite) SetUpTest(c *C) {
	suite.LoggingSuite.SetUpTest(c)
	suite.oldPath = os.Getenv("PATH")
	suite.home = testing.MakeSampleHome(c)
	os.Setenv("PATH", "/bin:"+testing.HomePath())
}

func (suite *PluginSuite) TearDownTest(c *C) {
	suite.home.Restore()
	os.Setenv("PATH", suite.oldPath)
	suite.LoggingSuite.TearDownTest(c)
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
	c.Assert(testing.Stdout(ctx), Equals, "foo some params\n")
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
	// Make plugins that will deadlock if we don't start them in parallel.
	// Each plugin depends on another one being started before they will
	// complete. They make a full loop, so no sequential ordering will ever
	// succeed.
	suite.makeFullPlugin(PluginParams{Name: "foo", Creates: "foo", DependsOn: "bar"})
	suite.makeFullPlugin(PluginParams{Name: "bar", Creates: "bar", DependsOn: "baz"})
	suite.makeFullPlugin(PluginParams{Name: "baz", Creates: "baz", DependsOn: "error"})
	suite.makeFullPlugin(PluginParams{Name: "error", ExitStatus: 1, Creates: "error", DependsOn: "foo"})

	// If the code was wrong, GetPluginDescriptions would deadlock,
	// so timeout after a short while
	resultChan := make(chan []PluginDescription)
	go func() {
		resultChan <- GetPluginDescriptions()
	}()
	// 10 seconds is arbitrary but should always be generously long. Test
	// actually only takes about 15ms in practice. But 10s allows for system hiccups, etc.
	waitTime := 10 * time.Second
	var results []PluginDescription
	select {
	case results = <-resultChan:
		break
	case <-time.After(waitTime):
		c.Fatalf("Took too more than %fs to complete.", waitTime.Seconds())
	}

	c.Assert(results, HasLen, 4)
	c.Assert(results[0].name, Equals, "bar")
	c.Assert(results[0].description, Equals, "bar description")
	c.Assert(results[1].name, Equals, "baz")
	c.Assert(results[1].description, Equals, "baz description")
	c.Assert(results[2].name, Equals, "error")
	c.Assert(results[2].description, Equals, "error occurred running 'juju-error --description'")
	c.Assert(results[3].name, Equals, "foo")
	c.Assert(results[3].description, Equals, "foo description")
}

func (suite *PluginSuite) TestHelpPluginsWithNoPlugins(c *C) {
	output := badrun(c, 0, "help", "plugins")
	c.Assert(output, HasPrefix, PluginTopicText)
	c.Assert(output, HasSuffix, "\n\nNo plugins found.\n")
}

func (suite *PluginSuite) TestHelpPluginsWithPlugins(c *C) {
	suite.makeFullPlugin(PluginParams{Name: "foo"})
	suite.makeFullPlugin(PluginParams{Name: "bar"})
	output := badrun(c, 0, "help", "plugins")
	c.Assert(output, HasPrefix, PluginTopicText)
	expectedPlugins := `

bar  bar description
foo  foo description
`
	c.Assert(output, HasSuffix, expectedPlugins)
}

func (suite *PluginSuite) TestHelpPluginName(c *C) {
	suite.makeFullPlugin(PluginParams{Name: "foo"})
	output := badrun(c, 0, "help", "foo")
	expectedHelp := `foo longer help

something useful
`
	c.Assert(output, Matches, expectedHelp)
}

func (suite *PluginSuite) TestHelpPluginNameNotAPlugin(c *C) {
	output := badrun(c, 0, "help", "foo")
	expectedHelp := "error: unknown command or topic for foo\n"
	c.Assert(output, Matches, expectedHelp)
}

func (suite *PluginSuite) TestHelpAsArg(c *C) {
	suite.makeFullPlugin(PluginParams{Name: "foo"})
	output := badrun(c, 0, "foo", "--help")
	expectedHelp := `foo longer help

something useful
`
	c.Assert(output, Matches, expectedHelp)
}

func (suite *PluginSuite) TestDebugAsArg(c *C) {
	suite.makeFullPlugin(PluginParams{Name: "foo"})
	output := badrun(c, 0, "foo", "--debug")
	expectedDebug := "some debug\n"
	c.Assert(output, Matches, expectedDebug)
}

func (suite *PluginSuite) makePlugin(name string, perm os.FileMode) {
	content := fmt.Sprintf("#!/bin/bash\necho %s $*", name)
	filename := testing.HomePath(JujuPluginPrefix + name)
	ioutil.WriteFile(filename, []byte(content), perm)
}

func (suite *PluginSuite) makeFailingPlugin(name string, exitStatus int) {
	content := fmt.Sprintf("#!/bin/bash\necho failing\nexit %d", exitStatus)
	filename := testing.HomePath(JujuPluginPrefix + name)
	ioutil.WriteFile(filename, []byte(content), 0755)
}

type PluginParams struct {
	Name       string
	ExitStatus int
	Creates    string
	DependsOn  string
}

const pluginTemplate = `#!/bin/bash

if [ "$1" = "--description" ]; then
  if [ -n "{{.Creates}}" ]; then
    touch "{{.Creates}}"
  fi
  if [ -n "{{.DependsOn}}" ]; then
    # Sleep 10ms while waiting to allow other stuff to do work
    while [ ! -e "{{.DependsOn}}" ]; do sleep 0.010; done
  fi
  echo "{{.Name}} description"
  exit {{.ExitStatus}}
fi

if [ "$1" = "--help" ]; then
  echo "{{.Name}} longer help"
  echo ""
  echo "something useful"
  exit {{.ExitStatus}}
fi

if [ "$1" = "--debug" ]; then
  echo "some debug"
  exit {{.ExitStatus}}
fi

echo {{.Name}} $*
exit {{.ExitStatus}}
`

func (suite *PluginSuite) makeFullPlugin(params PluginParams) {
	// Create a new template and parse the plugin into it.
	t := template.Must(template.New("plugin").Parse(pluginTemplate))
	content := &bytes.Buffer{}
	filename := testing.HomePath("juju-" + params.Name)
	// Create the files in the temp dirs, so we don't pollute the working space
	if params.Creates != "" {
		params.Creates = testing.HomePath(params.Creates)
	}
	if params.DependsOn != "" {
		params.DependsOn = testing.HomePath(params.DependsOn)
	}
	t.Execute(content, params)
	ioutil.WriteFile(filename, content.Bytes(), 0755)
}
