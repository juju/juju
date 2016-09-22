// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/juju/cmd"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/featureflag"
	"github.com/juju/utils/series"
	"github.com/juju/utils/set"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/application"
	"github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/cmd/modelcmd"
	cmdtesting "github.com/juju/juju/cmd/testing"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/juju/osenv"
	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

type MainSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	gitjujutesting.PatchExecHelper
}

var _ = gc.Suite(&MainSuite{})

func deployHelpText() string {
	return cmdtesting.HelpText(application.NewDeployCommand(), "juju deploy")
}
func configHelpText() string {
	return cmdtesting.HelpText(application.NewConfigCommand(), "juju config")
}

func syncToolsHelpText() string {
	return cmdtesting.HelpText(newSyncToolsCommand(), "juju sync-tools")
}

func (s *MainSuite) TestRunMain(c *gc.C) {
	// The test array structure needs to be inline here as some of the
	// expected values below use deployHelpText().  This constructs the deploy
	// command and runs gets the help for it.  When the deploy command is
	// setting the flags (which is needed for the help text) it is accessing
	// osenv.JujuXDGDataHome(), which panics if SetJujuXDGDataHome has not been called.
	// The FakeHome from testing does this.
	for i, t := range []struct {
		summary string
		args    []string
		code    int
		out     string
	}{{
		summary: "juju help foo doesn't exist",
		args:    []string{"help", "foo"},
		code:    1,
		out:     "ERROR unknown command or topic for foo\n",
	}, {
		summary: "juju help deploy shows the default help without global options",
		args:    []string{"help", "deploy"},
		code:    0,
		out:     deployHelpText(),
	}, {
		summary: "juju --help deploy shows the same help as 'help deploy'",
		args:    []string{"--help", "deploy"},
		code:    0,
		out:     deployHelpText(),
	}, {
		summary: "juju deploy --help shows the same help as 'help deploy'",
		args:    []string{"deploy", "--help"},
		code:    0,
		out:     deployHelpText(),
	}, {
		summary: "juju --help config shows the same help as 'help config'",
		args:    []string{"--help", "config"},
		code:    0,
		out:     configHelpText(),
	}, {
		summary: "juju config --help shows the same help as 'help config'",
		args:    []string{"config", "--help"},
		code:    0,
		out:     configHelpText(),
	}, {
		summary: "unknown command",
		args:    []string{"discombobulate"},
		code:    1,
		out:     "ERROR unrecognized command: juju discombobulate\n",
	}, {
		summary: "unknown option before command",
		args:    []string{"--cheese", "bootstrap"},
		code:    2,
		out:     "error: flag provided but not defined: --cheese\n",
	}, {
		summary: "unknown option after command",
		args:    []string{"bootstrap", "--cheese"},
		code:    2,
		out:     "error: flag provided but not defined: --cheese\n",
	}, {
		summary: "known option, but specified before command",
		args:    []string{"--model", "blah", "bootstrap"},
		code:    2,
		out:     "error: flag provided but not defined: --model\n",
	}, {
		summary: "juju sync-tools registered properly",
		args:    []string{"sync-tools", "--help"},
		code:    0,
		out:     syncToolsHelpText(),
	}, {
		summary: "check version command returns a fully qualified version string",
		args:    []string{"version"},
		code:    0,
		out: version.Binary{
			Number: jujuversion.Current,
			Arch:   arch.HostArch(),
			Series: series.HostSeries(),
		}.String() + "\n",
	}} {
		c.Logf("test %d: %s", i, t.summary)
		out := badrun(c, t.code, t.args...)
		c.Assert(out, gc.Equals, t.out)
	}
}

