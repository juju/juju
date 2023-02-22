// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"database/sql"

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

	var err error
	s.DB, err = sql.Open("sqlite3", ":memory:?_foreign_keys=1")
	c.Assert(err, jc.ErrorIsNil)

	tx, err := s.DB.Begin()
	c.Assert(err, jc.ErrorIsNil)

	for _, stmt := range schema.ControllerDDL() {
		_, err := tx.Exec(stmt)
		c.Assert(err, jc.ErrorIsNil)
	}

	err = tx.Commit()
	c.Assert(err, jc.ErrorIsNil)
}
