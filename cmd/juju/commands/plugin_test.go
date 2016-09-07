// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"runtime"
	"text/template"
	"time"

	"github.com/juju/cmd"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing"
)

type PluginSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	oldPath string
}

var _ = gc.Suite(&PluginSuite{})

func (suite *PluginSuite) SetUpTest(c *gc.C) {
	//TODO(bogdanteleaga): Fix bash tests
	if runtime.GOOS == "windows" {
		c.Skip("bug 1403084: tests use bash scrips, will be rewritten for windows")
	}
	suite.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	suite.oldPath = os.Getenv("PATH")
	os.Setenv("PATH", "/bin:"+gitjujutesting.HomePath())
}

func (suite *PluginSuite) TearDownTest(c *gc.C) {
	os.Setenv("PATH", suite.oldPath)
	suite.FakeJujuXDGDataHomeSuite.TearDownTest(c)
}

func (*PluginSuite) TestFindPlugins(c *gc.C) {
	plugins := findPlugins()
	c.Assert(plugins, gc.DeepEquals, []string{})
}

func (suite *PluginSuite) TestFindPluginsOrder(c *gc.C) {
	suite.makeWorkingPlugin("foo", 0744)
	suite.makeWorkingPlugin("bar", 0654)
	suite.makeWorkingPlugin("baz", 0645)
	plugins := findPlugins()
	c.Assert(plugins, gc.DeepEquals, []string{"juju-bar", "juju-baz", "juju-foo"})
}

func (suite *PluginSuite) TestFindPluginsBadNames(c *gc.C) {
	suite.makePlugin("juju-1foo", "", 0755)
	suite.makePlugin("juju--foo", "", 0755)
	suite.makePlugin("ajuju-foo", "", 0755)
	plugins := findPlugins()
	c.Assert(plugins, gc.DeepEquals, []string{})
}

func (suite *PluginSuite) TestFindPluginsIgnoreNotExec(c *gc.C) {
	suite.makeWorkingPlugin("foo", 0644)
	suite.makeWorkingPlugin("bar", 0666)
	plugins := findPlugins()
	c.Assert(plugins, gc.DeepEquals, []string{})
}

func (suite *PluginSuite) TestRunPluginExising(c *gc.C) {
	suite.makeWorkingPlugin("foo", 0755)
	ctx := testing.Context(c)
	err := RunPlugin(ctx, "foo", []string{"some params"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(ctx), gc.Equals, "foo some params\n")
	c.Assert(testing.Stderr(ctx), gc.Equals, "")
}

func (suite *PluginSuite) TestRunPluginWithFailing(c *gc.C) {
	suite.makeFailingPlugin("foo", 2)
	ctx := testing.Context(c)
	err := RunPlugin(ctx, "foo", []string{"some params"})
	c.Assert(err, gc.ErrorMatches, "subprocess encountered error code 2")
	c.Assert(err, jc.Satisfies, cmd.IsRcPassthroughError)
	c.Assert(testing.Stdout(ctx), gc.Equals, "failing\n")
	c.Assert(testing.Stderr(ctx), gc.Equals, "")
}

func (suite *PluginSuite) TestGatherDescriptionsInParallel(c *gc.C) {
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
		c.Fatalf("took longer than %fs to complete.", waitTime.Seconds())
	}

	c.Assert(results, gc.HasLen, 4)
	c.Assert(results[0].name, gc.Equals, "bar")
	c.Assert(results[0].description, gc.Equals, "bar description")
	c.Assert(results[1].name, gc.Equals, "baz")
	c.Assert(results[1].description, gc.Equals, "baz description")
	c.Assert(results[2].name, gc.Equals, "error")
	c.Assert(results[2].description, gc.Equals, "error occurred running 'juju-error --description'")
	c.Assert(results[3].name, gc.Equals, "foo")
	c.Assert(results[3].description, gc.Equals, "foo description")
}

