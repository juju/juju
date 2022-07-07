// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/collections/set"
	"github.com/juju/featureflag"
	"github.com/juju/gnuflag"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujucloud "github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/application"
	"github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/cmd/modelcmd"
	jujuos "github.com/juju/juju/core/os"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/testing"
)

type MainSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	jujutesting.PatchExecHelper
}

var _ = gc.Suite(&MainSuite{})

func helpText(command cmd.Command, name string) string {
	buff := &bytes.Buffer{}
	info := command.Info()
	info.Name = name
	f := gnuflag.NewFlagSetWithFlagKnownAs(info.Name, gnuflag.ContinueOnError, cmd.FlagAlias(command, "option"))
	command.SetFlags(f)

	superJuju := jujucmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:        "juju",
		FlagKnownAs: "option",
		Log:         jujucmd.DefaultLog,
	})
	superF := gnuflag.NewFlagSetWithFlagKnownAs("juju", gnuflag.ContinueOnError, "option")
	superJuju.SetFlags(superF)

	buff.Write(info.HelpWithSuperFlags(superF, f))
	return buff.String()
}

func deployHelpText() string {
	return helpText(application.NewDeployCommand(), "juju deploy")
}
func configHelpText() string {
	return helpText(application.NewConfigCommand(), "juju config")
}

func syncToolsHelpText() string {
	return helpText(newSyncToolsCommand(), "juju sync-agent-binaries")
}

func (s *MainSuite) TestRunMain(c *gc.C) {
	jujuclienttesting.SetupMinimalFileStore(c)

	missingCommandMessage := func(wanted, actual string) string {
		return fmt.Sprintf("ERROR %s\n", NotFoundCommand{
			ArgName:  wanted,
			CmdName:  actual,
			HelpHint: `See "juju --help"`,
		}.Error())
	}

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
		out:     missingCommandMessage("foo", "find"),
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
		summary: "unknown command with match",
		args:    []string{"discombobulate"},
		code:    1,
		out:     missingCommandMessage("discombobulate", "dashboard"),
	}, {
		summary: "unknown command",
		args:    []string{"pseudopseudohypoparathyroidism"},
		code:    1,
		out:     "ERROR unrecognized command: juju pseudopseudohypoparathyroidism\n",
	}, {
		summary: "unknown option before command",
		args:    []string{"--cheese", "bootstrap"},
		code:    2,
		out:     "ERROR option provided but not defined: --cheese\n",
	}, {
		summary: "unknown option after command",
		args:    []string{"bootstrap", "--cheese"},
		code:    2,
		out:     "ERROR option provided but not defined: --cheese\n",
	}, {
		summary: "known option, but specified before command",
		args:    []string{"--model", "blah", "bootstrap"},
		code:    2,
		out:     "ERROR option provided but not defined: --model\n",
	}, {
		summary: "juju sync-agent-binaries registered properly",
		args:    []string{"sync-agent-binaries", "--help"},
		code:    0,
		out:     syncToolsHelpText(),
	}, {
		summary: "check version command returns a fully qualified version string",
		args:    []string{"version"},
		code:    0,
		out:     testing.CurrentVersion(c).String() + "\n",
	}, {
		summary: "check --version command returns a fully qualified version string",
		args:    []string{"--version"},
		code:    0,
		out:     testing.CurrentVersion(c).String() + "\n",
	}} {
		c.Logf("test %d: %s", i, t.summary)
		out := badrun(c, t.code, t.args...)
		c.Assert(out, gc.Equals, t.out)
	}
}

