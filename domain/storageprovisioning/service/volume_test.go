// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"

	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/core/changestream"
	coreerrors "github.com/juju/juju/core/errors"
	coremachine "github.com/juju/juju/core/machine"
	machinetesting "github.com/juju/juju/core/machine/testing"
	unittesting "github.com/juju/juju/core/unit/testing"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/blockdevice"
	blockdeviceerrors "github.com/juju/juju/domain/blockdevice/errors"
	domainlife "github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/storageprovisioning"
	storageprovisioningerrors "github.com/juju/juju/domain/storageprovisioning/errors"
	domaintesting "github.com/juju/juju/domain/storageprovisioning/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

// volumeSuite provides a test suite for asserting the [Service] interface
// offered for volumes.
type volumeSuite struct {
	state          *MockState
	watcherFactory *MockWatcherFactory
}

// TestVolumeSuite runs the tests defined by [volumeSuite].
func TestVolumeSuite(t *testing.T) {
	tc.Run(t, &volumeSuite{})
}

func (s *volumeSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	s.watcherFactory = NewMockWatcherFactory(ctrl)
	c.Cleanup(func() {
		s.state = nil
		s.watcherFactory = nil
	})
	return ctrl
}

// TestWatchModelProvisionedVolumes tests that the model provisioned
// volume watcher is correctly created with the provided namespace from state
// and the initial query.
//
// This is a test that asserts the correct values are used and not the behaviour
// of the watcher itself.
func (s *volumeSuite) TestWatchModelProvisionedVolumes(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().InitialWatchStatementModelProvisionedVolumes().Return(
		"test_namespace", namespaceQueryReturningError(c.T),
	)
	matcher := eventSourceFilterMatcher{
		ChangeMask: changestream.All,
		Namespace:  "test_namespace",
	}
	s.watcherFactory.EXPECT().NewNamespaceWatcher(gomock.Any(), gomock.Any(), gomock.Any(), matcher)

	_, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		WatchModelProvisionedVolumes(c.Context())
	c.Check(err, tc.ErrorIsNil)
}

