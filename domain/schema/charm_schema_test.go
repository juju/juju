// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"context"
	"database/sql"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type charmSchemaSuite struct {
	schemaBaseSuite
}

var _ = gc.Suite(&charmSchemaSuite{})

func (s *charmSchemaSuite) TestCharmSequence(c *gc.C) {
	s.applyDDL(c, ModelDDL())

	// Insert the first sequence, then every other one is monotonically
	// increasing.

	s.assertExecSQL(c, "INSERT INTO sequence_charm_local (reference_name, sequence) VALUES ('foo', 1)")
	s.assertExecSQL(c, "UPDATE sequence_charm_local SET sequence=2 WHERE reference_name='foo'")
	s.assertExecSQL(c, "UPDATE sequence_charm_local SET sequence=3 WHERE reference_name='foo'")

	// Ensure we can't go backwards.

	s.assertExecSQLError(c, "UPDATE sequence_charm_local SET sequence=2 AND reference_name='foo'", "sequence number must monotonically increase")
}

func (s *charmSchemaSuite) TestCharmSequenceWithMultipleReferenceNames(c *gc.C) {
	s.applyDDL(c, ModelDDL())

	s.assertExecSQL(c, "INSERT INTO sequence_charm_local (reference_name, sequence) VALUES ('foo', 1)")
	s.assertExecSQL(c, "UPDATE sequence_charm_local SET sequence=2 WHERE reference_name='foo'")
	s.assertExecSQL(c, "INSERT INTO sequence_charm_local (reference_name, sequence) VALUES ('bar', 1)")
	s.assertExecSQL(c, "UPDATE sequence_charm_local SET sequence=3 WHERE reference_name='foo'")
	s.assertExecSQL(c, "UPDATE sequence_charm_local SET sequence=2 WHERE reference_name='bar'")

	s.assertSequence(c, "foo", 3)
	s.assertSequence(c, "bar", 2)

}

func (s *charmSchemaSuite) assertSequence(c *gc.C, name string, expected int) {
	var sequence int
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		row, err := tx.QueryContext(ctx, "SELECT sequence FROM sequence_charm_local WHERE reference_name=?", name)
		if err != nil {
			return err
		}
		defer row.Close()

		if !row.Next() {
			return sql.ErrNoRows
		}
		if err := row.Scan(&sequence); err != nil {
			return err
		}

		return row.Err()
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sequence, gc.Equals, expected)
}
