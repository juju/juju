// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	coreapplication "github.com/juju/juju/core/application"
	applicationtesting "github.com/juju/juju/core/application/testing"
	"github.com/juju/juju/core/changestream"
	coreerrors "github.com/juju/juju/core/errors"
	coremachine "github.com/juju/juju/core/machine"
	machinetesting "github.com/juju/juju/core/machine/testing"
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/storageprovisioning"
	storageprovisioningerrors "github.com/juju/juju/domain/storageprovisioning/errors"
	domaintesting "github.com/juju/juju/domain/storageprovisioning/testing"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

// filesystemSuite provides a test suite for asserting the [Service] interface
// offered for filesystems.
type filesystemSuite struct {
	state          *MockState
	watcherFactory *MockWatcherFactory
}

// TestFilesystemSuite runs the tests in [filesystemSuite].
func TestFilesystemSuite(t *testing.T) {
	tc.Run(t, &filesystemSuite{})
}

func (s *filesystemSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	s.watcherFactory = NewMockWatcherFactory(ctrl)
	c.Cleanup(func() {
		s.state = nil
		s.watcherFactory = nil
	})
	return ctrl
}
func (s *filesystemSuite) TestGetFilesystemForID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	fs := storageprovisioning.Filesystem{
		BackingVolume: &storageprovisioning.FilesystemBackingVolume{
			VolumeID: "vol-123",
		},
		FilesystemID: "123",
		ProviderID:   "fs-1234",
		SizeMiB:      100,
	}
	fsUUID := domaintesting.GenFilesystemUUID(c)
	s.state.EXPECT().GetFilesystemUUIDForID(c.Context(), "1234").Return(fsUUID, nil)
	s.state.EXPECT().GetFilesystem(c.Context(), fsUUID).Return(fs, nil)

	result, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).GetFilesystemForID(c.Context(), "1234")

	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, fs)
}

func (s *filesystemSuite) TestGetFilesystemForIDNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetFilesystemUUIDForID(c.Context(), "1234").Return("", storageprovisioningerrors.FilesystemNotFound)
	_, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).GetFilesystemForID(
		c.Context(), "1234",
	)
	c.Assert(err, tc.ErrorIs, storageprovisioningerrors.FilesystemNotFound)
}

func (s *filesystemSuite) TestGetFilesystemForIDNotFound2(c *tc.C) {
	defer s.setupMocks(c).Finish()

	fsUUID := domaintesting.GenFilesystemUUID(c)
	s.state.EXPECT().GetFilesystemUUIDForID(c.Context(), "1234").Return(fsUUID, nil)
	s.state.EXPECT().GetFilesystem(c.Context(), fsUUID).Return(
		storageprovisioning.Filesystem{}, storageprovisioningerrors.FilesystemNotFound,
	)

	_, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).GetFilesystemForID(c.Context(), "1234")
	c.Assert(err, tc.ErrorIs, storageprovisioningerrors.FilesystemNotFound)
}

func (s *filesystemSuite) TestGetFilesystemAttachmentForUnit(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)
	netNodeUUID, err := domainnetwork.NewNetNodeUUID()
	c.Assert(err, tc.ErrorIsNil)
	fsUUID := domaintesting.GenFilesystemUUID(c)
	fsaUUID := domaintesting.GenFilesystemAttachmentUUID(c)
	c.Assert(err, tc.ErrorIsNil)

	attachment := storageprovisioning.FilesystemAttachment{
		FilesystemID: "123",
		MountPoint:   "/mnt/fs-1234",
		ReadOnly:     true,
	}
	s.state.EXPECT().GetFilesystemUUIDForID(gomock.Any(), "1234").Return(fsUUID, nil)
	s.state.EXPECT().GetUnitNetNodeUUID(c.Context(), unitUUID).Return(netNodeUUID, nil)
	s.state.EXPECT().GetFilesystemAttachmentUUIDForFilesystemNetNode(gomock.Any(), fsUUID, netNodeUUID).Return(fsaUUID, nil)
	s.state.EXPECT().GetFilesystemAttachment(c.Context(), fsaUUID).Return(attachment, nil)

	svc := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))
	result, err := svc.GetFilesystemAttachmentForUnit(
		c.Context(), "1234", unitUUID,
	)

	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, attachment)
}