func (s *MainSuite) TestActualRunJujuArgOrder(c *gc.C) {
	s.PatchEnvironment(osenv.JujuControllerEnvKey, "current-controller")
	s.PatchEnvironment(osenv.JujuModelEnvKey, "current")
	logpath := filepath.Join(c.MkDir(), "log")
	tests := [][]string{
		{"--log-file", logpath, "--debug", "help"}, // global flags before
		{"help", "--log-file", logpath, "--debug"}, // after
		{"--log-file", logpath, "help", "--debug"}, // mixed
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

func (s *MainSuite) TestNoWarn2xFirstRun(c *gc.C) {
	// Code should only rnu on ubuntu series, so patch out the series for
	// when non-ubuntu OSes run this test.
	s.PatchValue(&jujuos.HostOS, func() jujuos.OSType { return jujuos.Ubuntu })

	argChan := make(chan []string, 1)
	// we shouldn't actually be running anything, but if we do, this will
	// provide some consistent results.
	execCommand := s.GetExecCommand(jujutesting.PatchExecConfig{
		Stdout: "1.25.0-trusty-amd64",
		Args:   argChan,
	})
	stub := &jujutesting.Stub{}
	s.PatchValue(&cloud.NewUpdatePublicCloudsCommand, func() cmd.Command {
		return &stubCommand{stub: stub}
	})

	// remove the new juju-home.
	err := os.RemoveAll(osenv.JujuXDGDataHomeDir())
	c.Assert(err, jc.ErrorIsNil)

	// create fake (empty) old juju home.
	path := c.MkDir()
	s.PatchEnvironment("JUJU_HOME", path)

	var code int
	stdout, stderr := jujutesting.CaptureOutput(c, func() {
		code = jujuMain{
			execCommand: execCommand,
		}.Run([]string{"juju", "version"})
	})

	c.Assert(code, gc.Equals, 0)

	assertNoArgs(c, argChan)
	c.Check(string(stderr), gc.Equals, `
Since Juju 3 is being run for the first time, it has downloaded the latest public cloud information.`[1:]+"\n")
	checkVersionOutput(c, string(stdout))
}

func (s *MainSuite) assertRunUpdateCloud(c *gc.C, expectedCalled bool) {
	argChan := make(chan []string, 1)
	execCommand := s.GetExecCommand(jujutesting.PatchExecConfig{
		Stdout: "1.25.0-trusty-amd64",
		Args:   argChan,
	})

	called := false
	s.PatchValue(&cloud.FetchAndMaybeUpdatePublicClouds,
		func(access cloud.PublicCloudsAccessDetails, updateClient bool) (map[string]jujucloud.Cloud, string, error) {
			called = true
			return nil, "", nil
		})
	var code int
	jujutesting.CaptureOutput(c, func() {
		code = jujuMain{
			execCommand: execCommand,
		}.Run([]string{"juju", "version"})
	})
	c.Assert(code, gc.Equals, 0)
	c.Assert(called, gc.Equals, expectedCalled)
}

func (s *MainSuite) TestFirstRunUpdateCloud(c *gc.C) {
	// remove the juju-home.
	err := os.RemoveAll(osenv.JujuXDGDataHomeDir())
	c.Assert(err, jc.ErrorIsNil)
	s.assertRunUpdateCloud(c, true)
}

func (s *MainSuite) TestRunNoUpdateCloud(c *gc.C) {
	s.assertRunUpdateCloud(c, false)
}

func checkVersionOutput(c *gc.C, output string) {
	ver := testing.CurrentVersion(c)
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
	"add-k8s",
	"add-machine",
	"add-model",
	"add-relation", // alias for 'integrate'
	"add-space",
	"add-ssh-key",
	"add-storage",
	"add-subnet",
	"add-unit",
	"add-user",
	"agree",
	"agreements",
	"attach",
	"attach-resource",
	"attach-storage",
	"autoload-credentials",
	"bind",
	"bootstrap",
	"budget",
	"cached-images",
	"cancel-task",
	"change-user-password",
	"charm-resources",
	"clouds",
	"collect-metrics",
	"config",
	"consume",
	"controller-config",
	"controllers",
	"create-backup",
	"create-storage-pool",
	"create-wallet",
	"credentials",
	"dashboard",
	"debug-code",
	"debug-hook",
	"debug-hooks",
	"debug-log",
	"default-credential",
	"default-region",
	"deploy",
	"destroy-controller",
	"destroy-model",
	"detach-storage",
	"diff-bundle",
	"disable-command",
	"disable-user",
	"disabled-commands",
	"download",
	"download-backup",
	"enable-command",
	"enable-destroy-controller",
	"enable-ha",
	"enable-user",
	"exec",
	"export-bundle",
	"expose",
	"find",
	"find-offers",
	"firewall-rules",
	"get-constraints",
	"get-model-constraints",
	"grant",
	"grant-cloud",
	"help",
	"help-tool",
	"hook-tool",
	"hook-tools",
	"import-filesystem",
	"import-ssh-key",
	"info",
	"integrate",
	"kill-controller",
	"list-actions",
	"list-agreements",
	"list-cached-images",
	"list-charm-resources",
	"list-clouds",
	"list-controllers",
	"list-credentials",
	"list-disabled-commands",
	"list-firewall-rules",
	"list-machines",
	"list-models",
	"list-offers",
	"list-operations",
	"list-payloads",
	"list-plans",
	"list-regions",
	"list-resources",
	"list-spaces",
	"list-ssh-keys",
	"list-storage",
	"list-storage-pools",
	"list-subnets",
	"list-users",
	"list-wallets",
	"login",
	"logout",
	"machines",
	"metrics",
	"migrate",
	"model-config",
	"model-default",
	"model-defaults",
	"models",
	"move-to-space",
	"offer",
	"offers",
	"operations",
	"payloads",
	"plans",
	"refresh",
	"regions",
	"register",
	"relate", // alias for integrate
	"reload-spaces",
	"remove-application",
	"remove-cached-images",
	"remove-cloud",
	"remove-consumed-application",
	"remove-credential",
	"remove-k8s",
	"remove-machine",
	"remove-offer",
	"remove-relation",
	"remove-saas",
	"remove-space",
	"remove-ssh-key",
	"remove-storage",
	"remove-storage-pool",
	"remove-unit",
	"remove-user",
	"rename-space",
	"resolved",
	"resolve",
	"resources",
	"resume-relation",
	"retry-provisioning",
	"revoke",
	"revoke-cloud",
	"run",
	"scale-application",
	"scp",
	"set-credential",
	"set-constraints",
	"set-default-credential",
	"set-default-region",
	"set-firewall-rule",
	"set-meter-status",
	"set-model-constraints",
	"set-plan",
	"set-series",
	"set-wallet",
	"show-action",
	"show-application",
	"show-cloud",
	"show-controller",
	"show-credential",
	"show-credentials",
	"show-machine",
	"show-model",
	"show-offer",
	"show-operation",
	"show-status",
	"show-status-log",
	"show-storage",
	"show-space",
	"show-task",
	"show-unit",
	"show-user",
	"show-wallet",
	"sla",
	"spaces",
	"ssh",
	"ssh-keys",
	"status",
	"storage",
	"storage-pools",
	"subnets",
	"suspend-relation",
	"switch",
	"sync-agent-binaries",
	"sync-tools",
	"trust",
	"unexpose",
	"unregister",
	"update-cloud",
	"update-k8s",
	"update-public-clouds",
	"update-credential",
	"update-credentials",
	"update-storage-pool",
	"upgrade-charm",
	"upgrade-controller",
	"upgrade-juju",
	"upgrade-model",
	"upgrade-series",
	"users",
	"version",
	"wait-for",
	"wallets",
	"whoami",
}

// optionalFeatures are feature flags that impact registration of commands.
var optionalFeatures = []string{
	feature.ActionsV2,
	feature.Secrets,
}

// These are the commands that are behind the `devFeatures`.
var commandNamesBehindFlags = set.NewStrings(
	"run", "show-task", "operations", "list-operations", "show-operation",
	"list-secrets", "secrets",
)

func (s *MainSuite) TestHelpCommands(c *gc.C) {
	// Check that we have correctly registered all the commands
	// by checking the help output.
	// First check default commands, and then check commands that are
	// activated by feature flags.

	// remove features behind dev_flag for the first test
	// since they are not enabled.
	// NB there are no such commands as of now, but leave this step
	// for when we add some again.
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
	setFeatureFlags(strings.Join(optionalFeatures, ","))
	registered = getHelpCommandNames(c)
	unknown = registered.Difference(cmdSet)
	c.Assert(unknown, jc.DeepEquals, set.NewStrings())
	missing = cmdSet.Difference(registered)
	c.Assert(missing.IsEmpty(), jc.IsTrue)
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
	stub := &jujutesting.Stub{}

	registry := &stubRegistry{stub: stub}
	registry.names = append(registry.names, "help") // implicit
	registerCommands(registry)
	sort.Strings(registry.names)

	expected := make([]string, len(commandNames))
	copy(expected, commandNames)
	sort.Strings(expected)
	c.Check(registry.names, jc.DeepEquals, expected)
}

func (s *MainSuite) TestRegisterCommandsWhitelist(c *gc.C) {
	stubRegistry := &stubRegistry{stub: &jujutesting.Stub{}}
	registry := jujuCommandRegistry{
		commandRegistry: stubRegistry,
		whitelist:       set.NewStrings("show-status"),
		excluded:        set.NewStrings(),
	}
	registerCommands(registry)
	c.Assert(stubRegistry.names, jc.SameContents, []string{"show-status", "status"})
}

func (s *MainSuite) TestRegisterCommandsEmbedded(c *gc.C) {
	store := jujuclienttesting.MinimalStore()
	stubRegistry := &stubRegistry{stub: &jujutesting.Stub{}}
	registry := jujuCommandRegistry{
		commandRegistry: stubRegistry,
		embedded:        true,
		store:           store,
		excluded:        set.NewStrings(),
	}
	stubCmd := &stubCommand{
		stub: &jujutesting.Stub{},
		info: &cmd.Info{
			Name: "test",
		},
	}
	registry.Register(stubCmd)
	c.Assert(stubRegistry.names, jc.SameContents, []string{"test"})
	c.Assert(stubCmd.Embedded, jc.IsTrue)
	c.Assert(stubCmd.ClientStore(), jc.DeepEquals, store)
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
	registerCommands(&commands)
	// There should not be any ModelCommands registered.
	// ModelCommands must be wrapped using modelcmd.Wrap.
	for _, command := range commands {
		c.Logf("%v", command.Info().Name)
		c.Check(command, gc.Not(gc.FitsTypeOf), modelcmd.ModelCommand(&bootstrapCommand{}))
	}
}

func (s *MainSuite) TestAllCommandsPurpose(c *gc.C) {
	// Verify each command that:
	// - the Purpose field is not empty.
	// - the Purpose ends with a full stop.
	// - if set, the Doc field either begins with the name of the
	// command or and uppercase letter.
	//
	// This:
	// - makes Purpose a required documentation.
	// - Standardises Purpose formatting across all commands.
	// - Brings "help commands"'s output in line with "help <cmd>"'s header.
	// - Makes the Doc content either start like a sentence, or start
	//   godoc-like by using the command's name in lowercase.
	var commands commands
	registerCommands(&commands)
	for _, command := range commands {
		info := command.Info()
		purpose := strings.TrimSpace(info.Purpose)
		doc := strings.TrimSpace(info.Doc)
		comment := func(message string) interface{} {
			return gc.Commentf("command %q %s", info.Name, message)
		}

		c.Check(purpose, gc.Not(gc.Equals), "", comment("has empty Purpose"))
		if purpose != "" {
			prefix := string(purpose[0])
			c.Check(prefix, gc.Equals, strings.ToUpper(prefix),
				comment("expected uppercase first-letter Purpose"))
			c.Check(strings.HasSuffix(purpose, "."), jc.IsTrue,
				comment("is missing full stop in Purpose"))
		}
		if doc != "" && !strings.HasPrefix(doc, info.Name) {
			prefix := string(doc[0])
			c.Check(prefix, gc.Equals, strings.ToUpper(prefix),
				comment("expected uppercase first-letter Doc"),
			)
		}
	}
}
