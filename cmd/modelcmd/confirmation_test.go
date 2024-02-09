// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd_test

import (
	"github.com/juju/cmd/v4/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/modelcmd/mocks"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/testing"
)

type DestroyConfirmationCommandBaseSuite struct{}

var _ = gc.Suite(&DestroyConfirmationCommandBaseSuite{})

func (*DestroyConfirmationCommandBaseSuite) getCmdBase(args []string) modelcmd.DestroyConfirmationCommandBase {
	f := cmdtesting.NewFlagSet()
	cmd := modelcmd.DestroyConfirmationCommandBase{}
	cmd.SetFlags(f)
	f.Parse(true, args)
	return cmd
}

func (s *DestroyConfirmationCommandBaseSuite) TestSimple(c *gc.C) {
	commandBase := s.getCmdBase([]string{"--foo", "bar"})

	c.Assert(commandBase.NeedsConfirmation(), jc.IsTrue)
}

func (s *DestroyConfirmationCommandBaseSuite) TestNoPromptFlag(c *gc.C) {
	commandBase := s.getCmdBase([]string{"--no-prompt", "--foo", "bar"})

	c.Assert(commandBase.NeedsConfirmation(), jc.IsFalse)
}

type RemoveConfirmationCommandBaseSuite struct {
	modelConfigAPI *mocks.MockModelConfigAPI
}

var _ = gc.Suite(&RemoveConfirmationCommandBaseSuite{})

func (s *RemoveConfirmationCommandBaseSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.modelConfigAPI = mocks.NewMockModelConfigAPI(ctrl)

	return ctrl
}

func (*RemoveConfirmationCommandBaseSuite) getCmdBase(args []string) modelcmd.RemoveConfirmationCommandBase {
	f := cmdtesting.NewFlagSet()
	cmd := modelcmd.RemoveConfirmationCommandBase{}
	cmd.SetFlags(f)
	f.Parse(true, args)
	return cmd
}

func (s *RemoveConfirmationCommandBaseSuite) TestSimpleFalse(c *gc.C) {
	defer s.setup(c).Finish()

	attrs := testing.FakeConfig().Merge(map[string]interface{}{config.ModeKey: ""})
	s.modelConfigAPI.EXPECT().ModelGet().Return(attrs, nil)

	commandBase := s.getCmdBase([]string{"--foo", "bar"})
	c.Assert(commandBase.NeedsConfirmation(s.modelConfigAPI), jc.IsFalse)
}

func (s *RemoveConfirmationCommandBaseSuite) TestSimpleTrue(c *gc.C) {
	defer s.setup(c).Finish()

	attrs := testing.FakeConfig().Merge(map[string]interface{}{config.ModeKey: config.RequiresPromptsMode})
	s.modelConfigAPI.EXPECT().ModelGet().Return(attrs, nil)

	commandBase := s.getCmdBase([]string{"--foo", "bar"})
	c.Assert(commandBase.NeedsConfirmation(s.modelConfigAPI), jc.IsTrue)
}

func (s *RemoveConfirmationCommandBaseSuite) TestModelGetError(c *gc.C) {
	defer s.setup(c).Finish()

	s.modelConfigAPI.EXPECT().ModelGet().Return(nil, errors.Errorf("doink"))

	commandBase := s.getCmdBase([]string{"--foo", "bar"})
	c.Assert(commandBase.NeedsConfirmation(s.modelConfigAPI), jc.IsTrue)
}

func (s *RemoveConfirmationCommandBaseSuite) TestNoPromptFlag(c *gc.C) {
	commandBase := s.getCmdBase([]string{"--no-prompt", "--foo", "bar"})
	c.Assert(commandBase.NeedsConfirmation(nil), jc.IsFalse)
}
