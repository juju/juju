// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/core/application"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/machine"
	machinetesting "github.com/juju/juju/core/machine/testing"
	"github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	agentpassworderrors "github.com/juju/juju/domain/agentpassword/errors"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	internalerrors "github.com/juju/juju/internal/errors"
	internalpassword "github.com/juju/juju/internal/password"
)

type serviceSuite struct {
	modelState      *MockModelState
	controllerState *MockControllerState
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

	s.modelState.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.modelState.EXPECT().SetUnitPasswordHash(gomock.Any(), unitUUID, hashPassword(password)).Return(nil)

	service := NewService(s.modelState, s.controllerState)
	err = service.SetUnitPassword(c.Context(), unitName, password)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSetUnitPasswordUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)

	unitName := unit.Name("unit/0")
	password, err := internalpassword.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)

	s.modelState.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, applicationerrors.UnitNotFound)

	service := NewService(s.modelState, s.controllerState)
	err = service.SetUnitPassword(c.Context(), unitName, password)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *serviceSuite) TestSetUnitPasswordInvalidName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("!!!")
	password, err := internalpassword.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)

	service := NewService(s.modelState, s.controllerState)
	err = service.SetUnitPassword(c.Context(), unitName, password)
	c.Assert(err, tc.ErrorIs, unit.InvalidUnitName)
}

func (s *serviceSuite) TestSetUnitPasswordInvalidPassword(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("unit/0")
	password := "foo"

	service := NewService(s.modelState, s.controllerState)
	err := service.SetUnitPassword(c.Context(), unitName, password)
	c.Assert(err, tc.ErrorMatches, "password is only 3 bytes long, and is not a valid Agent password.*")
}

func (s *serviceSuite) TestMatchesUnitPasswordHash(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)

	unitName := unit.Name("unit/0")
	password, err := internalpassword.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)

	s.modelState.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.modelState.EXPECT().MatchesUnitPasswordHash(gomock.Any(), unitUUID, hashPassword(password)).Return(true, nil)

	service := NewService(s.modelState, s.controllerState)
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

	s.modelState.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, applicationerrors.UnitNotFound)

	service := NewService(s.modelState, s.controllerState)
	_, err = service.MatchesUnitPasswordHash(c.Context(), unitName, password)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *serviceSuite) TestMatchesUnitPasswordHashInvalidName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("!!!")
	password, err := internalpassword.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)

	service := NewService(s.modelState, s.controllerState)
	_, err = service.MatchesUnitPasswordHash(c.Context(), unitName, password)
	c.Assert(err, tc.ErrorIs, unit.InvalidUnitName)
}

func (s *serviceSuite) TestMatchesUnitPasswordHashEmptyPassword(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("unit/0")

	service := NewService(s.modelState, s.controllerState)
	_, err := service.MatchesUnitPasswordHash(c.Context(), unitName, "")
	c.Assert(err, tc.ErrorIs, agentpassworderrors.EmptyPassword)
}

func (s *serviceSuite) TestMatchesUnitPasswordHashInvalidPassword(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unit.Name("unit/0")

	service := NewService(s.modelState, s.controllerState)
	_, err := service.MatchesUnitPasswordHash(c.Context(), unitName, "abc")
	c.Assert(err, tc.ErrorIs, agentpassworderrors.InvalidPassword)
}

func (s *serviceSuite) TestSetMachinePassword(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID := machinetesting.GenUUID(c)

	machineName := machine.Name("0")
	password, err := internalpassword.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)

	s.modelState.EXPECT().GetMachineUUID(gomock.Any(), machineName).Return(machineUUID, nil)
	s.modelState.EXPECT().SetMachinePasswordHash(gomock.Any(), machineUUID, hashPassword(password)).Return(nil)

	service := NewService(s.modelState, s.controllerState)
	err = service.SetMachinePassword(c.Context(), machineName, password)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSetMachinePasswordMachineNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID := machinetesting.GenUUID(c)

	machineName := machine.Name("0")
	password, err := internalpassword.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)

	s.modelState.EXPECT().GetMachineUUID(gomock.Any(), machineName).Return(machineUUID, applicationerrors.MachineNotFound)

	service := NewService(s.modelState, s.controllerState)
	err = service.SetMachinePassword(c.Context(), machineName, password)
	c.Assert(err, tc.ErrorIs, applicationerrors.MachineNotFound)
}

func (s *serviceSuite) TestSetMachinePasswordInvalidName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineName := machine.Name("!!!")
	password, err := internalpassword.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)

	service := NewService(s.modelState, s.controllerState)
	err = service.SetMachinePassword(c.Context(), machineName, password)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *serviceSuite) TestSetMachinePasswordInvalidPassword(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineName := machine.Name("0")
	password := "foo"

	service := NewService(s.modelState, s.controllerState)
	err := service.SetMachinePassword(c.Context(), machineName, password)
	c.Assert(err, tc.ErrorMatches, "password is only 3 bytes long, and is not a valid Agent password.*")
}