// TestWatchMachineProvisionedVolumes tests that the machine provisioned
// volume watcher is correctly created with the provided namespace from state
// and the initial query.
//
// This is a test that asserts the correct values are used and not the behaviour
// of the watcher itself.
func (s *volumeSuite) TestWatchMachineProvisionedVolumes(c *tc.C) {
	defer s.setupMocks(c).Finish()
	machineUUID := machinetesting.GenUUID(c)
	netNodeUUID, err := domainnetwork.NewNetNodeUUID()
	c.Assert(err, tc.ErrorIsNil)

	s.state.EXPECT().GetMachineNetNodeUUID(gomock.Any(), machineUUID).Return(
		netNodeUUID, nil,
	)
	s.state.EXPECT().InitialWatchStatementMachineProvisionedVolumes(
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
		WatchMachineProvisionedVolumes(c.Context(), machineUUID)
	c.Check(err, tc.ErrorIsNil)
}

// TestWatchMachineProvisionedvolumesNotValid tests that the caller gets
// back an error satisfying [coreerrors.NotValid] when the provided machine uuid
// is not valid.
func (s *volumeSuite) TestWatchMachineProvisionedVolumesNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		WatchMachineProvisionedVolumes(c.Context(), coremachine.UUID(""))
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestWatchMachineProvisionedVolumesNotFound tests that the caller gets
// back an error satisfying [machineerrors.MachineNotFound] when no machine
// exists for the provided machine uuid.
func (s *volumeSuite) TestWatchMachineProvisionedVolumesNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	machineUUID := machinetesting.GenUUID(c)

	s.state.EXPECT().GetMachineNetNodeUUID(gomock.Any(), machineUUID).Return(
		"", machineerrors.MachineNotFound,
	)

	_, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		WatchMachineProvisionedVolumes(c.Context(), machineUUID)
	c.Check(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

// TestWatchModelProvisionedVolumeAttachments tests that the model
// provisioned volume attachments watcher is correctly created with the
// provided namespace from state and the initial query.
//
// This is a test that asserts the correct values are used and not the behaviour
// of the watcher itself.
func (s *volumeSuite) TestWatchModelProvisionedVolumeAttachments(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().InitialWatchStatementModelProvisionedVolumeAttachments().Return(
		"test_namespace", namespaceQueryReturningError(c.T),
	)
	matcher := eventSourceFilterMatcher{
		ChangeMask: changestream.All,
		Namespace:  "test_namespace",
	}
	s.watcherFactory.EXPECT().NewNamespaceWatcher(gomock.Any(), gomock.Any(), gomock.Any(), matcher)

	_, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		WatchModelProvisionedVolumeAttachments(c.Context())
	c.Check(err, tc.ErrorIsNil)
}

// TestWatchMachineProvisionedVolumeAttachments tests that the machine
// provisioned volume watcher is correctly created with the provided
// namespace from state and the initial query.
//
// This is a test that asserts the correct values are used and not the behaviour
// of the watcher itself.
func (s *volumeSuite) TestWatchMachineProvisionedVolumeAttachments(c *tc.C) {
	defer s.setupMocks(c).Finish()
	machineUUID := machinetesting.GenUUID(c)
	netNodeUUID, err := domainnetwork.NewNetNodeUUID()
	c.Assert(err, tc.ErrorIsNil)

	s.state.EXPECT().GetMachineNetNodeUUID(gomock.Any(), machineUUID).Return(
		netNodeUUID, nil,
	)
	s.state.EXPECT().InitialWatchStatementMachineProvisionedVolumeAttachments(
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
		WatchMachineProvisionedVolumeAttachments(c.Context(), machineUUID)
	c.Check(err, tc.ErrorIsNil)
}

// TestWatchMachineProvisionedVolumeAttachmentssNotValid tests that the
// caller gets back an error satisfying [coreerrors.NotValid] when the provided
// machine uuid is not valid.
func (s *volumeSuite) TestWatchMachineProvisionedVolumeAttachmentsNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		WatchMachineProvisionedVolumes(c.Context(), coremachine.UUID(""))
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestWatchMachineProvisionedVolumeAttachmentsNotFound tests that the
// caller gets back an error satisfying [machineerrors.MachineNotFound] when no
// machine exists for the provided machine uuid.
func (s *volumeSuite) TestWatchMachineProvisionedVolumeAttachmentsNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	machineUUID := machinetesting.GenUUID(c)

	s.state.EXPECT().GetMachineNetNodeUUID(gomock.Any(), machineUUID).Return(
		"", machineerrors.MachineNotFound,
	)

	_, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		WatchMachineProvisionedVolumeAttachments(c.Context(), machineUUID)
	c.Check(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

// TestWatchVolumeAttachmentPlans tests that the watcher is correctly created
// with the provided namespace from state and the initial query.
//
// This is a test that asserts the correct values are used and not the behaviour
// of the watcher itself.
func (s *volumeSuite) TestWatchVolumeAttachmentPlans(c *tc.C) {
	defer s.setupMocks(c).Finish()
	machineUUID := machinetesting.GenUUID(c)
	netNodeUUID, err := domainnetwork.NewNetNodeUUID()
	c.Assert(err, tc.ErrorIsNil)

	s.state.EXPECT().GetMachineNetNodeUUID(gomock.Any(), machineUUID).Return(
		netNodeUUID, nil,
	)
	s.state.EXPECT().InitialWatchStatementVolumeAttachmentPlans(
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
		WatchVolumeAttachmentPlans(c.Context(), machineUUID)
	c.Check(err, tc.ErrorIsNil)
}

// TestWatchVolumeAttachmentsNotValid tests that the caller gets back an error
// satisfying [coreerrors.NotValid] when the provided machine uuid is not valid.
func (s *volumeSuite) TestWatchVolumeAttachmentsNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		WatchVolumeAttachmentPlans(c.Context(), coremachine.UUID(""))
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestWatchVolumeAttachmentPlansNotFound tests that the caller gets back an
// error satisfying [machineerrors.MachineNotFound] when no machine exists for
// the provided machine uuid.
func (s *volumeSuite) TestWatchVolumeAttachmentPlansNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	machineUUID := machinetesting.GenUUID(c)

	s.state.EXPECT().GetMachineNetNodeUUID(gomock.Any(), machineUUID).Return(
		"", machineerrors.MachineNotFound,
	)

	_, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		WatchVolumeAttachmentPlans(c.Context(), machineUUID)
	c.Check(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

// TestGetVolumeParams ensures the params are passed back without error.
func (s *volumeSuite) TestGetVolumeParams(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))
	volUUID := domaintesting.GenVolumeUUID(c)

	s.state.EXPECT().GetVolumeParams(gomock.Any(), volUUID).Return(
		storageprovisioning.VolumeParams{
			Attributes: map[string]string{
				"foo": "bar",
			},
			ID:       "spid",
			Provider: "myprovider",
			SizeMiB:  10,
		}, nil,
	)

	params, err := svc.GetVolumeParams(c.Context(), volUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(params, tc.DeepEquals, storageprovisioning.VolumeParams{
		Attributes: map[string]string{
			"foo": "bar",
		},
		ID:       "spid",
		Provider: "myprovider",
		SizeMiB:  10,
	})
}

// TestGetVolumeParams tests that a volume not found error is passed through.
func (s *filesystemSuite) TestGetVolumeParamsNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))
	volUUID := domaintesting.GenVolumeUUID(c)

	s.state.EXPECT().GetVolumeParams(gomock.Any(), volUUID).Return(
		storageprovisioning.VolumeParams{},
		storageprovisioningerrors.VolumeNotFound,
	)

	_, err := svc.GetVolumeParams(c.Context(), volUUID)
	c.Check(err, tc.ErrorIs, storageprovisioningerrors.VolumeNotFound)
}

// TestGetVolumeAttachmentParams tests that volume attachment params are
// returned.
func (s *volumeSuite) TestGetVolumeAttachmentParams(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))
	vaUUID := domaintesting.GenVolumeAttachmentUUID(c)

	s.state.EXPECT().GetVolumeAttachmentParams(gomock.Any(), vaUUID).Return(
		storageprovisioning.VolumeAttachmentParams{
			MachineInstanceID: "inst-1",
			Provider:          "myprovider",
			ProviderID:        "p-123",
			ReadOnly:          true,
		}, nil,
	)

	params, err := svc.GetVolumeAttachmentParams(c.Context(), vaUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(params, tc.DeepEquals, storageprovisioning.VolumeAttachmentParams{
		MachineInstanceID: "inst-1",
		Provider:          "myprovider",
		ProviderID:        "p-123",
		ReadOnly:          true,
	})
}

// TestGetVolumeAttachmentParamsNotFound ensures a volume attachment plan not
// found error is passed through.
func (s *volumeSuite) TestGetVolumeAttachmentParamsNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))
	vaUUID := domaintesting.GenVolumeAttachmentUUID(c)

	s.state.EXPECT().GetVolumeAttachmentParams(gomock.Any(), vaUUID).Return(
		storageprovisioning.VolumeAttachmentParams{},
		storageprovisioningerrors.VolumeAttachmentNotFound,
	)

	_, err := svc.GetVolumeAttachmentParams(c.Context(), vaUUID)
	c.Check(err, tc.ErrorIs, storageprovisioningerrors.VolumeAttachmentNotFound)
}

