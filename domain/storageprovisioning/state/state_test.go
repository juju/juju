// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"testing"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/uuid"
)

// stateSuite provides a test suite for testing the commonality parts of [State].
type stateSuite struct {
	schematesting.ModelSuite
}

// TestStateSuite registers and runs all of the tests located in [stateSuite].
func TestStateSuite(t *testing.T) {
	tc.Run(t, &stateSuite{})
}

// TestCheckNetNodeNotExist tests that the [State.checkNetNodeExists] method
// returns false when the net node does not exist.
func (s *stateSuite) TestCheckNetNodeNotExist(c *tc.C) {
	netNodeUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory())

	var exists bool
	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		exists, err = st.checkNetNodeExists(ctx, tx, netNodeUUID.String())
		return err
	})
	c.Check(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)
}

// TestCheckNetNodeExists tests that when a net node exists
// [State.checkNetNodeExists] returns true.
func (s *stateSuite) TestCheckNetNodeExists(c *tc.C) {
	netNodeUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		"INSERT INTO net_node (uuid) VALUES (?)",
		netNodeUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory())

	var exists bool
	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		exists, err = st.checkNetNodeExists(ctx, tx, netNodeUUID.String())
		return err
	})
	c.Check(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, true)
}
