package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"launchpad.net/gnuflag"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs/config"
	_ "launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/version"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	stdtesting "testing"
)

func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

type MainSuite struct{}

var _ = Suite(&MainSuite{})

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

func badrun(c *C, exit int, args ...string) string {
	localArgs := append([]string{"-test.run", "TestRunMain", "-run-main", "--", "juju"}, args...)
	ps := exec.Command(os.Args[0], localArgs...)
	ps.Env = append(os.Environ(), "JUJU_HOME="+config.JujuHome())
	output, err := ps.CombinedOutput()
	if exit != 0 {
		c.Assert(err, ErrorMatches, fmt.Sprintf("exit status %d", exit))
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
	return helpText(&DeployCommand{}, "juju deploy")
}

func syncToolsHelpText() string {
	return helpText(&SyncToolsCommand{}, "juju sync-tools")
}

var runMainTests = []struct {
	summary string
	args    []string
	code    int
	out     string
}{
	{
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
		out:     "error: unknown command or topic for foo\n",
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
		code:    2,
		out:     "error: unrecognized command: juju discombobulate\n",
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
}

func (s *MainSuite) TestRunMain(c *C) {
	defer config.SetJujuHome(config.SetJujuHome(c.MkDir()))
	for i, t := range runMainTests {
		c.Logf("test %d: %s", i, t.summary)
		out := badrun(c, t.code, t.args...)
		c.Assert(out, Equals, t.out)
	}
}

var brokenConfig = `
environments:
    one:
        type: dummy
        state-server: false
        authorized-keys: i-am-a-key
        broken: %s
`

// breakJuju forces the dummy environment to return an error when
// environMethod is called.
func breakJuju(c *C, environMethod string) (msg string) {
	yaml := fmt.Sprintf(brokenConfig, environMethod)
	err := ioutil.WriteFile(config.JujuHomePath("environments.yaml"), []byte(yaml), 0666)
	c.Assert(err, IsNil)

	return fmt.Sprintf("dummy.%s is broken", environMethod)
}

func (s *MainSuite) TestActualRunJujuArgsBeforeCommand(c *C) {
	defer testing.MakeFakeHomeNoEnvironments(c, "one").Restore()
	// Check global args work when specified before command
	msg := breakJuju(c, "Bootstrap")
	logpath := filepath.Join(c.MkDir(), "log")
	out := badrun(c, 1, "--log-file", logpath, "--verbose", "--debug", "bootstrap")
	c.Assert(out, Equals, "error: "+msg+"\n")
	content, err := ioutil.ReadFile(logpath)
	c.Assert(err, IsNil)
	fullmsg := fmt.Sprintf(`.*\n.*ERROR JUJU:juju:bootstrap juju bootstrap command failed: %s\n`, msg)
	c.Assert(string(content), Matches, fullmsg)
}

func (s *MainSuite) TestActualRunJujuArgsAfterCommand(c *C) {
	defer testing.MakeFakeHomeNoEnvironments(c, "one").Restore()
	// Check global args work when specified after command
	msg := breakJuju(c, "Bootstrap")
	logpath := filepath.Join(c.MkDir(), "log")
	out := badrun(c, 1, "bootstrap", "--log-file", logpath, "--verbose", "--debug")
	c.Assert(out, Equals, "error: "+msg+"\n")
	content, err := ioutil.ReadFile(logpath)
	c.Assert(err, IsNil)
	fullmsg := fmt.Sprintf(`.*\n.*ERROR JUJU:juju:bootstrap juju bootstrap command failed: %s\n`, msg)
	c.Assert(string(content), Matches, fullmsg)
}

var commandNames = []string{
	"add-relation",
	"add-unit",
	"bootstrap",
	"debug-log",
	"deploy",
	"destroy-environment",
	"destroy-machine",
	"destroy-relation",
	"destroy-service",
	"destroy-unit",
	"expose",
	"generate-config", // alias for init
	"get",
	"get-constraints",
	"help",
	"init",
	"publish",
	"remove-relation", // alias for destroy-relation
	"remove-unit",     // alias for destroy-unit
	"resolved",
	"scp",
	"set",
	"set-constraints",
	"ssh",
	"stat", // alias for status
	"status",
	"sync-tools",
	"terminate-machine", // alias for destroy-machine
	"unexpose",
	"upgrade-charm",
	"upgrade-juju",
	"version",
}

func (s *MainSuite) TestHelpCommands(c *C) {
	// Check that we have correctly registered all the commands
	// by checking the help output.
	defer config.SetJujuHome(config.SetJujuHome(c.MkDir()))
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
	c.Assert(names, DeepEquals, commandNames)
}

var topicNames = []string{
	"basics",
	"commands",
	"global-options",
	"topics",
}

func (s *MainSuite) TestHelpTopics(c *C) {
	// Check that we have correctly registered all the topics
	// by checking the help output.
	defer config.SetJujuHome(config.SetJujuHome(c.MkDir()))
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
	c.Assert(names, DeepEquals, topicNames)
}

var globalFlags = []string{
	"--debug .*",
	"-h, --help .*",
	"--log-file .*",
	"-v, --verbose .*",
}

func (s *MainSuite) TestHelpGlobalOptions(c *C) {
	// Check that we have correctly registered all the topics
	// by checking the help output.
	defer config.SetJujuHome(config.SetJujuHome(c.MkDir()))
	out := badrun(c, 0, "help", "global-options")
	c.Assert(out, Matches, `Global Options

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
	c.Assert(len(flags), Equals, len(globalFlags))
	for i, line := range flags {
		c.Assert(line, Matches, globalFlags[i])
	}
}
