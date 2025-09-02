// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"context"
	"database/sql"

	"github.com/juju/clock"
	"github.com/juju/tc"

	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/machine/state"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type baseSuite struct {
	schematesting.ModelSuite

	state *state.State
}

func (s *baseSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)

	s.state = state.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
}

// runQuery executes the provided SQL query string using the current state's database connection.
//
// It is a convenient function to setup test with a specific database state
func (s *baseSuite) runQuery(c *tc.C, query string, args ...any) {
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

// addNetNode adds a new net node to the database.
//
// It returns the net node UUID.
func (s *baseSuite) addNetNode(c *tc.C) string {
	netNodeUUID := uuid.MustNewUUID().String()
	s.runQuery(c, `INSERT INTO net_node (uuid) VALUES (?)`, netNodeUUID)
	return netNodeUUID
}

// addMachine adds a new machine to the database.
//
// It returns the machine UUID.
func (s *baseSuite) addMachine(c *tc.C, machineName, netNodeUUID string) string {
	machineUUID := uuid.MustNewUUID().String()
	s.runQuery(c, `INSERT INTO machine (uuid, name, life_id, net_node_uuid) VALUES (?,?,?,?)`,
		machineUUID, machineName, life.Alive, netNodeUUID)
	return machineUUID
}

func ptr[T any](v T) *T {
	return &v
}
