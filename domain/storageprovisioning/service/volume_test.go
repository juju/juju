// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/juju/core/changestream"
	coreerrors "github.com/juju/juju/core/errors"
	coremachine "github.com/juju/juju/core/machine"
	machinetesting "github.com/juju/juju/core/machine/testing"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"
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
		"test_namespace", testNamespaceQuery(c.T),
	)
	matcher := eventSourceFilterMatcher{
		ChangeMask: changestream.All,
		Namespace:  "test_namespace",
	}
	s.watcherFactory.EXPECT().NewNamespaceWatcher(gomock.Any(), matcher)

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
	netNodeUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	s.state.EXPECT().GetMachineNetNodeUUID(gomock.Any(), machineUUID).Return(
		netNodeUUID.String(), nil,
	)
	s.state.EXPECT().InitialWatchStatementMachineProvisionedVolumes(
		netNodeUUID.String(),
	).Return(
		"test_namespace", testNamespaceLifeQuery(c.T),
	)
	matcher := eventSourcePredFilterMatcher{
		ChangeMask: changestream.All,
		Namespace:  "test_namespace",
		Predicate:  netNodeUUID.String(),
	}
	s.watcherFactory.EXPECT().NewNamespaceMapperWatcher(
		gomock.Any(), gomock.Any(), matcher,
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
		"test_namespace", testNamespaceQuery(c.T),
	)
	matcher := eventSourceFilterMatcher{
		ChangeMask: changestream.All,
		Namespace:  "test_namespace",
	}
	s.watcherFactory.EXPECT().NewNamespaceWatcher(gomock.Any(), matcher)

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
	netNodeUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	s.state.EXPECT().GetMachineNetNodeUUID(gomock.Any(), machineUUID).Return(
		netNodeUUID.String(), nil,
	)
	s.state.EXPECT().InitialWatchStatementMachineProvisionedVolumeAttachments(
		netNodeUUID.String(),
	).Return(
		"test_namespace", testNamespaceLifeQuery(c.T),
	)
	matcher := eventSourcePredFilterMatcher{
		ChangeMask: changestream.All,
		Namespace:  "test_namespace",
		Predicate:  netNodeUUID.String(),
	}
	s.watcherFactory.EXPECT().NewNamespaceMapperWatcher(
		gomock.Any(), gomock.Any(), matcher,
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
	netNodeUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	s.state.EXPECT().GetMachineNetNodeUUID(gomock.Any(), machineUUID).Return(
		netNodeUUID.String(), nil,
	)
	s.state.EXPECT().InitialWatchStatementVolumeAttachmentPlans(
		netNodeUUID.String(),
	).Return(
		"test_namespace", testNamespaceLifeQuery(c.T),
	)
	matcher := eventSourcePredFilterMatcher{
		ChangeMask: changestream.All,
		Namespace:  "test_namespace",
		Predicate:  netNodeUUID.String(),
	}
	s.watcherFactory.EXPECT().NewNamespaceMapperWatcher(
		gomock.Any(), gomock.Any(), matcher,
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
