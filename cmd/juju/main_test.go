// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	stdtesting "testing"

	jc "github.com/juju/testing/checkers"
	"launchpad.net/gnuflag"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/cmd/envcmd"
	"launchpad.net/juju-core/juju/osenv"
	_ "launchpad.net/juju-core/provider/dummy"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/version"
)

func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

type MainSuite struct {
	testing.FakeJujuHomeSuite
}

var _ = gc.Suite(&MainSuite{})

var (
	flagRunMain = flag.Bool("run-main", false, "Run the application's main function for recursive testing")
)

// Reentrancy point for testing (something as close as possible to) the juju
// tool itself.
func TestRunMain(t *stdtesting.T) {
	if *flagRunMain {
		Main(flag.Args())
	}
}

func badrun(c *gc.C, exit int, args ...string) string {
	localArgs := append([]string{"-test.run", "TestRunMain", "-run-main", "--", "juju"}, args...)
	ps := exec.Command(os.Args[0], localArgs...)
	ps.Env = append(os.Environ(), osenv.JujuHomeEnvKey+"="+osenv.JujuHome())
	output, err := ps.CombinedOutput()
	c.Logf("command output: %q", output)
	if exit != 0 {
		c.Assert(err, gc.ErrorMatches, fmt.Sprintf("exit status %d", exit))
	}
	return string(output)
}

func helpText(command cmd.Command, name string) string {
	buff := &bytes.Buffer{}
	info := command.Info()
	info.Name = name
	f := gnuflag.NewFlagSet(info.Name, gnuflag.ContinueOnError)
	command.SetFlags(f)
	buff.Write(info.Help(f))
	return buff.String()
}

func deployHelpText() string {
	return helpText(envcmd.Wrap(&DeployCommand{}), "juju deploy")
}

func syncToolsHelpText() string {
	return helpText(envcmd.Wrap(&SyncToolsCommand{}), "juju sync-tools")
}

func (s *MainSuite) TestRunMain(c *gc.C) {
	// The test array structure needs to be inline here as some of the
	// expected values below use deployHelpText().  This constructs the deploy
	// command and runs gets the help for it.  When the deploy command is
	// setting the flags (which is needed for the help text) it is accessing
	// osenv.JujuHome(), which panics if SetJujuHome has not been called.
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
		out:     strings.TrimLeft(helpBasics, "\n"),
	}, {
		summary: "juju help is the same as juju",
		args:    []string{"help"},
		code:    0,
		out:     strings.TrimLeft(helpBasics, "\n"),
	}, {
		summary: "juju --help works too",
		args:    []string{"--help"},
		code:    0,
		out:     strings.TrimLeft(helpBasics, "\n"),
	}, {
		summary: "juju help basics is the same as juju",
		args:    []string{"help", "basics"},
		code:    0,
		out:     strings.TrimLeft(helpBasics, "\n"),
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
		args:    []string{"--environment", "blah", "bootstrap"},
		code:    2,
		out:     "error: flag provided but not defined: --environment\n",
	}, {
		summary: "juju sync-tools registered properly",
		args:    []string{"sync-tools", "--help"},
		code:    0,
		out:     syncToolsHelpText(),
	}, {
		summary: "check version command registered properly",
		args:    []string{"version"},
		code:    0,
		out:     version.Current.String() + "\n",
	},
	} {
		c.Logf("test %d: %s", i, t.summary)
		out := badrun(c, t.code, t.args...)
		c.Assert(out, gc.Equals, t.out)
	}
}

func (s *MainSuite) TestActualRunJujuArgOrder(c *gc.C) {
	logpath := filepath.Join(c.MkDir(), "log")
	tests := [][]string{
		{"--log-file", logpath, "--debug", "env"}, // global flags before
		{"env", "--log-file", logpath, "--debug"}, // after
		{"--log-file", logpath, "env", "--debug"}, // mixed
	}
	for i, test := range tests {
		c.Logf("test %d: %v", i, test)
		badrun(c, 0, test...)
		content, err := ioutil.ReadFile(logpath)
		c.Assert(err, gc.IsNil)
		c.Assert(string(content), gc.Matches, "(.|\n)*running juju(.|\n)*command finished(.|\n)*")
		err = os.Remove(logpath)
		c.Assert(err, gc.IsNil)
	}
}

var commandNames = []string{
	"add-machine",
	"add-relation",
	"add-unit",
	"api-endpoints",
	"authorised-keys", // alias for authorized-keys
	"authorized-keys",
	"bootstrap",
	"debug-hooks",
	"debug-log",
	"deploy",
	"destroy-environment",
	"destroy-machine",
	"destroy-relation",
	"destroy-service",
	"destroy-unit",
	"ensure-availability",
	"env", // alias for switch
	"expose",
	"generate-config", // alias for init
	"get",
	"get-constraints",
	"get-env", // alias for get-environment
	"get-environment",
	"help",
	"help-tool",
	"init",
	"publish",
	"remove-machine",  // alias for destroy-machine
	"remove-relation", // alias for destroy-relation
	"remove-service",  // alias for destroy-service
	"remove-unit",     // alias for destroy-unit
	"resolved",
	"retry-provisioning",
	"run",
	"scp",
	"set",
	"set-constraints",
	"set-env", // alias for set-environment
	"set-environment",
	"ssh",
	"stat", // alias for status
	"status",
	"switch",
	"sync-tools",
	"terminate-machine", // alias for destroy-machine
	"unexpose",
	"unset",
	"unset-env", // alias for unset-environment
	"unset-environment",
	"upgrade-charm",
	"upgrade-juju",
	"user",
	"version",
}

func (s *MainSuite) TestHelpCommands(c *gc.C) {
	// Check that we have correctly registered all the commands
	// by checking the help output.
	defer osenv.SetJujuHome(osenv.SetJujuHome(c.MkDir()))
	out := badrun(c, 0, "help", "commands")
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
	c.Assert(names, jc.DeepEquals, commandNames)
}

var topicNames = []string{
	"azure-provider",
	"basics",
	"commands",
	"constraints",
	"ec2-provider",
	"global-options",
	"glossary",
	"hpcloud-provider",
	"local-provider",
	"logging",
	"openstack-provider",
	"plugins",
	"topics",
}

func (s *MainSuite) TestHelpTopics(c *gc.C) {
	// Check that we have correctly registered all the topics
	// by checking the help output.
	defer osenv.SetJujuHome(osenv.SetJujuHome(c.MkDir()))
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
	defer osenv.SetJujuHome(osenv.SetJujuHome(c.MkDir()))
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

type commands []cmd.Command

func (r *commands) Register(c cmd.Command) {
	*r = append(*r, c)
}

func (s *MainSuite) TestEnvironCommands(c *gc.C) {
	var commands commands
	registerCommands(&commands, testing.Context(c))
	// There should not be any EnvironCommands registered.
	// EnvironCommands must be wrapped using envcmd.Wrap.
	for _, cmd := range commands {
		c.Logf("%v", cmd.Info().Name)
		c.Check(cmd, gc.Not(gc.FitsTypeOf), envcmd.EnvironCommand(&BootstrapCommand{}))
	}
}
