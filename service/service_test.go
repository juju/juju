// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package service_test

import (
	"fmt"
	"strings"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/service"
)

type serviceSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&serviceSuite{})

// checkShellSwitch examines the contents a fragment of shell script that implements a switch
// using an if, elif, else chain. It tests that each command in expectedCommands is used once
// and that the whole script fragment ends with "else exit 1". The order of commands in
// script doesn't matter.
func checkShellSwitch(c *gc.C, script string, expectedCommands []string) {
	cmds := strings.Split(script, "\n")

	// Ensure that we terminate the if, elif, else chain correctly
	last := len(cmds) - 1
	c.Check(cmds[last-1], gc.Equals, "else exit 1")
	c.Check(cmds[last], gc.Equals, "fi")

	// First line must start with if
	c.Check(cmds[0][0:3], gc.Equals, "if ")

	// Further lines must start with elif. Convert them to if <statement>
	for i := 1; i < last-1; i++ {
		c.Check(cmds[i][0:5], gc.Equals, "elif ")
		cmds[i] = cmds[i][2:]
	}

	c.Check(cmds[0:last-1], jc.SameContents, expectedCommands)
}

func (*serviceSuite) TestListServicesCommand(c *gc.C) {
	cmd := service.ListServicesCommand()

	line := `if [[ "$(cat /proc/1/cmdline | awk '{print $1}')" == "%s" ]]; then %s`
	upstart := `sudo initctl list | awk '{print $1}' | sort | uniq`
	systemd := `/bin/systemctl list-unit-files --no-legend --no-page -t service` +
		` | grep -o -P '^\w[\S]*(?=\.service)'`

	lines := []string{
		fmt.Sprintf(line, "/sbin/init", upstart),
		fmt.Sprintf(line, "/sbin/upstart", upstart),
		fmt.Sprintf(line, "/sbin/systemd", systemd),
		fmt.Sprintf(line, "/bin/systemd", systemd),
		fmt.Sprintf(line, "/lib/systemd/systemd", systemd),
	}

	checkShellSwitch(c, cmd, lines)
}
