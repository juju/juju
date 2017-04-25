// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	stdtesting "testing"

	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

// flagRunMain is used to indicate that the -run-main flag was used.
var flagRunMain = flag.Bool("run-main", false, "Run the application's main function for recursive testing")

type CmdSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&CmdSuite{})

func initSSHCommand(args ...string) (*sshCommand, error) {
	com := &sshCommand{}
	return com, cmdtesting.InitCommand(com, args)
}

func (*CmdSuite) TestSSHCommandInit(c *gc.C) {
	// missing args
	_, err := initSSHCommand()
	c.Assert(err, gc.ErrorMatches, "no target name specified")
}

func initSCPCommand(args ...string) (*scpCommand, error) {
	com := &scpCommand{}
	return com, cmdtesting.InitCommand(com, args)
}

func (*CmdSuite) TestSCPCommandInit(c *gc.C) {
	// missing args
	_, err := initSCPCommand()
	c.Assert(err, gc.ErrorMatches, "at least two arguments required")

	// not enough args
	_, err = initSCPCommand("mysql/0:foo")
	c.Assert(err, gc.ErrorMatches, "at least two arguments required")
}

// Reentrancy point for testing (something as close as possible to) the juju
// tool itself.
func TestRunMain(t *stdtesting.T) {
	if *flagRunMain {
		os.Exit(Main(flag.Args()))
	}
}

// badrun is used to run a command, check the exit code, and return the output.
func badrun(c *gc.C, exit int, args ...string) string {
	localArgs := append([]string{"-test.run", "TestRunMain", "-run-main", "--", "juju"}, args...)
	ps := exec.Command(os.Args[0], localArgs...)
	output, err := ps.CombinedOutput()
	c.Logf("command output: %q", output)
	if exit != 0 {
		c.Assert(err, gc.ErrorMatches, fmt.Sprintf("exit status %d", exit))
	}
	return string(output)
}
