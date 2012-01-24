package main_test

import (
	"flag"
	"fmt"
	. "launchpad.net/gocheck"
	main "launchpad.net/juju/go/cmd/juju"
	"os"
	"os/exec"
	"path"
	"strings"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type CommandSuite struct{}

var _ = Suite(&CommandSuite{})

// Note that we're using the plain "flag" package here, no need or use for gnuflag
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

func (s *CommandSuite) TestActualRunBadJujuArg(c *C) {
	lines := badrun(c, 2, "--cheese", "bootstrap")
	c.Assert(lines[0], Equals, "flag provided but not defined: --cheese")
	c.Assert(lines[1], Equals, "usage: juju [options] <command> ...")
}

func (s *CommandSuite) TestActualRunBadBootstrapArg(c *C) {
	lines := badrun(c, 2, "bootstrap", "--cheese")
	c.Assert(lines[0], Equals, "flag provided but not defined: --cheese")
	c.Assert(lines[1], Equals, "usage: juju bootstrap [options]")
}

func (s *CommandSuite) TestActualRunCreatesLog(c *C) {
	// Induce failure to load environments
	os.Setenv("HOME", "")
	logpath := path.Join(c.MkDir(), "log")
	badrun(c, 1, "--log-file", logpath, "bootstrap")
	_, err := os.Stat(logpath)
	c.Assert(err, IsNil)
}

func (s *CommandSuite) TestActualRunLogNotInterspersed(c *C) {
	logpath := path.Join(c.MkDir(), "log")
	lines := badrun(c, 2, "bootstrap", "--log-file", logpath)
	c.Assert(lines[0], Equals, "flag provided but not defined: --log-file")
	c.Assert(lines[1], Equals, "usage: juju bootstrap [options]")
	_, err := os.Stat(logpath)
	c.Assert(err, NotNil)
}
