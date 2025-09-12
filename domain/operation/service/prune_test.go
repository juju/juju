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
	return ctrl
}

func (s *pruneSuite) service() *Service {
	return NewService(s.state, s.clock, loggertesting.WrapCheckLog(nil), s.mockObjectStoreGetter)
}

// TestPruneOperationsSuccess verifies that the Service.PruneOperations method
// successfully delegates to state.PruneOperations when provided with valid
// arguments and returns no error.
func (s *pruneSuite) TestPruneOperationsSuccess(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	age := time.Hour
	sizeMB := 10
	s.state.EXPECT().PruneOperations(gomock.Any(), age, sizeMB).Return(nil)

	// Act
	err := s.service().PruneOperations(c.Context(), age, sizeMB)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *pruneSuite) TestPruneOperationsSuccessZeroMaxAge(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	age := 0 * time.Hour
	sizeMB := 10
	s.state.EXPECT().PruneOperations(gomock.Any(), age, sizeMB).Return(nil)

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
	s.state.EXPECT().PruneOperations(gomock.Any(), age, sizeMB).Return(nil)

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
	s.state.EXPECT().PruneOperations(gomock.Any(), age, sizeMB).Return(expectedErr)

	// Act
	err := s.service().PruneOperations(c.Context(), age, sizeMB)

	// Assert
	c.Assert(err, tc.ErrorIs, expectedErr)
}
