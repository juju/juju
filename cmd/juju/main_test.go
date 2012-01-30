package main_test

import (
	"flag"
	"fmt"
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
	lines := badrun(c, 2)
	c.Assert(lines[0], Equals, "no command specified")
	c.Assert(lines[1], Equals, "usage: juju <command> [options] ...")
}

func (s *MainSuite) TestActualRunBadCommand(c *C) {
	lines := badrun(c, 2, "discombobulate")
	c.Assert(lines[0], Equals, "unrecognised command: discombobulate")
	c.Assert(lines[1], Equals, "usage: juju <command> [options] ...")
}

func (s *MainSuite) TestActualRunBadJujuArg(c *C) {
	lines := badrun(c, 2, "--cheese", "bootstrap")
	c.Assert(lines[0], Equals, "flag provided but not defined: --cheese")
	c.Assert(lines[1], Equals, "usage: juju <command> [options] ...")
}

func (s *MainSuite) TestActualRunBadBootstrapArg(c *C) {
	lines := badrun(c, 2, "bootstrap", "--cheese")
	c.Assert(lines[0], Equals, "flag provided but not defined: --cheese")
	c.Assert(lines[1], Equals, "usage: juju bootstrap [options]")
}

func (s *MainSuite) TestActualRunSubcmdArgWorksNotInterspersed(c *C) {
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

func (s *MainSuite) TestActualRunCreatesLog(c *C) {
	msg, unbreak := breakJuju(c)
	defer unbreak()
	logpath := filepath.Join(c.MkDir(), "log")
	lines := badrun(c, 1, "--logfile", logpath, "--verbose", "bootstrap")
	c.Assert(lines[0], Equals, msg)
	_, err := os.Stat(logpath)
	c.Assert(err, IsNil)
}

func (s *MainSuite) TestActualRunLogfileWorksInterspersed(c *C) {
	msg, unbreak := breakJuju(c)
	defer unbreak()
	logpath := filepath.Join(c.MkDir(), "log")
	lines := badrun(c, 1, "bootstrap", "--logfile", logpath)
	c.Assert(lines[0], Equals, msg)
	_, err := os.Stat(logpath)
	c.Assert(err, IsNil)
}

func (s *MainSuite) TestActualRunVerboseWorksInterspersed(c *C) {
	msg, unbreak := breakJuju(c)
	defer unbreak()
	lines := badrun(c, 1, "bootstrap", "--verbose")
	c.Assert(lines[0], Equals, msg)
}

func (s *MainSuite) TestActualRunDebugWorksInterspersed(c *C) {
	msg, unbreak := breakJuju(c)
	defer unbreak()
	lines := badrun(c, 1, "bootstrap", "--debug")
	c.Assert(lines[0], Equals, msg)
}