func (s *volumeSuite) TestGetVolumeAttachmentLife(c *tc.C) {
	defer s.setupMocks(c).Finish()

	vaUUID := domaintesting.GenVolumeAttachmentUUID(c)

	s.state.EXPECT().GetVolumeAttachmentLife(c.Context(), vaUUID).Return(
		domainlife.Alive, nil,
	)

	rval, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		GetVolumeAttachmentLife(c.Context(), vaUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(rval, tc.Equals, domainlife.Alive)
}

func (s *volumeSuite) TestGetVolumeAttachmentLifeNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	vaUUID := domaintesting.GenVolumeAttachmentUUID(c)

	s.state.EXPECT().GetVolumeAttachmentLife(c.Context(), vaUUID).Return(
		-1, storageprovisioningerrors.VolumeAttachmentNotFound,
	)

	_, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		GetVolumeAttachmentLife(c.Context(), vaUUID)
	c.Check(err, tc.ErrorIs, storageprovisioningerrors.VolumeAttachmentNotFound)
}

func (s *volumeSuite) TestGetVolumeAttachmentLifeNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		GetVolumeAttachmentLife(c.Context(), "")
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *volumeSuite) TestGetVolumeAttachment(c *tc.C) {
	defer s.setupMocks(c).Finish()

	vaUUID := domaintesting.GenVolumeAttachmentUUID(c)

	va := storageprovisioning.VolumeAttachment{
		VolumeID:              "123",
		ReadOnly:              true,
		BlockDeviceName:       "abc",
		BlockDeviceLinks:      []string{"xyz"},
		BlockDeviceBusAddress: "addr",
	}
	s.state.EXPECT().GetVolumeAttachment(c.Context(), vaUUID).Return(va, nil)

	rval, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		GetVolumeAttachment(c.Context(), vaUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(rval, tc.DeepEquals, va)
}

func (s *volumeSuite) TestGetVolumeAttachmentNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		GetVolumeAttachment(c.Context(), "")
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *volumeSuite) TestGetVolumeAttachmentPlanUUIDForVolumeIDMachine(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID := machinetesting.GenUUID(c)
	netNodeUUID, err := domainnetwork.NewNetNodeUUID()
	c.Assert(err, tc.ErrorIsNil)
	volumeUUID := domaintesting.GenVolumeUUID(c)
	vapUUID := domaintesting.GenVolumeAttachmentPlanUUID(c)

	s.state.EXPECT().GetMachineNetNodeUUID(c.Context(), machineUUID).Return(netNodeUUID, nil)
	s.state.EXPECT().GetVolumeUUIDForID(c.Context(), "666").Return(volumeUUID, nil)
	s.state.EXPECT().GetVolumeAttachmentPlanUUIDForVolumeNetNode(
		c.Context(), volumeUUID, netNodeUUID,
	).Return(vapUUID, nil)

	rval, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		GetVolumeAttachmentPlanUUIDForVolumeIDMachine(
			c.Context(), "666", machineUUID,
		)
	c.Check(err, tc.ErrorIsNil)
	c.Check(rval, tc.Equals, vapUUID)
}

