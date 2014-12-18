// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"fmt"
	"os"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state/backups"
	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&mongoRestoreSuite{})

type mongoRestoreSuite struct {
	testing.BaseSuite
}

func (s *mongoRestoreSuite) TestMongorestorePathDefaultMongoExists(c *gc.C) {
	s.PatchValue(backups.OsStat, func(string) (os.FileInfo, error) { return nil, nil })
	mongoPath, err := backups.MongorestorePath()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mongoPath, gc.Equals, "/usr/lib/juju/bin/mongorestore")
}

func (s *mongoRestoreSuite) TestMongorestorePathNoDefaultMongo(c *gc.C) {
	s.PatchValue(backups.OsStat, func(string) (os.FileInfo, error) { return nil, fmt.Errorf("sorry no mongo") })
	s.PatchValue(backups.ExecLookPath, func(string) (string, error) { return "/a/fake/mongo/path", nil })
	mongoPath, err := backups.MongorestorePath()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mongoPath, gc.Equals, "/a/fake/mongo/path")
}
