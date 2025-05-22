// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"errors"
	stdtesting "testing"

	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	resolve "github.com/juju/juju/domain/resolve"
	resolveerrors "github.com/juju/juju/domain/resolve/errors"
)

type serviceSuite struct {
	state   *MockState
	service *Service
}

func TestServiceSuite(t *stdtesting.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	s.service = NewService(s.state)
	return ctrl
}

func (s *serviceSuite) TestUnitResolveModeRetryHooks(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/0")
	unitUUID := unittesting.GenUnitUUID(c)

	s.state.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.state.EXPECT().UnitResolveMode(gomock.Any(), unitUUID).Return(resolve.ResolveModeRetryHooks, nil)

	mode, err := s.service.UnitResolveMode(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(mode, tc.Equals, resolve.ResolveModeRetryHooks)
}

func (s *serviceSuite) TestUnitResolveModeNoHooks(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/0")
	unitUUID := unittesting.GenUnitUUID(c)

	s.state.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.state.EXPECT().UnitResolveMode(gomock.Any(), unitUUID).Return(resolve.ResolveModeNoHooks, nil)

	mode, err := s.service.UnitResolveMode(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(mode, tc.Equals, resolve.ResolveModeNoHooks)
}

func (s *serviceSuite) TestUnitResolveModeInvalidUnitName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("!!!")

	_, err := s.service.UnitResolveMode(c.Context(), unitName)
	c.Assert(err, tc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *serviceSuite) TestUnitResolveModeNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/0")

	s.state.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return("", resolveerrors.UnitNotFound)

	_, err := s.service.UnitResolveMode(c.Context(), unitName)
	c.Assert(err, tc.ErrorIs, resolveerrors.UnitNotFound)
}

func (s *serviceSuite) TestUnitResolveModeNotResolved(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/0")
	unitUUID := unittesting.GenUnitUUID(c)

	s.state.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.state.EXPECT().UnitResolveMode(gomock.Any(), unitUUID).Return(resolve.ResolveMode(""), resolveerrors.UnitNotResolved)

	_, err := s.service.UnitResolveMode(c.Context(), unitName)
	c.Assert(err, tc.ErrorIs, resolveerrors.UnitNotResolved)
}

func (s *serviceSuite) TestResolveUnitRetryHooks(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/0")
	unitUUID := unittesting.GenUnitUUID(c)

	s.state.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.state.EXPECT().ResolveUnit(gomock.Any(), unitUUID, resolve.ResolveModeRetryHooks).Return(nil)

	err := s.service.ResolveUnit(c.Context(), unitName, resolve.ResolveModeRetryHooks)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestResolveUnitNoRetryHooks(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/0")
	unitUUID := unittesting.GenUnitUUID(c)

	s.state.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.state.EXPECT().ResolveUnit(gomock.Any(), unitUUID, resolve.ResolveModeNoHooks).Return(nil)

	err := s.service.ResolveUnit(c.Context(), unitName, resolve.ResolveModeNoHooks)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestResolveUnitInvalidUnitName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("!!!")

	err := s.service.ResolveUnit(c.Context(), unitName, resolve.ResolveModeRetryHooks)
	c.Assert(err, tc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *serviceSuite) TestResolveUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/0")

	s.state.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return("", resolveerrors.UnitNotFound)

	err := s.service.ResolveUnit(c.Context(), unitName, "")
	c.Assert(err, tc.ErrorIs, resolveerrors.UnitNotFound)
}

func (s *serviceSuite) TestResolveUnitNotInErrorState(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/0")
	unitUUID := unittesting.GenUnitUUID(c)

	s.state.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.state.EXPECT().ResolveUnit(gomock.Any(), unitUUID, resolve.ResolveModeRetryHooks).Return(resolveerrors.UnitNotInErrorState)

	err := s.service.ResolveUnit(c.Context(), unitName, resolve.ResolveModeRetryHooks)
	c.Assert(err, tc.ErrorIs, resolveerrors.UnitNotInErrorState)
}

func (s *serviceSuite) TestResolveAllUnitsRetryHooks(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ResolveAllUnits(gomock.Any(), resolve.ResolveModeRetryHooks).Return(nil)

	err := s.service.ResolveAllUnits(c.Context(), resolve.ResolveModeRetryHooks)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestResolveAllUnitsNoRetryHooks(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ResolveAllUnits(gomock.Any(), resolve.ResolveModeNoHooks).Return(nil)

	err := s.service.ResolveAllUnits(c.Context(), resolve.ResolveModeNoHooks)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestResolveAllUnitsErrors(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ResolveAllUnits(gomock.Any(), resolve.ResolveModeRetryHooks).Return(errors.New("boom!"))

	err := s.service.ResolveAllUnits(c.Context(), resolve.ResolveModeRetryHooks)
	c.Assert(err, tc.ErrorMatches, "boom!")
}

func (s *serviceSuite) TestClearResolved(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/0")
	unitUUID := unittesting.GenUnitUUID(c)

	s.state.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.state.EXPECT().ClearResolved(gomock.Any(), unitUUID).Return(nil)

	err := s.service.ClearResolved(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestClearResolvedInvalidUnitName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("!!!")

	err := s.service.ClearResolved(c.Context(), unitName)
	c.Assert(err, tc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *serviceSuite) TestClearResolvedNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/0")

	s.state.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return("", resolveerrors.UnitNotFound)

	err := s.service.ClearResolved(c.Context(), unitName)
	c.Assert(err, tc.ErrorIs, resolveerrors.UnitNotFound)
}
