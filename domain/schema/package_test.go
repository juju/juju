// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"context"
	"database/sql"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/tc"

	"github.com/juju/juju/core/database/schema"
	databasetesting "github.com/juju/juju/internal/database/testing"
)

type schemaBaseSuite struct {
	databasetesting.DqliteSuite

	seq int64
}

// NewCleanDB returns a new sql.DB reference.
func (s *schemaBaseSuite) NewCleanDB(c *tc.C) *sql.DB {
	dir := c.MkDir()

	url := fmt.Sprintf("file:%s/db.sqlite3?_foreign_keys=1", dir)
	c.Logf("Opening sqlite3 db with: %v", url)

	db, err := sql.Open("sqlite3", url)
	c.Assert(err, tc.ErrorIsNil)

	return db
}

func (s *schemaBaseSuite) applyDDL(c *tc.C, ddl *schema.Schema) {
	if s.Verbose {
		ddl.Hook(func(i int, statement string) (string, error) {
			c.Logf("-- Applying schema change %d\n%s\n", i, statement)
			return statement, nil
		})
	}
	changeSet, err := ddl.Ensure(c.Context(), s.TxnRunner())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(changeSet.Current, tc.Equals, 0)
	c.Check(changeSet.Post, tc.Equals, ddl.Len())
}

func (s *schemaBaseSuite) assertExecSQL(c *tc.C, q string, args ...any) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, q, args...)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *schemaBaseSuite) assertExecSQLError(c *tc.C, q string, errMsg string, args ...any) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, q, args...)
		return err
	})
	c.Assert(err, tc.ErrorMatches, errMsg)
}

var (
	internalTableNames = set.NewStrings(
		"schema",
		"sqlite_sequence",
	)
)

func (s *schemaBaseSuite) nextSeq() int64 {
	// Currently tests are run sequentially, but just in case.
	return atomic.AddInt64(&s.seq, 1)
}

func (s *schemaBaseSuite) getNamespaceID(
	c *tc.C, namespace string,
) int {
	row := s.DB().QueryRowContext(
		c.Context(),
		"SELECT id FROM change_log_namespace WHERE namespace = ?",
		namespace,
	)
	var nsID int
	err := row.Scan(&nsID)
	c.Assert(err, tc.ErrorIsNil)
	return nsID
}

func (s *schemaBaseSuite) clearChangeEvents(
	c *tc.C, nsID int, changed string,
) {
	_, err := s.DB().Exec(
		"DELETE FROM change_log WHERE namespace_id = ? AND changed = ?",
		nsID, changed,
	)
	c.Assert(err, tc.ErrorIsNil)
}

// assertChangeEvent asserts that a single change event exists for the provided
// namespace and changed value. If successful the matching change event will be
// deleted from the database so subsequent calls can be made to this func within
// a single test.
func (s *schemaBaseSuite) assertChangeEvent(
	c *tc.C, namespace string, changed string,
) {
	nsID := s.getNamespaceID(c, namespace)

	row := s.DB().QueryRow(`
SELECT COUNT(*)
FROM   change_log
WHERE  namespace_id = ?
AND    changed = ?`, nsID, changed)
	var count int
	err := row.Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(count, tc.Equals, 1)

	s.clearChangeEvents(c, nsID, changed)
}

func readEntityNames(c *tc.C, db *sql.DB, entity_type string) []string {
	ctx, cancel := context.WithTimeout(c.Context(), 5*time.Second)
	defer cancel()

	tx, err := db.BeginTx(ctx, nil)
	c.Assert(err, tc.ErrorIsNil)

	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT name FROM sqlite_master WHERE type = ? ORDER BY name ASC;`, entity_type)
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = rows.Close() }()

	var names []string
	for rows.Next() {
		var name string
		err = rows.Scan(&name)
		c.Assert(err, tc.ErrorIsNil)
		names = append(names, name)
	}

	err = tx.Commit()
	c.Assert(err, tc.ErrorIsNil)

	return names
}
