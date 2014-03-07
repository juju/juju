// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local_test

import (
	"strings"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd/plugins/local"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/testbase"
)

type mainSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&mainSuite{})

func (*mainSuite) TestRegisteredCommands(c *gc.C) {
	expectedSubcommands := []string{
		"help",
		// TODO: add some as they get registered
	}
	plugin := local.JujuLocalPlugin()
	ctx, err := testing.RunCommand(c, plugin, []string{"help", "commands"})
	c.Assert(err, gc.IsNil)

	lines := strings.Split(testing.Stdout(ctx), "\n")
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