func (s *MainSuite) TestActualRunJujuArgOrder(c *gc.C) {
	//TODO(bogdanteleaga): cannot read the env file because of some suite
	//problems. The juju home, when calling something from the command line is
	//not the same as in the test suite.
	if runtime.GOOS == "windows" {
		c.Skip("bug 1403084: cannot read env file on windows because of suite problems")
	}
	s.PatchEnvironment(osenv.JujuModelEnvKey, "current")
	logpath := filepath.Join(c.MkDir(), "log")
	tests := [][]string{
		{"--log-file", logpath, "--debug", "controllers"}, // global flags before
		{"controllers", "--log-file", logpath, "--debug"}, // after
		{"--log-file", logpath, "controllers", "--debug"}, // mixed
	}
	for i, test := range tests {
		c.Logf("test %d: %v", i, test)
		badrun(c, 0, test...)
		content, err := ioutil.ReadFile(logpath)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(string(content), gc.Matches, "(.|\n)*running juju(.|\n)*command finished(.|\n)*")
		err = os.Remove(logpath)
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *MainSuite) TestFirstRun2xFrom1xOnUbuntu(c *gc.C) {
	if runtime.GOOS == "windows" {
		// This test can't work on Windows and shouldn't need to
		c.Skip("test doesn't work on Windows because Juju's 1.x and 2.x config directory are the same")
	}

	// Code should only run on ubuntu series, so patch out the series for
	// when non-ubuntu OSes run this test.
	s.PatchValue(&series.HostSeries, func() string { return "trusty" })

	argChan := make(chan []string, 1)

	execCommand := s.GetExecCommand(gitjujutesting.PatchExecConfig{
		Stdout: "1.25.0-trusty-amd64",
		Args:   argChan,
	})
	stub := &gitjujutesting.Stub{}
	s.PatchValue(&cloud.NewUpdateCloudsCommand, func() cmd.Command {
		return &stubCommand{stub: stub}
	})

	// remove the new juju-home and create a fake old juju home.
	err := os.RemoveAll(osenv.JujuXDGDataHomeDir())
	c.Assert(err, jc.ErrorIsNil)
	makeValidOldHome(c)

	var code int
	f := func() {
		code = main{
			execCommand: execCommand,
		}.Run([]string{"juju", "version"})
	}

	stdout, stderr := gitjujutesting.CaptureOutput(c, f)

	select {
	case args := <-argChan:
		c.Assert(args, gc.DeepEquals, []string{"juju-1", "version"})
	default:
		c.Fatalf("Exec function not called.")
	}

	c.Check(code, gc.Equals, 0)
	c.Check(string(stderr), gc.Equals, fmt.Sprintf(`
    Welcome to Juju %s. If you meant to use Juju 1.25.0 you can continue using it
    with the command juju-1 e.g. 'juju-1 switch'.
    See https://jujucharms.com/docs/stable/introducing-2 for more details.

Since Juju 2 is being run for the first time, downloading latest cloud information.`[1:]+"\n", jujuversion.Current))
	checkVersionOutput(c, string(stdout))
}

func (s *MainSuite) TestFirstRun2xFrom1xNotUbuntu(c *gc.C) {
	// Code should only run on ubuntu series, so pretend to be something else.
	s.PatchValue(&series.HostSeries, func() string { return "win8" })

	argChan := make(chan []string, 1)

	// we shouldn't actually be running anything, but if we do, this will
	// provide some consistent results.
	execCommand := s.GetExecCommand(gitjujutesting.PatchExecConfig{
		Stdout: "1.25.0-trusty-amd64",
		Args:   argChan,
	})
	stub := &gitjujutesting.Stub{}
	s.PatchValue(&cloud.NewUpdateCloudsCommand, func() cmd.Command {
		return &stubCommand{stub: stub}
	})

	// remove the new juju-home and create a fake old juju home.
	err := os.RemoveAll(osenv.JujuXDGDataHomeDir())
	c.Assert(err, jc.ErrorIsNil)

	makeValidOldHome(c)

	var code int
	stdout, stderr := gitjujutesting.CaptureOutput(c, func() {
		code = main{
			execCommand: execCommand,
		}.Run([]string{"juju", "version"})
	})

	c.Assert(code, gc.Equals, 0)

	assertNoArgs(c, argChan)

	c.Check(string(stderr), gc.Equals, `
Since Juju 2 is being run for the first time, downloading latest cloud information.`[1:]+"\n")
	checkVersionOutput(c, string(stdout))
}

func (s *MainSuite) TestNoWarn1xWith2xData(c *gc.C) {
	// Code should only rnu on ubuntu series, so patch out the series for
	// when non-ubuntu OSes run this test.
	s.PatchValue(&series.HostSeries, func() string { return "trusty" })

	argChan := make(chan []string, 1)

	// we shouldn't actually be running anything, but if we do, this will
	// provide some consistent results.
	execCommand := s.GetExecCommand(gitjujutesting.PatchExecConfig{
		Stdout: "1.25.0-trusty-amd64",
		Args:   argChan,
	})

	// there should be a 2x home directory already created by the test setup.

	// create a fake old juju home.
	makeValidOldHome(c)

	var code int
	stdout, stderr := gitjujutesting.CaptureOutput(c, func() {
		code = main{
			execCommand: execCommand,
		}.Run([]string{"juju", "version"})
	})

	c.Assert(code, gc.Equals, 0)

	assertNoArgs(c, argChan)
	c.Assert(string(stderr), gc.Equals, "")
	checkVersionOutput(c, string(stdout))
}

func (s *MainSuite) TestNoWarnWithNo1xOr2xData(c *gc.C) {
	// Code should only rnu on ubuntu series, so patch out the series for
	// when non-ubuntu OSes run this test.
	s.PatchValue(&series.HostSeries, func() string { return "trusty" })

	argChan := make(chan []string, 1)
	// we shouldn't actually be running anything, but if we do, this will
	// provide some consistent results.
	execCommand := s.GetExecCommand(gitjujutesting.PatchExecConfig{
		Stdout: "1.25.0-trusty-amd64",
		Args:   argChan,
	})
	stub := &gitjujutesting.Stub{}
	s.PatchValue(&cloud.NewUpdateCloudsCommand, func() cmd.Command {
		return &stubCommand{stub: stub}
	})

	// remove the new juju-home.
	err := os.RemoveAll(osenv.JujuXDGDataHomeDir())
	c.Assert(err, jc.ErrorIsNil)

	// create fake (empty) old juju home.
	path := c.MkDir()
	s.PatchEnvironment("JUJU_HOME", path)

	var code int
	stdout, stderr := gitjujutesting.CaptureOutput(c, func() {
		code = main{
			execCommand: execCommand,
		}.Run([]string{"juju", "version"})
	})

	c.Assert(code, gc.Equals, 0)

	assertNoArgs(c, argChan)
	c.Check(string(stderr), gc.Equals, `
Since Juju 2 is being run for the first time, downloading latest cloud information.`[1:]+"\n")
	checkVersionOutput(c, string(stdout))
}

func (s *MainSuite) assertRunCommandUpdateCloud(c *gc.C, expectedCall string) {
	argChan := make(chan []string, 1)
	execCommand := s.GetExecCommand(gitjujutesting.PatchExecConfig{
		Stdout: "1.25.0-trusty-amd64",
		Args:   argChan,
	})

	stub := &gitjujutesting.Stub{}
	s.PatchValue(&cloud.NewUpdateCloudsCommand, func() cmd.Command {
		return &stubCommand{stub: stub}

	})
	var code int
	gitjujutesting.CaptureOutput(c, func() {
		code = main{
			execCommand: execCommand,
		}.Run([]string{"juju", "version"})
	})
	c.Assert(code, gc.Equals, 0)
	c.Assert(stub.Calls()[0].FuncName, gc.Equals, expectedCall)
}

func (s *MainSuite) TestFirstRunUpdateCloud(c *gc.C) {
	// remove the juju-home.
	err := os.RemoveAll(osenv.JujuXDGDataHomeDir())
	c.Assert(err, jc.ErrorIsNil)
	s.assertRunCommandUpdateCloud(c, "Run")
}

func (s *MainSuite) TestRunNoUpdateCloud(c *gc.C) {
	s.assertRunCommandUpdateCloud(c, "Info")
}

func makeValidOldHome(c *gc.C) {
	oldhome := osenv.OldJujuHomeDir()
	err := os.MkdirAll(oldhome, 0700)
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(filepath.Join(oldhome, "environments.yaml"), []byte("boo!"), 0600)
	c.Assert(err, jc.ErrorIsNil)
}

func checkVersionOutput(c *gc.C, output string) {
	ver := version.Binary{
		Number: jujuversion.Current,
		Arch:   arch.HostArch(),
		Series: series.HostSeries(),
	}

	c.Check(output, gc.Equals, ver.String()+"\n")
}

func assertNoArgs(c *gc.C, argChan <-chan []string) {
	select {
	case args := <-argChan:
		c.Fatalf("Exec function called when it shouldn't have been (with args %q).", args)
	default:
		// this is the good path - there shouldn't be any args, which indicates
		// the executable was not called.
	}
}

var commandNames = []string{
	"actions",
	"add-cloud",
	"add-credential",
	"add-machine",
	"add-model",
	"add-relation",
	"add-space",
	"add-ssh-key",
	"add-storage",
	"add-subnet",
	"add-unit",
	"add-user",
	"agree",
	"agreements",
	"allocate",
	"autoload-credentials",
	"backups",
	"bootstrap",
	"budgets",
	"cached-images",
	"change-user-password",
	"charm",
	"clouds",
	"config",
	"collect-metrics",
	"controllers",
	"create-backup",
	"create-budget",
	"create-storage-pool",
	"credentials",
	"debug-hooks",
	"debug-log",
	"remove-user",
	"deploy",
	"destroy-controller",
	"destroy-model",
	"disable-command",
	"disable-user",
	"disabled-commands",
	"download-backup",
	"enable-ha",
	"enable-command",
	"enable-destroy-controller",
	"enable-user",
	"expose",
	"get-constraints",
	"get-controller-config",
	"get-model-constraints",
	"grant",
	"gui",
	"help",
	"help-tool",
	"import-ssh-key",
	"kill-controller",
	"list-actions",
	"list-agreements",
	"list-backups",
	"list-budgets",
	"list-cached-images",
	"list-clouds",
	"list-controllers",
	"list-credentials",
	"list-disabled-commands",
	"list-machines",
	"list-models",
	"list-plans",
	"list-ssh-keys",
	"list-spaces",
	"list-storage",
	"list-storage-pools",
	"list-subnets",
	"list-users",
	"login",
	"logout",
	"machines",
	"metrics",
	"model-config",
	"model-defaults",
	"models",
	"plans",
	"register",
	"relate", //alias for add-relation
	"remove-application",
	"remove-backup",
	"remove-cached-images",
	"remove-cloud",
	"remove-credential",
	"remove-machine",
	"remove-relation",
	"remove-ssh-key",
	"remove-unit",
	"resolved",
	"restore-backup",
	"retry-provisioning",
	"revoke",
	"run",
	"run-action",
	"scp",
	"set-budget",
	"set-constraints",
	"set-default-credential",
	"set-default-region",
	"set-meter-status",
	"set-model-constraints",
	"set-plan",
	"show-action-output",
	"show-action-status",
	"show-backup",
	"show-budget",
	"show-cloud",
	"show-controller",
	"show-machine",
	"show-model",
	"show-status",
	"show-status-log",
	"show-storage",
	"show-user",
	"spaces",
	"ssh",
	"ssh-keys",
	"status",
	"storage",
	"storage-pools",
	"subnets",
	"switch",
	"sync-tools",
	"unexpose",
	"update-allocation",
	"upload-backup",
	"unregister",
	"update-clouds",
	"upgrade-charm",
	"upgrade-gui",
	"upgrade-juju",
	"users",
	"version",
	"whoami",
}

// devFeatures are feature flags that impact registration of commands.
var devFeatures = []string{feature.Migration}

// These are the commands that are behind the `devFeatures`.
var commandNamesBehindFlags = set.NewStrings(
	"migrate",
)

func (s *MainSuite) TestHelpCommands(c *gc.C) {
	// Check that we have correctly registered all the commands
	// by checking the help output.
	// First check default commands, and then check commands that are
	// activated by feature flags.

	// remove features behind dev_flag for the first test
	// since they are not enabled.
	cmdSet := set.NewStrings(commandNames...)

	// 1. Default Commands. Disable all features.
	setFeatureFlags("")
	// Use sorted values here so we can better see what is wrong.
	registered := getHelpCommandNames(c)
	unknown := registered.Difference(cmdSet)
	c.Assert(unknown, jc.DeepEquals, set.NewStrings())
	missing := cmdSet.Difference(registered)
	c.Assert(missing, jc.DeepEquals, set.NewStrings())

	// 2. Enable development features, and test again.
	cmdSet = cmdSet.Union(commandNamesBehindFlags)
	setFeatureFlags(strings.Join(devFeatures, ","))
	registered = getHelpCommandNames(c)
	unknown = registered.Difference(cmdSet)
	c.Assert(unknown, jc.DeepEquals, set.NewStrings())
	missing = cmdSet.Difference(registered)
	c.Assert(missing, jc.DeepEquals, set.NewStrings())
}

func getHelpCommandNames(c *gc.C) set.Strings {
	out := badrun(c, 0, "help", "commands")
	lines := strings.Split(out, "\n")
	names := set.NewStrings()
	for _, line := range lines {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		names.Add(f[0])
	}
	return names
}

func setFeatureFlags(flags string) {
	if err := os.Setenv(osenv.JujuFeatureFlagEnvKey, flags); err != nil {
		panic(err)
	}
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)
}

