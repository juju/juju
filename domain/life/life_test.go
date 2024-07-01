// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package life

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corelife "github.com/juju/juju/core/life"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type lifeSuite struct {
	schematesting.ModelSuite
}

var _ = gc.Suite(&lifeSuite{})

// TestLifeDBValues ensures there's no skew between what's in the
// database table for life and the typed consts used in the state packages.
func (s *lifeSuite) TestLifeDBValues(c *gc.C) {
	db := s.DB()
	rows, err := db.Query("SELECT id, value FROM life")
	c.Assert(err, jc.ErrorIsNil)
	defer rows.Close()

	dbValues := make(map[Life]string)
	for rows.Next() {
		var (
			id    int
			value string
		)
		err := rows.Scan(&id, &value)
		c.Assert(err, jc.ErrorIsNil)
		dbValues[Life(id)] = value
	}
	c.Assert(dbValues, jc.DeepEquals, map[Life]string{
		Alive: "alive",
		Dying: "dying",
		Dead:  "dead",
	})
}

// TestToCoreLife ensures that the conversion from domain life to core life
// is correct.
func (s *lifeSuite) TestToCoreLife(c *gc.C) {
	a := Alive
	dy := Dying
	d := Dead
	c.Check((&a).ToCoreLife(), gc.Equals, corelife.Alive)
	c.Check((&dy).ToCoreLife(), gc.Equals, corelife.Dying)
	c.Check((&d).ToCoreLife(), gc.Equals, corelife.Dead)
}