func (s *serviceSuite) TestMatchesMachinePasswordHashWithNonce(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID := machinetesting.GenUUID(c)

	machineName := machine.Name("0")
	password, err := internalpassword.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)

	s.modelState.EXPECT().GetMachineUUID(gomock.Any(), machineName).Return(machineUUID, nil)
	s.modelState.EXPECT().MatchesMachinePasswordHashWithNonce(gomock.Any(), machineUUID, hashPassword(password), "foo").Return(true, nil)

	service := NewService(s.modelState, s.controllerState)
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

	s.modelState.EXPECT().GetMachineUUID(gomock.Any(), machineName).Return(machineUUID, applicationerrors.MachineNotFound)

	service := NewService(s.modelState, s.controllerState)
	_, err = service.MatchesMachinePasswordHashWithNonce(c.Context(), machineName, password, "foo")
	c.Assert(err, tc.ErrorIs, applicationerrors.MachineNotFound)
}

func (s *serviceSuite) TestMatchesMachinePasswordHashWithNonceInvalidName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineName := machine.Name("!!!")
	password, err := internalpassword.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)

	service := NewService(s.modelState, s.controllerState)
	_, err = service.MatchesMachinePasswordHashWithNonce(c.Context(), machineName, password, "")
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *serviceSuite) TestMatchesMachinePasswordHashWithNonceEmptyPassword(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineName := machine.Name("0")

	service := NewService(s.modelState, s.controllerState)
	_, err := service.MatchesMachinePasswordHashWithNonce(c.Context(), machineName, "", "")
	c.Assert(err, tc.ErrorIs, agentpassworderrors.EmptyPassword)
}

func (s *serviceSuite) TestMatchesMachinePasswordHashWithNonceInvalidPassword(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineName := machine.Name("0")

	service := NewService(s.modelState, s.controllerState)
	_, err := service.MatchesMachinePasswordHashWithNonce(c.Context(), machineName, "abc", "foo")
	c.Assert(err, tc.ErrorIs, agentpassworderrors.InvalidPassword)
}

func (s *serviceSuite) TestMatchesMachinePasswordHashWithNonceEmptyNonce(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineName := machine.Name("0")
	password, err := internalpassword.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)

	service := NewService(s.modelState, s.controllerState)
	_, err = service.MatchesMachinePasswordHashWithNonce(c.Context(), machineName, password, "")
	c.Assert(err, tc.ErrorIs, agentpassworderrors.EmptyNonce)
}

// TestIsMachineControllerSuccess asserts the happy path of the
// IsMachineController service.
func (s *serviceSuite) TestIsMachineControllerSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelState.EXPECT().IsMachineController(gomock.Any(), machine.Name("666")).Return(true, nil)

	isController, err := NewService(s.modelState, s.controllerState).
		IsMachineController(c.Context(), machine.Name("666"))
	c.Check(err, tc.ErrorIsNil)
	c.Assert(isController, tc.IsTrue)
}

// TestIsMachineControllerError asserts that an error coming from the modelState
// layer is preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestIsMachineControllerError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rErr := internalerrors.Errorf("boom")
	s.modelState.EXPECT().IsMachineController(gomock.Any(), machine.Name("666")).Return(false, rErr)

	isController, err := NewService(s.modelState, s.controllerState).
		IsMachineController(c.Context(), machine.Name("666"))
	c.Check(err, tc.ErrorIs, rErr)
	c.Check(isController, tc.IsFalse)
}

// TestIsMachineControllerNotFound asserts that the modelState layer returns a
// NotFound Error if a machine is not found with the given machineName, and that
// error is preserved and passed on to the service layer to be handled there.
func (s *serviceSuite) TestIsMachineControllerNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelState.EXPECT().IsMachineController(gomock.Any(), machine.Name("666")).Return(false, coreerrors.NotFound)

	isController, err := NewService(s.modelState, s.controllerState).
		IsMachineController(c.Context(), machine.Name("666"))
	c.Check(err, tc.ErrorIs, coreerrors.NotFound)
	c.Check(isController, tc.IsFalse)
}

func (s *serviceSuite) TestSetControllerNodePassword(c *tc.C) {
	defer s.setupMocks(c).Finish()

	password, err := internalpassword.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)

	s.controllerState.EXPECT().SetControllerNodePasswordHash(gomock.Any(), "0", hashPassword(password)).Return(nil)

	service := NewService(s.modelState, s.controllerState)
	err = service.SetControllerNodePassword(c.Context(), "0", password)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSetControllerNodePasswordInvalidName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	password, err := internalpassword.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)

	service := NewService(s.modelState, s.controllerState)
	err = service.SetControllerNodePassword(c.Context(), "", password)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *serviceSuite) TestSetControllerNodePasswordInvalidPassword(c *tc.C) {
	defer s.setupMocks(c).Finish()

	password := "foo"

	service := NewService(s.modelState, s.controllerState)
	err := service.SetControllerNodePassword(c.Context(), "0", password)
	c.Assert(err, tc.ErrorMatches, "password is only 3 bytes long, and is not a valid Agent password.*")
}

