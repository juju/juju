// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package db_test

import (
	"github.com/juju/names"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state/backups/db"
	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&connInfoSuite{})

type connInfoSuite struct {
	testing.BaseSuite
}

func (s *connInfoSuite) TestNewMongoConnInfoOkay(c *gc.C) {
	tag, err := names.ParseTag("machine-0")
	c.Assert(err, gc.IsNil)
	mgoInfo := mongo.MongoInfo{
		Info: mongo.Info{
			Addrs: []string{"localhost:8080"},
		},
		Tag:      tag,
		Password: "eggs",
	}

	connInfo := db.NewMongoConnInfo(&mgoInfo)
	addr, user, pw, err := connInfo.Check()
	c.Assert(err, gc.IsNil)

	c.Check(addr, gc.Equals, "localhost:8080")
	c.Check(user, gc.Equals, "machine-0")
	c.Check(pw, gc.Equals, "eggs")
}

func (s *connInfoSuite) TestNewMongoConnInfoMissingTag(c *gc.C) {
	mgoInfo := mongo.MongoInfo{
		Info: mongo.Info{
			Addrs: []string{"localhost:8080"},
		},
		Password: "eggs",
	}

	connInfo := db.NewMongoConnInfo(&mgoInfo)
	_, _, _, err := connInfo.Check()

	c.Check(err, gc.ErrorMatches, "missing username")
}

func (s *connInfoSuite) TestDBConnInfoCheckOkay(c *gc.C) {
	connInfo := db.NewConnInfo("a", "b", "c")
	addr, user, pw, err := connInfo.Check()
	c.Assert(err, gc.IsNil)

	c.Check(addr, gc.Equals, "a")
	c.Check(user, gc.Equals, "b")
	c.Check(pw, gc.Equals, "c")
}

func (s *connInfoSuite) TestDBConnInfoCheckMissing(c *gc.C) {
	connInfo := db.NewConnInfo("a", "b", "")
	_, _, _, err := connInfo.Check()

	c.Check(err, gc.ErrorMatches, "missing password")
}
