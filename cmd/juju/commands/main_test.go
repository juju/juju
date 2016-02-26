// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
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
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/juju/helptopics"
	"github.com/juju/juju/cmd/juju/service"
	"github.com/juju/juju/cmd/modelcmd"
	cmdtesting "github.com/juju/juju/cmd/testing"
	"github.com/juju/juju/juju/osenv"
	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type MainSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&MainSuite{})

func deployHelpText() string {
	return cmdtesting.HelpText(service.NewDeployCommand(), "juju deploy")
}
func setconfigHelpText() string {
	return cmdtesting.HelpText(service.NewSetCommand(), "juju set-config")
}

func syncToolsHelpText() string {
	return cmdtesting.HelpText(newSyncToolsCommand(), "juju sync-tools")
}

func blockHelpText() string {
	return cmdtesting.HelpText(block.NewSuperBlockCommand(), "juju block")
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
		summary: "no params shows help",
		args:    []string{},
		code:    0,
		out:     strings.TrimLeft(helptopics.Basics, "\n"),
	}, {
		summary: "juju help is the same as juju",
		args:    []string{"help"},
		code:    0,
		out:     strings.TrimLeft(helptopics.Basics, "\n"),
	}, {
		summary: "juju --help works too",
		args:    []string{"--help"},
		code:    0,
		out:     strings.TrimLeft(helptopics.Basics, "\n"),
	}, {
		summary: "juju help basics is the same as juju",
		args:    []string{"help", "basics"},
		code:    0,
		out:     strings.TrimLeft(helptopics.Basics, "\n"),
	}, {
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
		summary: "juju --help set-config shows the same help as 'help set-config'",
		args:    []string{"--help", "set-config"},
		code:    0,
		out:     setconfigHelpText(),
	}, {
		summary: "juju set-config --help shows the same help as 'help set-config'",
		args:    []string{"set-config", "--help"},
		code:    0,
		out:     setconfigHelpText(),
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
			Number: version.Current,
			Arch:   arch.HostArch(),
			Series: series.HostSeries(),
		}.String() + "\n",
	}, {
		summary: "check block command registered properly",
		args:    []string{"block", "-h"},
		code:    0,
		out:     blockHelpText(),
	}, {
		summary: "check unblock command registered properly",
		args:    []string{"unblock"},
		code:    0,
		out:     "error: must specify one of [destroy-model | remove-object | all-changes] to unblock\n",
	},
	} {
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
		{"--log-file", logpath, "--debug", "list-controllers"}, // global flags before
		{"list-controllers", "--log-file", logpath, "--debug"}, // after
		{"--log-file", logpath, "list-controllers", "--debug"}, // mixed
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

var commandNames = []string{
	"action",
	"add-cloud",
	"add-machine",
	"add-machines",
	"add-relation",
	"add-ssh-key",
	"add-ssh-keys",
	"add-unit",
	"add-units",
	"agree",
	"allocate",
	"add-space",
	"add-storage",
	"add-subnet",
	"add-user",
	"autoload-credentials",
	"backups",
	"block",
	"bootstrap",
	"cached-images",
	"change-user-password",
	"charm",
	"collect-metrics",
	"create-backup",
	"create-budget",
	"create-model",
	"debug-hooks",
	"debug-log",
	"debug-metrics",
	"deploy",
	"destroy-controller",
	"destroy-model",
	"destroy-relation",
	"destroy-service",
	"destroy-unit",
	"disable-user",
	"enable-ha",
	"enable-user",
	"expose",
	"get-config",
	"get-configs",
	"get-constraints",
	"get-model-config",
	"get-model-constraints",
	"help",
	"help-tool",
	"import-ssh-key",
	"import-ssh-keys",
	"kill-controller",
	"list-actions",
	"list-all-blocks",
	"list-budgets",
	"list-clouds",
	"list-controllers",
	"list-credentials",
	"list-machine",
	"list-machines",
	"list-models",
	"list-plans",
	"list-shares",
	"list-ssh-key",
	"list-ssh-keys",
	"list-spaces",
	"list-storage",
	"list-users",
	"machine",
	"machines",
	"publish",
	"register",
	"remove-all-blocks",
	"remove-machine",
	"remove-machines",
	"remove-relation", // alias for destroy-relation
	"remove-service",  // alias for destroy-service
	"remove-ssh-key",
	"remove-ssh-keys",
	"remove-unit", // alias for destroy-unit
	"resolved",
	"restore-backup",
	"retry-provisioning",
	"run",
	"run-action",
	"scp",
	"set-budget",
	"set-config",
	"set-configs",
	"set-constraints",
	"set-meter-status",
	"set-model-config",
	"set-model-constraints",
	"set-plan",
	"share-model",
	"ssh-key",
	"ssh-keys",
	"show-action-output",
	"show-action-status",
	"show-budget",
	"show-cloud",
	"show-controller",
	"show-controllers",
	"show-machine",
	"show-machines",
	"show-status",
	"show-storage",
	"show-user",
	"space",
	"ssh",
	"status",
	"status-history",
	"storage",
	"subnet",
	"switch",
	"switch-user",
	"sync-tools",
	"unblock",
	"unexpose",
	"update-allocation",
	"unset-model-config",
	"unshare-model",
	"upgrade-charm",
	"upgrade-juju",
	"version",
}

func (s *MainSuite) TestHelpCommands(c *gc.C) {
	defer osenv.SetJujuXDGDataHome(osenv.SetJujuXDGDataHome(c.MkDir()))

	// Check that we have correctly registered all the commands
	// by checking the help output.
	// First check default commands, and then check commands that are
	// activated by feature flags.

	// Here we can add feature flags for any commands we want to hide by default.
	devFeatures := []string{}

	// remove features behind dev_flag for the first test
	// since they are not enabled.
	cmdSet := set.NewStrings(commandNames...)
	for _, feature := range devFeatures {
		cmdSet.Remove(feature)
	}

	// 1. Default Commands. Disable all features.
	setFeatureFlags("")
	// Use sorted values here so we can better see what is wrong.
	registered := getHelpCommandNames(c)
	unknown := registered.Difference(cmdSet)
	c.Assert(unknown, jc.DeepEquals, set.NewStrings())
	missing := cmdSet.Difference(registered)
	c.Assert(missing, jc.DeepEquals, set.NewStrings())

	// 2. Enable development features, and test again.
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

var topicNames = []string{
	"azure-provider",
	"basics",
	"commands",
	"constraints",
	"controllers",
	"ec2-provider",
	"global-options",
	"glossary",
	"hpcloud-provider",
	"juju",
	"logging",
	"maas-provider",
	"openstack-provider",
	"placement",
	"plugins",
	"spaces",
	"topics",
	"users",
}

func (s *MainSuite) TestHelpTopics(c *gc.C) {
	// Check that we have correctly registered all the topics
	// by checking the help output.
	defer osenv.SetJujuXDGDataHome(osenv.SetJujuXDGDataHome(c.MkDir()))
	out := badrun(c, 0, "help", "topics")
	lines := strings.Split(out, "\n")
	var names []string
	for _, line := range lines {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		names = append(names, f[0])
	}
	// The names should be output in alphabetical order, so don't sort.
	c.Assert(names, gc.DeepEquals, topicNames)
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
	defer osenv.SetJujuXDGDataHome(osenv.SetJujuXDGDataHome(c.MkDir()))
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
	// - the Purpose field is not empty and begins with a lowercase
	// letter, and,
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
			c.Check(prefix, gc.Equals, strings.ToLower(prefix),
				comment("expected lowercase first-letter Purpose"),
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

func (s *MainSuite) TestTwoDotOhDeprecation(c *gc.C) {
	check := twoDotOhDeprecation("the replacement")

	// first check pre-2.0
	s.PatchValue(&version.Current, version.MustParse("1.26.4"))
	deprecated, replacement := check.Deprecated()
	c.Check(deprecated, jc.IsFalse)
	c.Check(replacement, gc.Equals, "")
	c.Check(check.Obsolete(), jc.IsFalse)

	s.PatchValue(&version.Current, version.MustParse("2.0-alpha1"))
	deprecated, replacement = check.Deprecated()
	c.Check(deprecated, jc.IsTrue)
	c.Check(replacement, gc.Equals, "the replacement")
	c.Check(check.Obsolete(), jc.IsFalse)

	s.PatchValue(&version.Current, version.MustParse("3.0-alpha1"))
	deprecated, replacement = check.Deprecated()
	c.Check(deprecated, jc.IsTrue)
	c.Check(replacement, gc.Equals, "the replacement")
	c.Check(check.Obsolete(), jc.IsTrue)
}
