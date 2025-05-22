// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package life

import (
	"testing"

	"github.com/juju/tc"

	corelife "github.com/juju/juju/core/life"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type lifeSuite struct {
	schematesting.ModelSuite
}

func TestLifeSuite(t *testing.T) {
	tc.Run(t, &lifeSuite{})
}

// TestLifeDBValues ensures there's no skew between what's in the
// database table for life and the typed consts used in the state packages.
func (s *lifeSuite) TestLifeDBValues(c *tc.C) {
	db := s.DB()
	rows, err := db.Query("SELECT id, value FROM life")
	c.Assert(err, tc.ErrorIsNil)
	defer rows.Close()

	dbValues := make(map[Life]string)
	for rows.Next() {
		var (
			id    int
			value string
		)
		err := rows.Scan(&id, &value)
		c.Assert(err, tc.ErrorIsNil)
		dbValues[Life(id)] = value
	}
	c.Assert(dbValues, tc.DeepEquals, map[Life]string{
		Alive: "alive",
		Dying: "dying",
		Dead:  "dead",
	})
}

func (s *lifeSuite) TestValueAlive(c *tc.C) {
	result, err := Alive.Value()
	c.Assert(err, tc.IsNil)
	c.Check(result, tc.Equals, corelife.Alive)
}

func (s *lifeSuite) TestValueDying(c *tc.C) {
	result, err := Dying.Value()
	c.Assert(err, tc.IsNil)
	c.Check(result, tc.Equals, corelife.Dying)
}

func (s *lifeSuite) TestValueDead(c *tc.C) {
	result, err := Dead.Value()
	c.Assert(err, tc.IsNil)
	c.Check(result, tc.Equals, corelife.Dead)
}