func (suite *PluginSuite) TestHelpPluginName(c *gc.C) {
	suite.makeFullPlugin(PluginParams{Name: "foo"})
	output := badrun(c, 0, "help", "foo")
	expectedHelp := `foo longer help

something useful
`
	c.Assert(output, gc.Matches, expectedHelp)
}

func (suite *PluginSuite) TestHelpPluginNameNotAPlugin(c *gc.C) {
	output := badrun(c, 0, "help", "foo")
	expectedHelp := "ERROR unknown command or topic for foo\n"
	c.Assert(output, gc.Matches, expectedHelp)
}

func (suite *PluginSuite) TestHelpAsArg(c *gc.C) {
	suite.makeFullPlugin(PluginParams{Name: "foo"})
	output := badrun(c, 0, "foo", "--help")
	expectedHelp := `foo longer help

something useful
`
	c.Assert(output, gc.Matches, expectedHelp)
}

func (suite *PluginSuite) TestDebugAsArg(c *gc.C) {
	suite.makeFullPlugin(PluginParams{Name: "foo"})
	output := badrun(c, 0, "foo", "--debug")
	expectedDebug := "some debug\n"
	c.Assert(output, gc.Matches, expectedDebug)
}

func (suite *PluginSuite) TestJujuEnvVars(c *gc.C) {
	// Plugins are run as model commands, and so require a current
	// account and model.
	store := jujuclient.NewFileClientStore()
	err := store.AddController("myctrl", jujuclient.ControllerDetails{
		ControllerUUID: testing.ControllerTag.Id(),
		CACert:         "fake",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = store.SetCurrentController("myctrl")
	c.Assert(err, jc.ErrorIsNil)
	err = store.UpdateAccount("myctrl", jujuclient.AccountDetails{
		User:     "admin@local",
		Password: "hunter2",
	})
	c.Assert(err, jc.ErrorIsNil)

	suite.makeFullPlugin(PluginParams{Name: "foo"})
	output := badrun(c, 0, "foo", "-m", "mymodel", "-p", "pluginarg")
	expectedDebug := "foo -m mymodel -p pluginarg\nmodel is:  mymodel\n"
	c.Assert(output, gc.Matches, expectedDebug)
}

func (suite *PluginSuite) makePlugin(fullName, script string, perm os.FileMode) {
	filename := gitjujutesting.HomePath(fullName)
	content := fmt.Sprintf("#!/bin/bash --norc\n%s", script)
	ioutil.WriteFile(filename, []byte(content), perm)
}

func (suite *PluginSuite) makeWorkingPlugin(name string, perm os.FileMode) {
	script := fmt.Sprintf("echo %s $*", name)
	suite.makePlugin(JujuPluginPrefix+name, script, perm)
}

func (suite *PluginSuite) makeFailingPlugin(name string, exitStatus int) {
	script := fmt.Sprintf("echo failing\nexit %d", exitStatus)
	suite.makePlugin(JujuPluginPrefix+name, script, 0755)
}

type PluginParams struct {
	Name       string
	ExitStatus int
	Creates    string
	DependsOn  string
}

const pluginTemplate = `#!/bin/bash --norc

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
echo "model is: " $JUJU_MODEL
exit {{.ExitStatus}}
`

func (suite *PluginSuite) makeFullPlugin(params PluginParams) {
	// Create a new template and parse the plugin into it.
	t := template.Must(template.New("plugin").Parse(pluginTemplate))
	content := &bytes.Buffer{}
	filename := gitjujutesting.HomePath("juju-" + params.Name)
	// Create the files in the temp dirs, so we don't pollute the working space
	if params.Creates != "" {
		params.Creates = gitjujutesting.HomePath(params.Creates)
	}
	if params.DependsOn != "" {
		params.DependsOn = gitjujutesting.HomePath(params.DependsOn)
	}
	t.Execute(content, params)
	ioutil.WriteFile(filename, content.Bytes(), 0755)
}
