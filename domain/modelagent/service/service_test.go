// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/internal/errors"
)

type suite struct {
	state      *MockState
	modelState *MockModelState
}

var _ = gc.Suite(&suite{})

func (s *suite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	s.modelState = NewMockModelState(ctrl)
	return ctrl
}

// TestGetModelAgentVersionSuccess tests the happy path for
// Service.GetModelAgentVersion.
func (s *suite) TestGetModelAgentVersionSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	modelID := modeltesting.GenModelUUID(c)
	expectedVersion, err := version.Parse("4.21.65")
	c.Assert(err, jc.ErrorIsNil)
	s.state.EXPECT().GetModelTargetAgentVersion(gomock.Any(), modelID).
		Return(expectedVersion, nil)

	svc := NewService(s.state)
	ver, err := svc.GetModelTargetAgentVersion(context.Background(), modelID)
	c.Check(err, jc.ErrorIsNil)
	c.Check(ver, jc.DeepEquals, expectedVersion)
}

// TestGetModelAgentVersionInvalidModelID tests that if an invalid model ID is
// passed to Service.GetModelAgentVersion, we return errors.NotValid.
func (s *suite) TestGetModelAgentVersionInvalidModelID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(s.state)
	_, err := svc.GetModelTargetAgentVersion(context.Background(), "invalid")
	c.Check(err, jc.ErrorIs, coreerrors.NotValid)
}

// TestGetModelAgentVersionModelNotFound tests that Service.GetModelAgentVersion
// returns an appropriate error when the specified model cannot be found.
func (s *suite) TestGetModelAgentVersionModelNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	modelID := modeltesting.GenModelUUID(c)
	modelNotFoundErr := errors.Errorf("%w for id %q", modelerrors.NotFound, modelID)
	s.state.EXPECT().GetModelTargetAgentVersion(gomock.Any(), modelID).
		Return(version.Zero, modelNotFoundErr)

	svc := NewService(s.state)
	_, err := svc.GetModelTargetAgentVersion(context.Background(), modelID)
	c.Check(err, jc.ErrorIs, modelerrors.NotFound)
}

// TestGetMachineTargetAgentVersion is asserting the happy path for getting
// a machine's target agent version.
func (s *suite) TestGetMachineTargetAgentVersion(c *gc.C) {
	defer s.setupMocks(c).Finish()

	machineName := machine.Name("0")
	modelUUID := modeltesting.GenModelUUID(c)
	ver := version.MustParse("4.0.0")

	s.modelState.EXPECT().GetModelUUID(gomock.Any()).Return(modelUUID, nil)
	s.modelState.EXPECT().CheckMachineExists(gomock.Any(), machineName).Return(nil)
	s.state.EXPECT().GetModelTargetAgentVersion(gomock.Any(), modelUUID).Return(ver, nil)

	rval, err := NewModelService(s.modelState, s.state, nil).GetMachineTargetAgentVersion(
		context.Background(),
		machineName,
	)
	c.Check(err, jc.ErrorIsNil)
	c.Check(rval, gc.Equals, ver)
}

// TestGetMachineTargetAgentVersionNotFound is testing that the service
// returns a [machineerrors.MachineNotFound] error when no machine exists for
// a given name.
func (s *suite) TestGetMachineTargetAgentVersionNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.modelState.EXPECT().CheckMachineExists(gomock.Any(), machine.Name("0")).Return(
		machineerrors.MachineNotFound,
	)

	_, err := NewModelService(s.modelState, s.state, nil).GetMachineTargetAgentVersion(
		context.Background(),
		machine.Name("0"),
	)
	c.Check(err, jc.ErrorIs, machineerrors.MachineNotFound)
}

// TestGetUnitTargetAgentVersion is asserting the happy path for getting
// a unit's target agent version.
func (s *suite) TestGetUnitTargetAgentVersion(c *gc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	ver := version.MustParse("4.0.0")

	s.modelState.EXPECT().GetModelUUID(gomock.Any()).Return(modelUUID, nil)
	s.modelState.EXPECT().CheckUnitExists(gomock.Any(), "foo/0").Return(nil)
	s.state.EXPECT().GetModelTargetAgentVersion(gomock.Any(), modelUUID).Return(ver, nil)

	rval, err := NewModelService(s.modelState, s.state, nil).GetUnitTargetAgentVersion(
		context.Background(),
		"foo/0",
	)
	c.Check(err, jc.ErrorIsNil)
	c.Check(rval, gc.Equals, ver)
}

// TestGetUnitTargetAgentVersionNotFound is testing that the service
// returns a [applicationerrors.UnitNotFound] error when no unit exists for
// a given name.
func (s *suite) TestGetUnitTargetAgentVersionNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.modelState.EXPECT().CheckUnitExists(gomock.Any(), "foo/0").Return(
		applicationerrors.UnitNotFound,
	)

	_, err := NewModelService(s.modelState, s.state, nil).GetUnitTargetAgentVersion(
		context.Background(),
		"foo/0",
	)
	c.Check(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

// TestWatchUnitTargetAgentVersionNotFound is testing that the service
// returns a [applicationerrors.UnitNotFound] error when no unit exists for
// a given name.
func (s *suite) TestWatchUnitTargetAgentVersionNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.modelState.EXPECT().CheckUnitExists(gomock.Any(), "foo/0").Return(
		applicationerrors.UnitNotFound,
	)

	_, err := NewModelService(s.modelState, s.state, nil).WatchUnitTargetAgentVersion(
		context.Background(),
		"foo/0",
	)
	c.Check(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

// TestWatchMachineTargetAgentVersionNotFound is testing that the service
// returns a [machineerrors.MachineNotFound] error when no machine exists for
// a given name.
func (s *suite) TestWatchMachineTargetAgentVersionNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.modelState.EXPECT().CheckMachineExists(gomock.Any(), machine.Name("0")).Return(
		machineerrors.MachineNotFound,
	)

	_, err := NewModelService(s.modelState, s.state, nil).WatchMachineTargetAgentVersion(
		context.Background(),
		machine.Name("0"),
	)
	c.Check(err, jc.ErrorIs, machineerrors.MachineNotFound)
}

// WatchModelTargetAgentVersion tests that ModelService.WatchModelTargetAgentVersion
// returns an appropriate error when the specified model cannot be found.
func (s *suite) TestWatchModelTargetAgentVersionNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	modelID := modeltesting.GenModelUUID(c)
	modelNotFoundErr := errors.Errorf("%w for id %q", modelerrors.NotFound, modelID)
	s.modelState.EXPECT().GetModelUUID(context.Background()).Return(model.UUID(""), modelNotFoundErr)

	_, err := NewModelService(s.modelState, s.state, nil).WatchModelTargetAgentVersion(
		context.Background(),
	)
	c.Check(err, jc.ErrorIs, modelerrors.NotFound)
}