func (s *volumeSuite) TestGetVolumeAttachmentPlanUUIDForVolumeIDMachineWithNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		GetVolumeAttachmentPlanUUIDForVolumeIDMachine(c.Context(), "", coremachine.UUID(""))
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *volumeSuite) TestGetVolumeAttachmentPlanUUIDForVolumeIDMachineWithMachineNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID := machinetesting.GenUUID(c)

	s.state.EXPECT().GetMachineNetNodeUUID(c.Context(), machineUUID).Return(
		"", machineerrors.MachineNotFound,
	)

	_, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		GetVolumeAttachmentPlanUUIDForVolumeIDMachine(c.Context(), "666", machineUUID)
	c.Check(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *volumeSuite) TestGetVolumeAttachmentPlanUUIDForVolumeIDMachineWithVolumeNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID := machinetesting.GenUUID(c)
	netNodeUUID, err := domainnetwork.NewNetNodeUUID()
	c.Assert(err, tc.ErrorIsNil)

	s.state.EXPECT().GetMachineNetNodeUUID(c.Context(), machineUUID).Return(netNodeUUID, nil)
	s.state.EXPECT().GetVolumeUUIDForID(c.Context(), "666").Return("", storageprovisioningerrors.VolumeNotFound)

	_, err = NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		GetVolumeAttachmentPlanUUIDForVolumeIDMachine(c.Context(), "666", machineUUID)
	c.Check(err, tc.ErrorIs, storageprovisioningerrors.VolumeNotFound)
}

func (s *volumeSuite) TestGetVolumeAttachmentPlanUUIDForVolumeIDMachineWithVolumeAttachmentNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID := machinetesting.GenUUID(c)
	netNodeUUID, err := domainnetwork.NewNetNodeUUID()
	c.Assert(err, tc.ErrorIsNil)
	volumeUUID := domaintesting.GenVolumeUUID(c)

	s.state.EXPECT().GetMachineNetNodeUUID(c.Context(), machineUUID).Return(netNodeUUID, nil)
	s.state.EXPECT().GetVolumeUUIDForID(c.Context(), "666").Return(volumeUUID, nil)
	s.state.EXPECT().GetVolumeAttachmentPlanUUIDForVolumeNetNode(
		c.Context(), volumeUUID, netNodeUUID,
	).Return("", storageprovisioningerrors.VolumeAttachmentNotFound)

	_, err = NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		GetVolumeAttachmentPlanUUIDForVolumeIDMachine(c.Context(), "666", machineUUID)
	c.Check(err, tc.ErrorIs, storageprovisioningerrors.VolumeAttachmentNotFound)
}

func (s *volumeSuite) TestGetVolumeAttachmentUUIDForVolumeIDMachine(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID := machinetesting.GenUUID(c)
	netNodeUUID, err := domainnetwork.NewNetNodeUUID()
	c.Assert(err, tc.ErrorIsNil)
	volumeUUID := domaintesting.GenVolumeUUID(c)
	vaUUID := domaintesting.GenVolumeAttachmentUUID(c)

	s.state.EXPECT().GetMachineNetNodeUUID(c.Context(), machineUUID).Return(netNodeUUID, nil)
	s.state.EXPECT().GetVolumeUUIDForID(c.Context(), "666").Return(volumeUUID, nil)
	s.state.EXPECT().GetVolumeAttachmentUUIDForVolumeNetNode(
		c.Context(), volumeUUID, netNodeUUID,
	).Return(vaUUID, nil)

	rval, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		GetVolumeAttachmentUUIDForVolumeIDMachine(
			c.Context(), "666", machineUUID,
		)
	c.Check(err, tc.ErrorIsNil)
	c.Check(rval, tc.Equals, vaUUID)
}

func (s *volumeSuite) TestGetVolumeAttachmentUUIDForVolumeIDMachineWithNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		GetVolumeAttachmentUUIDForVolumeIDMachine(c.Context(), "", coremachine.UUID(""))
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *volumeSuite) TestGetVolumeAttachmentUUIDForVolumeIDMachineWithMachineNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID := machinetesting.GenUUID(c)

	s.state.EXPECT().GetMachineNetNodeUUID(c.Context(), machineUUID).Return(
		"", machineerrors.MachineNotFound,
	)

	_, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		GetVolumeAttachmentUUIDForVolumeIDMachine(c.Context(), "666", machineUUID)
	c.Check(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *volumeSuite) TestGetVolumeAttachmentUUIDForVolumeIDMachineWithVolumeNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID := machinetesting.GenUUID(c)
	netNodeUUID, err := domainnetwork.NewNetNodeUUID()
	c.Assert(err, tc.ErrorIsNil)

	s.state.EXPECT().GetMachineNetNodeUUID(c.Context(), machineUUID).Return(netNodeUUID, nil)
	s.state.EXPECT().GetVolumeUUIDForID(c.Context(), "666").Return("", storageprovisioningerrors.VolumeNotFound)

	_, err = NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		GetVolumeAttachmentUUIDForVolumeIDMachine(c.Context(), "666", machineUUID)
	c.Check(err, tc.ErrorIs, storageprovisioningerrors.VolumeNotFound)
}

