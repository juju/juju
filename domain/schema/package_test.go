// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/tc"

	"github.com/juju/juju/core/database/schema"
	databasetesting "github.com/juju/juju/internal/database/testing"
)

type schemaBaseSuite struct {
	databasetesting.DqliteSuite
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
		ddl.Hook(func(i int, statement string) error {
			c.Logf("-- Applying schema change %d\n%s\n", i, statement)
			return nil
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
