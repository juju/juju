// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"github.com/juju/tc"

	"github.com/juju/juju/core/database/schema"
	"github.com/juju/juju/internal/database/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type migrationSuite struct {
	testing.DqliteSuite
}

var _ = tc.Suite(&migrationSuite{})

func (s *migrationSuite) TestMigrationSuccess(c *tc.C) {
	patches := schema.New()
	patches.Add(
		schema.MakePatch("CREATE TABLE band(name TEXT NOT NULL PRIMARY KEY);"),
		schema.MakePatch("INSERT INTO band VALUES (?);", "Blood Incantation"),
	)

	db := s.DB()
	m := NewDBMigration(&txnRunner{db: db}, loggertesting.WrapCheckLog(c), patches)
	c.Assert(m.Apply(c.Context()), tc.ErrorIsNil)

	rows, err := db.Query("SELECT * from band;")
	c.Assert(err, tc.ErrorIsNil)
	s.AddCleanup(func(*tc.C) { _ = rows.Close() })

	var band string
	c.Assert(rows.Next(), tc.IsTrue)
	c.Assert(rows.Scan(&band), tc.ErrorIsNil)
	c.Check(band, tc.Equals, "Blood Incantation")
}