func (s *filesystemSuite) TestGetFilesystemAttachmentForUnitNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))
	_, err := svc.GetFilesystemAttachmentForUnit(
		c.Context(), "1234", coreunit.UUID(""),
	)

	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *filesystemSuite) TestGetFilesystemAttachmentForUnitUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	unitUUID := unittesting.GenUnitUUID(c)
	s.state.EXPECT().GetUnitNetNodeUUID(c.Context(), unitUUID).Return("", applicationerrors.UnitNotFound)
	svc := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))

	_, err := svc.GetFilesystemAttachmentForUnit(c.Context(), "1234", unitUUID)
	c.Check(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *filesystemSuite) TestGetFilesystemAttachmentForUnitAttachmentNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	unitUUID := unittesting.GenUnitUUID(c)
	netNodeUUID, err := domainnetwork.NewNetNodeUUID()
	c.Assert(err, tc.ErrorIsNil)
	fsUUID := domaintesting.GenFilesystemUUID(c)
	s.state.EXPECT().GetUnitNetNodeUUID(c.Context(), unitUUID).Return(netNodeUUID, nil).AnyTimes()
	s.state.EXPECT().GetFilesystemUUIDForID(gomock.Any(), "1234").Return(fsUUID, nil).AnyTimes()
	svc := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))

	// Possible not found scenario 1:
	s.state.EXPECT().GetFilesystemAttachmentUUIDForFilesystemNetNode(gomock.Any(), fsUUID, netNodeUUID).Return(
		"", storageprovisioningerrors.FilesystemAttachmentNotFound,
	)
	_, err = svc.GetFilesystemAttachmentForUnit(c.Context(), "1234", unitUUID)
	c.Check(err, tc.ErrorIs, storageprovisioningerrors.FilesystemAttachmentNotFound)

	// Possible not found scenario 2:
	fsaUUID := domaintesting.GenFilesystemAttachmentUUID(c)
	s.state.EXPECT().GetFilesystemAttachmentUUIDForFilesystemNetNode(gomock.Any(), fsUUID, netNodeUUID).Return(
		fsaUUID, nil,
	)
	s.state.EXPECT().GetFilesystemAttachment(c.Context(), fsaUUID).Return(
		storageprovisioning.FilesystemAttachment{}, storageprovisioningerrors.FilesystemAttachmentNotFound,
	)
	_, err = svc.GetFilesystemAttachmentForUnit(c.Context(), "1234", unitUUID)
	c.Check(err, tc.ErrorIs, storageprovisioningerrors.FilesystemAttachmentNotFound)
}

func (s *filesystemSuite) TestGetFilesystemAttachmentForUnitFilesystemNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	unitUUID := unittesting.GenUnitUUID(c)
	netNodeUUID, err := domainnetwork.NewNetNodeUUID()
	c.Assert(err, tc.ErrorIsNil)
	s.state.EXPECT().GetUnitNetNodeUUID(c.Context(), unitUUID).Return(netNodeUUID, nil).AnyTimes()
	s.state.EXPECT().GetFilesystemUUIDForID(gomock.Any(), "1234").Return(
		"", storageprovisioningerrors.FilesystemNotFound,
	)
	svc := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))

	_, err = svc.GetFilesystemAttachmentForUnit(c.Context(), "1234", unitUUID)
	c.Check(err, tc.ErrorIs, storageprovisioningerrors.FilesystemNotFound)
}

