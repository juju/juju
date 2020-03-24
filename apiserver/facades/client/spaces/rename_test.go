// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces_test

import (
	"fmt"

	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/apiserver/facades/client/spaces"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/settings"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type SpaceRenameSuite struct {
	spaceName string

	state    *spaces.MockRenameSpaceState
	space    *spaces.MockRenameSpace
	settings *spaces.MockSettings
	cons1    *spaces.MockConstraints
	cons2    *spaces.MockConstraints
}

var _ = gc.Suite(&SpaceRenameSuite{})

func (s *SpaceRenameSuite) TestBuildSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	toName := "external"

	currentConfig := s.getDefaultControllerConfig(
		c, map[string]interface{}{controller.JujuHASpace: s.spaceName, controller.JujuManagementSpace: "nochange"})

	s.space.EXPECT().RenameSpaceOps(toName).Return([]txn.Op{{}})
	s.state.EXPECT().ControllerConfig().Return(currentConfig, nil)
	s.state.EXPECT().ConstraintsBySpaceName(s.spaceName).Return([]spaces.Constraints{s.cons1, s.cons2}, nil)
	s.cons1.EXPECT().ChangeSpaceNameOps(s.spaceName, toName).Return([]txn.Op{{}})
	s.cons2.EXPECT().ChangeSpaceNameOps(s.spaceName, toName).Return([]txn.Op{{}})

	expectedConfigDelta := settings.ItemChanges{{
		Type:     1,
		Key:      controller.JujuHASpace,
		OldValue: s.spaceName,
		NewValue: toName,
	}}
	s.settings.EXPECT().DeltaOps(state.ControllerSettingsGlobalKey, expectedConfigDelta).Return([]txn.Op{{}}, nil)

	op := spaces.NewRenameSpaceOp(true, s.settings, s.state, s.space, toName)
	ops, err := op.Build(0)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ops, gc.HasLen, 4)
}

func (s *SpaceRenameSuite) TestBuildNotControllerModelSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	toName := "external"

	s.space.EXPECT().RenameSpaceOps(toName).Return([]txn.Op{{}})
	s.state.EXPECT().ConstraintsBySpaceName(s.spaceName).Return([]spaces.Constraints{s.cons1, s.cons2}, nil)
	s.cons1.EXPECT().ChangeSpaceNameOps(s.spaceName, toName).Return([]txn.Op{{}})
	s.cons2.EXPECT().ChangeSpaceNameOps(s.spaceName, toName).Return([]txn.Op{{}})

	op := spaces.NewRenameSpaceOp(false, s.settings, s.state, s.space, toName)
	ops, err := op.Build(0)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ops, gc.HasLen, 3)
}

func (s *SpaceRenameSuite) TestBuildSettingsChangesError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	toName := "external"

	s.space.EXPECT().RenameSpaceOps(toName).Return([]txn.Op{{}})
	s.state.EXPECT().ConstraintsBySpaceName(s.spaceName).Return(nil, nil)

	bamErr := errors.New("bam")
	s.state.EXPECT().ControllerConfig().Return(nil, bamErr)

	op := spaces.NewRenameSpaceOp(true, s.settings, s.state, s.space, toName)
	_, err := op.Build(0)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("retrieving settings changes: %v", bamErr.Error()))
}

func (s *SpaceRenameSuite) TestBuildConstraintsRetrievalError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	toName := "external"
	bamErr := errors.New("bam")

	s.space.EXPECT().RenameSpaceOps(toName).Return([]txn.Op{{}})
	s.state.EXPECT().ConstraintsBySpaceName(s.spaceName).Return(nil, bamErr)

	op := spaces.NewRenameSpaceOp(true, s.settings, s.state, s.space, toName)
	_, err := op.Build(0)
	c.Assert(err, gc.ErrorMatches, bamErr.Error())
}

func (s *SpaceRenameSuite) getDefaultControllerConfig(c *gc.C, attr map[string]interface{}) controller.Config {
	cfg, err := controller.NewConfig(testing.ControllerTag.Id(), testing.CACert, attr)
	c.Assert(err, jc.ErrorIsNil)
	return cfg
}

func (s *SpaceRenameSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.spaceName = "db"
	s.space = spaces.NewMockRenameSpace(ctrl)
	s.space.EXPECT().Name().Return(s.spaceName).AnyTimes()

	s.state = spaces.NewMockRenameSpaceState(ctrl)
	s.settings = spaces.NewMockSettings(ctrl)
	s.cons1 = spaces.NewMockConstraints(ctrl)
	s.cons2 = spaces.NewMockConstraints(ctrl)

	return ctrl
}
