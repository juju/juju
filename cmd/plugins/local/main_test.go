// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local_test

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/cmd/plugins/local"
	coretesting "launchpad.net/juju-core/testing"
)

type mainSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&mainSuite{})

func (*mainSuite) TestRegisteredCommands(c *gc.C) {
	expectedSubcommands := []string{
		"help",
		// TODO: add some as they get registered
	}
	plugin := local.JujuLocalPlugin()
	ctx, err := coretesting.RunCommand(c, plugin, "help", "commands")
	c.Assert(err, gc.IsNil)

	lines := strings.Split(coretesting.Stdout(ctx), "\n")
	var names []string
	for _, line := range lines {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		names = append(names, f[0])
	}
	// The names should be output in alphabetical order, so don't sort.
	c.Assert(names, gc.DeepEquals, expectedSubcommands)
}

func (s *mainSuite) TestRunAsRootCallsFuncIfRoot(c *gc.C) {
	s.PatchValue(local.CheckIfRoot, func() bool { return true })
	called := false
	call := func(*cmd.Context) error {
		called = true
		return nil
	}
	args := []string{"ignored..."}
	err := local.RunAsRoot("juju-magic", args, coretesting.Context(c), call)
	c.Assert(err, gc.IsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *mainSuite) TestRunAsRootCallsSudoIfNotRoot(c *gc.C) {
	s.PatchValue(local.CheckIfRoot, func() bool { return false })
	testing.PatchExecutableAsEchoArgs(c, s, "sudo")
	// the command needs to be in the path...
	testing.PatchExecutableAsEchoArgs(c, s, "juju-magic")
	magicPath, err := exec.LookPath("juju-magic")
	c.Assert(err, gc.IsNil)
	callIgnored := func(*cmd.Context) error {
		panic("unreachable")
	}
	args := []string{"passed"}
	context := coretesting.Context(c)
	err = local.RunAsRoot("juju-magic", args, context, callIgnored)
	c.Assert(err, gc.IsNil)
	expected := fmt.Sprintf("sudo \"--preserve-env\" %q \"passed\"\n", magicPath)
	c.Assert(coretesting.Stdout(context), gc.Equals, expected)
}
