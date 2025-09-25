// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type pruneSuite struct {
	clock                 clock.Clock
	state                 *MockState
	mockObjectStoreGetter *MockModelObjectStoreGetter
	mockObjectStore       *MockObjectStore
	mockLeadershipService *MockLeadershipService
}

func TestPruneSuite(t *testing.T) {
	tc.Run(t, &pruneSuite{})
}

func (s *pruneSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	s.clock = clock.WallClock
	s.mockObjectStore = NewMockObjectStore(ctrl)
	s.mockObjectStoreGetter = NewMockModelObjectStoreGetter(ctrl)
	s.mockLeadershipService = NewMockLeadershipService(ctrl)
	return ctrl
}

func (s *pruneSuite) service() *Service {
	return NewService(s.state, s.clock, loggertesting.WrapCheckLog(nil), s.mockObjectStoreGetter, s.mockLeadershipService)
}

// TestPruneOperationsSuccess verifies that the Service.PruneOperations method
// successfully delegates to state.PruneOperations when provided with valid
// arguments and returns no error.
func (s *pruneSuite) TestPruneOperationsSuccess(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	age := time.Hour
	sizeMB := 10
	s.state.EXPECT().PruneOperations(gomock.Any(), age, sizeMB).Return(nil, nil)

	// Act
	err := s.service().PruneOperations(c.Context(), age, sizeMB)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

// TestPruneOperationsSuccessWithPathToRemove verifies the behavior when tasks
// require removal from the object store.
func (s *pruneSuite) TestPruneOperationsSuccessWithPathToRemove(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().PruneOperations(gomock.Any(), gomock.Any(), gomock.Any()).Return([]string{"/path1", "/path2"}, nil)
	s.mockObjectStoreGetter.EXPECT().GetObjectStore(gomock.Any()).Return(s.mockObjectStore, nil)
	s.mockObjectStore.EXPECT().Remove(gomock.Any(), "/path1").Return(nil)
	s.mockObjectStore.EXPECT().Remove(gomock.Any(), "/path2").Return(nil)

	// Act
	err := s.service().PruneOperations(c.Context(), 1, 1)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

// TestPruneOperationsSuccessWithPathToRemove verifies the behavior when tasks
// require removal from the object store, but there is an error getting the
// object store.
func (s *pruneSuite) TestPruneOperationsGetObjectStoreFailure(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	expectedErr := errors.New("boom")
	s.state.EXPECT().PruneOperations(gomock.Any(), gomock.Any(), gomock.Any()).Return([]string{"anything"}, nil)
	s.mockObjectStoreGetter.EXPECT().GetObjectStore(gomock.Any()).Return(nil, expectedErr)

	// Act
	err := s.service().PruneOperations(c.Context(), 1, 1)

	// Assert
	c.Assert(err, tc.ErrorIs, expectedErr)
}

// TestPruneOperationsSuccessWithPathToRemove verifies the behavior when tasks
// require removal from the object store, but there is an error removing the
// paths.
func (s *pruneSuite) TestPruneOperationsGetObjectStoreRemovePathFailure(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	expectedErr1 := errors.New("boom1")
	expectedErr2 := errors.New("boom2")
	s.state.EXPECT().PruneOperations(gomock.Any(), gomock.Any(), gomock.Any()).Return([]string{"1", "2"}, nil)
	s.mockObjectStoreGetter.EXPECT().GetObjectStore(gomock.Any()).Return(s.mockObjectStore, nil)
	s.mockObjectStore.EXPECT().Remove(gomock.Any(), "1").Return(expectedErr1)
	s.mockObjectStore.EXPECT().Remove(gomock.Any(), "2").Return(expectedErr2)

	// Act
	err := s.service().PruneOperations(c.Context(), 1, 1)

	// Assert: errors are joined
	c.Check(err, tc.ErrorIs, expectedErr1)
	c.Check(err, tc.ErrorIs, expectedErr2)
}

func (s *pruneSuite) TestPruneOperationsSuccessZeroMaxAge(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	age := 0 * time.Hour
	sizeMB := 10
	s.state.EXPECT().PruneOperations(gomock.Any(), age, sizeMB).Return(nil, nil)

	// Act
	err := s.service().PruneOperations(c.Context(), age, sizeMB)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *pruneSuite) TestPruneOperationsSuccessZeroMaxSize(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	age := time.Hour
	sizeMB := 0
	s.state.EXPECT().PruneOperations(gomock.Any(), age, sizeMB).Return(nil, nil)

	// Act
	err := s.service().PruneOperations(c.Context(), age, sizeMB)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

// TestPruneOperationsValidationErrorMaxAge ensures that validation fails when
// maxAge is not positive, returning a NotValid error without calling state.
func (s *pruneSuite) TestPruneOperationsValidationErrorNegativeMaxAge(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	age := -1 * time.Hour
	sizeMB := 1
	// No state expectation as validation should fail before any call.

	// Act
	err := s.service().PruneOperations(c.Context(), age, sizeMB)

	// Assert
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
	c.Assert(err, tc.ErrorMatches, "max age and size should be positive.*")
}

// TestPruneOperationsValidationErrorMaxSizeDB ensures that validation fails when
// maxSizeDB is not positive, returning a NotValid error without calling state.
func (s *pruneSuite) TestPruneOperationsValidationErrorNegativeMaxSizeDB(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	age := time.Minute
	sizeMB := -42
	// No state expectation as validation should fail before any call.

	// Act
	err := s.service().PruneOperations(c.Context(), age, sizeMB)

	// Assert
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
	c.Assert(err, tc.ErrorMatches, "max age and size should be positive.*")
}

// TestPruneOperationsStateError verifies that errors returned by the state layer
// are captured and propagated by Service.PruneOperations.
func (s *pruneSuite) TestPruneOperationsStateError(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	age := 2 * time.Hour
	sizeMB := 20
	expectedErr := errors.New("boom")
	s.state.EXPECT().PruneOperations(gomock.Any(), age, sizeMB).Return(nil, expectedErr)

	// Act
	err := s.service().PruneOperations(c.Context(), age, sizeMB)

	// Assert
	c.Assert(err, tc.ErrorIs, expectedErr)
}
