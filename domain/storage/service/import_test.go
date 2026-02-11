// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	stdtesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreerrors "github.com/juju/juju/core/errors"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/domain/storage/internal"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/uuid"
)

// importSuite is a set of tests to assert the interface and contracts
// importing storage into this state package.
type importSuite struct {
	testhelpers.IsolationSuite

	service *Service

	state *MockState
}

// TestImportSuite runs all of the tests contained in
// [importSuite].
func TestImportSuite(t *stdtesting.T) {
	tc.Run(t, &importSuite{})
}

func (s *importSuite) TestImportStorageInstances(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	expected := []internal.ImportStorageInstanceArgs{
		{
			UUID:             tc.Must(c, uuid.NewUUID).String(),
			StorageName:      "test1",
			StorageKind:      "block",
			StorageID:        "test1/0",
			UnitName:         "unit/3",
			RequestedSizeMiB: 1024,
			PoolName:         "ebs",
		}, {
			UUID:             tc.Must(c, uuid.NewUUID).String(),
			StorageName:      "test1",
			StorageKind:      "block",
			StorageID:        "test1/2",
			UnitName:         "unit/2",
			RequestedSizeMiB: 1024,
			PoolName:         "ebs",
		},
	}
	s.state.EXPECT().ImportStorageInstances(gomock.Any(), storageInstanceArgsMatcher{
		c:        c,
		expected: expected,
	}).Return(nil)

	args := []domainstorage.ImportStorageInstanceParams{
		{
			StorageName:      "test1",
			StorageKind:      "block",
			StorageID:        "test1/0",
			UnitName:         "unit/3",
			RequestedSizeMiB: 1024,
			PoolName:         "ebs",
		}, {
			StorageName:      "test1",
			StorageKind:      "block",
			StorageID:        "test1/2",
			UnitName:         "unit/2",
			RequestedSizeMiB: 1024,
			PoolName:         "ebs",
		},
	}

	// Act
	err := s.service.ImportStorageInstances(c.Context(), args)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

type storageInstanceArgsMatcher struct {
	c        *tc.C
	expected []internal.ImportStorageInstanceArgs
}

func (m storageInstanceArgsMatcher) Matches(arg any) bool {
	obtained, ok := arg.([]internal.ImportStorageInstanceArgs)
	if !ok {
		return false
	}
	mc := tc.NewMultiChecker()
	mc.AddExpr(`_.UUID`, tc.IsNonZeroUUID)
	return m.c.Check(obtained, tc.UnorderedMatch[[]internal.ImportStorageInstanceArgs](mc), m.expected)
}

func (m storageInstanceArgsMatcher) String() string {
	return "matches if the input slice of ImportStorageInstanceArgs matches expectation."
}

func (s *importSuite) TestImportStorageInstancesValidate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	args := []domainstorage.ImportStorageInstanceParams{
		{
			// There is not StorageID.
			StorageName:      "test1",
			StorageKind:      "block",
			UnitName:         "unit/2",
			RequestedSizeMiB: uint64(1024),
			PoolName:         "ebs",
		},
	}

	// Act
	err := s.service.ImportStorageInstances(c.Context(), args)

	// Assert
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *importSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.service = NewService(
		s.state, loggertesting.WrapCheckLog(c), nil,
	)

	c.Cleanup(func() {
		s.state = nil
		s.service = nil
	})

	return ctrl
}
