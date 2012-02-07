package main_test

import (
	"flag"
	. "launchpad.net/gocheck"
	main "launchpad.net/juju/go/cmd/jujud"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type MainSuite struct{}

var _ = Suite(&MainSuite{})

var flagRunMain = flag.Bool("run-main", false, "Run the application's main function for recursive testing")

// Reentrancy point for testing (something as close as possible to) the jujud
// tool itself.
func TestRunMain(t *testing.T) {
	if *flagRunMain {
		main.Main(flag.Args())
	}
}

func checkMessage(c *C, msg string, cmd ...string) {
	args := append([]string{"-test.run", "TestRunMain", "-run-main", "--", "jujud"}, cmd...)
	ps := exec.Command(os.Args[0], args...)
	output, err := ps.CombinedOutput()
	c.Assert(err, ErrorMatches, "exit status 2")
	lines := strings.Split(string(output), "\n")
	c.Assert(lines[0], Equals, msg)
}

func (s *MainSuite) TestParseErrors(c *C) {
	// Check all the obvious parse errors
	checkMessage(c, "no command specified")
	checkMessage(c, "unrecognised command: cavitate", "cavitate")

	msgf := "flag provided but not defined: --cheese"
	checkMessage(c, msgf, "--cheese", "cavitate")
	checkMessage(c, msgf, "initzk", "--cheese")
	checkMessage(c, msgf, "unit", "--cheese")
	checkMessage(c, msgf, "machine", "--cheese")
	checkMessage(c, msgf, "provisioning", "--cheese")

	msga := "unrecognised args: [toastie]"
	checkMessage(c, msga, "initzk",
		"--zookeeper-servers", "zk",
		"--provider-type", "pt",
		"--instance-id", "ii",
		"toastie")
	checkMessage(c, msga, "unit",
		"--zookeeper-servers", "zk",
		"--session-file", "sf",
		"--unit-name", "un",
		"toastie")
	checkMessage(c, msga, "machine",
		"--zookeeper-servers", "zk",
		"--session-file", "sf",
		"--machine-id", "42",
		"toastie")
	checkMessage(c, msga, "provisioning",
		"--zookeeper-servers", "zk",
		"--session-file", "sf",
		"toastie")
}
