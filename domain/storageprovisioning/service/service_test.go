// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"fmt"
	"testing"

	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/changestream"
	coreerrors "github.com/juju/juju/core/errors"
	machinetesting "github.com/juju/juju/core/machine/testing"
	modeltesting "github.com/juju/juju/core/model/testing"
	unittesting "github.com/juju/juju/core/unit/testing"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/blockdevice"
	domainlife "github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	storagetesting "github.com/juju/juju/domain/storage/testing"
	"github.com/juju/juju/domain/storageprovisioning"
	storageprovisioningtesting "github.com/juju/juju/domain/storageprovisioning/testing"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
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

	_, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		WatchMachineCloudInstance(
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

	_, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		WatchMachineCloudInstance(
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

	svc := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))
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

	appUUID := tc.Must(c, coreapplication.NewUUID)

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

	svc := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))
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

	appUUID := tc.Must(c, coreapplication.NewUUID)

	s.state.EXPECT().GetStorageResourceTagInfoForApplication(gomock.Any(),
		appUUID, "resource-tags").Return(storageprovisioning.ApplicationResourceTagInfo{},
		errors.New("oops"))

	svc := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))
	_, err := svc.GetStorageResourceTagsForApplication(c.Context(), appUUID)
	c.Assert(err, tc.NotNil)
}

// TestGetStorageResourceTagsForApplicationInvalidApplicationUUID tests that the
// caller gets an error if the application uuid provided is not valid.
func (s *serviceSuite) TestGetStorageResourceTagsForApplicationInvalidApplicationUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := coreapplication.UUID("$")
	svc := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))
	_, err := svc.GetStorageResourceTagsForApplication(c.Context(), appUUID)
	c.Assert(err, tc.NotNil)
}

func (s *serviceSuite) TestGetStorageAttachmentIDsForUnit(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)

	s.state.EXPECT().GetStorageAttachmentIDsForUnit(gomock.Any(), unitUUID).Return(
		[]string{"foo/1"}, nil,
	)

	svc := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))
	storageIDs, err := svc.GetStorageAttachmentIDsForUnit(c.Context(), unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(storageIDs, tc.DeepEquals, []string{"foo/1"})
}

func (s *serviceSuite) TestGetStorageAttachmentIDsForUnitWithNotValidUnitUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))
	_, err := svc.GetStorageAttachmentIDsForUnit(c.Context(), "")
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *serviceSuite) TestGetStorageAttachmentIDsForUnitWithUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)

	s.state.EXPECT().GetStorageAttachmentIDsForUnit(gomock.Any(), unitUUID).Return(
		nil, applicationerrors.UnitNotFound,
	)

	svc := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))
	_, err := svc.GetStorageAttachmentIDsForUnit(c.Context(), unitUUID)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *serviceSuite) TestGetAttachmentLife(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)
	storageInstanceUUID := storagetesting.GenStorageInstanceUUID(c)
	life := domainlife.Alive

	s.state.EXPECT().GetStorageInstanceUUIDByID(gomock.Any(), "foo/1").Return(
		storageInstanceUUID, nil,
	)
	s.state.EXPECT().GetStorageAttachmentLife(gomock.Any(), unitUUID, storageInstanceUUID).Return(
		life, nil,
	)

	svc := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))
	result, err := svc.GetStorageAttachmentLife(c.Context(), unitUUID, "foo/1")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.Equals, life)
}

func (s *serviceSuite) TestGetAttachmentLifeWithNotValidUnitUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))
	_, err := svc.GetStorageAttachmentLife(c.Context(), "", "foo/1")
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *serviceSuite) TestGetAttachmentLifeWithStorageInstanceNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)
	s.state.EXPECT().GetStorageInstanceUUIDByID(gomock.Any(), "foo/1").Return(
		"", storageerrors.StorageInstanceNotFound,
	)

	svc := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))
	_, err := svc.GetStorageAttachmentLife(c.Context(), unitUUID, "foo/1")
	c.Assert(err, tc.ErrorIs, storageerrors.StorageInstanceNotFound)
}

func (s *serviceSuite) TestGetAttachmentLifeWithUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)
	storageInstanceUUID := storagetesting.GenStorageInstanceUUID(c)

	s.state.EXPECT().GetStorageInstanceUUIDByID(gomock.Any(), "foo/1").Return(
		storageInstanceUUID, nil,
	)
	s.state.EXPECT().GetStorageAttachmentLife(
		gomock.Any(), unitUUID, storageInstanceUUID,
	).Return(-1, applicationerrors.UnitNotFound)

	svc := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))
	_, err := svc.GetStorageAttachmentLife(c.Context(), unitUUID, "foo/1")
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *serviceSuite) TestGetAttachmentLifeWithAttachmentNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)
	storageInstanceUUID := storagetesting.GenStorageInstanceUUID(c)

	s.state.EXPECT().GetStorageInstanceUUIDByID(gomock.Any(), "foo/1").Return(
		storageInstanceUUID, nil,
	)
	s.state.EXPECT().GetStorageAttachmentLife(
		gomock.Any(), unitUUID, storageInstanceUUID,
	).Return(-1, storageerrors.StorageAttachmentNotFound)

	svc := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))
	_, err := svc.GetStorageAttachmentLife(c.Context(), unitUUID, "foo/1")
	c.Assert(err, tc.ErrorIs, storageerrors.StorageAttachmentNotFound)
}

