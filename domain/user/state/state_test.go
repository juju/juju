// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"

	"github.com/juju/juju/internal/changestream/testing"
	jujudb "github.com/juju/juju/internal/database"
	jc "github.com/juju/testing/checkers"
	"golang.org/x/net/context"
	gc "gopkg.in/check.v1"
)

type stateSuite struct {
	testing.ControllerSuite
}

// TestNullUser is a regression test to make sure that we don't allow null
// user.
func (s *stateSuite) TestNullUser(c *gc.C) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO user (id, type) VALUES (99, NULL)")
		return err
	})
	c.Assert(jujudb.IsErrConstraintNotNull(err), jc.IsTrue)
}