func (s *serviceSuite) TestMatchesControllerNodePasswordHash(c *tc.C) {
	defer s.setupMocks(c).Finish()

	password, err := internalpassword.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)

	s.controllerState.EXPECT().MatchesControllerNodePasswordHash(gomock.Any(), "0", hashPassword(password)).Return(true, nil)

	service := NewService(s.modelState, s.controllerState)
	valid, err := service.MatchesControllerNodePasswordHash(c.Context(), "0", password)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(valid, tc.IsTrue)
}

func (s *serviceSuite) TestMatchesControllerNodePasswordHashInvalidName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	password, err := internalpassword.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)

	service := NewService(s.modelState, s.controllerState)
	_, err = service.MatchesControllerNodePasswordHash(c.Context(), "", password)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *serviceSuite) TestMatchesControllerNodePasswordHashEmptyPassword(c *tc.C) {
	defer s.setupMocks(c).Finish()

	service := NewService(s.modelState, s.controllerState)
	_, err := service.MatchesControllerNodePasswordHash(c.Context(), "0", "")
	c.Assert(err, tc.ErrorIs, agentpassworderrors.EmptyPassword)
}

func (s *serviceSuite) TestMatchesControllerNodePasswordHashInvalidPassword(c *tc.C) {
	defer s.setupMocks(c).Finish()

	service := NewService(s.modelState, s.controllerState)
	_, err := service.MatchesControllerNodePasswordHash(c.Context(), "0", "abc")
	c.Assert(err, tc.ErrorIs, agentpassworderrors.InvalidPassword)
}

func (s *serviceSuite) TestSetApplicationPassword(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appID, err := application.NewID()
	c.Assert(err, tc.ErrorIsNil)

	password, err := internalpassword.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)

	s.modelState.EXPECT().SetApplicationPasswordHash(gomock.Any(), appID, hashPassword(password)).Return(nil)

	service := NewService(s.modelState, s.controllerState)
	err = service.SetApplicationPassword(c.Context(), appID, password)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSetApplicationPasswordNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appID, err := application.NewID()
	c.Assert(err, tc.ErrorIsNil)

	password, err := internalpassword.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)

	s.modelState.EXPECT().SetApplicationPasswordHash(gomock.Any(), appID, hashPassword(password)).Return(applicationerrors.ApplicationNotFound)

	service := NewService(s.modelState, s.controllerState)
	err = service.SetApplicationPassword(c.Context(), appID, password)
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *serviceSuite) TestMatchesApplicationPasswordHash(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appID, err := application.NewID()
	c.Assert(err, tc.ErrorIsNil)

	appName := "foo"
	password, err := internalpassword.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)

	s.modelState.EXPECT().GetApplicationIDByName(gomock.Any(), appName).Return(appID, nil)
	s.modelState.EXPECT().MatchesApplicationPasswordHash(gomock.Any(), appID, hashPassword(password)).Return(true, nil)

	service := NewService(s.modelState, s.controllerState)
	valid, err := service.MatchesApplicationPasswordHash(c.Context(), appName, password)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(valid, tc.IsTrue)
}

func (s *serviceSuite) TestMatchesApplicationPasswordHashNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appID, err := application.NewID()
	c.Assert(err, tc.ErrorIsNil)

	appName := "foo"
	password, err := internalpassword.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)

	s.modelState.EXPECT().GetApplicationIDByName(gomock.Any(), appName).Return(appID, applicationerrors.ApplicationNotFound)

	service := NewService(s.modelState, s.controllerState)
	valid, err := service.MatchesApplicationPasswordHash(c.Context(), appName, password)
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
	c.Check(valid, tc.IsFalse)
}

func (s *serviceSuite) TestMatchesApplicationPasswordHashNotMatch(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appID, err := application.NewID()
	c.Assert(err, tc.ErrorIsNil)

	appName := "foo"
	password, err := internalpassword.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)

	s.modelState.EXPECT().GetApplicationIDByName(gomock.Any(), appName).Return(appID, nil)
	s.modelState.EXPECT().MatchesApplicationPasswordHash(gomock.Any(), appID, hashPassword(password)).Return(false, nil)

	service := NewService(s.modelState, s.controllerState)
	valid, err := service.MatchesApplicationPasswordHash(c.Context(), appName, password)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(valid, tc.IsFalse)
}

func (s *serviceSuite) TestMatchesApplicationPasswordHashEmptyPassword(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "foo"

	service := NewService(s.modelState, s.controllerState)
	valid, err := service.MatchesApplicationPasswordHash(c.Context(), appName, "")
	c.Assert(err, tc.ErrorIs, agentpassworderrors.EmptyPassword)
	c.Check(valid, tc.IsFalse)
}

func (s *serviceSuite) TestMatchesApplicationPasswordHashInvalidPassword(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "foo"

	service := NewService(s.modelState, s.controllerState)
	valid, err := service.MatchesApplicationPasswordHash(c.Context(), appName, "123")
	c.Assert(err, tc.ErrorIs, agentpassworderrors.InvalidPassword)
	c.Check(valid, tc.IsFalse)
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.modelState = NewMockModelState(ctrl)
	s.controllerState = NewMockControllerState(ctrl)

	return ctrl
}
