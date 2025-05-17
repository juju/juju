// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"slices"
	stdtesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/internal/errors"
)

type serviceSuite struct {
	controllerKeyProvider *MockControllerKeyProvider
	state                 *MockState
	controllerState       *MockControllerState

	modelId model.UUID
}

func TestServiceSuite(t *stdtesting.T) { tc.Run(t, &serviceSuite{}) }

var (
	controllerKeys = []string{
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIN8h8XBpjS9aBUG5cdoSWubs7wT2Lc/BEZIUQCqoaOZR juju-client-key",
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIN8h8XBpjS9aBUG5cdoSWubs7wT2Lc/BEZIUQCqoaOZR juju-system-key",
	}

	machineKeys = []string{
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC existing1",
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe existing2",
	}
)

func (s *serviceSuite) SetUpTest(c *tc.C) {
	s.modelId = modeltesting.GenModelUUID(c)
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.controllerKeyProvider = NewMockControllerKeyProvider(ctrl)
	s.state = NewMockState(ctrl)
	s.controllerState = NewMockControllerState(ctrl)
	return ctrl
}

// TestAuthorisedKeysForMachine is testing the happy path of
// [Service.AuthorisedKeysForMachine].
func (s *serviceSuite) TestAuthorisedKeysForMachine(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.controllerKeyProvider.EXPECT().ControllerAuthorisedKeys(gomock.Any()).Return(controllerKeys, nil)
	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(s.modelId, nil)
	s.state.EXPECT().CheckMachineExists(gomock.Any(), coremachine.Name("0")).Return(nil)
	s.controllerState.EXPECT().GetUserAuthorizedKeysForModel(gomock.Any(), s.modelId).Return(machineKeys, nil)

	expected := make([]string, 0, len(controllerKeys)+len(machineKeys))
	expected = append(expected, controllerKeys...)
	expected = append(expected, machineKeys...)

	keys, err := NewService(s.controllerKeyProvider, s.controllerState, s.state).GetAuthorisedKeysForMachine(
		c.Context(),
		coremachine.Name("0"),
	)
	c.Check(err, tc.ErrorIsNil)

	slices.Sort(expected)
	slices.Sort(keys)
	c.Check(keys, tc.DeepEquals, expected)
}

// TestAuthorisedKeysForMachineNoControllerKeys is asserting that if no
// controller keys are available we still succeed with no errors.
func (s *serviceSuite) TestAuthorisedKeysForMachineNoControllerKeys(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.controllerKeyProvider.EXPECT().ControllerAuthorisedKeys(gomock.Any()).Return(nil, nil)
	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(s.modelId, nil)
	s.state.EXPECT().CheckMachineExists(gomock.Any(), coremachine.Name("0")).Return(nil)
	s.controllerState.EXPECT().GetUserAuthorizedKeysForModel(gomock.Any(), s.modelId).Return(machineKeys, nil)

	expected := make([]string, 0, len(machineKeys))
	expected = append(expected, machineKeys...)

	keys, err := NewService(s.controllerKeyProvider, s.controllerState, s.state).GetAuthorisedKeysForMachine(
		c.Context(),
		coremachine.Name("0"),
	)
	c.Check(err, tc.ErrorIsNil)

	slices.Sort(expected)
	slices.Sort(keys)
	c.Check(keys, tc.DeepEquals, expected)
}

// TestAuthorisedKeysForMachineNotFound is asserting that if we ask for
// authorised keys for a machine that doesn't exist we get back a
// [machineerrors.MachineNotFound] error.
func (s *serviceSuite) TestAuthorisedKeysForMachineNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().CheckMachineExists(gomock.Any(), coremachine.Name("0")).Return(machineerrors.MachineNotFound)

	_, err := NewService(s.controllerKeyProvider, s.controllerState, s.state).GetAuthorisedKeysForMachine(
		c.Context(),
		coremachine.Name("0"),
	)
	c.Check(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

// TestGetInitialAuthorisedKeysForContainerSuccess tests the happy path for
// Service.GetInitialAuthorisedKeysForContainer.
func (s *serviceSuite) TestGetInitialAuthorisedKeysForContainerSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.controllerKeyProvider.EXPECT().ControllerAuthorisedKeys(gomock.Any()).Return(nil, nil)
	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(s.modelId, nil)
	s.controllerState.EXPECT().GetUserAuthorizedKeysForModel(gomock.Any(), s.modelId).Return(controllerKeys, nil)

	keys, err := NewService(s.controllerKeyProvider, s.controllerState, s.state).
		GetInitialAuthorisedKeysForContainer(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(keys, tc.DeepEquals, controllerKeys)
}

// TestGetInitialAuthorisedKeysForContainerSuccess checks that
// Service.GetInitialAuthorisedKeysForContainer surfaces errors from state.
func (s *serviceSuite) TestGetInitialAuthorisedKeysForContainerFailure(c *tc.C) {
	defer s.setupMocks(c).Finish()

	boom := errors.New("boom")

	s.controllerKeyProvider.EXPECT().ControllerAuthorisedKeys(gomock.Any()).Return(nil, nil).AnyTimes()
	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(s.modelId, nil)
	s.controllerState.EXPECT().GetUserAuthorizedKeysForModel(gomock.Any(), s.modelId).Return(
		nil,
		boom,
	)

	_, err := NewService(s.controllerKeyProvider, s.controllerState, s.state).
		GetInitialAuthorisedKeysForContainer(c.Context())
	c.Check(err, tc.ErrorIs, boom)
}
