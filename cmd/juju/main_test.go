package main_test

import (
	"flag"
	"fmt"
	"io/ioutil"
	. "launchpad.net/gocheck"
	main "launchpad.net/juju/go/cmd/juju"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type MainSuite struct{}

var _ = Suite(&MainSuite{})

var flagRunMain = flag.Bool("run-main", false, "Run the application's main function for recursive testing")

// Reentrancy point for testing (something as close as possible to) the juju
// tool itself.
func TestRunMain(t *testing.T) {
	if *flagRunMain {
		main.Main(flag.Args())
	}
}

func badrun(c *C, exit int, cmd ...string) []string {
	args := append([]string{"-test.run", "TestRunMain", "-run-main", "--", "juju"}, cmd...)
	ps := exec.Command(os.Args[0], args...)
	output, err := ps.CombinedOutput()
	c.Assert(err, ErrorMatches, fmt.Sprintf("exit status %d", exit))
	return strings.Split(string(output), "\n")
}

func (s *MainSuite) TestActualRunNoCommand(c *C) {
	// Check error when command not specified
	lines := badrun(c, 2)
	c.Assert(lines[0], Equals, "no command specified")
	c.Assert(lines[1], Equals, "usage: juju <command> [options] ...")
}

func (s *MainSuite) TestActualRunBadCommand(c *C) {
	// Check error when command unknown
	lines := badrun(c, 2, "discombobulate")
	c.Assert(lines[0], Equals, "unrecognised command: discombobulate")
	c.Assert(lines[1], Equals, "usage: juju <command> [options] ...")
}

func (s *MainSuite) TestActualRunBadJujuArg(c *C) {
	// Check error when unknown option specified before command
	lines := badrun(c, 2, "--cheese", "bootstrap")
	c.Assert(lines[0], Equals, "flag provided but not defined: --cheese")
	c.Assert(lines[1], Equals, "usage: juju <command> [options] ...")
}

func (s *MainSuite) TestActualRunBadBootstrapArg(c *C) {
	// Check error when unknown option specified after command
	lines := badrun(c, 2, "bootstrap", "--cheese")
	c.Assert(lines[0], Equals, "flag provided but not defined: --cheese")
	c.Assert(lines[1], Equals, "usage: juju bootstrap [options]")
}

func (s *MainSuite) TestActualRunSubcmdArgWorksNotInterspersed(c *C) {
	// Check error when otherwise-valid option specified before command
	lines := badrun(c, 2, "--environment", "erewhon", "bootstrap")
	c.Assert(lines[0], Equals, "flag provided but not defined: --environment")
	c.Assert(lines[1], Equals, "usage: juju <command> [options] ...")
}

// Induce failure to load environments and hence break Run.
func breakJuju(c *C) (string, func()) {
	home := os.Getenv("HOME")
	path := c.MkDir()
	os.Setenv("HOME", path)
	msg := fmt.Sprintf("open %s/.juju/environments.yaml: no such file or directory", path)
	return msg, func() { os.Setenv("HOME", home) }
}

func (s *MainSuite) TestActualRunJujuArgsBeforeCommand(c *C) {
	// Check global args work when specified before command
	msg, unbreak := breakJuju(c)
	defer unbreak()
	logpath := filepath.Join(c.MkDir(), "log")
	lines := badrun(c, 1, "--log-file", logpath, "--verbose", "--debug", "bootstrap")
	c.Assert(lines[0], Equals, msg)
	content, err := ioutil.ReadFile(logpath)
	c.Assert(err, IsNil)
	fullmsg := fmt.Sprintf(`.* JUJU:DEBUG bootstrap command failed: %s\n`, msg)
	c.Assert(string(content), Matches, fullmsg)
}

func (s *MainSuite) TestActualRunJujuArgsAfterCommand(c *C) {
	// Check global args work when specified after command
	msg, unbreak := breakJuju(c)
	defer unbreak()
	logpath := filepath.Join(c.MkDir(), "log")
	lines := badrun(c, 1, "bootstrap", "--log-file", logpath, "--verbose", "--debug")
	c.Assert(lines[0], Equals, msg)
	content, err := ioutil.ReadFile(logpath)
	c.Assert(err, IsNil)
	fullmsg := fmt.Sprintf(`.* JUJU:DEBUG bootstrap command failed: %s\n`, msg)
	c.Assert(string(content), Matches, fullmsg)
}
