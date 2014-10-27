// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package db_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state/backups/db"
	"github.com/juju/juju/testing"
)

type dumpSuite struct {
	testing.BaseSuite

	ranCommand bool
}

var _ = gc.Suite(&dumpSuite{}) // Register the suite.

func (s *dumpSuite) patch(c *gc.C) {
	s.PatchValue(db.GetMongodumpPath, func() (string, error) {
		return "bogusmongodump", nil
	})

	s.PatchValue(db.RunCommand, func(cmd string, args ...string) error {
		s.ranCommand = true
		return nil
	})
}

func (s *dumpSuite) TestDump(c *gc.C) {
	s.patch(c)

	connInfo := db.ConnInfo{"a", "b", "c"}
	dbInfo := db.Info{connInfo, []string{"juju", "admin"}}
	dumper, err := db.NewDumper(dbInfo)
	c.Assert(err, gc.IsNil)

	err = dumper.Dump("spam")
	c.Assert(err, gc.IsNil)

	c.Assert(s.ranCommand, gc.Equals, true)
}
