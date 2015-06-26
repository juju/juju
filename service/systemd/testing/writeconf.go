// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"fmt"
	"sort"
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

// WriteConfTest is used in tests to verify that the shell commands
// to write a service conf to disk are correct.
type WriteConfTest struct {
	Service  string
	DataDir  string
	Expected string
	Script   string
}

func (wct WriteConfTest) dirname() string {
	return fmt.Sprintf("'%s/init/%s'", wct.DataDir, wct.Service)
}

func (wct WriteConfTest) filename() string {
	return fmt.Sprintf("'%[1]s/init/%[2]s/%[2]s.service'", wct.DataDir, wct.Service)
}

func (wct WriteConfTest) scriptname() string {
	return fmt.Sprintf("'%s/init/%s/exec-start.sh'", wct.DataDir, wct.Service)
}

func (wct WriteConfTest) servicename() string {
	return fmt.Sprintf("%s.service", wct.Service)
}

// CheckCommands checks the given commands against the test's expectations.
func (wct WriteConfTest) CheckCommands(c *gc.C, commands []string) {
	c.Check(commands[0], gc.Equals, "mkdir -p "+wct.dirname())
	commands = commands[1:]
	if wct.Script != "" {
		wct.checkWriteExecScript(c, commands[:2])
		commands = commands[2:]
	}
	wct.checkWriteConf(c, commands)
}

func (wct WriteConfTest) CheckInstallAndStartCommands(c *gc.C, commands []string) {
	wct.CheckCommands(c, commands[:len(commands)-1])
	c.Check(commands[len(commands)-1], gc.Equals, "/bin/systemctl start "+wct.servicename())
}

func (wct WriteConfTest) checkWriteExecScript(c *gc.C, commands []string) {
	script := "#!/usr/bin/env bash\n\n" + wct.Script
	testing.CheckWriteFileCommand(c, commands[0], wct.scriptname(), script, nil)

	// Check the remaining commands.
	c.Check(commands[1:], jc.DeepEquals, []string{
		"chmod 0755 " + wct.scriptname(),
	})
}

func (wct WriteConfTest) checkWriteConf(c *gc.C, commands []string) {
	// This check must be done without regard to map order.
	parse := func(lines []string) interface{} {
		return parseConfSections(lines)
	}
	testing.CheckWriteFileCommand(c, commands[0], wct.filename(), wct.Expected, parse)

	// Check the remaining commands.
	c.Check(commands[1:], jc.DeepEquals, []string{
		"/bin/systemctl link " + wct.filename(),
		"/bin/systemctl daemon-reload",
		"/bin/systemctl enable " + wct.filename(),
	})
}

// parseConfSections is a poor man's ini parser.
func parseConfSections(lines []string) map[string][]string {
	sections := make(map[string][]string)

	var section string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			if section != "" {
				sort.Strings(sections[section])
			}
			section = line[1 : len(line)-1]
			sections[section] = nil
		} else {
			sections[section] = append(sections[section], line)
		}
	}
	if section != "" {
		sort.Strings(sections[section])
	}

	return sections
}