func (s *volumeSuite) TestGetVolumeAttachmentUUIDForVolumeIDMachineWithVolumeAttachmentNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID := machinetesting.GenUUID(c)
	netNodeUUID, err := domainnetwork.NewNetNodeUUID()
	c.Assert(err, tc.ErrorIsNil)
	volumeUUID := domaintesting.GenVolumeUUID(c)

	s.state.EXPECT().GetMachineNetNodeUUID(c.Context(), machineUUID).Return(netNodeUUID, nil)
	s.state.EXPECT().GetVolumeUUIDForID(c.Context(), "666").Return(volumeUUID, nil)
	s.state.EXPECT().GetVolumeAttachmentUUIDForVolumeNetNode(
		c.Context(), volumeUUID, netNodeUUID,
	).Return("", storageprovisioningerrors.VolumeAttachmentNotFound)

	_, err = NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		GetVolumeAttachmentUUIDForVolumeIDMachine(c.Context(), "666", machineUUID)
	c.Check(err, tc.ErrorIs, storageprovisioningerrors.VolumeAttachmentNotFound)
}

func (s *volumeSuite) TestGetVolumeAttachmentUUIDForVolumeIDUnit(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)
	volumeUUID := domaintesting.GenVolumeUUID(c)
	vaUUID := domaintesting.GenVolumeAttachmentUUID(c)
	netNodeUUID, err := domainnetwork.NewNetNodeUUID()
	c.Assert(err, tc.ErrorIsNil)

	s.state.EXPECT().GetUnitNetNodeUUID(c.Context(), unitUUID).Return(
		netNodeUUID, nil,
	)
	s.state.EXPECT().GetVolumeUUIDForID(c.Context(), "666").Return(volumeUUID, nil)
	s.state.EXPECT().GetVolumeAttachmentUUIDForVolumeNetNode(
		c.Context(), volumeUUID, netNodeUUID,
	).Return(vaUUID, nil)

	rval, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		GetVolumeAttachmentUUIDForVolumeIDUnit(
			c.Context(), "666", unitUUID,
		)
	c.Check(err, tc.ErrorIsNil)
	c.Check(rval, tc.Equals, vaUUID)
}

func (s *volumeSuite) TestGetVolumeAttachmentUUIDForVolumeIDUnitWithNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		GetVolumeAttachmentUUIDForVolumeIDUnit(c.Context(), "", "")
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *volumeSuite) TestGetVolumeAttachmentUUIDForVolumeIDUnitWithUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)

	s.state.EXPECT().GetUnitNetNodeUUID(c.Context(), unitUUID).Return(
		"", applicationerrors.UnitNotFound,
	)

	_, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		GetVolumeAttachmentUUIDForVolumeIDUnit(c.Context(), "666", unitUUID)
	c.Check(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *volumeSuite) TestGetVolumeAttachmentUUIDForVolumeIDUnitWithVolumeNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)
	netNodeUUID, err := domainnetwork.NewNetNodeUUID()
	c.Assert(err, tc.ErrorIsNil)

	s.state.EXPECT().GetUnitNetNodeUUID(c.Context(), unitUUID).Return(netNodeUUID, nil)
	s.state.EXPECT().GetVolumeUUIDForID(c.Context(), "666").Return("", storageprovisioningerrors.VolumeNotFound)

	_, err = NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		GetVolumeAttachmentUUIDForVolumeIDUnit(c.Context(), "666", unitUUID)
	c.Check(err, tc.ErrorIs, storageprovisioningerrors.VolumeNotFound)
}

func (s *volumeSuite) TestGetVolumeAttachmentUUIDForVolumeIDUnitWithVolumeAttachmentNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)
	netNodeUUID, err := domainnetwork.NewNetNodeUUID()
	c.Assert(err, tc.ErrorIsNil)
	volumeUUID := domaintesting.GenVolumeUUID(c)

	s.state.EXPECT().GetUnitNetNodeUUID(c.Context(), unitUUID).Return(netNodeUUID, nil)
	s.state.EXPECT().GetVolumeUUIDForID(c.Context(), "666").Return(volumeUUID, nil)
	s.state.EXPECT().GetVolumeAttachmentUUIDForVolumeNetNode(
		c.Context(), volumeUUID, netNodeUUID,
	).Return("", storageprovisioningerrors.VolumeAttachmentNotFound)

	_, err = NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		GetVolumeAttachmentUUIDForVolumeIDUnit(c.Context(), "666", unitUUID)
	c.Check(err, tc.ErrorIs, storageprovisioningerrors.VolumeAttachmentNotFound)
}

func (s *volumeSuite) TestGetVolumeLife(c *tc.C) {
	defer s.setupMocks(c).Finish()

	volumeUUID := domaintesting.GenVolumeUUID(c)

	s.state.EXPECT().GetVolumeLife(c.Context(), volumeUUID).Return(
		domainlife.Alive, nil,
	)

	rval, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		GetVolumeLife(c.Context(), volumeUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(rval, tc.Equals, domainlife.Alive)
}

func (s *volumeSuite) TestGetVolumeLifeNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		GetVolumeLife(c.Context(), "")
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *volumeSuite) TestGetVolumeLifeWithVolumeNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	volumeUUID := domaintesting.GenVolumeUUID(c)

	s.state.EXPECT().GetVolumeLife(c.Context(), volumeUUID).Return(
		-1, storageprovisioningerrors.VolumeNotFound,
	)

	_, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		GetVolumeLife(c.Context(), volumeUUID)
	c.Check(err, tc.ErrorIs, storageprovisioningerrors.VolumeNotFound)
}

