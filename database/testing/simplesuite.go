// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"database/sql"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	_ "github.com/mattn/go-sqlite3"
	gc "gopkg.in/check.v1"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/domain/schema"
)

// ControllerSuite is used to provide a sql.DB reference to tests.
// It is pre-populated with the controller schema.
type ControllerSuite struct {
	testing.IsolationSuite

	db        *sql.DB
	txnRunner coredatabase.TxnRunner
}

// SetUpTest creates a new sql.DB reference and ensures that the
// controller schema is applied successfully.
func (s *ControllerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	// Do not be tempted in moving to :memory: mode for this test suite. It will
	// fail in non-deterministic ways. Unfortunately :memory: mode is not
	// completely goroutine safe.
	s.db = s.NewCleanDB(c)

	s.txnRunner = &txnRunner{
		db: sqlair.NewDB(s.db),
	}

	s.ApplyControllerDDL(c)
}

func (s *ControllerSuite) TearDownTest(c *gc.C) {
	if s.db != nil {
		c.Logf("Closing DB")
		err := s.db.Close()
		c.Assert(err, jc.ErrorIsNil)
	}

	s.IsolationSuite.TearDownTest(c)
}

// DB returns a sql.DB reference.
func (s *ControllerSuite) DB() *sql.DB {
	return s.db
}

// TxnRunner returns the suite's transaction runner.
func (s *ControllerSuite) TxnRunner() coredatabase.TxnRunner {
	return s.txnRunner
}

// NewCleanDB returns a new sql.DB reference.
func (s *ControllerSuite) NewCleanDB(c *gc.C) *sql.DB {
	dir := c.MkDir()

	url := fmt.Sprintf("file:%s/db.sqlite3?_foreign_keys=1", dir)
	c.Logf("Opening sqlite3 db with: %v", url)

	db, err := sql.Open("sqlite3", url)
	c.Assert(err, jc.ErrorIsNil)

	return db
}

// ApplyControllerDDL applies the controller schema to the provided sql.DB.
// This is useful for tests that need to apply the schema to a new DB.
func (s *ControllerSuite) ApplyControllerDDL(c *gc.C) {
	tx, err := s.db.Begin()
	c.Assert(err, jc.ErrorIsNil)

	for idx, delta := range schema.ControllerDDL(0x2dc171858c3155be) {
		c.Logf("Executing schema DDL index: %v", idx)
		_, err := tx.Exec(delta.Stmt(), delta.Args()...)
		c.Assert(err, jc.ErrorIsNil)
	}

	c.Logf("Committing schema DDL")
	err = tx.Commit()
	c.Assert(err, jc.ErrorIsNil)
}
