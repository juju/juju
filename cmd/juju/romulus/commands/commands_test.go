// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands_test

import (
	stdtesting "testing"

	"github.com/juju/cmd"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/romulus/commands"
)

type commandSuite struct{}

var _ = gc.Suite(&commandSuite{})

type mockRegister struct {
	commands []string
}

func (m *mockRegister) Register(command cmd.Command) {
	m.commands = append(m.commands, command.Info().Name)
}

func TestAll(t *stdtesting.T) {
	gc.TestingT(t)
}

func (s *commandSuite) TestRegister(c *gc.C) {
	m := &mockRegister{}
	commands.RegisterAll(m)
	c.Assert(m.commands, gc.DeepEquals, []string{
		"agree",
		"agreements",
		"allocate",
		"budgets",
		"create-budget",
		"plans",
		"set-budget",
		"set-plan",
		"show-budget",
		"update-allocation",
	})
}
