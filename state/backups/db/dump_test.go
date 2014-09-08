// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package db_test

import (
	gc "launchpad.net/gocheck"

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

	dumper := db.NewDumper(db.ConnInfo{"a", "b", "c"})
	err := dumper.Dump("spam")
	c.Assert(err, gc.IsNil)

	c.Assert(s.ranCommand, gc.Equals, true)
}
