// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	applicationtesting "github.com/juju/juju/core/application/testing"
	machinetesting "github.com/juju/juju/core/machine/testing"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/domain/storageprovisioning"
	"github.com/juju/juju/internal/uuid"
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

// TestGetStorageResourceTagsForApplication tests that the model config resource
// tags value is parsed and returned as key-value pairs with controller uuid,
// model uuid and application name overlayed.
func (s *serviceSuite) TestGetStorageResourceTagsForApplication(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	ri := storageprovisioning.ResourceTagInfo{
		BaseResourceTags: "a=x b=y",
		ModelUUID:        uuid.MustNewUUID().String(),
		ControllerUUID:   uuid.MustNewUUID().String(),
		ApplicationName:  "foo",
	}
	s.state.EXPECT().GetStorageResourceTagInfoForApplication(gomock.Any(),
		appUUID, "resource-tags").Return(ri, nil)

	svc := NewService(s.state, s.watcherFactory)
	tags, err := svc.GetStorageResourceTagsForApplication(c.Context(), appUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(tags, tc.DeepEquals, map[string]string{
		"a":                    "x",
		"b":                    "y",
		"juju-controller-uuid": ri.ControllerUUID,
		"juju-model-uuid":      ri.ModelUUID,
		"juju-storage-owner":   ri.ApplicationName,
	})
}
