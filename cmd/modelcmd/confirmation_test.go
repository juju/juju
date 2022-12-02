// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd_test

import (
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type ConfirmationCommandBaseSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ConfirmationCommandBaseSuite{})

func (s *ConfirmationCommandBaseSuite) getCmdBase(args []string) modelcmd.ConfirmationCommandBase {
	f := cmdtesting.NewFlagSet()
	cmd := modelcmd.ConfirmationCommandBase{}
	cmd.SetFlags(f)
	f.Parse(true, args)
	cmd.Init(f.Args())
	return cmd
}

func (s *ConfirmationCommandBaseSuite) TestSimple(c *gc.C) {
	commandBase := s.getCmdBase([]string{"--foo", "bar"})

	c.Assert(commandBase.NeedsConfirmation(), jc.IsTrue)
}

func (s *ConfirmationCommandBaseSuite) TestNoPromptFlag(c *gc.C) {
	commandBase := s.getCmdBase([]string{"--no-prompt", "--foo", "bar"})

	c.Assert(commandBase.NeedsConfirmation(), jc.IsFalse)
}

func (s *ConfirmationCommandBaseSuite) TestSkipConfVarTrue(c *gc.C) {
	for _, trueVal := range []string{"1", "t", "true", "TRUE"} {
		s.PatchEnvironment(osenv.JujuSkipConfirmationEnvKey, trueVal)

		commandBase := s.getCmdBase([]string{"--foo", "bar"})

		c.Assert(commandBase.NeedsConfirmation(), jc.IsFalse)
	}
}

func (s *ConfirmationCommandBaseSuite) TestSkipConfVarFalse(c *gc.C) {
	for _, falseVal := range []string{"0", "f", "false", "FALSE"} {
		s.PatchEnvironment(osenv.JujuSkipConfirmationEnvKey, falseVal)

		commandBase := s.getCmdBase([]string{"--foo", "bar"})

		c.Assert(commandBase.NeedsConfirmation(), jc.IsTrue)
	}
}

func (s *ConfirmationCommandBaseSuite) TestPrecedence(c *gc.C) {
	s.PatchEnvironment(osenv.JujuSkipConfirmationEnvKey, "0")

	commandBase := s.getCmdBase([]string{"--no-prompt", "--foo", "bar"})

	c.Assert(commandBase.NeedsConfirmation(), jc.IsFalse)
}