func (s *serviceSuite) TestGetStorageAttachmentUUIDForUnit(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)
	storageAttachmentUUID := storageprovisioningtesting.GenStorageAttachmentUUID(c)

	s.state.EXPECT().GetStorageAttachmentUUIDForUnit(
		gomock.Any(), "foo/1", unitUUID,
	).Return(storageAttachmentUUID, nil)

	svc := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))
	result, err := svc.GetStorageAttachmentUUIDForUnit(c.Context(), "foo/1", unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.Equals, storageAttachmentUUID)
}

func (s *serviceSuite) TestGetStorageAttachmentUUIDForUnitWithUnitUUIDNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))
	_, err := svc.GetStorageAttachmentUUIDForUnit(c.Context(), "foo/1", "")
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *serviceSuite) TestGetStorageAttachmentUUIDForUnitWithUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)

	s.state.EXPECT().GetStorageAttachmentUUIDForUnit(
		gomock.Any(), "foo/1", unitUUID,
	).Return("", applicationerrors.UnitNotFound)

	svc := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))
	_, err := svc.GetStorageAttachmentUUIDForUnit(c.Context(), "foo/1", unitUUID)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *serviceSuite) TestGetStorageAttachmentUUIDForUnitWithStorageInstanceNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)

	s.state.EXPECT().GetStorageAttachmentUUIDForUnit(gomock.Any(), "foo/1", unitUUID).Return(
		"", storageerrors.StorageInstanceNotFound,
	)

	svc := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))
	_, err := svc.GetStorageAttachmentUUIDForUnit(c.Context(), "foo/1", unitUUID)
	c.Assert(err, tc.ErrorIs, storageerrors.StorageInstanceNotFound)
}

func (s *serviceSuite) TestGetStorageAttachmentUUIDForUnitWithStorageAttachmentNotFoundd(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)

	s.state.EXPECT().GetStorageAttachmentUUIDForUnit(gomock.Any(), "foo/1", unitUUID).Return(
		"", storageerrors.StorageAttachmentNotFound,
	)

	svc := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))
	_, err := svc.GetStorageAttachmentUUIDForUnit(c.Context(), "foo/1", unitUUID)
	c.Assert(err, tc.ErrorIs, storageerrors.StorageAttachmentNotFound)
}

func (s *serviceSuite) TestWatchStorageAttachmentsForUnit(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)

	s.state.EXPECT().InitialWatchStatementForUnitStorageAttachments(gomock.Any(), unitUUID).
		Return(
			"namespace_foo",
			namespaceLifeQueryReturningError(c.T),
		)
	matcher := eventSourcePredFilterMatcher{
		ChangeMask: changestream.All,
		Namespace:  "namespace_foo",
		Predicate:  unitUUID.String(),
	}
	s.watcherFactory.EXPECT().NewNamespaceMapperWatcher(
		gomock.Any(), gomock.Any(),
		fmt.Sprintf("storage attachments watcher for unit %q", unitUUID),
		gomock.Any(),
		matcher,
	)

	svc := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))
	_, err := svc.WatchStorageAttachmentsForUnit(c.Context(), unitUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestWatchStorageAttachment(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageAttachmentUUID := storageprovisioningtesting.GenStorageAttachmentUUID(c)

	s.state.EXPECT().NamespaceForStorageAttachment().Return("foo_namespace")
	s.watcherFactory.EXPECT().NewNotifyWatcher(gomock.Any(),
		fmt.Sprintf("storage attachment watcher for %q", storageAttachmentUUID),
		eventSourcePredFilterMatcher{
			ChangeMask: changestream.All,
			Namespace:  "foo_namespace",
			Predicate:  storageAttachmentUUID.String(),
		},
	)

	svc := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))
	_, err := svc.WatchStorageAttachment(c.Context(), storageAttachmentUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestGetUnitStorageAttachmentInfoForVolume(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageAttachmentUUID := storageprovisioningtesting.GenStorageAttachmentUUID(c)
	bdUUID := tc.Must(c, blockdevice.NewBlockDeviceUUID)
	info := storageprovisioning.StorageAttachmentInfo{
		Kind:            domainstorage.StorageKindBlock,
		Life:            domainlife.Alive,
		BlockDeviceUUID: bdUUID,
	}

	s.state.EXPECT().GetStorageAttachmentInfo(gomock.Any(), storageAttachmentUUID).
		Return(info, nil)

	svc := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))
	result, err := svc.GetUnitStorageAttachmentInfo(c.Context(), storageAttachmentUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.Equals, storageprovisioning.StorageAttachmentInfo{
		Kind:            domainstorage.StorageKindBlock,
		Life:            domainlife.Alive,
		BlockDeviceUUID: bdUUID,
	})
}

func (s *serviceSuite) TestGetUnitStorageAttachmentInfoForFilesystem(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageAttachmentUUID := storageprovisioningtesting.GenStorageAttachmentUUID(c)
	info := storageprovisioning.StorageAttachmentInfo{
		Kind:                 domainstorage.StorageKindFilesystem,
		Life:                 domainlife.Alive,
		FilesystemMountPoint: "/mnt/data",
	}

	s.state.EXPECT().GetStorageAttachmentInfo(gomock.Any(), storageAttachmentUUID).
		Return(info, nil)

	svc := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))
	result, err := svc.GetUnitStorageAttachmentInfo(c.Context(), storageAttachmentUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.Equals, storageprovisioning.StorageAttachmentInfo{
		Kind:                 domainstorage.StorageKindFilesystem,
		Life:                 domainlife.Alive,
		FilesystemMountPoint: "/mnt/data",
	})
}
