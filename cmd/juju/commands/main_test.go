// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/gnuflag"
	"github.com/juju/tc"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/application"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/featureflag"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
)

type MainSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = tc.Suite(&MainSuite{})

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

func versionHelpText() string {
	return helpText(newVersionCommand(), "juju version")
}

func syncToolsHelpText() string {
	return helpText(newSyncAgentBinaryCommand(), "juju sync-agent-binary")
}

func (s *MainSuite) TestRunMain(c *tc.C) {
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
		summary: "juju sync-agent-binary registered properly",
		args:    []string{"sync-agent-binary", "--help"},
		code:    0,
		out:     syncToolsHelpText(),
	}, {
		summary: "--version option after command is not changed to version command",
		args:    []string{"bootstrap", "--version"},
		code:    0,
		out:     "ERROR option provided but not defined: --version\n",
	}, {
		summary: "check command is identified as an unrecognised option after --version option",
		args:    []string{"--version", "bootstrap"},
		code:    0,
		out:     "ERROR unrecognized args: [\"bootstrap\"]\n",
	}, {
		summary: "juju help --version shows the same help as 'help version'",
		args:    []string{"help", "--version"},
		code:    0,
		out:     versionHelpText(),
	}, {
		summary: "juju --version --help shows the same help as 'help version'",
		args:    []string{"--version", "--help"},
		code:    0,
		out:     versionHelpText(),
	}} {
		c.Logf("test %d: %s", i, t.summary)
		out := badrun(c, t.code, t.args...)
		c.Assert(out, tc.Equals, t.out)
	}
}

func (s *MainSuite) TestActualRunJujuArgOrder(c *tc.C) {
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
		content, err := os.ReadFile(logpath)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(string(content), tc.Matches, "(.|\n)*running juju(.|\n)*command finished(.|\n)*")
		err = os.Remove(logpath)
		c.Assert(err, tc.ErrorIsNil)
	}
}

var commandNames = []string{
	"actions",
	"add-cloud",
	"add-credential",
	"add-k8s",
	"add-machine",
	"add-model",
	"add-secret-backend",
	"add-secret",
	"add-space",
	"add-ssh-key",
	"add-storage",
	"add-unit",
	"add-user",
	"attach-resource",
	"attach-storage",
	"autoload-credentials",
	"bind",
	"bootstrap",
	"cancel-task",
	"change-user-password",
	"charm-resources",
	"clouds",
	"config",
	"constraints",
	"consume",
	"controller-config",
	"controllers",
	"create-backup",
	"create-storage-pool",
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
	"documentation",
	"download-backup",
	"download",
	"enable-command",
	"enable-destroy-controller",
	"enable-ha",
	"enable-user",
	"exec",
	"export-bundle",
	"expose",
	"find-offers",
	"find",
	"firewall-rules",
	"grant-cloud",
	"grant-secret",
	"grant",
	"help",
	"help-action-commands",
	"help-hook-commands",
	"import-filesystem",
	"import-ssh-key",
	"info",
	"integrate",
	"kill-controller",
	"list-actions",
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
	"list-regions",
	"list-resources",
	"list-secret-backends",
	"list-secrets",
	"list-spaces",
	"list-ssh-keys",
	"list-storage-pools",
	"list-storage",
	"list-subnets",
	"list-users",
	"login",
	"logout",
	"machines",
	"migrate",
	"model-config",
	"model-constraints",
	"model-default",
	"model-defaults",
	"model-secret-backend",
	"models",
	"move-to-space",
	"offer",
	"offers",
	"operations",
	"refresh",
	"regions",
	"register",
	"relate", // alias for integrate
	"reload-spaces",
	"remove-application",
	"remove-cloud",
	"remove-credential",
	"remove-k8s",
	"remove-machine",
	"remove-offer",
	"remove-relation",
	"remove-saas",
	"remove-secret-backend",
	"remove-secret",
	"remove-space",
	"remove-ssh-key",
	"remove-storage-pool",
	"remove-storage",
	"remove-unit",
	"remove-user",
	"rename-space",
	"resolve",
	"resolved",
	"resources",
	"resume-relation",
	"retry-provisioning",
	"revoke-cloud",
	"revoke-secret",
	"revoke",
	"run",
	"scale-application",
	"scp",
	"secret-backends",
	"secrets",
	"set-constraints",
	"set-credential",
	"set-default-credentials",
	"set-default-region",
	"set-firewall-rule",
	"set-model-constraints",
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
	"show-secret-backend",
	"show-secret",
	"show-space",
	"show-status-log",
	"show-storage",
	"show-task",
	"show-unit",
	"show-user",
	"spaces",
	"ssh-keys",
	"ssh",
	"status",
	"storage-pools",
	"storage",
	"subnets",
	"suspend-relation",
	"switch",
	"sync-agent-binary",
	"trust",
	"unexpose",
	"unregister",
	"update-cloud",
	"update-credential",
	"update-credentials",
	"update-k8s",
	"update-public-clouds",
	"update-secret-backend",
	"update-secret",
	"update-storage-pool",
	"upgrade-controller",
	"upgrade-model",
	"users",
	"version",
	"whoami",
}

