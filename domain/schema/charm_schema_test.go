// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	context "context"
	"database/sql"
	"fmt"

	"github.com/juju/juju/core/database/schema"
	jc "github.com/juju/testing/checkers"

	gc "gopkg.in/check.v1"
)

type charmSchemaSuite struct {
	schemaBaseSuite
}

var _ = gc.Suite(&charmSchemaSuite{})

// NewCleanDB returns a new sql.DB reference.
func (s *charmSchemaSuite) NewCleanDB(c *gc.C) *sql.DB {
	dir := c.MkDir()

	url := fmt.Sprintf("file:%s/db.sqlite3?_foreign_keys=1", dir)
	c.Logf("Opening sqlite3 db with: %v", url)

	db, err := sql.Open("sqlite3", url)
	c.Assert(err, jc.ErrorIsNil)

	return db
}

func (s *charmSchemaSuite) applyDDL(c *gc.C, ddl *schema.Schema) {
	if s.Verbose {
		ddl.Hook(func(i int, statement string) error {
			c.Logf("-- Applying schema change %d\n%s\n", i, statement)
			return nil
		})
	}
	changeSet, err := ddl.Ensure(context.Background(), s.TxnRunner())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(changeSet.Current, gc.Equals, 0)
	c.Check(changeSet.Post, gc.Equals, ddl.Len())
}

func (s *charmSchemaSuite) assertExecSQL(c *gc.C, q string, args ...any) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, q, args...)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *charmSchemaSuite) TestCharmSequence(c *gc.C) {
	s.applyDDL(c, ModelDDL())

	// Insert the first sequence, then every other one is monotonically
	// increasing.

	s.assertExecSQL(c, "INSERT INTO charm_local_sequence (source_id, reference_name, sequence) VALUES ('1', 'foo', 1)")
	s.assertExecSQL(c, "INSERT INTO charm (uuid, source_id, reference_name, revision) VALUES ('abc', '1', 'foo', 1)")

	s.assertExecSQL(c, "UPDATE charm_local_sequence SET sequence=2 WHERE source_id='1' AND reference_name='foo'")
	s.assertExecSQL(c, "INSERT INTO charm (uuid, source_id, reference_name, revision) VALUES ('def', '1', 'foo', 2)")

	s.assertExecSQL(c, "UPDATE charm_local_sequence SET sequence=3 WHERE source_id='1' AND reference_name='foo'")
	s.assertExecSQL(c, "INSERT INTO charm (uuid, source_id, reference_name, revision) VALUES ('ghi', '1', 'foo', 3)")

	// Ensure we can't go backwards.

	s.assertExecSQLError(c, "UPDATE charm_local_sequence SET sequence=2 WHERE source_id='1' AND reference_name='foo'", "sequence number must monotonically increase")
}