var globalFlags = []string{
	"--debug .*",
	"--description .*",
	"-h, --help .*",
	"--log-file .*",
	"--logging-config .*",
	"-q, --quiet .*",
	"--show-log .*",
	"-v, --verbose .*",
}

func (s *MainSuite) TestHelpGlobalOptions(c *gc.C) {
	// Check that we have correctly registered all the topics
	// by checking the help output.
	out := badrun(c, 0, "help", "global-options")
	c.Assert(out, gc.Matches, `Global Options

These options may be used with any command, and may appear in front of any
command\.(.|\n)*`)
	lines := strings.Split(out, "\n")
	var flags []string
	for _, line := range lines {
		f := strings.Fields(line)
		if len(f) == 0 || line[0] != '-' {
			continue
		}
		flags = append(flags, line)
	}
	c.Assert(len(flags), gc.Equals, len(globalFlags))
	for i, line := range flags {
		c.Assert(line, gc.Matches, globalFlags[i])
	}
}

func (s *MainSuite) TestRegisterCommands(c *gc.C) {
	stub := &gitjujutesting.Stub{}
	extraNames := []string{"cmd-a", "cmd-b"}
	for i := range extraNames {
		name := extraNames[i]
		RegisterCommand(func() cmd.Command {
			return &stubCommand{
				stub: stub,
				info: &cmd.Info{
					Name: name,
				},
			}
		})
	}

	registry := &stubRegistry{stub: stub}
	registry.names = append(registry.names, "help", "version") // implicit
	registerCommands(registry, testing.Context(c))
	sort.Strings(registry.names)

	expected := make([]string, len(commandNames))
	copy(expected, commandNames)
	expected = append(expected, extraNames...)
	sort.Strings(expected)
	c.Check(registry.names, jc.DeepEquals, expected)
}