func (s *filesystemSuite) TestGetFilesystemAttachmentForMachine(c *tc.C) {
	defer s.setupMocks(c).Finish()
	machineUUID := machinetesting.GenUUID(c)
	netNodeUUID, err := domainnetwork.NewNetNodeUUID()
	c.Assert(err, tc.ErrorIsNil)
	attachment := storageprovisioning.FilesystemAttachment{
		FilesystemID: "123",
		MountPoint:   "/mnt/fs-1234",
		ReadOnly:     true,
	}
	fsUUID := domaintesting.GenFilesystemUUID(c)
	fsaUUID := domaintesting.GenFilesystemAttachmentUUID(c)
	s.state.EXPECT().GetMachineNetNodeUUID(c.Context(), machineUUID).Return(netNodeUUID, nil)
	s.state.EXPECT().GetFilesystemUUIDForID(c.Context(), "1234").Return(fsUUID, nil)
	s.state.EXPECT().GetFilesystemAttachmentUUIDForFilesystemNetNode(gomock.Any(), fsUUID, netNodeUUID).Return(fsaUUID, nil)
	s.state.EXPECT().GetFilesystemAttachment(c.Context(), fsaUUID).Return(attachment, nil)
	svc := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))

	result, err := svc.GetFilesystemAttachmentForMachine(
		c.Context(), "1234", machineUUID,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, attachment)
}

func (s *filesystemSuite) TestGetFilesystemAttachmentForMachineNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))

	_, err := svc.GetFilesystemAttachmentForMachine(c.Context(), "1234", "")
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *filesystemSuite) TestGetFilesystemAttachmentForMachineMachineNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	fsUUID := domaintesting.GenFilesystemUUID(c)
	machineUUID := machinetesting.GenUUID(c)
	s.state.EXPECT().GetFilesystemUUIDForID(c.Context(), "1234").Return(fsUUID, nil).AnyTimes()
	s.state.EXPECT().GetMachineNetNodeUUID(c.Context(), machineUUID).Return("", machineerrors.MachineNotFound)
	svc := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))

	_, err := svc.GetFilesystemAttachmentForMachine(c.Context(), "1234", machineUUID)
	c.Check(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *filesystemSuite) TestGetFilesystemAttachmentForMachineAttachmentNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	machineUUID := machinetesting.GenUUID(c)
	netNodeUUID, err := domainnetwork.NewNetNodeUUID()
	c.Assert(err, tc.ErrorIsNil)
	fsUUID := domaintesting.GenFilesystemUUID(c)
	s.state.EXPECT().GetMachineNetNodeUUID(c.Context(), machineUUID).Return(netNodeUUID, nil).AnyTimes()
	s.state.EXPECT().GetFilesystemUUIDForID(c.Context(), "1234").Return(fsUUID, nil).AnyTimes()
	svc := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))

	// scenario 1:
	s.state.EXPECT().GetFilesystemAttachmentUUIDForFilesystemNetNode(gomock.Any(), fsUUID, netNodeUUID).Return(
		"", storageprovisioningerrors.FilesystemAttachmentNotFound,
	)
	_, err = svc.GetFilesystemAttachmentForMachine(c.Context(), "1234", machineUUID)
	c.Check(err, tc.ErrorIs, storageprovisioningerrors.FilesystemAttachmentNotFound)

	// scenario 2:
	fsaUUID := domaintesting.GenFilesystemAttachmentUUID(c)
	s.state.EXPECT().GetFilesystemAttachmentUUIDForFilesystemNetNode(gomock.Any(), fsUUID, netNodeUUID).Return(
		fsaUUID, nil,
	)
	s.state.EXPECT().GetFilesystemAttachment(gomock.Any(), fsaUUID).Return(
		storageprovisioning.FilesystemAttachment{}, storageprovisioningerrors.FilesystemAttachmentNotFound,
	)
	_, err = svc.GetFilesystemAttachmentForMachine(c.Context(), "1234", machineUUID)
	c.Check(err, tc.ErrorIs, storageprovisioningerrors.FilesystemAttachmentNotFound)
}

func (s *filesystemSuite) TestGetFilesystemAttachmentForMachineFilesystemNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	machineUUID := machinetesting.GenUUID(c)
	netNodeUUID, err := domainnetwork.NewNetNodeUUID()
	c.Assert(err, tc.ErrorIsNil)
	s.state.EXPECT().GetMachineNetNodeUUID(c.Context(), machineUUID).
		Return(netNodeUUID, nil).AnyTimes()
	s.state.EXPECT().GetFilesystemUUIDForID(gomock.Any(), "1234").Return(
		"", storageprovisioningerrors.FilesystemNotFound,
	)
	svc := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))

	_, err = svc.GetFilesystemAttachmentForMachine(c.Context(), "1234", machineUUID)
	c.Check(err, tc.ErrorIs, storageprovisioningerrors.FilesystemNotFound)
}

