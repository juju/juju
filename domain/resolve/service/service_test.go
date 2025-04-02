// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"errors"

	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	resolve "github.com/juju/juju/domain/resolve"
	resolveerrors "github.com/juju/juju/domain/resolve/errors"
)

type serviceSuite struct {
	state   *MockState
	service *Service
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	s.service = NewService(s.state)
	return ctrl
}

func (s *serviceSuite) TestUnitResolveModeRetryHooks(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/0")
	unitUUID := unittesting.GenUnitUUID(c)

	s.state.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.state.EXPECT().UnitResolveMode(gomock.Any(), unitUUID).Return(resolve.ResolveModeRetryHooks, nil)

	mode, err := s.service.UnitResolveMode(context.Background(), unitName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mode, gc.Equals, resolve.ResolveModeRetryHooks)
}

func (s *serviceSuite) TestUnitResolveModeNoHooks(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/0")
	unitUUID := unittesting.GenUnitUUID(c)

	s.state.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.state.EXPECT().UnitResolveMode(gomock.Any(), unitUUID).Return(resolve.ResolveModeNoHooks, nil)

	mode, err := s.service.UnitResolveMode(context.Background(), unitName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mode, gc.Equals, resolve.ResolveModeNoHooks)
}

func (s *serviceSuite) TestUnitResolveModeInvalidUnitName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("!!!")

	_, err := s.service.UnitResolveMode(context.Background(), unitName)
	c.Assert(err, jc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *serviceSuite) TestUnitResolveModeNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/0")

	s.state.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return("", resolveerrors.UnitNotFound)

	_, err := s.service.UnitResolveMode(context.Background(), unitName)
	c.Assert(err, jc.ErrorIs, resolveerrors.UnitNotFound)
}

func (s *serviceSuite) TestUnitResolveModeNotResolved(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/0")
	unitUUID := unittesting.GenUnitUUID(c)

	s.state.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.state.EXPECT().UnitResolveMode(gomock.Any(), unitUUID).Return(resolve.ResolveMode(""), resolveerrors.UnitNotResolved)

	_, err := s.service.UnitResolveMode(context.Background(), unitName)
	c.Assert(err, jc.ErrorIs, resolveerrors.UnitNotResolved)
}

func (s *serviceSuite) TestResolveUnitRetryHooks(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/0")
	unitUUID := unittesting.GenUnitUUID(c)

	s.state.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.state.EXPECT().ResolveUnit(gomock.Any(), unitUUID, resolve.ResolveModeRetryHooks).Return(nil)

	err := s.service.ResolveUnit(context.Background(), unitName, resolve.ResolveModeRetryHooks)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestResolveUnitNoRetryHooks(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/0")
	unitUUID := unittesting.GenUnitUUID(c)

	s.state.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.state.EXPECT().ResolveUnit(gomock.Any(), unitUUID, resolve.ResolveModeNoHooks).Return(nil)

	err := s.service.ResolveUnit(context.Background(), unitName, resolve.ResolveModeNoHooks)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestResolveUnitInvalidUnitName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("!!!")

	err := s.service.ResolveUnit(context.Background(), unitName, resolve.ResolveModeRetryHooks)
	c.Assert(err, jc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *serviceSuite) TestResolveUnitNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/0")

	s.state.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return("", resolveerrors.UnitNotFound)

	err := s.service.ResolveUnit(context.Background(), unitName, "")
	c.Assert(err, jc.ErrorIs, resolveerrors.UnitNotFound)
}

func (s *serviceSuite) TestResolveUnitNotInErrorState(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/0")
	unitUUID := unittesting.GenUnitUUID(c)

	s.state.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.state.EXPECT().ResolveUnit(gomock.Any(), unitUUID, resolve.ResolveModeRetryHooks).Return(resolveerrors.UnitNotInErrorState)

	err := s.service.ResolveUnit(context.Background(), unitName, resolve.ResolveModeRetryHooks)
	c.Assert(err, jc.ErrorIs, resolveerrors.UnitNotInErrorState)
}

func (s *serviceSuite) TestResolveAllUnitsRetryHooks(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ResolveAllUnits(gomock.Any(), resolve.ResolveModeRetryHooks).Return(nil)

	err := s.service.ResolveAllUnits(context.Background(), resolve.ResolveModeRetryHooks)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestResolveAllUnitsNoRetryHooks(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ResolveAllUnits(gomock.Any(), resolve.ResolveModeNoHooks).Return(nil)

	err := s.service.ResolveAllUnits(context.Background(), resolve.ResolveModeNoHooks)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestResolveAllUnitsErrors(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ResolveAllUnits(gomock.Any(), resolve.ResolveModeRetryHooks).Return(errors.New("boom!"))

	err := s.service.ResolveAllUnits(context.Background(), resolve.ResolveModeRetryHooks)
	c.Assert(err, gc.ErrorMatches, "boom!")
}

func (s *serviceSuite) TestClearResolved(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/0")
	unitUUID := unittesting.GenUnitUUID(c)

	s.state.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.state.EXPECT().ClearResolved(gomock.Any(), unitUUID).Return(nil)

	err := s.service.ClearResolved(context.Background(), unitName)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestClearResolvedInvalidUnitName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("!!!")

	err := s.service.ClearResolved(context.Background(), unitName)
	c.Assert(err, jc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *serviceSuite) TestClearResolvedNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/0")

	s.state.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return("", resolveerrors.UnitNotFound)

	err := s.service.ClearResolved(context.Background(), unitName)
	c.Assert(err, jc.ErrorIs, resolveerrors.UnitNotFound)
}