type commands []cmd.Command

func (r *commands) Register(c cmd.Command) {
	*r = append(*r, c)
}

func (r *commands) RegisterDeprecated(c cmd.Command, check cmd.DeprecationCheck) {
	if !check.Obsolete() {
		*r = append(*r, c)
	}
}

func (r *commands) RegisterSuperAlias(name, super, forName string, check cmd.DeprecationCheck) {
	// Do nothing.
}

func (s *MainSuite) TestModelCommands(c *gc.C) {
	var commands commands
	registerCommands(&commands, testing.Context(c))
	// There should not be any ModelCommands registered.
	// ModelCommands must be wrapped using modelcmd.Wrap.
	for _, cmd := range commands {
		c.Logf("%v", cmd.Info().Name)
		c.Check(cmd, gc.Not(gc.FitsTypeOf), modelcmd.ModelCommand(&bootstrapCommand{}))
	}
}

func (s *MainSuite) TestAllCommandsPurposeDocCapitalization(c *gc.C) {
	// Verify each command that:
	// - the Purpose field is not empty
	// - if set, the Doc field either begins with the name of the
	// command or and uppercase letter.
	//
	// The first makes Purpose a required documentation. Also, makes
	// both "help commands"'s output and "help <cmd>"'s header more
	// uniform. The second makes the Doc content either start like a
	// sentence, or start godoc-like by using the command's name in
	// lowercase.
	var commands commands
	registerCommands(&commands, testing.Context(c))
	for _, cmd := range commands {
		info := cmd.Info()
		c.Logf("%v", info.Name)
		purpose := strings.TrimSpace(info.Purpose)
		doc := strings.TrimSpace(info.Doc)
		comment := func(message string) interface{} {
			return gc.Commentf("command %q %s", info.Name, message)
		}

		c.Check(purpose, gc.Not(gc.Equals), "", comment("has empty Purpose"))
		if purpose != "" {
			prefix := string(purpose[0])
			c.Check(prefix, gc.Equals, strings.ToUpper(prefix),
				comment("expected uppercase first-letter Purpose"),
			)
		}
		if doc != "" && !strings.HasPrefix(doc, info.Name) {
			prefix := string(doc[0])
			c.Check(prefix, gc.Equals, strings.ToUpper(prefix),
				comment("expected uppercase first-letter Doc"),
			)
		}
	}
}
