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
	"github.com/juju/juju/apiserver/facades/client/spaces/mocks"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/settings"
	"github.com/juju/juju/testing"
)

type SpaceRenameSuite struct {
	state    *mocks.MockRenameSpaceState
	space    *mocks.MockRenameSpace
	settings *mocks.MockSettings
	model    *mocks.MockModel
}

var _ = gc.Suite(&SpaceRenameSuite{})

func (s *SpaceRenameSuite) TearDownTest(c *gc.C) {
}

func (s *SpaceRenameSuite) TestSuccess(c *gc.C) {
	toName := "blub"
	fromName := "db"
	controllerKey := "something"

	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.space.EXPECT().Name().Return(fromName).Times(2)

	config := s.getDefaultControllerConfig(c, map[string]interface{}{controller.JujuHASpace: fromName, controller.JujuManagementSpace: "nochange"})
	s.state.EXPECT().ControllerConfig().Return(config, nil)

	s.model.EXPECT().IsControllerModel().Return(true)
	s.state.EXPECT().ControllerSettingsGlobalKey().Return(controllerKey)

	currentConstraints := map[string]constraints.Value{
		"DOCID_1": {Spaces: &[]string{fromName, "nochange"}},
		"DOCID_2": {Spaces: &[]string{"nochange"}},
		"DOCID_3": {},
	}
	s.state.EXPECT().ConstraintsBySpaceName(fromName).Return(currentConstraints, nil)

	expectedDelta := settings.ItemChanges{{
		Type:     1,
		Key:      controller.JujuHASpace,
		OldValue: fromName,
		NewValue: toName,
	}}

	expectedNewConstraints := map[string]constraints.Value{
		"DOCID_1": {Spaces: &[]string{toName, "nochange"}},
		"DOCID_2": {Spaces: &[]string{"nochange"}},
		"DOCID_3": {},
	}
	s.settings.EXPECT().DeltaOps(controllerKey, expectedDelta).Return(nil, nil)

	s.state.EXPECT().GetConstraintsOps(expectedNewConstraints).Return([]txn.Op{}, nil)
	s.space.EXPECT().RenameSpaceCompleteOps(toName).Return(nil, nil)

	op := spaces.NewRenameSpaceModelOp(s.model, s.settings, s.state, s.space, toName)
	ops, err := op.Build(0)
	c.Assert(err, jc.ErrorIsNil)

	// this is because the code itself does not test for the ops but for expected constraints and delta,
	// which are used to create the ops.
	c.Assert(ops, gc.HasLen, 0)
}

func (s *SpaceRenameSuite) TestNotControllerModelSuccess(c *gc.C) {
	toName := "blub"
	fromName := "db"
	controllerKey := "something"

	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.space.EXPECT().Name().Return(fromName).Times(2)
	s.state.EXPECT().ControllerSettingsGlobalKey().Return(controllerKey)
	s.model.EXPECT().IsControllerModel().Return(false)

	currentConstraints := map[string]constraints.Value{
		"DOCID_1": {Spaces: &[]string{fromName, "nochange"}},
		"DOCID_2": {Spaces: &[]string{"nochange"}},
		"DOCID_3": {},
	}
	s.state.EXPECT().ConstraintsBySpaceName(fromName).Return(currentConstraints, nil)

	expectedNewConstraints := map[string]constraints.Value{
		"DOCID_1": {Spaces: &[]string{toName, "nochange"}},
		"DOCID_2": {Spaces: &[]string{"nochange"}},
		"DOCID_3": {},
	}
	s.settings.EXPECT().DeltaOps(controllerKey, nil).Return(nil, nil)

	s.state.EXPECT().GetConstraintsOps(expectedNewConstraints).Return([]txn.Op{}, nil)
	s.space.EXPECT().RenameSpaceCompleteOps(toName).Return(nil, nil)

	op := spaces.NewRenameSpaceModelOp(s.model, s.settings, s.state, s.space, toName)
	ops, err := op.Build(0)
	c.Assert(err, jc.ErrorIsNil)

	// this is because the code itself does not test for the ops but for expected constraints and delta,
	// which are used to create the ops.
	c.Assert(ops, gc.HasLen, 0)
}

func (s *SpaceRenameSuite) TestErrorSettingsChanges(c *gc.C) {
	toName := "blub"
	fromName := "db"

	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.space.EXPECT().Name().Return(fromName).Times(1)

	bamErr := errors.New("bam")
	s.model.EXPECT().IsControllerModel().Return(true)
	s.state.EXPECT().ControllerConfig().Return(nil, bamErr)

	op := spaces.NewRenameSpaceModelOp(s.model, s.settings, s.state, s.space, toName)
	ops, err := op.Build(0)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("retrieving setting changes: %v", bamErr.Error()))

	c.Assert(ops, gc.HasLen, 0)
}

func (s *SpaceRenameSuite) TestErrorConstraintsChanges(c *gc.C) {
	toName := "blub"
	fromName := "db"

	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.space.EXPECT().Name().Return(fromName).Times(2)
	s.model.EXPECT().IsControllerModel().Return(true)
	s.state.EXPECT().ControllerConfig().Return(nil, nil)

	bamErr := errors.New("bam")
	s.state.EXPECT().ConstraintsBySpaceName(fromName).Return(nil, bamErr)

	op := spaces.NewRenameSpaceModelOp(s.model, s.settings, s.state, s.space, toName)
	ops, err := op.Build(0)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("retrieving constraint changes: %v", bamErr.Error()))

	c.Assert(ops, gc.HasLen, 0)
}

func (s *SpaceRenameSuite) getDefaultControllerConfig(c *gc.C, attr map[string]interface{}) controller.Config {
	cfg, err := controller.NewConfig(testing.ControllerTag.Id(), testing.CACert, attr)
	c.Assert(err, jc.ErrorIsNil)
	return cfg
}

func (s *SpaceRenameSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.space = mocks.NewMockRenameSpace(ctrl)
	s.model = mocks.NewMockModel(ctrl)
	s.state = mocks.NewMockRenameSpaceState(ctrl)
	s.settings = mocks.NewMockSettings(ctrl)

	return ctrl
}
