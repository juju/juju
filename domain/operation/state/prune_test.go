// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"testing"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"
)

type pruneByAgeSuite struct {
	baseSuite
}

func TestPruneByAgeSuite(t *testing.T) {
	tc.Run(t, &pruneByAgeSuite{})
}

// TestPruneCompletedOperationsOlderThan tests that the prune completed operation
// by age function deletes the completed operations.
func (s *pruneByAgeSuite) TestPruneCompletedOperationsOlderThan(c *tc.C) {
	// Arrange: three operation, on is not completed, one is recently completed,
	// on need to be deleted by the prune.
	s.addCompletedOperation(c, time.Minute)
	controlCompleted := s.addCompletedOperation(c, time.Second)
	controlNotCompleted := s.addOperation(c)

	// Act: prune all completed operation older than 30 sec.
	err := s.state.pruneCompletedOperationsOlderThan(c.Context(), 30*time.Second)

	// Assert: the operation is deleted.
	c.Assert(err, tc.ErrorIsNil)
	c.Check(s.selectDistinctValues(c, "uuid", "operation"), tc.SameContents, []string{controlNotCompleted, controlCompleted})
}

// TestGetCompletedOperationsOlderThan tests that the get completed operations
// by age function returns the completed operations.
func (s *pruneByAgeSuite) TestGetCompletedOperationsOlderThan(c *tc.C) {
	// Arrange: three operation, on is not completed, one is recently completed,
	// on need to be deleted by the prune.
	completed := s.addCompletedOperation(c, time.Minute)
	controlCompleted := s.addCompletedOperation(c, time.Second)
	controlNotCompleted := s.addOperation(c)

	// Act: get old completed operations.
	var opUUIDs []string
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		opUUIDs, err = s.state.getCompletedOperationUUIDsOlderThan(ctx, tx, 30*time.Second)
		return err
	})

	// Assert: the operation is deleted.
	c.Assert(err, tc.ErrorIsNil)
	c.Check(opUUIDs, tc.DeepEquals, []string{completed})
	c.Check(s.selectDistinctValues(c, "uuid", "operation"), tc.SameContents, []string{controlNotCompleted,
		controlCompleted, completed})
}

// TestGetCompletedOperationOlderThanNegativeAge tests that the get completed
// operation by age function returns no results when the age is negative.
func (s *pruneByAgeSuite) TestGetCompletedOperationOlderThanNegativeAge(c *tc.C) {
	// Arrange
	s.addCompletedOperation(c, time.Minute)

	// Act
	var opUUIDs []string
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		opUUIDs, err = s.state.getCompletedOperationUUIDsOlderThan(ctx, tx, -30*time.Second)
		return err
	})
	// Assert: no error, no results
	c.Assert(err, tc.ErrorIsNil)
	c.Check(opUUIDs, tc.HasLen, 0)
}

// TestGetCompletedOperationOlderThanZeroAge verifies that no completed
// operations are returned when the age is zero.
func (s *pruneByAgeSuite) TestGetCompletedOperationOlderThanZeroAge(c *tc.C) {
	// Arrange
	s.addCompletedOperation(c, time.Minute)

	// Act
	var opUUIDs []string
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		opUUIDs, err = s.state.getCompletedOperationUUIDsOlderThan(ctx, tx, 0)
		return err
	})
	// Assert: no error, no results
	c.Assert(err, tc.ErrorIsNil)
	c.Check(opUUIDs, tc.HasLen, 0)
}
