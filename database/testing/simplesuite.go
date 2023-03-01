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

	coredb "github.com/juju/juju/core/db"
	"github.com/juju/juju/database/schema"
)

// ControllerSuite is used to provide an in-memory sql.DB reference to tests.
// It is pre-populated with the controller schema.
type ControllerSuite struct {
	testing.IsolationSuite

	db        *sql.DB
	trackedDB coredb.TrackedDB
}

// SetUpTest creates a new sql.DB reference and ensures that the
// controller schema is applied successfully.
func (s *ControllerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	dir := c.MkDir()

	// Do not be tempted in moving to :memory: mode for this test suite. It will
	// fail in non-deterministic ways. Unfortunately :memory: mode is not
	// completely goroutine safe.
	var err error
	s.db, err = sql.Open("sqlite3", fmt.Sprintf("file:%s/db.sqlite3?_foreign_keys=1", dir))
	c.Assert(err, jc.ErrorIsNil)

	s.trackedDB = &trackedDB{
		db: s.db,
	}

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

func (s *ControllerSuite) TearDownTest(c *gc.C) {
	c.Logf("Closing DB")
	if s.db != nil {
		err := s.db.Close()
		c.Assert(err, jc.ErrorIsNil)
	}

	s.IsolationSuite.TearDownTest(c)
}

func (s *ControllerSuite) DB() *sql.DB {
	return s.db
}

func (s *ControllerSuite) TrackedDB() coredb.TrackedDB {
	return s.trackedDB
}
