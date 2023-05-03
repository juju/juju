// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	_ "github.com/mattn/go-sqlite3"
	gc "gopkg.in/check.v1"
)

type schemaSuite struct {
	testing.IsolationSuite

	db *sql.DB
}

var _ = gc.Suite(&schemaSuite{})

// SetUpTest creates a new sql.DB reference and ensures that the
// controller schema is applied successfully.
func (s *schemaSuite) TestDDLApply(c *gc.C) {
	// Do not be tempted in moving to :memory: mode for this test suite. It will
	// fail in non-deterministic ways. Unfortunately :memory: mode is not
	// completely goroutine safe.
	s.db = s.NewCleanDB(c)

	s.AddCleanup(func(*gc.C) {
		err := s.db.Close()
		c.Assert(err, jc.ErrorIsNil)
	})

	tx, err := s.db.Begin()
	c.Assert(err, jc.ErrorIsNil)

	for idx, stmt := range ControllerDDL() {
		c.Logf("Executing schema DDL index: %v", idx)
		_, err := tx.Exec(stmt)
		c.Assert(err, jc.ErrorIsNil)
	}

	c.Logf("Committing schema DDL")
	err = tx.Commit()
	c.Assert(err, jc.ErrorIsNil)

	// Ensure that each table is present.
	expected := set.NewStrings(
		// Leases
		"lease",
		"lease_type",
		"lease_pin",

		// Change log
		"change_log",
		"change_log_edit_type",
		"change_log_namespace",

		// Cloud
		"cloud",
		"auth_type",
		"cloud_auth_type",
		"ca_cert",
		"cloud_ca_cert",
		"cloud_region",
		"cloud_type",

		// External controller
		"external_controller",
		"external_controller_address",
		"external_model",
	)
	c.Assert(readTableNames(c, s.db), jc.SameContents, expected.Union(internalTableNames).SortedValues())
}

// NewCleanDB returns a new sql.DB reference.
func (s *schemaSuite) NewCleanDB(c *gc.C) *sql.DB {
	dir := c.MkDir()

	url := fmt.Sprintf("file:%s/db.sqlite3?_foreign_keys=1", dir)
	c.Logf("Opening sqlite3 db with: %v", url)

	db, err := sql.Open("sqlite3", url)
	c.Assert(err, jc.ErrorIsNil)

	return db
}

var (
	internalTableNames = set.NewStrings(
		"sqlite_sequence",
	)
)

func readTableNames(c *gc.C, db *sql.DB) []string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tx, err := db.BeginTx(ctx, nil)
	c.Assert(err, jc.ErrorIsNil)

	rows, err := tx.QueryContext(ctx, "SELECT tbl_name FROM sqlite_master")
	c.Assert(err, jc.ErrorIsNil)
	defer func() { _ = rows.Close() }()

	var tables []string
	for rows.Next() {
		var table string
		err = rows.Scan(&table)
		c.Assert(err, jc.ErrorIsNil)
		tables = append(tables, table)
	}

	err = tx.Commit()
	c.Assert(err, jc.ErrorIsNil)

	return set.NewStrings(tables...).SortedValues()
}