func (s *volumeSuite) TestGetVolumeUUIDForID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	volUUID := domaintesting.GenVolumeUUID(c)
	s.state.EXPECT().GetVolumeUUIDForID(c.Context(), "123").Return(volUUID, nil)

	rval, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		GetVolumeUUIDForID(c.Context(), "123")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(rval, tc.Equals, volUUID)
}

func (s *volumeSuite) TestGetVolume(c *tc.C) {
	defer s.setupMocks(c).Finish()

	volUUID := domaintesting.GenVolumeUUID(c)

	vol := storageprovisioning.Volume{
		VolumeID:   "123",
		ProviderID: "abc",
		SizeMiB:    1234,
		HardwareID: "hwid",
		WWN:        "wwn",
		Persistent: true,
	}
	s.state.EXPECT().GetVolumeUUIDForID(c.Context(), "123").Return(volUUID, nil)
	s.state.EXPECT().GetVolume(c.Context(), volUUID).Return(vol, nil)

	rval, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		GetVolumeByID(c.Context(), "123")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(rval, tc.DeepEquals, vol)
}

func (s *volumeSuite) TestGetVolumeNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetVolumeUUIDForID(c.Context(), "123").Return(
		"", storageprovisioningerrors.VolumeNotFound)

	_, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		GetVolumeByID(c.Context(), "123")
	c.Assert(err, tc.ErrorIs, storageprovisioningerrors.VolumeNotFound)
}

func (s *volumeSuite) TestGetBlockDeviceForVolumeAttachment(c *tc.C) {
	defer s.setupMocks(c).Finish()

	vaUUID := domaintesting.GenVolumeAttachmentUUID(c)
	bdUUID := tc.Must(c, blockdevice.NewBlockDeviceUUID)

	s.state.EXPECT().GetBlockDeviceForVolumeAttachment(
		c.Context(), vaUUID).Return(bdUUID, nil)

	result, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		GetBlockDeviceForVolumeAttachment(c.Context(), vaUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.Equals, bdUUID)
}

func (s *volumeSuite) TestGetBlockDeviceForVolumeAttachmentNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	vaUUID := domaintesting.GenVolumeAttachmentUUID(c)

	s.state.EXPECT().GetBlockDeviceForVolumeAttachment(c.Context(),
		vaUUID).Return("", storageprovisioningerrors.VolumeAttachmentNotFound)

	_, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		GetBlockDeviceForVolumeAttachment(c.Context(), vaUUID)
	c.Assert(err, tc.ErrorIs,
		storageprovisioningerrors.VolumeAttachmentNotFound)
}

func (s *volumeSuite) TestGetBlockDeviceForVolumeAttachmentInvalidUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	vaUUID := storageprovisioning.VolumeAttachmentUUID("foo")

	_, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		GetBlockDeviceForVolumeAttachment(c.Context(), vaUUID)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *volumeSuite) TestSetVolumeProvisionedInfo(c *tc.C) {
	defer s.setupMocks(c).Finish()

	volUUID := domaintesting.GenVolumeUUID(c)

	info := storageprovisioning.VolumeProvisionedInfo{
		ProviderID: "vol-123",
		SizeMiB:    1234,
		HardwareID: "hwid",
		WWN:        "wwn",
		Persistent: true,
	}

	s.state.EXPECT().GetVolumeUUIDForID(
		c.Context(), "123").Return(volUUID, nil)
	s.state.EXPECT().SetVolumeProvisionedInfo(
		c.Context(), volUUID, info).Return(nil)

	err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		SetVolumeProvisionedInfo(c.Context(), "123", info)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *volumeSuite) TestSetVolumeProvisionedInfoNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	info := storageprovisioning.VolumeProvisionedInfo{}

	s.state.EXPECT().GetVolumeUUIDForID(c.Context(), "123").Return(
		"", storageprovisioningerrors.VolumeNotFound)

	err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		SetVolumeProvisionedInfo(c.Context(), "123", info)
	c.Assert(err, tc.ErrorIs, storageprovisioningerrors.VolumeNotFound)
}

