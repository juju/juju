// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package paths_test

import (
	"fmt"
	"os"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/juju/paths"
	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&pathsSuite{})

type pathsSuite struct {
	testing.BaseSuite
}

func (s *pathsSuite) TestMongorestorePathDefaultMongoExists(c *gc.C) {
	s.PatchValue(paths.OsStat, func(string) (os.FileInfo, error) { return nil, nil })
	mongoPath, err := paths.MongorestorePath()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mongoPath, gc.Equals, "/usr/lib/juju/bin/mongorestore")
}

func (s *pathsSuite) TestMongorestorePathNoDefaultMongo(c *gc.C) {
	s.PatchValue(paths.OsStat, func(string) (os.FileInfo, error) { return nil, fmt.Errorf("sorry no mongo") })
	s.PatchValue(paths.ExecLookPath, func(string) (string, error) { return "/a/fake/mongo/path", nil })
	mongoPath, err := paths.MongorestorePath()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mongoPath, gc.Equals, "/a/fake/mongo/path")
}
