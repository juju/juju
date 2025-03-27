// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/semversion"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	modelerrors "github.com/juju/juju/domain/model/errors"
)

type suite struct {
	state *MockState
}

var _ = gc.Suite(&suite{})

func (s *suite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	return ctrl
}

// TestGetModelAgentVersionSuccess tests the happy path for
// Service.GetModelAgentVersion.
func (s *suite) TestGetModelAgentVersionSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	expectedVersion, err := semversion.Parse("4.21.65")
	c.Assert(err, jc.ErrorIsNil)
	s.state.EXPECT().GetTargetAgentVersion(gomock.Any()).Return(expectedVersion, nil)

	svc := NewService(s.state, nil)
	ver, err := svc.GetModelTargetAgentVersion(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(ver, jc.DeepEquals, expectedVersion)
}

// TestGetModelAgentVersionNotFound tests that Service.GetModelAgentVersion
// returns an appropriate error when the agent version cannot be found.
func (s *suite) TestGetModelAgentVersionModelNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetTargetAgentVersion(gomock.Any()).Return(semversion.Zero, modelerrors.AgentVersionNotFound)

	svc := NewService(s.state, nil)
	_, err := svc.GetModelTargetAgentVersion(context.Background())
	c.Check(err, jc.ErrorIs, modelerrors.AgentVersionNotFound)
}

// TestGetMachineTargetAgentVersion is asserting the happy path for getting
// a machine's target agent version.
func (s *suite) TestGetMachineTargetAgentVersion(c *gc.C) {
	defer s.setupMocks(c).Finish()

	machineName := machine.Name("0")
	ver := semversion.MustParse("4.0.0")

	s.state.EXPECT().CheckMachineExists(gomock.Any(), machineName).Return(nil)
	s.state.EXPECT().GetTargetAgentVersion(gomock.Any()).Return(ver, nil)

	rval, err := NewService(s.state, nil).GetMachineTargetAgentVersion(context.Background(), machineName)
	c.Check(err, jc.ErrorIsNil)
	c.Check(rval, gc.Equals, ver)
}

// TestGetMachineTargetAgentVersionNotFound is testing that the service
// returns a [machineerrors.MachineNotFound] error when no machine exists for
// a given name.
func (s *suite) TestGetMachineTargetAgentVersionNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().CheckMachineExists(gomock.Any(), machine.Name("0")).Return(
		machineerrors.MachineNotFound,
	)

	_, err := NewService(s.state, nil).GetMachineTargetAgentVersion(
		context.Background(),
		machine.Name("0"),
	)
	c.Check(err, jc.ErrorIs, machineerrors.MachineNotFound)
}

// TestGetUnitTargetAgentVersion is asserting the happy path for getting
// a unit's target agent version.
func (s *suite) TestGetUnitTargetAgentVersion(c *gc.C) {
	defer s.setupMocks(c).Finish()

	ver := semversion.MustParse("4.0.0")

	s.state.EXPECT().CheckUnitExists(gomock.Any(), "foo/0").Return(nil)
	s.state.EXPECT().GetTargetAgentVersion(gomock.Any()).Return(ver, nil)

	rval, err := NewService(s.state, nil).GetUnitTargetAgentVersion(context.Background(), "foo/0")
	c.Check(err, jc.ErrorIsNil)
	c.Check(rval, gc.Equals, ver)
}

// TestGetUnitTargetAgentVersionNotFound is testing that the service
// returns a [applicationerrors.UnitNotFound] error when no unit exists for
// a given name.
func (s *suite) TestGetUnitTargetAgentVersionNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().CheckUnitExists(gomock.Any(), "foo/0").Return(
		applicationerrors.UnitNotFound,
	)

	_, err := NewService(s.state, nil).GetUnitTargetAgentVersion(
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

	s.state.EXPECT().CheckUnitExists(gomock.Any(), "foo/0").Return(
		applicationerrors.UnitNotFound,
	)

	_, err := NewService(s.state, nil).WatchUnitTargetAgentVersion(
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

	s.state.EXPECT().CheckMachineExists(gomock.Any(), machine.Name("0")).Return(
		machineerrors.MachineNotFound,
	)

	_, err := NewService(s.state, nil).WatchMachineTargetAgentVersion(context.Background(), "0")
	c.Check(err, jc.ErrorIs, machineerrors.MachineNotFound)
}
