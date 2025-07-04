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
	machineerrors "github.com/juju/juju/domain/machine/errors"
	domainnetwork "github.com/juju/juju/domain/network"
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
	s.watcherFactory.EXPECT().NewNamespaceWatcher(gomock.Any(), matcher)

	_, err := NewService(s.state, s.watcherFactory).
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
		gomock.Any(), gomock.Any(), matcher,
	)

	_, err = NewService(s.state, s.watcherFactory).
		WatchMachineProvisionedFilesystems(c.Context(), machineUUID)
	c.Check(err, tc.ErrorIsNil)
}

// TestWatchMachineProvisionedFilesystemsNotValid tests that the caller gets
// back an error satisfying [coreerrors.NotValid] when the provided machine uuid
// is not valid.
func (s *filesystemSuite) TestWatchMachineProvisionedFilesystemsNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state, s.watcherFactory).
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

	_, err := NewService(s.state, s.watcherFactory).
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
	s.watcherFactory.EXPECT().NewNamespaceWatcher(gomock.Any(), matcher)

	_, err := NewService(s.state, s.watcherFactory).
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
		gomock.Any(), gomock.Any(), matcher,
	)

	_, err = NewService(s.state, s.watcherFactory).
		WatchMachineProvisionedFilesystemAttachments(c.Context(), machineUUID)
	c.Check(err, tc.ErrorIsNil)
}

// TestWatchMachineProvisionedFilesystemAttachmentssNotValid tests that the
// caller gets back an error satisfying [coreerrors.NotValid] when the provided
// machine uuid is not valid.
func (s *filesystemSuite) TestWatchMachineProvisionedFilesystemAttachmentsNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state, s.watcherFactory).
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

	_, err := NewService(s.state, s.watcherFactory).
		WatchMachineProvisionedFilesystemAttachments(c.Context(), machineUUID)
	c.Check(err, tc.ErrorIs, machineerrors.MachineNotFound)
}