func (s *volumeSuite) TestSetVolumeAttachmentProvisionedInfo(c *tc.C) {
	defer s.setupMocks(c).Finish()

	vaUUID := domaintesting.GenVolumeAttachmentUUID(c)
	bdUUID := tc.Must(c, blockdevice.NewBlockDeviceUUID)

	info := storageprovisioning.VolumeAttachmentProvisionedInfo{
		ReadOnly:        true,
		BlockDeviceUUID: &bdUUID,
	}

	s.state.EXPECT().SetVolumeAttachmentProvisionedInfo(
		gomock.Any(), vaUUID, info).Return(nil)

	err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		SetVolumeAttachmentProvisionedInfo(c.Context(), vaUUID, info)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *volumeSuite) TestSetVolumeAttachmentProvisionedInfoNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	vaUUID := domaintesting.GenVolumeAttachmentUUID(c)
	bdUUID := tc.Must(c, blockdevice.NewBlockDeviceUUID)

	info := storageprovisioning.VolumeAttachmentProvisionedInfo{
		ReadOnly:        true,
		BlockDeviceUUID: &bdUUID,
	}

	s.state.EXPECT().SetVolumeAttachmentProvisionedInfo(gomock.Any(),
		vaUUID, info).Return(blockdeviceerrors.BlockDeviceNotFound)

	err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		SetVolumeAttachmentProvisionedInfo(c.Context(), vaUUID, info)
	c.Assert(err, tc.ErrorIs, blockdeviceerrors.BlockDeviceNotFound)
}

func (s *volumeSuite) TestSetVolumeAttachmentProvisionedInfoInvalidAttachmentUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	vaUUIDInvalid := storageprovisioning.VolumeAttachmentUUID("invalid")
	bdUUIDValid := tc.Must(c, blockdevice.NewBlockDeviceUUID)

	info := storageprovisioning.VolumeAttachmentProvisionedInfo{
		BlockDeviceUUID: &bdUUIDValid,
	}

	err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		SetVolumeAttachmentProvisionedInfo(c.Context(), vaUUIDInvalid, info)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *volumeSuite) TestSetVolumeAttachmentProvisionedInfoInvalidBlockDeviceUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	vaUUIDValid := domaintesting.GenVolumeAttachmentUUID(c)
	bdUUIDInvalid := blockdevice.BlockDeviceUUID("invalid")

	info := storageprovisioning.VolumeAttachmentProvisionedInfo{
		BlockDeviceUUID: &bdUUIDInvalid,
	}

	err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		SetVolumeAttachmentProvisionedInfo(c.Context(), vaUUIDValid, info)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *volumeSuite) TestGetVolumeAttachmentPlan(c *tc.C) {
	defer s.setupMocks(c).Finish()

	vapUUID := domaintesting.GenVolumeAttachmentPlanUUID(c)

	vap := storageprovisioning.VolumeAttachmentPlan{
		Life:       domainlife.Dying,
		DeviceType: storageprovisioning.PlanDeviceTypeISCSI,
		DeviceAttributes: map[string]string{
			"a": "x",
		},
	}
	s.state.EXPECT().GetVolumeAttachmentPlan(gomock.Any(), vapUUID).Return(
		vap, nil)

	result, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		GetVolumeAttachmentPlan(c.Context(), vapUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, vap)
}

func (s *volumeSuite) TestGetVolumeAttachmentPlanNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	vapUUID := domaintesting.GenVolumeAttachmentPlanUUID(c)

	s.state.EXPECT().GetVolumeAttachmentPlan(gomock.Any(), vapUUID).Return(
		storageprovisioning.VolumeAttachmentPlan{},
		storageprovisioningerrors.VolumeAttachmentPlanNotFound)

	_, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		GetVolumeAttachmentPlan(c.Context(), vapUUID)
	c.Assert(err, tc.ErrorIs,
		storageprovisioningerrors.VolumeAttachmentPlanNotFound)
}

func (s *volumeSuite) TestGetVolumeAttachmentPlanInvalidUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	vapUUID := storageprovisioning.VolumeAttachmentPlanUUID("foo")

	_, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		GetVolumeAttachmentPlan(c.Context(), vapUUID)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *volumeSuite) TestCreateVolumeAttachmentPlan(c *tc.C) {
	defer s.setupMocks(c).Finish()

	vaUUID := domaintesting.GenVolumeAttachmentUUID(c)

	planType := storageprovisioning.PlanDeviceTypeISCSI
	attrs := map[string]string{
		"a": "x",
	}

	var gotUUID storageprovisioning.VolumeAttachmentPlanUUID
	s.state.EXPECT().CreateVolumeAttachmentPlan(gomock.Any(), gomock.Any(),
		vaUUID, planType, attrs).DoAndReturn(
		func(
			_ context.Context,
			vapUUID storageprovisioning.VolumeAttachmentPlanUUID,
			_ storageprovisioning.VolumeAttachmentUUID,
			_ storageprovisioning.PlanDeviceType,
			_ map[string]string) error {
			gotUUID = vapUUID
			return nil
		},
	)

	uuid, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		CreateVolumeAttachmentPlan(c.Context(), vaUUID, planType, attrs)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuid, tc.IsNonZeroUUID)
	c.Check(uuid, tc.Equals, gotUUID)
}

func (s *volumeSuite) TestCreateVolumeAttachmentPlanNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	vaUUID := domaintesting.GenVolumeAttachmentUUID(c)

	planType := storageprovisioning.PlanDeviceTypeISCSI
	attrs := map[string]string{
		"a": "x",
	}

	s.state.EXPECT().CreateVolumeAttachmentPlan(gomock.Any(), gomock.Any(),
		vaUUID, planType, attrs).Return(
		storageprovisioningerrors.VolumeAttachmentNotFound)

	_, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		CreateVolumeAttachmentPlan(c.Context(), vaUUID, planType, attrs)
	c.Assert(err, tc.ErrorIs, storageprovisioningerrors.VolumeAttachmentNotFound)
}

