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

// CoreSuite is used to provide a sql.DB reference to tests.
// It is not pre-populated with any schema and is the job the users of this
// Suite to call ApplyDDL after SetupTest has been called.
type CoreSuite struct {
	testing.IsolationSuite

	db        *sql.DB
	txnRunner coredatabase.TxnRunner
}

// ControllerSuite is used to provide a sql.DB reference to tests.
// It is pre-populated with the controller schema.
type ControllerSuite struct {
	CoreSuite
}

// ModelSuite is used to provide an in-memory sql.DB reference to tests.
// It is pre-populated with the model schema.
type ModelSuite struct {
	CoreSuite
}

// ApplyDDL is a helper manager for the test suites to apply a set of DDL string
// on top of a pre-established database.
func (s *CoreSuite) ApplyDDL(c *gc.C, deltas []coredatabase.Delta) {
	tx, err := s.db.Begin()
	c.Assert(err, jc.ErrorIsNil)

	for idx, delta := range deltas {
		c.Logf("Executing schema DDL index: %v", idx)
		_, err := tx.Exec(delta.Stmt(), delta.Args()...)
		c.Assert(err, jc.ErrorIsNil)
	}

	c.Logf("Committing schema DDL")
	err = tx.Commit()
	c.Assert(err, jc.ErrorIsNil)
}

// DB returns a sql.DB reference.
func (s *CoreSuite) DB() *sql.DB {
	return s.db
}

// NewCleanDB returns a new sql.DB reference.
func (s *CoreSuite) NewCleanDB(c *gc.C) *sql.DB {
	dir := c.MkDir()

	url := fmt.Sprintf("file:%s/db.sqlite3?_foreign_keys=1", dir)
	c.Logf("Opening sqlite3 db with: %v", url)

	db, err := sql.Open("sqlite3", url)
	c.Assert(err, jc.ErrorIsNil)

	return db
}

// SetUpTest creates a new sql.DB reference and ensures that the
// controller schema is applied successfully.
func (s *CoreSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	// Do not be tempted in moving to :memory: mode for this test suite. It will
	// fail in non-deterministic ways. Unfortunately :memory: mode is not
	// completely goroutine safe.
	s.db = s.NewCleanDB(c)

	s.txnRunner = &txnRunner{
		db: sqlair.NewDB(s.db),
	}
}

// TearDownTest is responsible for cleaning up the testing resources created
// with the ControllerSuite
func (s *CoreSuite) TearDownTest(c *gc.C) {
	if s.db != nil {
		c.Logf("Closing DB")
		err := s.db.Close()
		c.Assert(err, jc.ErrorIsNil)
	}

	s.IsolationSuite.TearDownTest(c)
}

// TxnRunner returns the suite's transaction runner.
func (s *CoreSuite) TxnRunner() coredatabase.TxnRunner {
	return s.txnRunner
}

// SetUpTest is responsible for setting up a testing database suite initialised
// with the controller schema.
func (s *ControllerSuite) SetUpTest(c *gc.C) {
	s.CoreSuite.SetUpTest(c)
	s.CoreSuite.ApplyDDL(c, schema.ControllerDDL(0x2dc171858c3155be))
}

// SetUpTest is responsible for setting up a testing database suite initialised
// with the model schema.
func (s *ModelSuite) SetUpTest(c *gc.C) {
	s.CoreSuite.SetUpTest(c)
	s.CoreSuite.ApplyDDL(c, schema.ModelDDL())
}
