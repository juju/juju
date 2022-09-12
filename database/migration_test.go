// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"github.com/juju/juju/database/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type migrationSuite struct {
	testing.DBSuite
}

var _ = gc.Suite(&migrationSuite{})

func (s *migrationSuite) TestMigrationSuccess(c *gc.C) {
	delta := []string{
		"CREATE TABLE band(name TEXT PRIMARY KEY);",
		"INSERT INTO band VALUES ('Blood Incantation');",
	}

	m := NewMigration(s.DB(), stubLogger{}, delta)
	c.Assert(m.Apply(), jc.ErrorIsNil)
}
