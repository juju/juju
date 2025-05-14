// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd_test

import (
	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/modelcmd/mocks"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testing"
)

type DestroyConfirmationCommandBaseSuite struct{}

var _ = tc.Suite(&DestroyConfirmationCommandBaseSuite{})

func (*DestroyConfirmationCommandBaseSuite) getCmdBase(args []string) modelcmd.DestroyConfirmationCommandBase {
	f := cmdtesting.NewFlagSet()
	cmd := modelcmd.DestroyConfirmationCommandBase{}
	cmd.SetFlags(f)
	f.Parse(true, args)
	return cmd
}

func (s *DestroyConfirmationCommandBaseSuite) TestSimple(c *tc.C) {
	commandBase := s.getCmdBase([]string{"--foo", "bar"})

	c.Assert(commandBase.NeedsConfirmation(), tc.IsTrue)
}

func (s *DestroyConfirmationCommandBaseSuite) TestNoPromptFlag(c *tc.C) {
	commandBase := s.getCmdBase([]string{"--no-prompt", "--foo", "bar"})

	c.Assert(commandBase.NeedsConfirmation(), tc.IsFalse)
}

type RemoveConfirmationCommandBaseSuite struct {
	modelConfigAPI *mocks.MockModelConfigAPI
}

var _ = tc.Suite(&RemoveConfirmationCommandBaseSuite{})

func (s *RemoveConfirmationCommandBaseSuite) setup(c *tc.C) *gomock.Controller {
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

func (s *RemoveConfirmationCommandBaseSuite) TestSimpleFalse(c *tc.C) {
	defer s.setup(c).Finish()

	attrs := testing.FakeConfig().Merge(map[string]interface{}{config.ModeKey: ""})
	s.modelConfigAPI.EXPECT().ModelGet(gomock.Any()).Return(attrs, nil)

	commandBase := s.getCmdBase([]string{"--foo", "bar"})
	c.Assert(commandBase.NeedsConfirmation(c.Context(), s.modelConfigAPI), tc.IsFalse)
}

func (s *RemoveConfirmationCommandBaseSuite) TestSimpleTrue(c *tc.C) {
	defer s.setup(c).Finish()

	attrs := testing.FakeConfig().Merge(map[string]interface{}{config.ModeKey: config.RequiresPromptsMode})
	s.modelConfigAPI.EXPECT().ModelGet(gomock.Any()).Return(attrs, nil)

	commandBase := s.getCmdBase([]string{"--foo", "bar"})
	c.Assert(commandBase.NeedsConfirmation(c.Context(), s.modelConfigAPI), tc.IsTrue)
}

func (s *RemoveConfirmationCommandBaseSuite) TestModelGetError(c *tc.C) {
	defer s.setup(c).Finish()

	s.modelConfigAPI.EXPECT().ModelGet(gomock.Any()).Return(nil, errors.Errorf("doink"))

	commandBase := s.getCmdBase([]string{"--foo", "bar"})
	c.Assert(commandBase.NeedsConfirmation(c.Context(), s.modelConfigAPI), tc.IsTrue)
}

func (s *RemoveConfirmationCommandBaseSuite) TestNoPromptFlag(c *tc.C) {
	commandBase := s.getCmdBase([]string{"--no-prompt", "--foo", "bar"})
	c.Assert(commandBase.NeedsConfirmation(c.Context(), nil), tc.IsFalse)
}