// optionalFeatures are feature flags that impact registration of commands.
var optionalFeatures = []string{}

// These are the commands that are behind the `devFeatures`.
var commandNamesBehindFlags = set.NewStrings(
	"list-secrets", "secrets", "show-secret",
)

func (s *MainSuite) TestHelpCommands(c *tc.C) {
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
	c.Assert(unknown, tc.DeepEquals, set.NewStrings())
	missing := cmdSet.Difference(registered)
	c.Assert(missing, tc.DeepEquals, set.NewStrings())

	// 2. Enable development features, and test again.
	cmdSet = cmdSet.Union(commandNamesBehindFlags)
	setFeatureFlags(strings.Join(optionalFeatures, ","))
	registered = getHelpCommandNames(c)
	unknown = registered.Difference(cmdSet)
	c.Assert(unknown, tc.DeepEquals, set.NewStrings())
	missing = cmdSet.Difference(registered)
	c.Assert(missing.IsEmpty(), tc.IsTrue)
}

func getHelpCommandNames(c *tc.C) set.Strings {
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

func (s *MainSuite) TestHelpGlobalOptions(c *tc.C) {
	// Check that we have correctly registered all the topics
	// by checking the help output.
	out := badrun(c, 0, "help", "global-options")
	c.Assert(out, tc.Matches, `Global Options

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
	c.Assert(len(flags), tc.Equals, len(globalFlags))
	for i, line := range flags {
		c.Assert(line, tc.Matches, globalFlags[i])
	}
}

func (s *MainSuite) TestRegisterCommands(c *tc.C) {
	stub := &testhelpers.Stub{}

	registry := &stubRegistry{stub: stub}
	registry.names = append(registry.names, "help")          // implicit
	registry.names = append(registry.names, "documentation") //implicit
	registerCommands(registry)
	sort.Strings(registry.names)

	expected := make([]string, len(commandNames))
	copy(expected, commandNames)
	sort.Strings(expected)
	c.Check(registry.names, tc.DeepEquals, expected)
}

func (s *MainSuite) TestRegisterCommandsWhitelist(c *tc.C) {
	stubRegistry := &stubRegistry{stub: &testhelpers.Stub{}}
	registry := jujuCommandRegistry{
		commandRegistry: stubRegistry,
		whitelist:       set.NewStrings("status"),
		excluded:        set.NewStrings(),
	}
	registerCommands(registry)
	c.Assert(stubRegistry.names, tc.SameContents, []string{"status"})
}

func (s *MainSuite) TestRegisterCommandsEmbedded(c *tc.C) {
	store := jujuclienttesting.MinimalStore()
	stubRegistry := &stubRegistry{stub: &testhelpers.Stub{}}
	registry := jujuCommandRegistry{
		commandRegistry: stubRegistry,
		embedded:        true,
		store:           store,
		excluded:        set.NewStrings(),
	}
	stubCmd := &stubCommand{
		stub: &testhelpers.Stub{},
		info: &cmd.Info{
			Name: "test",
		},
	}
	registry.Register(stubCmd)
	c.Assert(stubRegistry.names, tc.SameContents, []string{"test"})
	c.Assert(stubCmd.Embedded, tc.IsTrue)
	c.Assert(stubCmd.ClientStore(), tc.DeepEquals, store)
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

func (s *MainSuite) TestModelCommands(c *tc.C) {
	var commands commands
	registerCommands(&commands)
	// There should not be any ModelCommands registered.
	// ModelCommands must be wrapped using modelcmd.Wrap.
	for _, command := range commands {
		c.Logf("%v", command.Info().Name)
		c.Check(command, tc.Not(tc.FitsTypeOf), modelcmd.ModelCommand(&bootstrapCommand{}))
	}
}

func (s *MainSuite) TestAllCommandsPurpose(c *tc.C) {
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
			return tc.Commentf("command %q %s", info.Name, message)
		}

		c.Check(purpose, tc.Not(tc.Equals), "", comment("has empty Purpose"))
		if purpose != "" {
			prefix := string(purpose[0])
			c.Check(prefix, tc.Equals, strings.ToUpper(prefix),
				comment("expected uppercase first-letter Purpose"))
			c.Check(strings.HasSuffix(purpose, "."), tc.IsTrue,
				comment("is missing full stop in Purpose"))
		}
		if doc != "" && !strings.HasPrefix(doc, info.Name) {
			prefix := string(doc[0])
			c.Check(prefix, tc.Equals, strings.ToUpper(prefix),
				comment("expected uppercase first-letter Doc"),
			)
		}
	}
}
