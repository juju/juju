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
	c.Assert(lines[1], Equals, "usage: juju [options] <command> ...")
}

func (s *MainSuite) TestActualRunBadCommand(c *C) {
	lines := badrun(c, 2, "discombobulate")
	c.Assert(lines[0], Equals, "unrecognised command: discombobulate")
	c.Assert(lines[1], Equals, "usage: juju [options] <command> ...")
}

func (s *MainSuite) TestActualRunBadJujuArg(c *C) {
	lines := badrun(c, 2, "--cheese", "bootstrap")
	c.Assert(lines[0], Equals, "flag provided but not defined: --cheese")
	c.Assert(lines[1], Equals, "usage: juju [options] <command> ...")
}

func (s *MainSuite) TestActualRunBadBootstrapArg(c *C) {
	lines := badrun(c, 2, "bootstrap", "--cheese")
	c.Assert(lines[0], Equals, "flag provided but not defined: --cheese")
	c.Assert(lines[1], Equals, "usage: juju bootstrap [options]")
}

func (s *MainSuite) TestActualRunCreatesLog(c *C) {
	// Induce failure to load environments and hence break bootstrap
	os.Setenv("HOME", "")
	logpath := path.Join(c.MkDir(), "log")
	badrun(c, 1, "--log-file", logpath, "--verbose", "bootstrap")
	_, err := os.Stat(logpath)
	c.Assert(err, IsNil)
}

func (s *MainSuite) TestActualRunVerboseNotInterspersed(c *C) {
	lines := badrun(c, 2, "bootstrap", "--verbose")
	c.Assert(lines[0], Equals, "flag provided but not defined: --verbose")
	c.Assert(lines[1], Equals, "usage: juju bootstrap [options]")
}

func (s *MainSuite) TestActualRunLogNotInterspersed(c *C) {
	logpath := path.Join(c.MkDir(), "log")
	lines := badrun(c, 2, "bootstrap", "--log-file", logpath)
	c.Assert(lines[0], Equals, "flag provided but not defined: --log-file")
	c.Assert(lines[1], Equals, "usage: juju bootstrap [options]")
	_, err := os.Stat(logpath)
	c.Assert(err, NotNil)
}
