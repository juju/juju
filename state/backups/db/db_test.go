// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package db

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/names"

	"github.com/juju/juju/mongo"
	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&dbSuite{})

type dbSuite struct {
	testing.BaseSuite
}

func (s *dbSuite) TestNewMongoConnInfoOkay(c *gc.C) {
	tag, err := names.ParseTag("machine-0")
	c.Assert(err, gc.IsNil)
	mgoInfo := mongo.MongoInfo{
		Info: mongo.Info{
			Addrs: []string{"localhost:8080"},
		},
		Tag:      tag,
		Password: "eggs",
	}

	connInfo := newMongoConnInfo(&mgoInfo)
	err = connInfo.Validate()
	c.Assert(err, gc.IsNil)

	c.Check(connInfo.Address, gc.Equals, "localhost:8080")
	c.Check(connInfo.Username, gc.Equals, "machine-0")
	c.Check(connInfo.Password, gc.Equals, "eggs")
}

func (s *dbSuite) TestNewMongoConnInfoMissingTag(c *gc.C) {
	mgoInfo := mongo.MongoInfo{
		Info: mongo.Info{
			Addrs: []string{"localhost:8080"},
		},
		Password: "eggs",
	}

	connInfo := newMongoConnInfo(&mgoInfo)
	err := connInfo.Validate()

	c.Check(err, gc.ErrorMatches, "missing username")
}
