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
	c.Assert(lines[0], Equals, "ERROR: "+msg)
}

func (s *MainSuite) TestParseErrors(c *C) {
	// Check all the obvious parse errors
	checkMessage(c, "no command specified")
	checkMessage(c, "unrecognised command: jujud cavitate", "cavitate")
	msgf := "flag provided but not defined: --cheese"
	checkMessage(c, msgf, "--cheese", "cavitate")

	cmds := []string{"initzk", "unit", "machine", "provisioning"}
	msgz := `invalid value "localhost:2181,zk" for flag --zookeeper-servers: "zk" is not a valid zookeeper address`
	for _, cmd := range cmds {
		checkMessage(c, msgf, cmd, "--cheese")
		checkMessage(c, msgz, cmd, "--zookeeper-servers", "localhost:2181,zk")
	}

	msga := "unrecognised args: [toastie]"
	checkMessage(c, msga, "initzk",
		"--zookeeper-servers", "zk:2181",
		"--instance-id", "ii",
		"--env-type", "et",
		"toastie")
	checkMessage(c, msga, "unit",
		"--zookeeper-servers", "localhost:2181,zk:2181",
		"--session-file", "sf",
		"--unit-name", "un/0",
		"toastie")
	checkMessage(c, msga, "machine",
		"--zookeeper-servers", "zk:2181",
		"--session-file", "sf",
		"--machine-id", "42",
		"toastie")
	checkMessage(c, msga, "provisioning",
		"--zookeeper-servers", "127.0.0.1:2181",
		"--session-file", "sf",
		"toastie")
}
