// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package hooks_test

import (
	"github.com/juju/cmd"
	"github.com/juju/juju/worker/common/hooks"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	hookstesting "github.com/juju/juju/worker/common/hooks/testing"
)

type NewCommandSuite struct {
	hookstesting.ContextSuite
}

var _ = gc.Suite(&NewCommandSuite{})

type command struct {
	cmd.Command
}

func (s *NewCommandSuite) TestNewCommand(c *gc.C) {
	ctx := s.ContextSuite.NewHookContext(c)
	f := func() map[string]hooks.NewCommandFunc {
		return map[string]hooks.NewCommandFunc{
			"command": func(hooks.Context) (cmd.Command, error) {
				return &command{}, nil
			},
		}
	}
	com, err := hooks.NewCommand(ctx, "command", f)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(com, gc.NotNil)
	_, err = hooks.NewCommand(ctx, "invalid", f)
	c.Assert(err, gc.ErrorMatches, "unknown command.*")
}
