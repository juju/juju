// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"fmt"
	"sort"
	"strings"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/internal/testing"
)

// WriteConfTest is used in tests to verify that the shell commands
// to write a service conf to disk are correct.
type WriteConfTest struct {
	Service  string
	DataDir  string
	Expected string
	Script   string
}

func (wct WriteConfTest) fileName() string {
	return fmt.Sprintf("'%s/%s.service'", wct.DataDir, wct.Service)
}

func (wct WriteConfTest) scriptName() string {
	return fmt.Sprintf("'%s/%s-exec-start.sh'", wct.DataDir, wct.Service)
}

// CheckCommands checks the given commands against the test's expectations.
func (wct WriteConfTest) CheckCommands(c *tc.C, commands []string) {
	if wct.Script != "" {
		wct.checkWriteExecScript(c, commands[:2])
		commands = commands[2:]
	}
	wct.checkWriteConf(c, commands)
}

func (wct WriteConfTest) checkWriteExecScript(c *tc.C, commands []string) {
	script := "#!/usr/bin/env bash\n\n" + wct.Script
	testing.CheckWriteFileCommand(c, commands[0], wct.scriptName(), script, nil)

	// Check the remaining commands.
	c.Check(commands[1:], jc.DeepEquals, []string{
		"chmod 0755 " + wct.scriptName(),
	})
}

func (wct WriteConfTest) checkWriteConf(c *tc.C, commands []string) {
	// This check must be done without regard to map order.
	parse := func(lines []string) interface{} {
		return parseConfSections(lines)
	}
	testing.CheckWriteFileCommand(c, commands[0], wct.fileName(), wct.Expected, parse)

	// Check the remaining commands.
	c.Check(commands[1:], jc.DeepEquals, []string{
		"/bin/systemctl link " + wct.fileName(),
		"/bin/systemctl daemon-reload",
		"/bin/systemctl enable " + wct.fileName(),
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
