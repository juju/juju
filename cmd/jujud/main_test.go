package main

import (
	"flag"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/testing"
	"os"
	"os/exec"
	"strings"
	stdtesting "testing"
)

func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

type MainSuite struct{}

var _ = Suite(&MainSuite{})

var flagRunMain = flag.Bool("run-main", false, "Run the application's main function for recursive testing")

// Reentrancy point for testing (something as close as possible to) the jujud
// tool itself.
func TestRunMain(t *stdtesting.T) {
	if *flagRunMain {
		Main(flag.Args())
	}
}

func checkMessage(c *C, msg string, cmd ...string) {
	args := append([]string{"-test.run", "TestRunMain", "-run-main", "--", "jujud"}, cmd...)
	c.Logf("check %#v", args)
	ps := exec.Command(os.Args[0], args...)
	output, err := ps.CombinedOutput()
	c.Logf(string(output))
	c.Assert(err, ErrorMatches, "exit status 2")
	lines := strings.Split(string(output), "\n")
	c.Assert(lines[len(lines)-2], Equals, "error: "+msg)
}

func (s *MainSuite) TestParseErrors(c *C) {
	// Check all the obvious parse errors
	checkMessage(c, "no command specified")
	checkMessage(c, "unrecognized command: jujud cavitate", "cavitate")
	msgf := "flag provided but not defined: --cheese"
	checkMessage(c, msgf, "--cheese", "cavitate")

	cmds := []string{"bootstrap-state", "unit", "machine", "provisioning"}
	msgz := `invalid value "localhost:37017,srv" for flag --state-servers: "srv" is not a valid state server address`
	for _, cmd := range cmds {
		checkMessage(c, msgf, cmd, "--cheese")
		checkMessage(c, msgz, cmd, "--state-servers", "localhost:37017,srv")
	}

	msga := `unrecognized args: ["toastie"]`
	checkMessage(c, msga,
		"bootstrap-state",
		"--state-servers", "st:37017",
		"--instance-id", "ii",
		"--env-config", b64yaml{"blah": "blah"}.encode(),
		"toastie")
	checkMessage(c, msga, "unit",
		"--state-servers", "localhost:37017,st:37017",
		"--unit-name", "un/0",
		"toastie")
	checkMessage(c, msga, "machine",
		"--state-servers", "st:37017",
		"--machine-id", "42",
		"toastie")
	checkMessage(c, msga, "provisioning",
		"--state-servers", "127.0.0.1:37017",
		"toastie")
}
