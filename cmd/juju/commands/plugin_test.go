// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"bytes"
	"fmt"
	"os"
	"runtime"
	"text/template"
	"time"

	"github.com/juju/tc"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v4"

	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
)

type PluginSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	oldPath string
}

var _ = tc.Suite(&PluginSuite{})

func (suite *PluginSuite) SetUpTest(c *tc.C) {
	suite.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	suite.oldPath = os.Getenv("PATH")

	// We have to be careful to leave the default cmds that we need for tests
	// like "touch" (which its in /usr/bin on mac but in /bin on linux).
	// We doings this, because we need to add "binaries" from the tmp dir and reduce
	// tests execution, since we are looking into all paths in $PATH to find juju plugins
	path := "/bin:%s"
	if runtime.GOOS == "darwin" {
		path = "/bin:/usr/bin:%s"
	}

	os.Setenv("PATH", fmt.Sprintf(path, jujutesting.HomePath()))

	jujuclienttesting.SetupMinimalFileStore(c)
}

func (suite *PluginSuite) TearDownTest(c *tc.C) {
	os.Setenv("PATH", suite.oldPath)
	suite.FakeJujuXDGDataHomeSuite.TearDownTest(c)
}

func (*PluginSuite) TestFindPlugins(c *tc.C) {
	plugins := findPlugins()
	c.Assert(plugins, tc.DeepEquals, []string{})
}

func (suite *PluginSuite) TestFindPluginsOrder(c *tc.C) {
	suite.makeWorkingPlugin("foo", 0744)
	suite.makeWorkingPlugin("bar", 0654)
	suite.makeWorkingPlugin("baz", 0645)
	plugins := findPlugins()
	c.Assert(plugins, tc.DeepEquals, []string{"juju-bar", "juju-baz", "juju-foo"})
}

func (suite *PluginSuite) TestFindPluginsBadNames(c *tc.C) {
	suite.makePlugin("juju-1foo", "", 0755)
	suite.makePlugin("juju--foo", "", 0755)
	suite.makePlugin("ajuju-foo", "", 0755)
	plugins := findPlugins()
	c.Assert(plugins, tc.DeepEquals, []string{})
}

func (suite *PluginSuite) TestFindPluginsIgnoreNotExec(c *tc.C) {
	suite.makeWorkingPlugin("foo", 0644)
	suite.makeWorkingPlugin("bar", 0666)
	plugins := findPlugins()
	c.Assert(plugins, tc.DeepEquals, []string{})
}

func (suite *PluginSuite) TestRunPluginExising(c *tc.C) {
	suite.makeWorkingPlugin("foo", 0755)
	ctx := cmdtesting.Context(c)
	err := RunPlugin(func(ctx *cmd.Context, subcommand string, args []string) error {
		c.Fatal("failed if called")
		return nil
	})(ctx, "foo", []string{"some params"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "foo some params\n")
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "")
}

func (suite *PluginSuite) TestRunPluginWithFailing(c *tc.C) {
	suite.makeFailingPlugin("foo", 2)
	ctx := cmdtesting.Context(c)
	err := RunPlugin(func(ctx *cmd.Context, subcommand string, args []string) error {
		c.Fatal("failed if called")
		return nil
	})(ctx, "foo", []string{"some params"})
	c.Assert(err, tc.ErrorMatches, "subprocess encountered error code 2")
	c.Assert(err, jc.Satisfies, utils.IsRcPassthroughError)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "failing\n")
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "")
}

func (suite *PluginSuite) TestGatherDescriptionsInParallel(c *tc.C) {
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

	c.Assert(results, tc.HasLen, 4)
	c.Assert(results[0].name, tc.Equals, "bar")
	c.Assert(results[0].description, tc.Equals, "bar description")
	c.Assert(results[1].name, tc.Equals, "baz")
	c.Assert(results[1].description, tc.Equals, "baz description")
	c.Assert(results[2].name, tc.Equals, "error")
	c.Assert(results[2].description, tc.Equals, "error occurred running 'juju-error --description'")
	c.Assert(results[3].name, tc.Equals, "foo")
	c.Assert(results[3].description, tc.Equals, "foo description")
}

func (suite *PluginSuite) TestHelpPluginName(c *tc.C) {
	suite.makeFullPlugin(PluginParams{Name: "foo"})
	output := badrun(c, 0, "help", "foo")
	expectedHelp := `foo longer help

something useful
`
	c.Assert(output, tc.Matches, expectedHelp)
}