// TestWatchModelProvisionedFilesystems tests that the model provisioned
// fileystem watcher is correctly created with the provided namespace from state
// and the initial query.
//
// This is a test that asserts the correct values are used and not the behaviour
// of the watcher itself.
func (s *filesystemSuite) TestWatchModelProvisionedFilesystems(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().InitialWatchStatementModelProvisionedFilesystems().Return(
		"test_namespace", namespaceQueryReturningError(c.T),
	)
	matcher := eventSourceFilterMatcher{
		ChangeMask: changestream.All,
		Namespace:  "test_namespace",
	}
	s.watcherFactory.EXPECT().NewNamespaceWatcher(gomock.Any(), gomock.Any(), gomock.Any(), matcher)

	_, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		WatchModelProvisionedFilesystems(c.Context())
	c.Check(err, tc.ErrorIsNil)
}

// TestWatchMachineProvisionedFilesystems tests that the machine provisioned
// fileystem watcher is correctly created with the provided namespace from state
// and the initial query.
//
// This is a test that asserts the correct values are used and not the behaviour
// of the watcher itself.
func (s *filesystemSuite) TestWatchMachineProvisionedFilesystems(c *tc.C) {
	defer s.setupMocks(c).Finish()
	machineUUID := machinetesting.GenUUID(c)
	netNodeUUID, err := domainnetwork.NewNetNodeUUID()
	c.Assert(err, tc.ErrorIsNil)

	s.state.EXPECT().GetMachineNetNodeUUID(gomock.Any(), machineUUID).Return(
		netNodeUUID, nil,
	)
	s.state.EXPECT().InitialWatchStatementMachineProvisionedFilesystems(
		netNodeUUID,
	).Return(
		"test_namespace", namespaceLifeQueryReturningError(c.T),
	)
	matcher := eventSourcePredFilterMatcher{
		ChangeMask: changestream.All,
		Namespace:  "test_namespace",
		Predicate:  netNodeUUID.String(),
	}
	s.watcherFactory.EXPECT().NewNamespaceMapperWatcher(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), matcher,
	)

	_, err = NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		WatchMachineProvisionedFilesystems(c.Context(), machineUUID)
	c.Check(err, tc.ErrorIsNil)
}

// TestWatchMachineProvisionedFilesystemsNotValid tests that the caller gets
// back an error satisfying [coreerrors.NotValid] when the provided machine uuid
// is not valid.
func (s *filesystemSuite) TestWatchMachineProvisionedFilesystemsNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		WatchMachineProvisionedFilesystems(c.Context(), coremachine.UUID(""))
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestWatchMachineProvisionedFilesystemsNotFound tests that the caller gets
// back an error satisfying [machineerrors.MachineNotFound] when no machine
// exists for the provided machine uuid.
func (s *filesystemSuite) TestWatchMachineProvisionedFilesystemsNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	machineUUID := machinetesting.GenUUID(c)

	s.state.EXPECT().GetMachineNetNodeUUID(gomock.Any(), machineUUID).Return(
		"", machineerrors.MachineNotFound,
	)

	_, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		WatchMachineProvisionedFilesystems(c.Context(), machineUUID)
	c.Check(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

// TestWatchModelProvisionedFilesystemAttachments tests that the model
// provisioned fileystem attachments watcher is correctly created with the
// provided namespace from state and the initial query.
//
// This is a test that asserts the correct values are used and not the behaviour
// of the watcher itself.
func (s *filesystemSuite) TestWatchModelProvisionedFilesystemAttachments(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().InitialWatchStatementModelProvisionedFilesystemAttachments().Return(
		"test_namespace", namespaceQueryReturningError(c.T),
	)
	matcher := eventSourceFilterMatcher{
		ChangeMask: changestream.All,
		Namespace:  "test_namespace",
	}
	s.watcherFactory.EXPECT().NewNamespaceWatcher(gomock.Any(), gomock.Any(), gomock.Any(), matcher)

	_, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		WatchModelProvisionedFilesystemAttachments(c.Context())
	c.Check(err, tc.ErrorIsNil)
}

// TestWatchMachineProvisionedFilesystemAttachments tests that the machine
// provisioned fileystem watcher is correctly created with the provided
// namespace from state and the initial query.
//
// This is a test that asserts the correct values are used and not the behaviour
// of the watcher itself.
func (s *filesystemSuite) TestWatchMachineProvisionedFilesystemAttachments(c *tc.C) {
	defer s.setupMocks(c).Finish()
	machineUUID := machinetesting.GenUUID(c)
	netNodeUUID, err := domainnetwork.NewNetNodeUUID()
	c.Assert(err, tc.ErrorIsNil)

	s.state.EXPECT().GetMachineNetNodeUUID(gomock.Any(), machineUUID).Return(
		netNodeUUID, nil,
	)
	s.state.EXPECT().InitialWatchStatementMachineProvisionedFilesystemAttachments(
		netNodeUUID,
	).Return(
		"test_namespace", namespaceLifeQueryReturningError(c.T),
	)
	matcher := eventSourcePredFilterMatcher{
		ChangeMask: changestream.All,
		Namespace:  "test_namespace",
		Predicate:  netNodeUUID.String(),
	}
	s.watcherFactory.EXPECT().NewNamespaceMapperWatcher(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), matcher,
	)

	_, err = NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		WatchMachineProvisionedFilesystemAttachments(c.Context(), machineUUID)
	c.Check(err, tc.ErrorIsNil)
}

