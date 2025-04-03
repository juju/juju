// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	passworderrors "github.com/juju/juju/domain/password/errors"
	internalpassword "github.com/juju/juju/internal/password"
)

type serviceSuite struct {
	state *MockState
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) TestSetUnitPassword(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)

	unitName := unit.Name("unit/0")
	password, err := internalpassword.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)

	s.state.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.state.EXPECT().SetUnitPasswordHash(gomock.Any(), unitUUID, hashPassword(password)).Return(nil)

	service := NewService(s.state)
	err = service.SetUnitPassword(context.Background(), unitName, password)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestSetUnitPasswordUnitNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)

	unitName := unit.Name("unit/0")
	password, err := internalpassword.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)

	s.state.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, passworderrors.UnitNotFound)

	service := NewService(s.state)
	err = service.SetUnitPassword(context.Background(), unitName, password)
	c.Assert(err, jc.ErrorIs, passworderrors.UnitNotFound)
}

func (s *serviceSuite) TestSetUnitPasswordInvalidName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("!!!")
	password, err := internalpassword.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)

	service := NewService(s.state)
	err = service.SetUnitPassword(context.Background(), unitName, password)
	c.Assert(err, jc.ErrorIs, unit.InvalidUnitName)
}

func (s *serviceSuite) TestSetUnitPasswordInvalidPassword(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("unit/0")
	password := "foo"

	service := NewService(s.state)
	err := service.SetUnitPassword(context.Background(), unitName, password)
	c.Assert(err, gc.ErrorMatches, "password is only 3 chars long, and is not a valid Agent password.*")
}

func (s *serviceSuite) TestMatchesUnitPasswordHash(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)

	unitName := unit.Name("unit/0")
	password, err := internalpassword.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)

	s.state.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.state.EXPECT().MatchesUnitPasswordHash(gomock.Any(), unitUUID, hashPassword(password)).Return(true, nil)

	service := NewService(s.state)
	valid, err := service.MatchesUnitPasswordHash(context.Background(), unitName, password)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(valid, jc.IsTrue)
}

func (s *serviceSuite) TestMatchesUnitPasswordHashUnitNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)

	unitName := unit.Name("unit/0")
	password, err := internalpassword.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)

	s.state.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, passworderrors.UnitNotFound)

	service := NewService(s.state)
	_, err = service.MatchesUnitPasswordHash(context.Background(), unitName, password)
	c.Assert(err, jc.ErrorIs, passworderrors.UnitNotFound)
}

func (s *serviceSuite) TestMatchesUnitPasswordHashInvalidName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("!!!")
	password, err := internalpassword.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)

	service := NewService(s.state)
	_, err = service.MatchesUnitPasswordHash(context.Background(), unitName, password)
	c.Assert(err, jc.ErrorIs, unit.InvalidUnitName)
}

func (s *serviceSuite) TestMatchesUnitPasswordHashEmptyPassword(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("unit/0")

	service := NewService(s.state)
	_, err := service.MatchesUnitPasswordHash(context.Background(), unitName, "")
	c.Assert(err, jc.ErrorIs, passworderrors.EmptyPassword)
}

func (s *serviceSuite) TestMatchesUnitPasswordHashInvalidPassword(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("unit/0")

	service := NewService(s.state)
	_, err := service.MatchesUnitPasswordHash(context.Background(), unitName, "abc")
	c.Assert(err, jc.ErrorIs, passworderrors.InvalidPassword)
}

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)

	return ctrl
}
