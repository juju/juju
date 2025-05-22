// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

// flagRunMain is used to indicate that the -run-main flag was used.
var flagRunMain = flag.Bool("run-main", false, "Run the application's main function for recursive testing")

type CmdSuite struct {
	testhelpers.IsolationSuite
}

func TestCmdSuite(t *testing.T) {
	tc.Run(t, &CmdSuite{})
}

// Reentrancy point for testing (something as close as possible to) the juju
// tool itself.
func TestRunMain(t *testing.T) {
	if *flagRunMain {
		os.Exit(Main(flag.Args()))
	}
}

// badrun is used to run a command, check the exit code, and return the output.
func badrun(c *tc.C, exit int, args ...string) string {
	localArgs := append([]string{"-test.run", "TestRunMain", "-run-main", "--", "juju"}, args...)
	ps := exec.Command(os.Args[0], localArgs...)
	output, err := ps.CombinedOutput()
	c.Logf("command output: %q", output)
	if exit != 0 {
		c.Assert(err, tc.ErrorMatches, fmt.Sprintf("exit status %d", exit))
	}
	return string(output)
}
