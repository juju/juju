// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	agentpassworderrors "github.com/juju/juju/domain/agentpassword/errors"
	internalpassword "github.com/juju/juju/internal/password"
)

type serviceSuite struct {
	state *MockState
}

func TestServiceSuite(t *testing.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) TestSetUnitPassword(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)

	unitName := unit.Name("unit/0")
	password, err := internalpassword.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)

	s.state.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.state.EXPECT().SetUnitPasswordHash(gomock.Any(), unitUUID, hashPassword(password)).Return(nil)

	service := NewService(s.state)
	err = service.SetUnitPassword(c.Context(), unitName, password)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSetUnitPasswordUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)

	unitName := unit.Name("unit/0")
	password, err := internalpassword.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)

	s.state.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, agentpassworderrors.UnitNotFound)

	service := NewService(s.state)
	err = service.SetUnitPassword(c.Context(), unitName, password)
	c.Assert(err, tc.ErrorIs, agentpassworderrors.UnitNotFound)
}

func (s *serviceSuite) TestSetUnitPasswordInvalidName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("!!!")
	password, err := internalpassword.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)

	service := NewService(s.state)
	err = service.SetUnitPassword(c.Context(), unitName, password)
	c.Assert(err, tc.ErrorIs, unit.InvalidUnitName)
}

func (s *serviceSuite) TestSetUnitPasswordInvalidPassword(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("unit/0")
	password := "foo"

	service := NewService(s.state)
	err := service.SetUnitPassword(c.Context(), unitName, password)
	c.Assert(err, tc.ErrorMatches, "password is only 3 chars long, and is not a valid Agent password.*")
}

func (s *serviceSuite) TestMatchesUnitPasswordHash(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)

	unitName := unit.Name("unit/0")
	password, err := internalpassword.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)

	s.state.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.state.EXPECT().MatchesUnitPasswordHash(gomock.Any(), unitUUID, hashPassword(password)).Return(true, nil)

	service := NewService(s.state)
	valid, err := service.MatchesUnitPasswordHash(c.Context(), unitName, password)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(valid, tc.IsTrue)
}

func (s *serviceSuite) TestMatchesUnitPasswordHashUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)

	unitName := unit.Name("unit/0")
	password, err := internalpassword.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)

	s.state.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, agentpassworderrors.UnitNotFound)

	service := NewService(s.state)
	_, err = service.MatchesUnitPasswordHash(c.Context(), unitName, password)
	c.Assert(err, tc.ErrorIs, agentpassworderrors.UnitNotFound)
}

func (s *serviceSuite) TestMatchesUnitPasswordHashInvalidName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("!!!")
	password, err := internalpassword.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)

	service := NewService(s.state)
	_, err = service.MatchesUnitPasswordHash(c.Context(), unitName, password)
	c.Assert(err, tc.ErrorIs, unit.InvalidUnitName)
}

func (s *serviceSuite) TestMatchesUnitPasswordHashEmptyPassword(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("unit/0")

	service := NewService(s.state)
	_, err := service.MatchesUnitPasswordHash(c.Context(), unitName, "")
	c.Assert(err, tc.ErrorIs, agentpassworderrors.EmptyPassword)
}

func (s *serviceSuite) TestMatchesUnitPasswordHashInvalidPassword(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("unit/0")

	service := NewService(s.state)
	_, err := service.MatchesUnitPasswordHash(c.Context(), unitName, "abc")
	c.Assert(err, tc.ErrorIs, agentpassworderrors.InvalidPassword)
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)

	return ctrl
}
