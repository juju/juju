// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package db_test

import (
	"os"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/set"
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

	connInfo := db.ConnInfo{"a", "b", "c"} // dummy values to satisfy Dump
	targets := set.NewStrings("juju", "admin")
	dbInfo := db.Info{connInfo, &targets}
	dumper, err := db.NewDumper(dbInfo)
	c.Assert(err, gc.IsNil)

	// Make the dump directories.
	dumpDir := c.MkDir()
	for _, dbName := range targets.Values() {
		err := os.Mkdir(filepath.Join(dumpDir, dbName), 0777)
		c.Assert(err, gc.IsNil)
	}
	ignoredDir := filepath.Join(dumpDir, "backups") // a non-target dir
	err = os.Mkdir(ignoredDir, 0777)
	c.Assert(err, gc.IsNil)

	// Run the dump command.
	err = dumper.Dump(dumpDir)
	c.Assert(err, gc.IsNil)

	c.Assert(s.ranCommand, gc.Equals, true)

	// Check that the ignored directory was deleted.
	for _, dbName := range targets.Values() {
		_, err := os.Stat(filepath.Join(dumpDir, dbName))
		c.Check(err, gc.IsNil)
	}
	_, err = os.Stat(ignoredDir)
	c.Check(err, jc.Satisfies, os.IsNotExist)
}
