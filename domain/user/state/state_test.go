// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"
	"time"

	jc "github.com/juju/testing/checkers"
	"golang.org/x/net/context"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/changestream/testing"
	"github.com/juju/juju/internal/database"
)

type stateSuite struct {
	testing.ControllerSuite
}

var _ = gc.Suite(&stateSuite{})

// TestSingletonActiveUser asserts the idx_singleton_active_user unique index
// in the DDL. What we need in the DDL is the ability to have multiple users
// with the same username. However only one username can exist in the table
// where the username has not been removed. We are free to have as many removed
// identical usernames as we want.
//
// This test will make 3 users called "bob" that have all been removed. This
// check asserts that we can have more then one removed bob.
// We will then add another user called "bob" that is not removed
// (an active user). This should not fail.
// We will then try and add a 5 user called "bob" that is also not removed and
// this will produce a unique index constraint error.
func (s *stateSuite) TestSingletonActiveUser(c *gc.C) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO user (uuid, name, removed, created_at)
			VALUES (?, ?, ?, ?)
		`, "123", "bob", true, time.Now())
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO user (uuid, name, removed, created_at)
			VALUES (?, ?, ?, ?)
		`, "124", "bob", true, time.Now())
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO user (uuid, name, removed, created_at)
			VALUES (?, ?, ?, ?)
		`, "125", "bob", true, time.Now())
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	// Insert the first non removed (active) Bob user.
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO user (uuid, name, removed, created_at)
			VALUES (?, ?, ?, ?)
		`, "126", "bob", false, time.Now())
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	// Try and insert the second non removed (active) Bob user. This should blow
	// up the constraint.
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO user (uuid, name, removed, created_at)
			VALUES (?, ?, ?, ?)
		`, "127", "bob", false, time.Now())
		return err
	})
	c.Assert(database.IsErrConstraintUnique(err), jc.IsTrue)
}
