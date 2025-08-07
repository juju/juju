// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	coreapplication "github.com/juju/juju/core/application"
	applicationtesting "github.com/juju/juju/core/application/testing"
	coreerror "github.com/juju/juju/core/errors"
	machinetesting "github.com/juju/juju/core/machine/testing"
	unittesting "github.com/juju/juju/core/unit/testing"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainlife "github.com/juju/juju/domain/life"
	modeltesting "github.com/juju/juju/core/model/testing"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	storagetesting "github.com/juju/juju/domain/storage/testing"
	"github.com/juju/juju/domain/storageprovisioning"
	storageprovisioningerrors "github.com/juju/juju/domain/storageprovisioning/errors"
	"github.com/juju/juju/internal/errors"
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

func (s *serviceSuite) TestGetStorageResourceTagsForModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ri := storageprovisioning.ModelResourceTagInfo{
		BaseResourceTags: "a=x b=y juju-drop-me=bad",
		ModelUUID:        modeltesting.GenModelUUID(c).String(),
		ControllerUUID:   uuid.MustNewUUID().String(),
	}
	s.state.EXPECT().GetStorageResourceTagInfoForModel(gomock.Any(), "resource-tags").Return(
		ri, nil,
	)

	svc := NewService(s.state, s.watcherFactory)
	tags, err := svc.GetStorageResourceTagsForModel(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(tags, tc.DeepEquals, map[string]string{
		"a":                    "x",
		"b":                    "y",
		"juju-controller-uuid": ri.ControllerUUID,
		"juju-model-uuid":      ri.ModelUUID,
	})
}

// TestGetStorageResourceTagsForApplication tests that the model config resource
// tags value is parsed and returned as key-value pairs with controller uuid,
// model uuid and application name overlayed.
func (s *serviceSuite) TestGetStorageResourceTagsForApplication(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	ri := storageprovisioning.ApplicationResourceTagInfo{
		ModelResourceTagInfo: storageprovisioning.ModelResourceTagInfo{
			BaseResourceTags: "a=x b=y juju-drop-me=bad",
			ModelUUID:        uuid.MustNewUUID().String(),
			ControllerUUID:   uuid.MustNewUUID().String(),
		},
		ApplicationName: "foo",
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

// TestGetStorageResourceTagsForApplicationErrors tests that the caller gets an
// error if the service layer errors.
func (s *serviceSuite) TestGetStorageResourceTagsForApplicationErrors(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().GetStorageResourceTagInfoForApplication(gomock.Any(),
		appUUID, "resource-tags").Return(storageprovisioning.ApplicationResourceTagInfo{},
		errors.New("oops"))

	svc := NewService(s.state, s.watcherFactory)
	_, err := svc.GetStorageResourceTagsForApplication(c.Context(), appUUID)
	c.Assert(err, tc.NotNil)
}

// TestGetStorageResourceTagsForApplicationInvalidApplicationUUID tests that the
// caller gets an error if the application uuid provided is not valid.
func (s *serviceSuite) TestGetStorageResourceTagsForApplicationInvalidApplicationUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := coreapplication.ID("$")
	svc := NewService(s.state, s.watcherFactory)
	_, err := svc.GetStorageResourceTagsForApplication(c.Context(), appUUID)
	c.Assert(err, tc.NotNil)
}

func (s *serviceSuite) TestGetStorageAttachmentIDsForUnit(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)

	s.state.EXPECT().GetStorageAttachmentIDsForUnit(gomock.Any(), unitUUID.String()).Return(
		[]string{"foo/1"}, nil,
	)

	svc := NewService(s.state, s.watcherFactory)
	storageIDs, err := svc.GetStorageAttachmentIDsForUnit(c.Context(), unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(storageIDs, tc.DeepEquals, []string{"foo/1"})
}

func (s *serviceSuite) TestGetStorageAttachmentIDsForUnitWithNotValidUnitUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(s.state, s.watcherFactory)
	_, err := svc.GetStorageAttachmentIDsForUnit(c.Context(), "")
	c.Assert(err, tc.ErrorIs, coreerror.NotValid)
}

func (s *serviceSuite) TestGetStorageAttachmentIDsForUnitWithUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)

	s.state.EXPECT().GetStorageAttachmentIDsForUnit(gomock.Any(), unitUUID.String()).Return(
		nil, applicationerrors.UnitNotFound,
	)

	svc := NewService(s.state, s.watcherFactory)
	_, err := svc.GetStorageAttachmentIDsForUnit(c.Context(), unitUUID)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *serviceSuite) TestGetAttachmentLife(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)
	storageInstanceUUID := storagetesting.GenStorageInstanceUUID(c)
	life := domainlife.Alive

	s.state.EXPECT().GetStorageInstanceUUIDByID(gomock.Any(), "foo/1").Return(
		storageInstanceUUID.String(), nil,
	)
	s.state.EXPECT().GetStorageAttachmentLife(gomock.Any(), unitUUID.String(), storageInstanceUUID.String()).Return(
		life, nil,
	)

	svc := NewService(s.state, s.watcherFactory)
	result, err := svc.GetStorageAttachmentLife(c.Context(), unitUUID, "foo/1")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.Equals, life)
}

func (s *serviceSuite) TestGetAttachmentLifeWithNotValidUnitUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(s.state, s.watcherFactory)
	_, err := svc.GetStorageAttachmentLife(c.Context(), "", "foo/1")
	c.Assert(err, tc.ErrorIs, coreerror.NotValid)
}

func (s *serviceSuite) TestGetAttachmentLifeWithStorageInstanceNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)
	s.state.EXPECT().GetStorageInstanceUUIDByID(gomock.Any(), "foo/1").Return(
		"", storageprovisioningerrors.StorageInstanceNotFound,
	)

	svc := NewService(s.state, s.watcherFactory)
	_, err := svc.GetStorageAttachmentLife(c.Context(), unitUUID, "foo/1")
	c.Assert(err, tc.ErrorIs, storageprovisioningerrors.StorageInstanceNotFound)
}

func (s *serviceSuite) TestGetAttachmentLifeWithUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)
	storageInstanceUUID := storagetesting.GenStorageInstanceUUID(c)

	s.state.EXPECT().GetStorageInstanceUUIDByID(gomock.Any(), "foo/1").Return(
		storageInstanceUUID.String(), nil,
	)
	s.state.EXPECT().GetStorageAttachmentLife(gomock.Any(), unitUUID.String(), storageInstanceUUID.String()).Return(
		-1, applicationerrors.UnitNotFound,
	)

	svc := NewService(s.state, s.watcherFactory)
	_, err := svc.GetStorageAttachmentLife(c.Context(), unitUUID, "foo/1")
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *serviceSuite) TestGetAttachmentLifeWithAttachmentNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)
	storageInstanceUUID := storagetesting.GenStorageInstanceUUID(c)

	s.state.EXPECT().GetStorageInstanceUUIDByID(gomock.Any(), "foo/1").Return(
		storageInstanceUUID.String(), nil,
	)
	s.state.EXPECT().GetStorageAttachmentLife(gomock.Any(), unitUUID.String(), storageInstanceUUID.String()).Return(
		-1, storageprovisioningerrors.StorageAttachmentNotFound,
	)

	svc := NewService(s.state, s.watcherFactory)
	_, err := svc.GetStorageAttachmentLife(c.Context(), unitUUID, "foo/1")
	c.Assert(err, tc.ErrorIs, storageprovisioningerrors.StorageAttachmentNotFound)
}