func (s *volumeSuite) TestCreateVolumeAttachmentPlanInvalidAttachmentUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	vaUUID := storageprovisioning.VolumeAttachmentUUID("foo")

	planType := storageprovisioning.PlanDeviceTypeISCSI
	attrs := map[string]string{
		"a": "x",
	}

	_, err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		CreateVolumeAttachmentPlan(c.Context(), vaUUID, planType, attrs)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *volumeSuite) TestSetVolumeAttachmentPlanProvisionedInfo(c *tc.C) {
	defer s.setupMocks(c).Finish()

	vapUUID := domaintesting.GenVolumeAttachmentPlanUUID(c)

	info := storageprovisioning.VolumeAttachmentPlanProvisionedInfo{
		DeviceType: storageprovisioning.PlanDeviceTypeISCSI,
		DeviceAttributes: map[string]string{
			"a": "x",
		},
	}

	s.state.EXPECT().SetVolumeAttachmentPlanProvisionedInfo(gomock.Any(),
		vapUUID, info).Return(nil)

	err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		SetVolumeAttachmentPlanProvisionedInfo(c.Context(), vapUUID, info)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *volumeSuite) TestSetVolumeAttachmentPlanProvisionedInfoNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	vapUUID := domaintesting.GenVolumeAttachmentPlanUUID(c)

	info := storageprovisioning.VolumeAttachmentPlanProvisionedInfo{
		DeviceType: storageprovisioning.PlanDeviceTypeISCSI,
		DeviceAttributes: map[string]string{
			"a": "x",
		},
	}

	s.state.EXPECT().SetVolumeAttachmentPlanProvisionedInfo(gomock.Any(),
		vapUUID, info,
	).Return(storageprovisioningerrors.VolumeAttachmentPlanNotFound)

	err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		SetVolumeAttachmentPlanProvisionedInfo(c.Context(), vapUUID, info)
	c.Assert(err, tc.ErrorIs,
		storageprovisioningerrors.VolumeAttachmentPlanNotFound)
}

func (s *volumeSuite) TestSetVolumeAttachmentPlanProvisionedInfoInvalidUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	vapUUID := storageprovisioning.VolumeAttachmentPlanUUID("foo")

	info := storageprovisioning.VolumeAttachmentPlanProvisionedInfo{
		DeviceType: storageprovisioning.PlanDeviceTypeISCSI,
		DeviceAttributes: map[string]string{
			"a": "x",
		},
	}

	err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		SetVolumeAttachmentPlanProvisionedInfo(c.Context(), vapUUID, info)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *volumeSuite) TestSetVolumeAttachmentPlanProvisionedBlockDevice(c *tc.C) {
	defer s.setupMocks(c).Finish()

	vapUUID := domaintesting.GenVolumeAttachmentPlanUUID(c)
	bdUUID := tc.Must(c, blockdevice.NewBlockDeviceUUID)

	s.state.EXPECT().SetVolumeAttachmentPlanProvisionedBlockDevice(gomock.Any(),
		vapUUID, bdUUID).Return(nil)

	err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		SetVolumeAttachmentPlanProvisionedBlockDevice(
			c.Context(), vapUUID, bdUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *volumeSuite) TestSetVolumeAttachmentPlanProvisionedBlockDeviceNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	vapUUID := domaintesting.GenVolumeAttachmentPlanUUID(c)
	bdUUID := tc.Must(c, blockdevice.NewBlockDeviceUUID)

	s.state.EXPECT().SetVolumeAttachmentPlanProvisionedBlockDevice(gomock.Any(),
		vapUUID, bdUUID).Return(blockdeviceerrors.BlockDeviceNotFound)

	err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		SetVolumeAttachmentPlanProvisionedBlockDevice(
			c.Context(), vapUUID, bdUUID)
	c.Assert(err, tc.ErrorIs, blockdeviceerrors.BlockDeviceNotFound)
}

func (s *volumeSuite) TestSetVolumeAttachmentPlanProvisionedBlockDeviceInvalidBlockDeviceUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	vapUUID := domaintesting.GenVolumeAttachmentPlanUUID(c)
	bdUUID := blockdevice.BlockDeviceUUID("foo")

	err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		SetVolumeAttachmentPlanProvisionedBlockDevice(
			c.Context(), vapUUID, bdUUID)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *volumeSuite) TestSetVolumeAttachmentPlanProvisionedBlockDeviceInvalidPlanUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	vapUUID := storageprovisioning.VolumeAttachmentPlanUUID("foo")
	bdUUID := tc.Must(c, blockdevice.NewBlockDeviceUUID)

	err := NewService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c)).
		SetVolumeAttachmentPlanProvisionedBlockDevice(
			c.Context(), vapUUID, bdUUID)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}
