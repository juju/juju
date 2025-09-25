// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreerrors "github.com/juju/juju/core/errors"
	coremachine "github.com/juju/juju/core/machine"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/errors"
)

// querySuite provides tests for query-related service methods.
//
// It embeds serviceSuite to reuse its setup helpers and mocks.
type querySuite struct {
	serviceSuite
}

func TestQuerySuite(t *testing.T) {
	tc.Run(t, &querySuite{})
}

// TestGetMachineTaskIDsWithStatusHappyPath verifies that a valid machine name
// and status filter result in delegating to state and returning the IDs.
func (s *querySuite) TestGetMachineTaskIDsWithStatusHappyPath(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	mName := coremachine.Name("0")
	status := corestatus.Running
	expected := []string{"t-1", "t-2"}
	s.state.EXPECT().GetMachineTaskIDsWithStatus(gomock.Any(), mName.String(), status.String()).Return(expected, nil)

	// Act
	ids, err := s.service().GetMachineTaskIDsWithStatus(c.Context(), mName, status)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Check(ids, tc.DeepEquals, expected)
}

// TestGetMachineTaskIDsWithStatusNameValidationError ensures that an invalid machine
// name triggers a validation error before any state interaction.
func (s *querySuite) TestGetMachineTaskIDsWithStatusNameValidationError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	// Invalid machine name (empty) should fail validation via coremachine.Name.Validate.
	var mName coremachine.Name

	// No expectation set on state: should not be called on validation error.

	// Act
	_, err := s.service().GetMachineTaskIDsWithStatus(c.Context(), mName, corestatus.Running)

	// Assert
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestGetMachineTaskIDsWithStatusStatusValidationError ensures that an invalid
// status triggers a validation error before any state interaction.
func (s *querySuite) TestGetMachineTaskIDsWithStatusStatusValidationError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	mName := coremachine.Name("0")
	status := corestatus.Allocating // invalid status

	// No expectation set on state: should not be called on validation error.

	// Act
	_, err := s.service().GetMachineTaskIDsWithStatus(c.Context(), mName, status)

	// Assert
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestGetMachineTaskIDsWithStatusStateError validates that state errors are
// captured and returned by the service method.
func (s *querySuite) TestGetMachineTaskIDsWithStatusStateError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	mName := coremachine.Name("1")
	status := corestatus.Running
	stateErr := errors.New("boom")
	s.state.EXPECT().GetMachineTaskIDsWithStatus(gomock.Any(), mName.String(), status.String()).Return(nil, stateErr)

	// Act
	_, err := s.service().GetMachineTaskIDsWithStatus(c.Context(), mName, status)

	// Assert
	c.Assert(err, tc.ErrorIs, stateErr)
}