func (suite *PluginSuite) TestHelpPluginNameNotAPlugin(c *tc.C) {
	output := badrun(c, 0, "help", "foo")
	expectedHelp := `ERROR juju: "foo" is not a juju command. See "juju --help".

Did you mean:
	find
`
	c.Assert(output, tc.Matches, expectedHelp)
}

func (suite *PluginSuite) TestHelpPluginNameAsPathIsNotAPlugin(c *tc.C) {
	output := badrun(c, 0, "help", "/foo")
	expectedHelp := `ERROR juju: "/foo" is not a juju command. See "juju --help".

Did you mean:
	info
`
	c.Assert(output, tc.Matches, expectedHelp)
}

func (suite *PluginSuite) TestHelpPluginNameWithSpecialPrefixWhichIsNotAPathAndPlugin(c *tc.C) {
	output := badrun(c, 0, "help", ".foo")
	expectedHelp := `ERROR juju: ".foo" is not a juju command. See "juju --help".

Did you mean:
	info
`
	c.Assert(output, tc.Matches, expectedHelp)
}

func (suite *PluginSuite) TestHelpAsArg(c *tc.C) {
	suite.makeFullPlugin(PluginParams{Name: "foo"})
	output := badrun(c, 0, "foo", "--help")
	expectedHelp := `foo longer help

something useful
`
	c.Assert(output, tc.Matches, expectedHelp)
}

func (suite *PluginSuite) TestDebugAsArg(c *tc.C) {
	suite.makeFullPlugin(PluginParams{Name: "foo"})
	output := badrun(c, 0, "foo", "--debug")
	expectedDebug := "some debug\n"
	c.Assert(output, tc.Matches, expectedDebug)
}

func (suite *PluginSuite) setupClientStore(c *tc.C) {
	// Plugins are run as either controller or model commands,
	// and so require a current controller or account and model.
	store := jujuclient.NewFileClientStore()
	err := store.AddController("myctrl", jujuclient.ControllerDetails{
		ControllerUUID: testing.ControllerTag.Id(),
		CACert:         "fake",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = store.SetCurrentController("myctrl")
	c.Assert(err, jc.ErrorIsNil)
	err = store.UpdateAccount("myctrl", jujuclient.AccountDetails{
		User:     "admin",
		Password: "hunter2",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (suite *PluginSuite) TestJujuModelEnvVars(c *tc.C) {
	suite.setupClientStore(c)
	suite.makeFullPlugin(PluginParams{Name: "foo"})
	output := badrun(c, 0, "foo", "-m", "mymodel", "-p", "pluginarg")
	expectedDebug := "foo -m mymodel -p pluginarg\nmodel is:  mymodel\n"
	c.Assert(output, tc.Matches, expectedDebug)
}

func (suite *PluginSuite) TestJujuControllerEnvVars(c *tc.C) {
	suite.setupClientStore(c)
	suite.makeFullPlugin(PluginParams{Name: "foo"})
	output := badrun(c, 0, "foo", "-c", "myctrl", "-p", "pluginarg")
	expectedDebug := "foo -c myctrl -p pluginarg\ncontroller is:  myctrl\n"
	c.Assert(output, tc.Matches, expectedDebug)
}

func (suite *PluginSuite) makePlugin(fullName, script string, perm os.FileMode) {
	filename := jujutesting.HomePath(fullName)
	content := fmt.Sprintf("#!/bin/bash --norc\n%s", script)
	os.WriteFile(filename, []byte(content), perm)
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
if [ -n "$JUJU_MODEL" ]; then
  echo "model is: " $JUJU_MODEL
fi
if [ -n "$JUJU_CONTROLLER" ]; then
  echo "controller is: " $JUJU_CONTROLLER
fi
exit {{.ExitStatus}}
`

func (suite *PluginSuite) makeFullPlugin(params PluginParams) {
	// Create a new template and parse the plugin into it.
	t := template.Must(template.New("plugin").Parse(pluginTemplate))
	content := &bytes.Buffer{}
	filename := jujutesting.HomePath("juju-" + params.Name)
	// Create the files in the temp dirs, so we don't pollute the working space
	if params.Creates != "" {
		params.Creates = jujutesting.HomePath(params.Creates)
	}
	if params.DependsOn != "" {
		params.DependsOn = jujutesting.HomePath(params.DependsOn)
	}
	t.Execute(content, params)
	os.WriteFile(filename, content.Bytes(), 0755)
}
