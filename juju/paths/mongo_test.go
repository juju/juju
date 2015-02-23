// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package paths_test

import (
	"os"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/juju/paths"
	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&mongoSuite{})

type mongoSuite struct {
	testing.BaseSuite
}

func (s *mongoSuite) writeScript(c *gc.C, name, content string) (string, string) {
	dirname := c.MkDir()
	filename := filepath.Join(dirname, name)

	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY, 0755)
	c.Assert(err, jc.ErrorIsNil)
	defer file.Close()

	_, err = file.Write([]byte(content))
	c.Assert(err, jc.ErrorIsNil)

	return dirname, filename
}

func (s *mongoSuite) TestFindMongoRestorePathDefaultExists(c *gc.C) {
	jujudir, expected := s.writeScript(c, "mongorestore", "echo 'mongorestore'")

	jujuRestore := paths.NewMongoTest(jujudir).RestorePath()
	mongoPath, err := paths.Find(jujuRestore)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(mongoPath, gc.Equals, expected)
}

func (s *mongoSuite) TestFindMongoRestorePathDefaultNotExists(c *gc.C) {
	jujudir, _ := s.writeScript(c, "mongod", "echo 'mongod'")
	pathdir, expected := s.writeScript(c, "mongorestore", "echo 'mongorestore'")
	s.PatchEnvironment("PATH", pathdir)
	c.Logf(os.Getenv("PATH"))
	c.Logf(expected)

	jujuRestore := paths.NewMongoTest(jujudir).RestorePath()
	mongoPath, err := paths.Find(jujuRestore)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(mongoPath, gc.Equals, expected)
}
