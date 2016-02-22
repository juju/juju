// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmcmd_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/charmcmd"
)

type CharmSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&CharmSuite{})

func (s *CharmSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
}

// TODO(ericsnow) Copy some tests from cmd/juju/commands/main_test.go?

func (s *CharmSuite) Test(c *gc.C) {
	return
	// TODO(ericsnow) Finish!
	chCmd := charmcmd.NewSuperCommand()

	c.Check(chCmd, jc.DeepEquals, nil)
}
