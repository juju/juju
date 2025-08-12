// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/core/changestream"
	coreerrors "github.com/juju/juju/core/errors"
	coremachine "github.com/juju/juju/core/machine"
	machinetesting "github.com/juju/juju/core/machine/testing"
	unittesting "github.com/juju/juju/core/unit/testing"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainlife "github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	domainnetwork "github.com/juju/juju/domain/network"
	storageprovisioningerrors "github.com/juju/juju/domain/storageprovisioning/errors"
	domaintesting "github.com/juju/juju/domain/storageprovisioning/testing"
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

	_, err := NewService(s.state, s.watcherFactory).
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

	_, err = NewService(s.state, s.watcherFactory).
		WatchMachineProvisionedVolumes(c.Context(), machineUUID)
	c.Check(err, tc.ErrorIsNil)
}

// TestWatchMachineProvisionedvolumesNotValid tests that the caller gets
// back an error satisfying [coreerrors.NotValid] when the provided machine uuid
// is not valid.
func (s *volumeSuite) TestWatchMachineProvisionedVolumesNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state, s.watcherFactory).
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

	_, err := NewService(s.state, s.watcherFactory).
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

	_, err := NewService(s.state, s.watcherFactory).
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

	_, err = NewService(s.state, s.watcherFactory).
		WatchMachineProvisionedVolumeAttachments(c.Context(), machineUUID)
	c.Check(err, tc.ErrorIsNil)
}

// TestWatchMachineProvisionedVolumeAttachmentssNotValid tests that the
// caller gets back an error satisfying [coreerrors.NotValid] when the provided
// machine uuid is not valid.
func (s *volumeSuite) TestWatchMachineProvisionedVolumeAttachmentsNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state, s.watcherFactory).
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

	_, err := NewService(s.state, s.watcherFactory).
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

	_, err = NewService(s.state, s.watcherFactory).
		WatchVolumeAttachmentPlans(c.Context(), machineUUID)
	c.Check(err, tc.ErrorIsNil)
}

// TestWatchVolumeAttachmentsNotValid tests that the caller gets back an error
// satisfying [coreerrors.NotValid] when the provided machine uuid is not valid.
func (s *volumeSuite) TestWatchVolumeAttachmentsNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state, s.watcherFactory).
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

	_, err := NewService(s.state, s.watcherFactory).
		WatchVolumeAttachmentPlans(c.Context(), machineUUID)
	c.Check(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *volumeSuite) TestGetVolumeAttachmentLife(c *tc.C) {
	defer s.setupMocks(c).Finish()

	vaUUID := domaintesting.GenVolumeAttachmentUUID(c)

	s.state.EXPECT().GetVolumeAttachmentLife(c.Context(), vaUUID).Return(
		domainlife.Alive, nil,
	)

	rval, err := NewService(s.state, s.watcherFactory).
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

	_, err := NewService(s.state, s.watcherFactory).
		GetVolumeAttachmentLife(c.Context(), vaUUID)
	c.Check(err, tc.ErrorIs, storageprovisioningerrors.VolumeAttachmentNotFound)
}

func (s *volumeSuite) TestGetVolumeAttachmentLifeNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state, s.watcherFactory).
		GetVolumeAttachmentLife(c.Context(), "")
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
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

	rval, err := NewService(s.state, s.watcherFactory).
		GetVolumeAttachmentUUIDForVolumeIDMachine(
			c.Context(), "666", machineUUID,
		)
	c.Check(err, tc.ErrorIsNil)
	c.Check(rval, tc.Equals, vaUUID)
}

func (s *volumeSuite) TestGetVolumeAttachmentUUIDForVolumeIDMachineWithNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state, s.watcherFactory).
		GetVolumeAttachmentUUIDForVolumeIDMachine(c.Context(), "", coremachine.UUID(""))
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *volumeSuite) TestGetVolumeAttachmentUUIDForVolumeIDMachineWithMachineNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID := machinetesting.GenUUID(c)

	s.state.EXPECT().GetMachineNetNodeUUID(c.Context(), machineUUID).Return(
		"", machineerrors.MachineNotFound,
	)

	_, err := NewService(s.state, s.watcherFactory).
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

	_, err = NewService(s.state, s.watcherFactory).
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

	_, err = NewService(s.state, s.watcherFactory).
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

	rval, err := NewService(s.state, s.watcherFactory).
		GetVolumeAttachmentUUIDForVolumeIDUnit(
			c.Context(), "666", unitUUID,
		)
	c.Check(err, tc.ErrorIsNil)
	c.Check(rval, tc.Equals, vaUUID)
}

func (s *volumeSuite) TestGetVolumeAttachmentUUIDForVolumeIDUnitWithNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state, s.watcherFactory).
		GetVolumeAttachmentUUIDForVolumeIDUnit(c.Context(), "", "")
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *volumeSuite) TestGetVolumeAttachmentUUIDForVolumeIDUnitWithUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)

	s.state.EXPECT().GetUnitNetNodeUUID(c.Context(), unitUUID).Return(
		"", applicationerrors.UnitNotFound,
	)

	_, err := NewService(s.state, s.watcherFactory).
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

	_, err = NewService(s.state, s.watcherFactory).
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

	_, err = NewService(s.state, s.watcherFactory).
		GetVolumeAttachmentUUIDForVolumeIDUnit(c.Context(), "666", unitUUID)
	c.Check(err, tc.ErrorIs, storageprovisioningerrors.VolumeAttachmentNotFound)
}

func (s *volumeSuite) TestGetVolumeLife(c *tc.C) {
	defer s.setupMocks(c).Finish()

	volumeUUID := domaintesting.GenVolumeUUID(c)

	s.state.EXPECT().GetVolumeLife(c.Context(), volumeUUID).Return(
		domainlife.Alive, nil,
	)

	rval, err := NewService(s.state, s.watcherFactory).
		GetVolumeLife(c.Context(), volumeUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(rval, tc.Equals, domainlife.Alive)
}

func (s *volumeSuite) TestGetVolumeLifeNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state, s.watcherFactory).
		GetVolumeLife(c.Context(), "")
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *volumeSuite) TestGetVolumeLifeWithVolumeNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	volumeUUID := domaintesting.GenVolumeUUID(c)

	s.state.EXPECT().GetVolumeLife(c.Context(), volumeUUID).Return(
		-1, storageprovisioningerrors.VolumeNotFound,
	)

	_, err := NewService(s.state, s.watcherFactory).
		GetVolumeLife(c.Context(), volumeUUID)
	c.Check(err, tc.ErrorIs, storageprovisioningerrors.VolumeNotFound)
}