// TestWatchMachineProvisionedFilesystemAttachmentssNotValid tests that the
// caller gets back an error satisfying [coreerrors.NotValid] when the provided
// machine uuid is not valid.
func (s *filesystemSuite) TestWatchMachineProvisionedFilesystemAttachmentsNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		WatchMachineProvisionedFilesystems(c.Context(), coremachine.UUID(""))
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestWatchMachineProvisionedFilesystemAttachmentsNotFound tests that the
// caller gets back an error satisfying [machineerrors.MachineNotFound] when no
// machine exists for the provided machine uuid.
func (s *filesystemSuite) TestWatchMachineProvisionedFilesystemAttachmentsNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	machineUUID := machinetesting.GenUUID(c)

	s.state.EXPECT().GetMachineNetNodeUUID(gomock.Any(), machineUUID).Return(
		"", machineerrors.MachineNotFound,
	)

	_, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		WatchMachineProvisionedFilesystemAttachments(c.Context(), machineUUID)
	c.Check(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

// TestGetFilesystemsTemplateForApplication tests the caller gets filesystem
// templates back.
func (s *filesystemSuite) TestGetFilesystemsTemplateForApplication(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appID := applicationtesting.GenApplicationUUID(c)
	expectedResult := []storageprovisioning.FilesystemTemplate{{
		StorageName:  "a",
		Count:        1,
		MaxCount:     10,
		SizeMiB:      1234,
		ProviderType: "foo",
		ReadOnly:     true,
		Location:     "bar",
		Attributes: map[string]string{
			"laz": "baz",
		},
	}}
	s.state.EXPECT().GetFilesystemTemplatesForApplication(gomock.Any(), appID).
		Return(expectedResult, nil)

	svc := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))
	result, err := svc.GetFilesystemTemplatesForApplication(c.Context(), appID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, expectedResult)
}

// TestGetFilesystemsTemplateForApplicationErrors tests the caller gets an error when
// the state errors.
func (s *filesystemSuite) TestGetFilesystemsTemplateForApplicationErrors(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appID := applicationtesting.GenApplicationUUID(c)
	s.state.EXPECT().GetFilesystemTemplatesForApplication(gomock.Any(), appID).
		Return(nil, errors.New("oops"))

	svc := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))
	_, err := svc.GetFilesystemTemplatesForApplication(c.Context(), appID)
	c.Assert(err, tc.NotNil)
}

// TestGetFilesystemsTemplateForApplicationInvalidApplicationUUID tests the
// caller gets an error when the application UUID is invalid.
func (s *filesystemSuite) TestGetFilesystemsTemplateForApplicationInvalidApplicationUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appID := coreapplication.ID("$")
	svc := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))
	_, err := svc.GetFilesystemTemplatesForApplication(c.Context(), appID)
	c.Assert(err, tc.NotNil)
}

