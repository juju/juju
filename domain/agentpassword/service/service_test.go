// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/machine"
	machinetesting "github.com/juju/juju/core/machine/testing"
	"github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	agentpassworderrors "github.com/juju/juju/domain/agentpassword/errors"
	applicationerrors "github.com/juju/juju/domain/application/errors"
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

	s.state.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, applicationerrors.UnitNotFound)

	service := NewService(s.state)
	err = service.SetUnitPassword(c.Context(), unitName, password)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
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

	s.state.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, applicationerrors.UnitNotFound)

	service := NewService(s.state)
	_, err = service.MatchesUnitPasswordHash(c.Context(), unitName, password)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
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

func (s *serviceSuite) TestSetMachinePassword(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID := machinetesting.GenUUID(c)

	machineName := machine.Name("0")
	password, err := internalpassword.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)

	s.state.EXPECT().GetMachineUUID(gomock.Any(), machineName).Return(machineUUID, nil)
	s.state.EXPECT().SetMachinePasswordHash(gomock.Any(), machineUUID, hashPassword(password)).Return(nil)

	service := NewService(s.state)
	err = service.SetMachinePassword(c.Context(), machineName, password)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSetMachinePasswordMachineNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID := machinetesting.GenUUID(c)

	machineName := machine.Name("0")
	password, err := internalpassword.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)

	s.state.EXPECT().GetMachineUUID(gomock.Any(), machineName).Return(machineUUID, applicationerrors.MachineNotFound)

	service := NewService(s.state)
	err = service.SetMachinePassword(c.Context(), machineName, password)
	c.Assert(err, tc.ErrorIs, applicationerrors.MachineNotFound)
}

func (s *serviceSuite) TestSetMachinePasswordInvalidName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineName := machine.Name("!!!")
	password, err := internalpassword.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)

	service := NewService(s.state)
	err = service.SetMachinePassword(c.Context(), machineName, password)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *serviceSuite) TestSetMachinePasswordInvalidPassword(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineName := machine.Name("0")
	password := "foo"

	service := NewService(s.state)
	err := service.SetMachinePassword(c.Context(), machineName, password)
	c.Assert(err, tc.ErrorMatches, "password is only 3 chars long, and is not a valid Agent password.*")
}

func (s *serviceSuite) TestMatchesMachinePasswordHashWithNonce(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID := machinetesting.GenUUID(c)

	machineName := machine.Name("0")
	password, err := internalpassword.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)

	s.state.EXPECT().GetMachineUUID(gomock.Any(), machineName).Return(machineUUID, nil)
	s.state.EXPECT().MatchesMachinePasswordHashWithNonce(gomock.Any(), machineUUID, hashPassword(password), "foo").Return(true, nil)

	service := NewService(s.state)
	valid, err := service.MatchesMachinePasswordHashWithNonce(c.Context(), machineName, password, "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(valid, tc.IsTrue)
}

func (s *serviceSuite) TestMatchesMachinePasswordHashWithNonceMachineNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID := machinetesting.GenUUID(c)

	machineName := machine.Name("0")
	password, err := internalpassword.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)

	s.state.EXPECT().GetMachineUUID(gomock.Any(), machineName).Return(machineUUID, applicationerrors.MachineNotFound)

	service := NewService(s.state)
	_, err = service.MatchesMachinePasswordHashWithNonce(c.Context(), machineName, password, "foo")
	c.Assert(err, tc.ErrorIs, applicationerrors.MachineNotFound)
}

func (s *serviceSuite) TestMatchesMachinePasswordHashWithNonceInvalidName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineName := machine.Name("!!!")
	password, err := internalpassword.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)

	service := NewService(s.state)
	_, err = service.MatchesMachinePasswordHashWithNonce(c.Context(), machineName, password, "")
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *serviceSuite) TestMatchesMachinePasswordHashWithNonceEmptyPassword(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineName := machine.Name("0")

	service := NewService(s.state)
	_, err := service.MatchesMachinePasswordHashWithNonce(c.Context(), machineName, "", "")
	c.Assert(err, tc.ErrorIs, agentpassworderrors.EmptyPassword)
}

func (s *serviceSuite) TestMatchesMachinePasswordHashWithNonceInvalidPassword(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineName := machine.Name("0")

	service := NewService(s.state)
	_, err := service.MatchesMachinePasswordHashWithNonce(c.Context(), machineName, "abc", "foo")
	c.Assert(err, tc.ErrorIs, agentpassworderrors.InvalidPassword)
}

func (s *serviceSuite) TestMatchesMachinePasswordHashWithNonceEmptyNonce(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineName := machine.Name("0")
	password, err := internalpassword.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)

	service := NewService(s.state)
	_, err = service.MatchesMachinePasswordHashWithNonce(c.Context(), machineName, password, "")
	c.Assert(err, tc.ErrorIs, agentpassworderrors.EmptyNonce)
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)

	return ctrl
}
