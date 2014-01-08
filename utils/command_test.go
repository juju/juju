// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	"io/ioutil"
	"path/filepath"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/utils"
)

type EnvironmentPatcher interface {
	PatchEnvironment(name, value string)
}

func patchExecutable(patcher EnvironmentPatcher, dir, execName, script string) {
	patcher.PatchEnvironment("PATH", dir)
	filename := filepath.Join(dir, execName)
	ioutil.WriteFile(filename, []byte(script), 0755)
}

type commandSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&commandSuite{})

func (s *commandSuite) TestRunCommandCombinesOutput(c *gc.C) {
	content := `#!/bin/bash --norc
echo stdout
echo stderr 1>&2
`
	patchExecutable(s, c.MkDir(), "test-output", content)
	output, err := utils.RunCommand("test-output")
	c.Assert(err, gc.IsNil)
	c.Assert(output, gc.Equals, "stdout\nstderr\n")
}

func (s *commandSuite) TestRunCommandNonZeroExit(c *gc.C) {
	content := `#!/bin/bash --norc
echo stdout
exit 42
`
	patchExecutable(s, c.MkDir(), "test-output", content)
	output, err := utils.RunCommand("test-output")
	c.Assert(err, gc.ErrorMatches, `exit status 42`)
	c.Assert(output, gc.Equals, "stdout\n")
}
