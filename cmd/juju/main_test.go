package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	. "launchpad.net/gocheck"
	_ "launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/testing"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	stdtesting "testing"
)

func TestPackage(t *stdtesting.T) {
	testing.ZkTestPackage(t)
}

type MainSuite struct{}

var _ = Suite(&MainSuite{})

var flagRunMain = flag.Bool("run-main", false, "Run the application's main function for recursive testing")

// Reentrancy point for testing (something as close as possible to) the juju
// tool itself.
func TestRunMain(t *stdtesting.T) {
	if *flagRunMain {
		Main(flag.Args())
	}
}

func badrun(c *C, exit int, cmd ...string) []string {
	args := append([]string{"-test.run", "TestRunMain", "-run-main", "--", "juju"}, cmd...)
	ps := exec.Command(os.Args[0], args...)
	output, err := ps.CombinedOutput()
	if exit != 0 {
		c.Assert(err, ErrorMatches, fmt.Sprintf("exit status %d", exit))
	}
	return strings.Split(string(output), "\n")
}

func assertError(c *C, lines []string, err string) {
	c.Assert(lines[len(lines)-2], Equals, err)
	c.Assert(lines[len(lines)-1], Equals, "")
}

func (s *MainSuite) TestActualRunNoCommand(c *C) {
	// Check error when command not specified
	lines := badrun(c, 2)
	c.Assert(lines[0], Equals, "usage: juju [options] <command> ...")
	assertError(c, lines, "error: no command specified")
}

func (s *MainSuite) TestActualRunBadCommand(c *C) {
	// Check error when command unknown
	lines := badrun(c, 2, "discombobulate")
	c.Assert(lines[0], Equals, "usage: juju [options] <command> ...")
	assertError(c, lines, "error: unrecognized command: juju discombobulate")
}

func (s *MainSuite) TestActualRunBadJujuArg(c *C) {
	// Check error when unknown option specified before command
	lines := badrun(c, 2, "--cheese", "bootstrap")
	c.Assert(lines[0], Equals, "usage: juju [options] <command> ...")
	assertError(c, lines, "error: flag provided but not defined: --cheese")
}

func (s *MainSuite) TestActualRunBadBootstrapArg(c *C) {
	// Check error when unknown option specified after command
	lines := badrun(c, 2, "bootstrap", "--cheese")
	c.Assert(lines[0], Equals, "usage: juju bootstrap [options]")
	assertError(c, lines, "error: flag provided but not defined: --cheese")
}

func (s *MainSuite) TestActualRunSubcmdArgWorksNotInterspersed(c *C) {
	// Check error when otherwise-valid option specified before command
	lines := badrun(c, 2, "--environment", "erewhon", "bootstrap")
	c.Assert(lines[0], Equals, "usage: juju [options] <command> ...")
	assertError(c, lines, "error: flag provided but not defined: --environment")
}

var brokenConfig = `
environments:
    one:
        type: dummy
        zookeeper: false
        broken: true
        authorized-keys: i-am-a-key
`

// Induce failure to load environments and hence break Run.
func breakJuju(c *C) (string, func()) {
	home := os.Getenv("HOME")
	path := c.MkDir()
	os.Setenv("HOME", path)

	jujuDir := filepath.Join(path, ".juju")
	err := os.Mkdir(jujuDir, 0777)
	c.Assert(err, IsNil)

	err = ioutil.WriteFile(filepath.Join(jujuDir, "environments.yaml"), []byte(brokenConfig), 0666)
	c.Assert(err, IsNil)

	msg := "broken environment"
	return msg, func() { os.Setenv("HOME", home) }
}

func (s *MainSuite) TestActualRunJujuArgsBeforeCommand(c *C) {
	// Check global args work when specified before command
	msg, unbreak := breakJuju(c)
	defer unbreak()
	logpath := filepath.Join(c.MkDir(), "log")
	lines := badrun(c, 1, "--log-file", logpath, "--verbose", "--debug", "bootstrap")
	assertError(c, lines, "error: "+msg)
	content, err := ioutil.ReadFile(logpath)
	c.Assert(err, IsNil)
	fullmsg := fmt.Sprintf(`.* JUJU:DEBUG juju bootstrap command failed: %s\n`, msg)
	c.Assert(string(content), Matches, fullmsg)
}

func (s *MainSuite) TestActualRunJujuArgsAfterCommand(c *C) {
	// Check global args work when specified after command
	msg, unbreak := breakJuju(c)
	defer unbreak()
	logpath := filepath.Join(c.MkDir(), "log")
	lines := badrun(c, 1, "bootstrap", "--log-file", logpath, "--verbose", "--debug")
	assertError(c, lines, "error: "+msg)
	content, err := ioutil.ReadFile(logpath)
	c.Assert(err, IsNil)
	fullmsg := fmt.Sprintf(`.* JUJU:DEBUG juju bootstrap command failed: %s\n`, msg)
	c.Assert(string(content), Matches, fullmsg)
}

var commandNames = []string{
	"bootstrap",
	"deploy",
	"destroy-environment",
}

func (s *MainSuite) TestHelp(c *C) {
	// Check that we have correctly registered all the commands
	// by checking the help output.

	lines := badrun(c, 0, "-help")
	c.Assert(lines[0], Matches, `usage: juju .*`)

	for ; len(lines) > 0; lines = lines[1:] {
		if lines[0] == "commands:" {
			break
		}
	}
	c.Assert(lines, Not(HasLen), 0)

	var names []string
	for lines = lines[1:]; len(lines) > 0; lines = lines[1:] {
		f := strings.Fields(lines[0])
		if len(f) == 0 {
			continue
		}
		c.Assert(f, Not(HasLen), 0)
		names = append(names, f[0])
	}
	sort.Strings(names)
	c.Assert(names, DeepEquals, commandNames)
}
