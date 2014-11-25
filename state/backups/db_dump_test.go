// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"os"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state/backups"
	"github.com/juju/juju/testing"
)

type dumpSuite struct {
	testing.BaseSuite

	targets    set.Strings
	dbInfo     *backups.DBInfo
	dumpDir    string
	ranCommand bool
}

var _ = gc.Suite(&dumpSuite{}) // Register the suite.

func (s *dumpSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	targets := set.NewStrings("juju", "admin")
	s.dbInfo = &backups.DBInfo{"a", "b", "c", targets}
	s.targets = targets
	s.dumpDir = c.MkDir()
}

func (s *dumpSuite) patch(c *gc.C) {
	s.PatchValue(backups.GetMongodumpPath, func() (string, error) {
		return "bogusmongodump", nil
	})

	s.PatchValue(backups.RunCommand, func(cmd string, args ...string) error {
		s.ranCommand = true
		return nil
	})
}

func (s *dumpSuite) prepDB(c *gc.C, name string) string {
	dirName := filepath.Join(s.dumpDir, name)
	err := os.Mkdir(dirName, 0777)
	c.Assert(err, jc.ErrorIsNil)
	return dirName
}

func (s *dumpSuite) prep(c *gc.C, targetDBs ...string) backups.DBDumper {
	dumper, err := backups.NewDBDumper(s.dbInfo)
	c.Assert(err, jc.ErrorIsNil)

	// Prep each of the target databases.
	for _, dbName := range targetDBs {
		s.prepDB(c, dbName)
	}

	return dumper
}

func (s *dumpSuite) checkDBs(c *gc.C, dbNames ...string) {
	for _, dbName := range dbNames {
		_, err := os.Stat(filepath.Join(s.dumpDir, dbName))
		c.Check(err, jc.ErrorIsNil)
	}
}

func (s *dumpSuite) checkStripped(c *gc.C, dbName string) {
	dirName := filepath.Join(s.dumpDir, dbName)
	_, err := os.Stat(dirName)
	c.Check(err, jc.Satisfies, os.IsNotExist)
}

func (s *dumpSuite) TestDumpRanCommand(c *gc.C) {
	s.patch(c)
	dumper := s.prep(c, "juju", "admin")

	err := dumper.Dump(s.dumpDir)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.ranCommand, jc.IsTrue)
}

func (s *dumpSuite) TestDumpStripped(c *gc.C) {
	s.patch(c)
	dumper := s.prep(c, "juju", "admin")
	s.prepDB(c, "backups") // ignored

	err := dumper.Dump(s.dumpDir)
	c.Assert(err, jc.ErrorIsNil)

	s.checkDBs(c, "juju", "admin")
	s.checkStripped(c, "backups")
}

func (s *dumpSuite) TestDumpStrippedMultiple(c *gc.C) {
	s.patch(c)
	dumper := s.prep(c, "juju", "admin")
	s.prepDB(c, "backups")  // ignored
	s.prepDB(c, "presence") // ignored

	err := dumper.Dump(s.dumpDir)
	c.Assert(err, jc.ErrorIsNil)

	s.checkDBs(c, "juju", "admin")
	// Only "backups" is actually ignored when dumping.  Restore takes
	// care of removing the other ignored databases (like presence).
	s.checkDBs(c, "presence")
	s.checkStripped(c, "backups")
}

func (s *dumpSuite) TestDumpNothingIgnored(c *gc.C) {
	s.patch(c)
	dumper := s.prep(c, "juju", "admin")

	err := dumper.Dump(s.dumpDir)
	c.Assert(err, jc.ErrorIsNil)

	s.checkDBs(c, "juju", "admin")
}
