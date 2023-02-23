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

	"github.com/juju/juju/database/schema"
)

// ControllerSuite is used to provide an in-memory sql.DB reference to tests.
// It is pre-populated with the controller schema.
type ControllerSuite struct {
	testing.IsolationSuite

	DB *sql.DB
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
	s.DB, err = sql.Open("sqlite3", fmt.Sprintf("file:%s/db.sqlite3?_foreign_keys=1", dir))
	c.Assert(err, jc.ErrorIsNil)

	tx, err := s.DB.Begin()
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
	err := s.DB.Close()
	c.Assert(err, jc.ErrorIsNil)
}
