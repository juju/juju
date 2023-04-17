// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"database/sql"
	"fmt"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	_ "github.com/mattn/go-sqlite3"
	gc "gopkg.in/check.v1"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/database/schema"
)

// ControllerSuite is used to provide an in-memory sql.DB reference to tests.
// It is pre-populated with the controller schema.
type ControllerSuite struct {
	testing.IsolationSuite

	db        *sql.DB
	trackedDB coredatabase.TrackedDB
}

// SetUpTest creates a new sql.DB reference and ensures that the
// controller schema is applied successfully.
func (s *ControllerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	// Do not be tempted in moving to :memory: mode for this test suite. It will
	// fail in non-deterministic ways. Unfortunately :memory: mode is not
	// completely goroutine safe.
	s.db = s.NewCleanDB(c)

	s.trackedDB = &trackedDB{
		db: s.db,
	}

	s.ApplyControllerDDL(c, s.db)
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

// TrackedDB returns a TrackedDB reference.
func (s *ControllerSuite) TrackedDB() coredatabase.TrackedDB {
	return s.trackedDB
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
func (s *ControllerSuite) ApplyControllerDDL(c *gc.C, db *sql.DB) {
	tx, err := s.db.Begin()
	c.Assert(err, jc.ErrorIsNil)

	for idx, stmt := range schema.ControllerDDL() {
		c.Logf("Executing schema DDL index: %v", idx)
		_, err := tx.Exec(stmt)
		c.Assert(err, jc.ErrorIsNil)
	}

	c.Logf("Committing schema DDL")
	err = tx.Commit()
	c.Assert(err, jc.ErrorIsNil)
}