func (s *filesystemSuite) TestSetFilesystemProvisionedInfo(c *tc.C) {
	defer s.setupMocks(c).Finish()

	fsID := "123"
	fsUUID := domaintesting.GenFilesystemUUID(c)
	info := storageprovisioning.FilesystemProvisionedInfo{
		ProviderID: "x",
		SizeMiB:    100,
	}

	s.state.EXPECT().GetFilesystemUUIDForID(gomock.Any(), fsID).Return(fsUUID,
		nil)
	s.state.EXPECT().SetFilesystemProvisionedInfo(gomock.Any(), fsUUID, info).
		Return(nil)

	svc := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))
	err := svc.SetFilesystemProvisionedInfo(c.Context(), fsID, info)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *filesystemSuite) TestSetFilesystemAttachmentProvisionedInfoForMachine(c *tc.C) {
	defer s.setupMocks(c).Finish()

	fsID := "123"
	fsUUID := domaintesting.GenFilesystemUUID(c)
	fsAttachmentUUID, err := storageprovisioning.NewFilesystemAttachmentUUID()
	c.Assert(err, tc.ErrorIsNil)
	info := storageprovisioning.FilesystemAttachmentProvisionedInfo{
		MountPoint: "x",
		ReadOnly:   true,
	}
	netNodeUUID, err := domainnetwork.NewNetNodeUUID()
	c.Assert(err, tc.ErrorIsNil)
	machineUUID := machinetesting.GenUUID(c)

	s.state.EXPECT().GetFilesystemUUIDForID(gomock.Any(), fsID).Return(fsUUID,
		nil)
	s.state.EXPECT().GetMachineNetNodeUUID(gomock.Any(), machineUUID).Return(
		netNodeUUID, nil)
	s.state.EXPECT().GetFilesystemAttachmentUUIDForFilesystemNetNode(
		gomock.Any(), fsUUID, netNodeUUID).Return(fsAttachmentUUID, nil)
	s.state.EXPECT().SetFilesystemAttachmentProvisionedInfo(gomock.Any(),
		fsAttachmentUUID, info).Return(nil)

	svc := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))
	err = svc.SetFilesystemAttachmentProvisionedInfoForMachine(c.Context(),
		fsID, machineUUID, info)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *filesystemSuite) TestSetFilesystemAttachmentProvisionedInfoForMachineInvalidMachineUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	fsID := "123"
	info := storageprovisioning.FilesystemAttachmentProvisionedInfo{
		MountPoint: "x",
		ReadOnly:   true,
	}
	machineUUID := coremachine.UUID("$")

	svc := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))
	err := svc.SetFilesystemAttachmentProvisionedInfoForMachine(c.Context(),
		fsID, machineUUID, info)
	c.Assert(err, tc.NotNil)
}

func (s *filesystemSuite) TestSetFilesystemAttachmentProvisionedInfoForUnit(c *tc.C) {
	defer s.setupMocks(c).Finish()

	fsID := "123"
	fsUUID := domaintesting.GenFilesystemUUID(c)
	fsAttachmentUUID, err := storageprovisioning.NewFilesystemAttachmentUUID()
	c.Assert(err, tc.ErrorIsNil)
	info := storageprovisioning.FilesystemAttachmentProvisionedInfo{
		MountPoint: "x",
		ReadOnly:   true,
	}
	netNodeUUID, err := domainnetwork.NewNetNodeUUID()
	c.Assert(err, tc.ErrorIsNil)
	unitUUID := unittesting.GenUnitUUID(c)

	s.state.EXPECT().GetFilesystemUUIDForID(gomock.Any(), fsID).Return(fsUUID,
		nil)
	s.state.EXPECT().GetUnitNetNodeUUID(gomock.Any(), unitUUID).Return(
		netNodeUUID, nil)
	s.state.EXPECT().GetFilesystemAttachmentUUIDForFilesystemNetNode(
		gomock.Any(), fsUUID, netNodeUUID).Return(fsAttachmentUUID, nil)
	s.state.EXPECT().SetFilesystemAttachmentProvisionedInfo(gomock.Any(),
		fsAttachmentUUID, info).Return(nil)

	svc := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))
	err = svc.SetFilesystemAttachmentProvisionedInfoForUnit(c.Context(),
		fsID, unitUUID, info)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *filesystemSuite) TestSetFilesystemAttachmentProvisionedInfoForUnitInvalidUnitUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	fsID := "123"
	info := storageprovisioning.FilesystemAttachmentProvisionedInfo{
		MountPoint: "x",
		ReadOnly:   true,
	}
	unitUUID := coreunit.UUID("$")

	svc := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))
	err := svc.SetFilesystemAttachmentProvisionedInfoForUnit(c.Context(),
		fsID, unitUUID, info)
	c.Assert(err, tc.NotNil)
}

