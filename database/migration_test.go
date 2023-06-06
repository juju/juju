// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/database/testing"
)

type migrationSuite struct {
	testing.DBSuite
}

var _ = gc.Suite(&migrationSuite{})

func (s *migrationSuite) TestMigrationSuccess(c *gc.C) {
	deltas := []database.Delta{
		database.MakeDelta("CREATE TABLE band(name TEXT PRIMARY KEY);"),
		database.MakeDelta("INSERT INTO band VALUES (?);", "Blood Incantation"),
	}

	db := s.DB()
	m := NewDBMigration(db, stubLogger{}, deltas)
	c.Assert(m.Apply(context.Background()), jc.ErrorIsNil)

	rows, err := db.Query("SELECT * from band;")
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(*gc.C) { _ = rows.Close() })

	var band string
	c.Assert(rows.Next(), jc.IsTrue)
	c.Assert(rows.Scan(&band), jc.ErrorIsNil)
	c.Check(band, gc.Equals, "Blood Incantation")
}
