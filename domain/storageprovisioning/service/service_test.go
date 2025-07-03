// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	machinetesting "github.com/juju/juju/core/machine/testing"
	machineerrors "github.com/juju/juju/domain/machine/errors"
)

// serviceSuite is a test suite for the [Service] to test the common non storage
// interface items that are not specific to storage.
type serviceSuite struct {
	state          *MockState
	watcherFactory *MockWatcherFactory
}

// TestServiceSuite runs the tests in [serviceSuite].
func TestServiceSuite(t *testing.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	s.watcherFactory = NewMockWatcherFactory(ctrl)
	c.Cleanup(func() {
		s.state = nil
		s.watcherFactory = nil
	})
	return ctrl
}

// TestWatchMachineCloudInstanceNotFound tests that when a machine does not
// exist in the model the caller gets back an error satisfying
// [machineerrors.MachineNotFound] when trying to watch a machine cloud instance.
func (s *serviceSuite) TestWatchMachineCloudInstanceNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID := machinetesting.GenUUID(c)

	s.state.EXPECT().CheckMachineIsDead(gomock.Any(), machineUUID).Return(
		false, machineerrors.MachineNotFound,
	)

	_, err := NewService(s.state, s.watcherFactory).WatchMachineCloudInstance(
		c.Context(), machineUUID,
	)
	c.Check(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

// TestWatchMachineCloudInstanceDead tests that when a machine is dead an the
// caller attempts to watch a machine cloud instance changes the call fails with
// an error satisfying [machineerrors.MachineIsDead] returned.
func (s *serviceSuite) TestWatchMachineCloudInstanceDead(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID := machinetesting.GenUUID(c)

	s.state.EXPECT().CheckMachineIsDead(gomock.Any(), machineUUID).Return(
		true, nil,
	)

	_, err := NewService(s.state, s.watcherFactory).WatchMachineCloudInstance(
		c.Context(), machineUUID,
	)
	c.Check(err, tc.ErrorIs, machineerrors.MachineIsDead)
}