// TestGetFilesystemParams is a happy path test of [Service.GetFilesystemParams].
func (s *filesystemSuite) TestGetFilesystemParams(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))
	fsUUID := domaintesting.GenFilesystemUUID(c)

	s.state.EXPECT().GetFilesystemParams(gomock.Any(), fsUUID).Return(
		storageprovisioning.FilesystemParams{
			Attributes: map[string]string{
				"foo": "bar",
			},
			ID:       "spid",
			Provider: "myprovider",
			SizeMiB:  10,
		}, nil,
	)

	params, err := svc.GetFilesystemParams(c.Context(), fsUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(params, tc.DeepEquals, storageprovisioning.FilesystemParams{
		Attributes: map[string]string{
			"foo": "bar",
		},
		ID:       "spid",
		Provider: "myprovider",
		SizeMiB:  10,
	})
}

// TestGetFilesystemParamsNotFound tests that when no filesystem exists a call
// to [Service.GetFilesystemparams] returns an error satisfying
// [storageprovisioningerrors.FilesystemNotFound].
func (s *filesystemSuite) TestGetFilesystemParamsNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))
	fsUUID := domaintesting.GenFilesystemUUID(c)

	s.state.EXPECT().GetFilesystemParams(gomock.Any(), fsUUID).Return(
		storageprovisioning.FilesystemParams{},
		storageprovisioningerrors.FilesystemNotFound,
	)

	_, err := svc.GetFilesystemParams(c.Context(), fsUUID)
	c.Check(err, tc.ErrorIs, storageprovisioningerrors.FilesystemNotFound)
}

// TestGetFilesystemAttachmentParams is a happy path test of
// [Service.GetFilesystemAttachmentParams].
func (s *filesystemSuite) TestGetFilesystemAttachmentParams(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))
	fsaUUID := domaintesting.GenFilesystemAttachmentUUID(c)

	s.state.EXPECT().GetFilesystemAttachmentParams(gomock.Any(), fsaUUID).Return(
		storageprovisioning.FilesystemAttachmentParams{
			MachineInstanceID: "inst-1",
			Provider:          "myprovider",
			ProviderID:        "p-123",
			MountPoint:        "/var/foo",
			ReadOnly:          true,
		}, nil,
	)

	params, err := svc.GetFilesystemAttachmentParams(c.Context(), fsaUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(params, tc.DeepEquals, storageprovisioning.FilesystemAttachmentParams{
		MachineInstanceID: "inst-1",
		Provider:          "myprovider",
		ProviderID:        "p-123",
		MountPoint:        "/var/foo",
		ReadOnly:          true,
	})
}

// TestGetFilesystemAttachmentParamsNotFound tests that when no filesystem
// attachment exists for a call to [Service.GetFilesystemAttachmentParams]
// returns an error satisfying [storageprovisioningerrors.FilesystemNotFound].
func (s *filesystemSuite) TestGetFilesystemAttachmentParamsNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))
	fsaUUID := domaintesting.GenFilesystemAttachmentUUID(c)

	s.state.EXPECT().GetFilesystemAttachmentParams(gomock.Any(), fsaUUID).Return(
		storageprovisioning.FilesystemAttachmentParams{},
		storageprovisioningerrors.FilesystemAttachmentNotFound,
	)

	_, err := svc.GetFilesystemAttachmentParams(c.Context(), fsaUUID)
	c.Check(err, tc.ErrorIs, storageprovisioningerrors.FilesystemAttachmentNotFound)
}
