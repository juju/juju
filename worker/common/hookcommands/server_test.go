// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package hookcommands_test

import (
	"github.com/juju/cmd"
	"github.com/juju/juju/worker/common/hookcommands"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/common/hookcommands/hooktesting"
)

type NewCommandSuite struct {
	hooktesting.ContextSuite
}

var _ = gc.Suite(&NewCommandSuite{})

type command struct {
	cmd.Command
}

func (s *NewCommandSuite) TestNewCommand(c *gc.C) {
	ctx := s.ContextSuite.NewHookContext(c)
	f := func() map[string]hookcommands.NewCommandFunc {
		return map[string]hookcommands.NewCommandFunc{
			"command": func(hookcommands.Context) (cmd.Command, error) {
				return &command{}, nil
			},
		}
	}
	com, err := hookcommands.NewCommand(ctx, "command", f)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(com, gc.NotNil)
	_, err = hookcommands.NewCommand(ctx, "invalid", f)
	c.Assert(err, gc.ErrorMatches, "unknown command.*")
}
