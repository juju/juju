// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	machinetesting "github.com/juju/juju/core/machine/testing"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type linkLayerBaseSuite struct {
	schematesting.ModelSuite
	state *State
}

func (s *linkLayerBaseSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)
	s.state = NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	c.Cleanup(func() {
		s.state = nil
	})
}

// txn executes a transactional function within a database context,
// ensuring proper error handling and assertion.
func (s *linkLayerBaseSuite) txn(c *tc.C, fn func(ctx context.Context, tx *sqlair.TX) error) error {
	db, err := s.state.DB()
	c.Assert(err, tc.ErrorIsNil)
	return db.Txn(c.Context(), fn)
}

// query executes a given SQL query with optional arguments within a
// transactional context using the test database.
func (s *linkLayerBaseSuite) query(c *tc.C, query string, args ...any) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return errors.Errorf("%w: query: %s (args: %s)", err, query, args)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to populate DB: %v",
		errors.ErrorStack(err)))
}

func (s *linkLayerBaseSuite) addNetNode(c *tc.C) string {
	netNodeUUID := uuid.MustNewUUID().String()
	s.query(c, "INSERT INTO net_node (uuid) VALUES (?)", netNodeUUID)
	return netNodeUUID
}

func (s *linkLayerBaseSuite) addMachine(c *tc.C, name string, netNodeUUID string) {
	machineUUID := machinetesting.GenUUID(c).String()
	s.query(c, "INSERT INTO machine (uuid, net_node_uuid, name, life_id) VALUES (?, ?, ? ,?)",
		machineUUID, netNodeUUID, name, 0)
}

// checkRowCount checks that the given table has the expected number of rows.
func (s *linkLayerBaseSuite) checkRowCount(c *tc.C, table string, expected int) {
	obtained := -1
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		query := fmt.Sprintf("SELECT COUNT(*) FROM %s", table)
		return tx.QueryRowContext(ctx, query).Scan(&obtained)
	})
	c.Assert(err, tc.IsNil, tc.Commentf("counting rows in table %q", table))
	c.Check(obtained, tc.Equals, expected, tc.Commentf("count of %q rows", table))
}
